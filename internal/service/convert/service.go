// Package convert 是 ConvertService 的实现（契约第 4 节）：预设管理，以及把文件转换提交给任务管理器。
package convert

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

const (
	// MaxInputsPerSubmit 是一次 Submit 最多的文件数（更多的由前端分批提交）。
	MaxInputsPerSubmit = 50
	maxUserPresets     = 100
	maxPresetNameRunes = 60
)

// PresetStore 是预设持久化能力，*store.Store 实现它。
type PresetStore interface {
	SeedPresets(ctx context.Context, rows []store.PresetRow) error
	ListPresets(ctx context.Context) ([]store.PresetRow, error)
	CountUserPresets(ctx context.Context) (int, error)
	SaveUserPreset(ctx context.Context, r store.PresetRow, insert bool) (store.PresetRow, error)
	DeleteUserPreset(ctx context.Context, id string) error
}

// Inspector 探测输入文件（*media.Service 实现）。
type Inspector interface {
	Inspect(ctx context.Context, path string) (store.MediaInfo, error)
}

// TaskSubmitter 是任务管理器的能力（*task.Manager 实现）。
type TaskSubmitter interface {
	Submit(spec task.Spec, r task.Runner) (task.Task, error)
	RegisterFactory(t task.Type, f task.Factory)
}

// TaskRecords 是转换记录（契约 v0.23，6.14）需要的任务管理器能力（*task.Manager 实现）。
type TaskRecords interface {
	TaskSubmitter
	Get(id string) (task.Task, error)
	Live(ts []task.Task)
	PeekOutputName(desired string, typ task.Type) string
	DeleteRecords(ids []string, typ task.Type, deleteOutputs bool, beforeRemove func(path string)) (task.DeleteResult, error)
	TaskFile(taskID, which string) (string, task.Task, error)
	// v0.24：
	ListActive() []task.Task
	Reconvert(taskID string, spec task.ReconvertSpec, r task.Runner) (task.Task, error)
	SetReconvertChecker(f task.ReconvertChecker)
}

var _ TaskRecords = (*task.Manager)(nil)

// Config 是 Service 的依赖。
type Config struct {
	Presets PresetStore
	Media   Inspector
	Tasks   TaskSubmitter
	// Require 返回当前 ffmpeg，默认 ffmpeg.Require（缺失返回 FFMPEG_NOT_FOUND）。
	Require func() (ffmpeg.Binaries, error)
	// WaitFFmpeg 在转换组件检测进行中时等它有结果（最多到 ctx 结束），默认 ffmpeg.WaitDetected。
	// 源文件行懒探测前调用（最多 15 秒）：应用启动时转换页的 ListSources 比首次检测先到，不等就探测不了、也不落库。
	// 格式目录同样在检测进行中调用它，但上限是 6 秒（v0.25.4，catalogDetectWait）。
	WaitFFmpeg func(ctx context.Context)
	// DefaultOutputDir 返回 outputDir 传空时用的目录（v0.24：实际输出目录，自定义优先，否则 <base>/output，6.15.2 第 5 条）。
	// 返回空字符串时（测试、旧配置）退回源文件同目录。可为 nil。
	DefaultOutputDir func(ctx context.Context) string
	// DataDir 是应用数据目录：输出目录不能在它里面（<DataDir>/output 及其子文件夹除外，契约 v0.24.1 改写的 6.12 规则）。空 = 不检查。
	DataDir string
	// Encoder 按用户偏好与设备缓存解析 H.264 / HEVC 编码器（契约 9.7）；nil = 一律 CPU。
	// 每个任务在提交 / 重试时解析一次。
	Encoder ffmpeg.EncoderResolver

	// 以下是转换记录（契约 v0.23，6.14）的依赖；为 nil 时相关接口返回 INTERNAL。
	// Sources 为 nil 且 Presets 实现了 SourceStore（*store.Store）时用 Presets。
	Sources SourceStore
	// Thumbs 生成默认缩略图（*media.Service 实现）；为 nil 且 Media 实现了它时用 Media。
	Thumbs Thumbnailer
	// Preview 是 6.13 的 convert 登记表（*localassets.Registry）。
	Preview Previewer
	// Open 用系统默认程序打开文件（system.Manager.OpenWithDefaultApp）。
	Open func(path string) error
	// Reveal 在文件管理器里显示文件（system.Manager.RevealRegisteredPath）。
	Reveal func(path string) error
	// Now 返回当前时间（Unix 毫秒），测试用；nil 用 time.Now。
	Now func() int64

	// 以下是 v0.24 的依赖。
	// UploadsDir 返回实际上传目录（6.15.2）；nil 时 AddSources 不做副本（copyState=none，同 v0.23）。
	UploadsDir func(ctx context.Context) string
	// Emitter 发 convert:copy 事件；可为 nil。
	Emitter task.Emitter
	// FreeSpace 返回目录所在磁盘的可用空间，测试用；nil 用平台实现。
	FreeSpace func(dir string) (int64, error)
	// CopyProgressInterval 是 convert:copy 进度的最小间隔，0 = 250ms，负数不限（测试用）。
	CopyProgressInterval time.Duration
	// Logf 记录内部错误；可为 nil。
	Logf func(format string, args ...any)
	// InterruptedReconverts 是启动时 task.RecoverReconverts 恢复的条数（TakeInterruptedReconverts 返回一次）。
	InterruptedReconverts int
	// CapsProbe 替换格式目录的检测命令（测试用）。
	CapsProbe func(ctx context.Context, exe string) (muxers, encoders string, err error)
}

