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

// Config 是 Service 的依赖。
type Config struct {
	Presets PresetStore
	Media   Inspector
	Tasks   TaskSubmitter
	// Require 返回当前 ffmpeg，默认 ffmpeg.Require（缺失返回 FFMPEG_NOT_FOUND）。
	Require func() (ffmpeg.Binaries, error)
	// DefaultOutputDir 返回设置里的默认输出目录，空字符串表示与源文件同目录。可为 nil。
	DefaultOutputDir func(ctx context.Context) string
}

// Service 实现转换：无后台协程。
type Service struct{ cfg Config }

// New 创建 Service，写入内置预设，并注册 convert 任务的重试工厂。
func New(ctx context.Context, cfg Config) (*Service, error) {
	if cfg.Require == nil {
		cfg.Require = ffmpeg.Require
	}
	s := &Service{cfg: cfg}
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
	}
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

// validateOptions 校验转换参数；按目标大小压缩（两遍编码）暂缓，传 >0 直接拒绝。
func validateOptions(o ffmpeg.ConvertOptions) error {
	if o.TargetSizeMB != 0 {
		return apperr.New(apperr.InvalidArgument, "暂不支持按目标大小压缩")
	}
	return ffmpeg.ValidateConvertOptions(o)
}

// ---------- 提交 ----------

// params 是 convert 任务的 Params JSON（Retry 用它重建 Runner）。
type params struct {
	Input     string                `json:"input"`
	Options   ffmpeg.ConvertOptions `json:"options"`
	OutputDir string                `json:"outputDir"` // 已解析的最终输出目录
}

// Submit 为每个输入文件提交一个 convert 任务，返回的任务与 inputs 一一对应。
//
// 先校验全部输入（ffmpeg 就绪、参数合法、每个文件都能探测且与参数兼容），任何一个不通过整体失败、不提交任何任务，
// 错误的 detail 指出是哪个文件。最多 50 个文件（更多的由前端分批）。
// outputDir 为空时用设置里的默认输出目录，仍为空则输出到各自源文件所在的文件夹；输出名为 <源文件名>.<新扩展名>，
// 重名自动追加 (1)、(2)，绝不覆盖已有文件。
func (s *Service) Submit(ctx context.Context, inputs []string, opts ffmpeg.ConvertOptions, outputDir string) ([]task.Task, error) {
	if s.cfg.Tasks == nil || s.cfg.Media == nil {
		return nil, apperr.New(apperr.Internal, "转换服务尚未初始化")
	}
	if len(inputs) == 0 {
		return nil, apperr.New(apperr.InvalidArgument, "没有要转换的文件")
	}
	if len(inputs) > MaxInputsPerSubmit {
		return nil, apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次最多提交 %d 个文件，请分批提交", MaxInputsPerSubmit))
	}
	if err := validateOptions(opts); err != nil {
		return nil, err
	}
	bin, err := s.cfg.Require()
	if err != nil {
		return nil, err
	}
	dir, err := s.resolveOutputDir(ctx, outputDir)
	if err != nil {
		return nil, err
	}

	type job struct {
		in  string
		out string
		dur float64
		src ffmpeg.ConvertSource
	}
	jobs := make([]job, len(inputs))
	for i, raw := range inputs {
		in, _, err := paths.Normalize(raw)
		if err != nil {
			return nil, apperr.Wrap(apperr.InvalidArgument, "路径不合法", err).WithDetail(raw)
		}
		j, err := s.prepare(ctx, in, opts, dir)
		if err != nil {
			return nil, withInput(err, in)
		}
		jobs[i] = job{in: in, out: j.out, dur: j.dur, src: j.src}
	}

	var out []task.Task
	for _, j := range jobs {
		t, err := s.submitOne(bin, j.in, j.out, opts, dir, j.dur, j.src)
		if err != nil {
			return out, err // 已提交的保留（不回滚），调用方按返回的任务列表处理
		}
		out = append(out, t)
	}
	return out, nil
}

type prepared struct {
	out string
	dur float64
	src ffmpeg.ConvertSource
}

// prepare 探测输入、确定期望输出路径并用 PlanConvert 验证参数与输入兼容。
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
	plan, err := ffmpeg.PlanConvert(in, out, "", opts, src)
	if err != nil {
		return prepared{}, err
	}
	return prepared{out: out, dur: plan.OutDurationSec, src: src}, nil
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

// resolveOutputDir 解析输出目录：参数 > 设置里的默认目录 > 空（源文件同目录）。
// 必须是绝对路径；已存在的必须是文件夹；不存在的会在任务开始时创建。
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
	return dir, nil
}

func (s *Service) newRunner(bin ffmpeg.Binaries, in, out string, opts ffmpeg.ConvertOptions, dur float64, src ffmpeg.ConvertSource) *task.FFmpegRunner {
	return &task.FFmpegRunner{
		Exe:         bin.FFmpeg,
		Output:      out,
		DurationSec: dur,
		Classify:    ffmpeg.ClassifyConvertError,
		BuildArgs: func(part string) []string {
			plan, err := ffmpeg.PlanConvert(in, part, "", opts, src)
			if err != nil {
				return nil // prepare 已经用同样的参数验证过，不会走到这里；ffmpeg 会因缺少输出而失败
			}
			return plan.Final
		},
	}
}

func (s *Service) submitOne(bin ffmpeg.Binaries, in, out string, opts ffmpeg.ConvertOptions, dir string, dur float64, src ffmpeg.ConvertSource) (task.Task, error) {
	pj, _ := json.Marshal(params{Input: in, Options: opts, OutputDir: dir})
	spec := task.Spec{
		Type:       task.TypeConvert,
		Title:      filepath.Base(in) + " → " + strings.ToUpper(opts.Container),
		InputPaths: []string{in},
		OutputPath: out,
		Params:     string(pj),
	}
	return s.cfg.Tasks.Submit(spec, s.newRunner(bin, in, out, opts, dur, src))
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
	j, err := s.prepare(context.Background(), p.Input, p.Options, p.OutputDir)
	if err != nil {
		return nil, withInput(err, p.Input)
	}
	return s.newRunner(bin, p.Input, j.out, p.Options, j.dur, j.src), nil
}