// Service 实现转换。v0.24 起有一个后台复制队列（副本，6.15.4）。
type Service struct {
	docMu       sync.Mutex
	docRC       DocReconverter
	cfg         Config
	catalog     catalogCache
	copier      *copier
	interrupted atomic.Int64
}

// New 创建 Service，写入内置预设，并注册 convert 任务的重试工厂。
func New(ctx context.Context, cfg Config) (*Service, error) {
	if cfg.Require == nil {
		cfg.Require = ffmpeg.Require
	}
	if cfg.WaitFFmpeg == nil {
		cfg.WaitFFmpeg = ffmpeg.WaitDetected
	}
	if cfg.Sources == nil {
		if ss, ok := cfg.Presets.(SourceStore); ok {
			cfg.Sources = ss
		}
	}
	if cfg.Thumbs == nil {
		if th, ok := cfg.Media.(Thumbnailer); ok {
			cfg.Thumbs = th
		}
	}
	if cfg.Now == nil {
		cfg.Now = func() int64 { return time.Now().UnixMilli() }
	}
	s := &Service{cfg: cfg}
	s.copier = newCopier(s)
	s.catalog.probe = cfg.CapsProbe
	s.interrupted.Store(int64(cfg.InterruptedReconverts))
	if cfg.Presets != nil {
		var rows []store.PresetRow
		for i, p := range builtinPresets() {
			rows = append(rows, toRow(p, i))
		}
		if err := cfg.Presets.SeedPresets(ctx, rows); err != nil {
			return nil, apperr.Wrap(apperr.IOError, "写入内置预设失败", err)
		}
	}
	if cfg.Tasks != nil {
		cfg.Tasks.RegisterFactory(task.TypeConvert, s.retryFactory)
		if tr, ok := cfg.Tasks.(TaskRecords); ok {
			tr.SetReconvertChecker(s.reconvertBlock)
		}
	}
	s.recoverCopies(ctx)
	return s, nil
}

// ---------- 预设 ----------

// ListPresets 返回全部预设，内置在前。
func (s *Service) ListPresets(ctx context.Context) ([]Preset, error) {
	if s.cfg.Presets == nil {
		return nil, apperr.New(apperr.Internal, "预设存储不可用")
	}
	rows, err := s.cfg.Presets.ListPresets(ctx)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "读取预设失败", err)
	}
	out := make([]Preset, len(rows))
	for i, r := range rows {
		out[i] = fromRow(r)
	}
	return out, nil
}

// SavePreset 新建（ID 为空）或更新用户预设，返回保存后的预设。
// 名称必须非空（最多 60 个字），参数必须通过校验；builtIn 字段被忽略；内置预设不能修改（INVALID_ARGUMENT）；
// 更新不存在的 ID 返回 NOT_FOUND；用户预设最多 100 个。
func (s *Service) SavePreset(ctx context.Context, p Preset) (Preset, error) {
	if s.cfg.Presets == nil {
		return Preset{}, apperr.New(apperr.Internal, "预设存储不可用")
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || utf8.RuneCountInString(p.Name) > maxPresetNameRunes {
		return Preset{}, apperr.New(apperr.InvalidArgument, "预设名称不能为空，最多 60 个字")
	}
	if err := validateOptions(p.Options); err != nil {
		return Preset{}, err
	}
	insert := p.ID == ""
	if insert {
		n, err := s.cfg.Presets.CountUserPresets(ctx)
		if err != nil {
			return Preset{}, apperr.Wrap(apperr.IOError, "读取预设失败", err)
		}
		if n >= maxUserPresets {
			return Preset{}, apperr.New(apperr.InvalidArgument, "自定义预设最多 100 个，请先删除不用的")
		}
		p.ID = id.New()
	}
	p.BuiltIn = false
	row, err := s.cfg.Presets.SaveUserPreset(ctx, toRow(p, 0), insert)
	switch {
	case errors.Is(err, store.ErrPresetBuiltIn):
		return Preset{}, apperr.New(apperr.InvalidArgument, "内置预设不能修改，请另存为新预设")
	case errors.Is(err, sql.ErrNoRows):
		return Preset{}, apperr.New(apperr.NotFound, "预设不存在")
	case err != nil:
		return Preset{}, apperr.Wrap(apperr.IOError, "保存预设失败", err)
	}
	return fromRow(row), nil
}

// DeletePreset 删除用户预设。内置预设 INVALID_ARGUMENT，不存在 NOT_FOUND。
func (s *Service) DeletePreset(ctx context.Context, presetID string) error {
	if s.cfg.Presets == nil {
		return apperr.New(apperr.Internal, "预设存储不可用")
	}
	err := s.cfg.Presets.DeleteUserPreset(ctx, presetID)
	switch {
	case errors.Is(err, store.ErrPresetBuiltIn):
		return apperr.New(apperr.InvalidArgument, "内置预设不能删除")
	case errors.Is(err, sql.ErrNoRows):
		return apperr.New(apperr.NotFound, "预设不存在")
	case err != nil:
		return apperr.Wrap(apperr.IOError, "删除预设失败", err)
	}
	return nil
}

// validateOptions 校验转换参数；按目标大小压缩（两遍编码）暂缓，ValidateConvertOptions 对 TargetSizeMB != 0 直接拒绝。
func validateOptions(o ffmpeg.ConvertOptions) error { return ffmpeg.ValidateConvertOptions(o) }

// ---------- 提交 ----------

// params 是 convert 任务的 Params JSON（Retry 用它重建 Runner）。presetId / presetName / paramsSummary 是提交时的快照
// （契约 v0.23，6.14.2），之后不再变（原地重试不动、Reconvert 照抄）；v0.23 之前的旧任务没有这三个键。
type params struct {
	Input         string                `json:"input"`
	Options       ffmpeg.ConvertOptions `json:"options"`
	OutputDir     string                `json:"outputDir"` // 已解析的最终输出目录
	PresetID      string                `json:"presetId"`
	PresetName    string                `json:"presetName"`
	ParamsSummary string                `json:"paramsSummary"`
}

// Submit 为每个输入文件提交一个 convert 任务，返回的任务与 inputs 一一对应（兼容保留，契约 6.14.3）。
//
// 先校验全部输入（ffmpeg 就绪、参数合法、每个文件都能探测且与参数兼容），任何一个不通过整体失败、不提交任何任务，
// 错误的 detail 指出是哪个文件。最多 50 个文件（更多的由前端分批）。
// outputDir 为空时用设置里的默认输出目录，仍为空则输出到各自源文件所在的文件夹；输出名为 <源文件名>.<新扩展名>，
// 重名自动追加 " (1)"、" (2)"（v0.23 带空格），提交时就定名并占位，绝不覆盖已有文件。
// 校验通过后按 path_key 找到或创建源文件行（同 AddSources），每个任务都有 sourceId；presetId / presetName 为空。
func (s *Service) Submit(ctx context.Context, inputs []string, opts ffmpeg.ConvertOptions, outputDir string) ([]task.Task, error) {
	if len(inputs) > MaxInputsPerSubmit {
		return nil, apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次最多提交 %d 个文件，请分批提交", MaxInputsPerSubmit))
	}
	jobs := make([]submitJob, len(inputs))
	for i, raw := range inputs {
		in, _, err := paths.Normalize(raw)
		if err != nil {
			return nil, apperr.Wrap(apperr.InvalidArgument, "路径不合法", err).WithDetail(raw)
		}
		jobs[i] = submitJob{in: in}
	}
	return s.submitWrap(ctx, jobs, opts, outputDir, "", "")
}

// submitWrap 把 ctx 取消统一成 CANCELED（已提交的任务仍随返回值带回）。
func (s *Service) submitWrap(ctx context.Context, jobs []submitJob, opts ffmpeg.ConvertOptions, outputDir, presetID, presetName string) ([]task.Task, error) {
	out, err := s.submit(ctx, jobs, opts, outputDir, presetID, presetName)
	if err != nil && ctx.Err() != nil {
		// ctx 被取消（应用退出等）：不要报 INTERNAL，统一返回 CANCELED；已提交的任务仍随 out 返回。
		return out, apperr.Wrap(apperr.Canceled, "操作已取消", ctx.Err())
	}
	return out, err
}

// submitJob 是一个要提交的文件：in 是规范化后的输入路径；sourceID 为空时校验通过后按路径找到 / 创建源文件行。
type submitJob struct {
	in       string
	sourceID string
}

func (s *Service) submit(ctx context.Context, jobs []submitJob, opts ffmpeg.ConvertOptions, outputDir, presetID, presetName string) ([]task.Task, error) {
	if s.cfg.Tasks == nil || s.cfg.Media == nil {
		return nil, apperr.New(apperr.Internal, "转换服务尚未初始化")
	}
	if len(jobs) == 0 {
		return nil, apperr.New(apperr.InvalidArgument, "没有要转换的文件")
	}
	if len(jobs) > MaxInputsPerSubmit {
		return nil, apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次最多提交 %d 个文件，请分批提交", MaxInputsPerSubmit))
	}
	if err := validateOptions(opts); err != nil {
		return nil, err
	}
	bin, err := s.cfg.Require()
	if err != nil {
		return nil, err
	}
	if err := s.checkFormat(ctx, opts); err != nil { // 6.16.6：整个请求只查一次
		return nil, err
	}
	dir, err := s.resolveOutputDir(ctx, outputDir)
	if err != nil {
		return nil, err
	}

	preps := make([]prepared, len(jobs))
	for i, jb := range jobs {
		if err := ctx.Err(); err != nil {
			return nil, apperr.Wrap(apperr.Canceled, "操作已取消", err)
		}
		if jb.in == "" {
			return nil, apperr.New(apperr.NotFound, "源文件路径无效（记录损坏），请重新添加文件")
		}
		j, err := s.prepare(ctx, jb.in, opts, dir)
		if err != nil {
			return nil, withInput(err, jb.in)
		}
		preps[i] = j
	}

	snap := params{Options: opts, OutputDir: dir, PresetID: presetID, PresetName: presetName, ParamsSummary: ParamsSummary(opts)}
	var out []task.Task
	var touched []string
	for i, jb := range jobs {
		if jb.sourceID == "" && s.cfg.Sources != nil {
			_, key, _ := paths.Normalize(jb.in)
			src, _, err := s.cfg.Sources.UpsertConvertSource(ctx, jb.in, key, s.cfg.Now())
			if err != nil {
				return out, apperr.Wrap(apperr.Internal, "保存源文件行失败", err)
			}
			jb.sourceID = src.SourceID
		} else if jb.sourceID != "" {
			touched = append(touched, jb.sourceID)
		}
		p := snap
		p.Input = jb.in
		t, err := s.submitOne(bin, p, jb.sourceID, preps[i])
		if err != nil {
			s.touch(ctx, touched)
			return out, err // 已提交的保留（不回滚），调用方按返回的任务列表处理
		}
		out = append(out, t)
	}
	s.touch(ctx, touched)
	return out, nil
}

// touch 把被提交的源文件行的 lastActivityAt 设为现在（失败不影响已提交的任务）。
func (s *Service) touch(ctx context.Context, ids []string) {
	if s.cfg.Sources == nil || len(ids) == 0 {
		return
	}
	_ = s.cfg.Sources.TouchConvertSources(ctx, ids, s.cfg.Now())
}

type prepared struct {
	out string
	dur float64
	src ffmpeg.ConvertSource
	// expected 是 short_output 用的预期时长（输入时长已知时 = 输入时长 − 裁剪，否则 0）。
	expected float64
}

// prepare 探测输入、确定期望输出路径并用 PlanConvert 验证参数与输入兼容。
// 输出名按源文件行的原文件名取（6.15.4 第 7 条）：副本 uploads/<sourceId>/<原文件名> 的文件名就是原文件名。
func (s *Service) prepare(ctx context.Context, in string, opts ffmpeg.ConvertOptions, dir string) (prepared, error) {
	info, err := s.cfg.Media.Inspect(ctx, in)
	if err != nil {
		return prepared{}, err
	}
	src := ffmpeg.ConvertSource{DurationSec: info.Duration, HasVideo: info.HasVideo, HasAudio: info.HasAudio}
	if info.HasVideo {
		src.VideoIndex = firstVideoIndex(info)
	}
	outDir := dir
	if outDir == "" {
		outDir = filepath.Dir(in)
	}
	stem := strings.TrimSuffix(filepath.Base(in), filepath.Ext(in))
	out := filepath.Join(outDir, stem+"."+opts.Container)
	plan, err := ffmpeg.PlanConvert(in, out, opts, src)
	if err != nil {
		return prepared{}, err
	}
	j := prepared{out: out, dur: plan.OutDurationSec, src: src}
	if ffmpeg.IsImageContainer(opts.Container) {
		j.dur = 0 // 单帧输出没有时长进度（6.16.5）
	} else if src.DurationSec > 0 {
		j.expected = plan.OutDurationSec
	}
	return j, nil
}

func firstVideoIndex(m store.MediaInfo) int {
	for _, st := range m.Streams {
		if st.Type == "video" && !st.AttachedPic {
			return st.Index
		}
	}
	return 0
}

func withInput(err error, in string) error {
	ae := apperr.From(err)
	cp := *ae
	if cp.Detail == "" {
		cp.Detail = in
	} else {
		cp.Detail = in + "\n" + cp.Detail
	}
	return &cp
}

// resolveOutputDir 解析输出目录：参数 > 实际输出目录（v0.24：自定义优先，否则 <base>/output）> 空（源文件同目录，v0.24 起走不到）。
// 必须是绝对路径；已存在的必须是文件夹；不能在应用数据目录内（<dataDir>/output 除外）；不存在的会在任务开始时创建。
func (s *Service) resolveOutputDir(ctx context.Context, dir string) (string, error) {
	if dir == "" && s.cfg.DefaultOutputDir != nil {
		dir = s.cfg.DefaultOutputDir(ctx)
	}
	if dir == "" {
		return "", nil
	}
	if !filepath.IsAbs(dir) {
		return "", apperr.New(apperr.InvalidArgument, "输出目录必须是绝对路径").WithDetail(dir)
	}
	dir = filepath.Clean(dir)
	if fi, err := os.Stat(dir); err == nil && !fi.IsDir() {
		return "", apperr.New(apperr.InvalidArgument, "输出位置不是文件夹").WithDetail(dir)
	}
	if paths.InsideDataDir(s.cfg.DataDir, dir) {
		return "", apperr.New(apperr.InvalidArgument, "输出目录不能在应用数据目录内").WithDetail("outputDir 不能在应用数据目录内\n" + dir)
	}
	return dir, nil
}

// newFFmpegRunner 造 convert 的 FFmpegRunner；direct=true 时直接写 out（原地重转的临时文件），不走 RunWithPart。
func (s *Service) newFFmpegRunner(bin ffmpeg.Binaries, in, out string, direct bool, opts ffmpeg.ConvertOptions, dur float64, src ffmpeg.ConvertSource) *task.FFmpegRunner {
	// 只有真正重编码 H.264 / H.265 时才用硬件；copy、VP9、GIF、音频转换、两遍编码一律 CPU（契约 9.7）。
	codec := ffmpeg.ConvertHWCodec(opts)
	hw, info := ffmpeg.DecideEncoding(context.Background(), s.cfg.Encoder, codec, codec != "")
	cpuInfo := ffmpeg.EncoderInfo{Encoder: ffmpeg.ConvertEncoderName(opts), Device: "cpu"}
	if codec == "" {
		info = cpuInfo
		if info.Encoder == "copy" || info.Encoder == "" {
			info.Device = "" // copy / 没有视频编码：不属于任何设备
		}
	}
	build := func(hwEnc string) func(part string) []string {
		return func(part string) []string {
			plan, err := ffmpeg.PlanConvertHW(in, part, opts, src, hwEnc)
			if err != nil {
				return nil // prepare 已经用同样的参数验证过，不会走到这里；ffmpeg 会因缺少输出而失败
			}
			return plan.Final
		}
	}
	r := &task.FFmpegRunner{
		Exe:         bin.FFmpeg,
		Output:      out,
		DurationSec: dur,
		Classify:    ffmpeg.ClassifyConvertError,
		BuildArgs:   build(hw),
		Encoding:    info,
	}
	if direct {
		r.Output, r.DirectOutput = "", out
	}
	if hw != "" {
		r.HWEncoder, r.BuildCPUArgs, r.CPUEncoding = hw, build(""), cpuInfo
	}
	// 图片输出、时长未知的视频：-ss 1 没有产出时按第 0 帧重试一次（6.16.5）。
	if plan, err := ffmpeg.PlanConvertHW(in, out, opts, src, ""); err == nil && len(plan.Fallback) > 0 {
		r.BuildFallbackArgs = func(part string) []string {
			p, err := ffmpeg.PlanConvertHW(in, part, opts, src, "")
			if err != nil {
				return nil
			}
			return p.Fallback
		}
	}
	return r
}

// submitOne 提交一个 convert 任务：out 是期望名，任务管理器在落库之前按 "a (1).mp4" 格式定名并占位（契约 6.14.5）。
func (s *Service) submitOne(bin ffmpeg.Binaries, p params, sourceID string, j prepared) (task.Task, error) {
	out := j.out
	pj, _ := json.Marshal(p)
	spec := task.Spec{
		Type:          task.TypeConvert,
		Title:         filepath.Base(p.Input) + " → " + strings.ToUpper(p.Options.Container),
		InputPaths:    []string{p.Input},
		OutputPath:    out,
		Params:        string(pj),
		SourceID:      sourceID,
		ReserveOutput: true,
	}
	return s.cfg.Tasks.Submit(spec, s.newRunnerTo(bin, p.Input, out, false, p.Options, j))
}

// retryFactory 用 Params 重建 Runner：重新探测输入（文件可能已变化或被删除），重新验证参数。
func (s *Service) retryFactory(old task.Task) (task.Runner, error) {
	var p params
	if err := json.Unmarshal([]byte(old.Params), &p); err != nil || p.Input == "" {
		return nil, apperr.New(apperr.InvalidArgument, "转换任务参数无效，无法重试")
	}
	if s.cfg.Media == nil {
		return nil, apperr.New(apperr.Internal, "转换服务尚未初始化")
	}
	if err := validateOptions(p.Options); err != nil {
		return nil, err
	}
	bin, err := s.cfg.Require()
	if err != nil {
		return nil, err
	}
	if err := s.checkFormat(context.Background(), p.Options); err != nil { // 6.16.6 第 4 条
		return nil, err
	}
	j, err := s.prepare(context.Background(), p.Input, p.Options, p.OutputDir)
	if err != nil {
		return nil, withInput(err, p.Input)
	}
	return s.newRunnerTo(bin, p.Input, j.out, false, p.Options, j), nil
}

// newRunnerTo 见 newFFmpegRunner；外面包一层 resultRunner，成功后探测输出写 Task.Result（契约 6.14.6），
// 图片输出只留 sizeBytes / width / height，其余做 short_output 检查。
func (s *Service) newRunnerTo(bin ffmpeg.Binaries, in, out string, direct bool, opts ffmpeg.ConvertOptions, j prepared) task.Runner {
	return &resultRunner{FFmpegRunner: s.newFFmpegRunner(bin, in, out, direct, opts, j.dur, j.src), probe: s.probeResult,
		image: ffmpeg.IsImageContainer(opts.Container), expected: j.expected}
}
