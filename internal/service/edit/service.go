package edit

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/fsutil"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/localassets"
	"FFmpegFree/internal/proc"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// ProjectStore 是工程持久化能力，*store.Store 实现它。
type ProjectStore interface {
	SaveEditProject(ctx context.Context, r store.EditProjectRow, insert bool) error
	GetEditProject(ctx context.Context, id string) (store.EditProjectRow, error)
	ListEditProjects(ctx context.Context, limit int) ([]store.EditProjectMetaRow, error)
	DeleteEditProject(ctx context.Context, id string) error
}

// TaskLister 列任务（启动时清理 interrupted 任务遗留的 .part），*store.Store 实现它。
type TaskLister interface {
	ListTasks(ctx context.Context, f store.TaskFilter) (store.TaskPage, error)
}

// Inspector 探测素材（*media.Service 实现）。
type Inspector interface {
	Inspect(ctx context.Context, path string) (store.MediaInfo, error)
}

// TaskSubmitter 是任务管理器的能力（*task.Manager 实现）。
type TaskSubmitter interface {
	Submit(spec task.Spec, r task.Runner) (task.Task, error)
	RegisterFactory(t task.Type, f task.Factory)
}

// Previewer 是 /local/<token> 登记表（*localassets.Registry 实现）。
type Previewer interface {
	Register(path string) (localassets.Entry, error)
}

// Config 是 Service 的依赖。
type Config struct {
	Projects ProjectStore
	Tasks    TaskSubmitter
	Lister   TaskLister
	Media    Inspector
	Preview  Previewer
	// Require 返回当前 ffmpeg / ffprobe，默认 ffmpeg.RequireProbe。
	Require func() (ffmpeg.Binaries, error)
	// DefaultOutputDir 返回设置里的默认输出目录，空 = 未设置。可为 nil。
	DefaultOutputDir func(ctx context.Context) string
	// TempDir 是任务临时目录（filtergraph 文件放在这下面的 edit-<随机> 里），空用 os.TempDir()。
	TempDir string
	// GOOS 用于 Windows 路径长度检查，默认 runtime.GOOS（测试里可改）。
	GOOS string
	// SupportsScript 功能探测 ffmpeg 从文件读 filtergraph 的选项，返回选中的选项（OptFilterFile 或 OptFilterScript）；
	// 两个都不支持返回 UNSUPPORTED。默认 probeFilterScript，结果按 ffmpeg 二进制（路径 + 大小 + 修改时间）缓存。
	SupportsScript func(ctx context.Context, exe string) (option string, err error)
	Now            func() time.Time
	// Encoder 按用户偏好与设备缓存解析 H.264 编码器（契约 9.7）；nil = 一律 CPU。每次导出（含重试）解析一次。
	Encoder ffmpeg.EncoderResolver
}

// Service 实现 EditService，无后台协程。
type Service struct {
	cfg      Config
	scriptOK sync.Map  // 路径|大小|mtime → 选中的选项（探测成功的）
	started  time.Time // 进程（服务）启动时间：只清理修改时间早于它的 .part
}

// New 创建 Service 并注册 edit_export 的重试工厂。
func New(cfg Config) *Service {
	if cfg.Require == nil {
		cfg.Require = ffmpeg.RequireProbe
	}
	if cfg.GOOS == "" {
		cfg.GOOS = runtime.GOOS
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Service{cfg: cfg, started: cfg.Now()}
	if cfg.SupportsScript == nil {
		s.cfg.SupportsScript = s.probeFilterScript
	}
	if cfg.Tasks != nil {
		cfg.Tasks.RegisterFactory(task.TypeEditExport, s.retryFactory)
	}
	return s
}

func canceledOr(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil {
		return apperr.Wrap(apperr.Canceled, "操作已取消", ctx.Err())
	}
	return err
}

// ---------- 校验 ----------

// ValidateProject 不落盘、不启动导出：探测素材并做全部校验，返回规范化后的时长与警告。
func (s *Service) ValidateProject(ctx context.Context, p EditProject) (EditPlan, error) {
	if s.cfg.Media == nil {
		return EditPlan{}, apperr.New(apperr.Internal, "剪辑服务尚未初始化")
	}
	pl, err := s.build(ctx, p)
	if err != nil {
		return EditPlan{}, canceledOr(ctx, err)
	}
	return pl.toPlan(), nil
}

// ---------- 导出 ----------

// params 是 edit_export 任务的 Params JSON（Retry 用它重建 Runner）。
type params struct {
	Project   EditProject       `json:"project"`
	Options   EditExportOptions `json:"options"`
	OutputDir string            `json:"outputDir"` // 已解析的最终输出目录
}

// Export 提交一个 edit_export 任务。先整体校验再提交，任何一项失败（含输出目录、输出路径过长、ffmpeg 不支持
// -/filter_complex 与 -filter_complex_script 都不可用）都不产生任务。
func (s *Service) Export(ctx context.Context, p EditProject, opts EditExportOptions) (task.Task, error) {
	if s.cfg.Tasks == nil || s.cfg.Media == nil {
		return task.Task{}, apperr.New(apperr.Internal, "剪辑服务尚未初始化")
	}
	t, err := s.export(ctx, p, opts, "")
	return t, canceledOr(ctx, err)
}

func (s *Service) export(ctx context.Context, p EditProject, opts EditExportOptions, fixedDir string) (task.Task, error) {
	r, spec, err := s.prepareExport(ctx, p, opts, fixedDir)
	if err != nil {
		return task.Task{}, err
	}
	return s.cfg.Tasks.Submit(spec, r)
}

// prepareExport 做提交前的全部检查并造出 Runner 与 Spec（Export 与 Retry 共用）。
// fixedDir 非空（Retry）时沿用原来解析好的输出目录。
func (s *Service) prepareExport(ctx context.Context, p EditProject, opts EditExportOptions, fixedDir string) (task.Runner, task.Spec, error) {
	bin, err := s.cfg.Require()
	if err != nil {
		return nil, task.Spec{}, err
	}
	pl, err := s.build(ctx, p)
	if err != nil {
		return nil, task.Spec{}, err
	}
	dir, err := s.resolveOutputDir(ctx, opts.OutputDir, fixedDir, p)
	if err != nil {
		return nil, task.Spec{}, err
	}
	if err := checkWritableDir(dir); err != nil {
		return nil, task.Spec{}, err
	}
	name := fsutil.SanitizeFileNameOr(firstNonEmpty(opts.OutputName, p.Name), fsutil.DefaultName)
	ext := "." + pl.format
	if fsutil.OutputPathTooLong(s.cfg.GOOS, dir, name, ext) {
		return nil, task.Spec{}, projectErr(apperr.InvalidArgument, "输出路径太长（Windows 上整条路径不能超过 259 个字符），请换一个较短的输出文件夹或文件名",
			fmt.Sprintf("path_length=%d limit=%d", fsutil.OutputPathLength(dir, name, ext), fsutil.WindowsMaxPath))
	}

	out := filepath.Join(dir, name+ext)
	pj, err := json.Marshal(params{Project: p, Options: opts, OutputDir: dir})
	if err != nil {
		return nil, task.Spec{}, apperr.Wrap(apperr.Internal, "序列化任务参数失败", err)
	}
	spec := task.Spec{
		Type:       task.TypeEditExport,
		Title:      name + ext,
		InputPaths: pl.inputs,
		OutputPath: out,
		Params:     string(pj),
	}
	return s.newRunner(bin, pl, out), spec, nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// resolveOutputDir：Retry 的固定目录 > 参数 > 设置里的默认目录 > 第一个视频 clip 所在文件夹。必须是绝对路径。
func (s *Service) resolveOutputDir(ctx context.Context, dir, fixed string, p EditProject) (string, error) {
	if fixed != "" {
		dir = fixed
	}
	if dir == "" && s.cfg.DefaultOutputDir != nil {
		dir = s.cfg.DefaultOutputDir(ctx)
	}
	if dir == "" && len(p.VideoTrack) > 0 {
		dir = filepath.Dir(p.VideoTrack[0].Path)
	}
	if hasDevicePrefix(dir) {
		return "", projectErr(apperr.InvalidArgument, `输出目录不能以 \\?\ 或 \\.\ 开头`, "outputDir="+cleanLine(dir))
	}
	if dir == "" || !filepath.IsAbs(dir) || strings.IndexFunc(dir, isCtrl) >= 0 {
		return "", projectErr(apperr.InvalidArgument, "输出目录必须是绝对路径", "outputDir="+cleanLine(dir))
	}
	return filepath.Clean(dir), nil
}

// checkWritableDir 在提交时校验输出目录：已存在的必须是文件夹且可写；不存在的（任务开始时才创建）要求最近的已存在上级是可写文件夹。
// 用创建再删除的临时文件探测（比看权限位可靠，Windows 上也一样）。
func checkWritableDir(dir string) error {
	probe := dir
	for {
		fi, err := os.Stat(probe)
		if err == nil {
			if !fi.IsDir() {
				return projectErr(apperr.InvalidArgument, "输出位置不是文件夹", "outputDir="+cleanLine(probe))
			}
			break
		}
		if errors.Is(err, syscall.ENOTDIR) { // 上级路径里有一段是文件
			return projectErr(apperr.InvalidArgument, "输出位置的上级不是文件夹", "outputDir="+cleanLine(dir))
		}
		if !errors.Is(err, os.ErrNotExist) {
			return apperr.Wrap(apperr.IOError, "无法访问输出目录", err)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return projectErr(apperr.InvalidArgument, "输出目录不存在且无法创建", "outputDir="+cleanLine(dir))
		}
		probe = parent
	}
	f, err := os.CreateTemp(probe, ".ffmpegfree-write-test-*")
	if err != nil {
		return apperr.Wrap(apperr.IOError, "没有权限写入输出目录", err)
	}
	f.Close()
	os.Remove(f.Name())
	return nil
}

func (s *Service) retryFactory(old task.Task) (task.Runner, error) {
	var p params
	if err := json.Unmarshal([]byte(old.Params), &p); err != nil || len(p.Project.VideoTrack) == 0 {
		return nil, apperr.New(apperr.InvalidArgument, "剪辑任务参数无效，无法重试")
	}
	if s.cfg.Media == nil {
		return nil, apperr.New(apperr.Internal, "剪辑服务尚未初始化")
	}
	r, _, err := s.prepareExport(context.Background(), p.Project, p.Options, p.OutputDir)
	return r, err
}

// ---------- 预览 ----------

var previewExts = map[string]bool{".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".flv": true, ".webm": true,
	".m4v": true, ".mp3": true, ".wav": true, ".aac": true, ".m4a": true, ".flac": true, ".ogg": true}

// GetPreviewURL 登记文件并返回 /local/<token>。路径必须是绝对路径、扩展名在允许列表内、是普通文件。
// 前端遇到 404（token 失效：淘汰、文件被换）应重新调用。
func (s *Service) GetPreviewURL(path string) (PreviewURL, error) {
	if s.cfg.Preview == nil {
		return PreviewURL{}, apperr.New(apperr.Internal, "预览服务尚未初始化")
	}
	if path == "" || !filepath.IsAbs(path) {
		return PreviewURL{}, apperr.New(apperr.InvalidArgument, "预览路径必须是绝对路径").WithDetail(cleanLine(path))
	}
	if hasDevicePrefix(path) {
		return PreviewURL{}, apperr.New(apperr.InvalidArgument, `预览路径不能以 \\?\ 或 \\.\ 开头`).WithDetail(cleanLine(path))
	}
	if !previewExts[strings.ToLower(filepath.Ext(path))] {
		return PreviewURL{}, apperr.New(apperr.InvalidArgument, "不支持预览这种文件类型").WithDetail(cleanLine(path))
	}
	e, err := s.cfg.Preview.Register(path)
	switch {
	case err == nil:
		return PreviewURL{URL: e.URL, Mime: e.Mime, Size: e.Size}, nil
	case errors.Is(err, localassets.ErrNotRegular):
		return PreviewURL{}, apperr.New(apperr.InvalidArgument, "这不是一个普通文件").WithDetail(cleanLine(path))
	case errors.Is(err, os.ErrNotExist):
		return PreviewURL{}, apperr.New(apperr.NotFound, "文件不存在").WithDetail(cleanLine(path))
	default:
		return PreviewURL{}, apperr.Wrap(apperr.IOError, "无法读取文件", err)
	}
}

// ---------- 工程存取 ----------

// SaveProject 保存工程（id 空 = 新建）。只校验数量 / 大小上限、名称和 schemaVersion，不校验同轨重叠、素材是否存在
// （草稿可保存；重叠只在 ValidateProject / Export 里报）。
func (s *Service) SaveProject(ctx context.Context, p EditProject) (EditProjectMeta, error) {
	if s.cfg.Projects == nil {
		return EditProjectMeta{}, apperr.New(apperr.Internal, "工程存储不可用")
	}
	if p.SchemaVersion == 0 {
		p.SchemaVersion = 1
	}
	p.Name = strings.TrimSpace(p.Name)
	if err := checkName(p.Name); err != nil {
		return EditProjectMeta{}, err
	}
	insert := p.ID == ""
	if insert {
		p.ID = id.New()
	} else if !idOK(p.ID) {
		return EditProjectMeta{}, apperr.New(apperr.NotFound, "工程不存在")
	}
	p.UpdatedAt = s.cfg.Now().UnixMilli()
	if p.Sources == nil {
		p.Sources = []string{}
	}
	if p.VideoTrack == nil {
		p.VideoTrack = []VideoClip{}
	}
	if p.AudioTrack == nil {
		p.AudioTrack = []AudioClip{}
	}
	if err := checkCounts(p); err != nil { // 含 schemaVersion > 1 → UNSUPPORTED
		return EditProjectMeta{}, err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return EditProjectMeta{}, apperr.Wrap(apperr.InvalidArgument, "工程无法序列化", err)
	}
	dur, n := roughDuration(p), len(p.VideoTrack)+len(p.AudioTrack)
	row := store.EditProjectRow{ID: p.ID, Name: p.Name, Project: string(b), DurationSec: dur, ClipCount: n, UpdatedAt: p.UpdatedAt}
	if err := s.cfg.Projects.SaveEditProject(ctx, row, insert); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EditProjectMeta{}, apperr.New(apperr.NotFound, "工程不存在")
		}
		return EditProjectMeta{}, canceledOr(ctx, apperr.Wrap(apperr.IOError, "保存工程失败", err))
	}
	return EditProjectMeta{ID: p.ID, Name: p.Name, DurationSec: dur, ClipCount: n, UpdatedAt: p.UpdatedAt}, nil
}

func idOK(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// roughDuration 不探测素材，按 clip 自己填的值估算时长（草稿里可能有非法值，忽略它们）。
func roughDuration(p EditProject) float64 {
	end := func(start, in, out, speed float64) float64 {
		if !finite(start, in, out, speed) || start < 0 || out <= in {
			return 0
		}
		if speed <= 0 {
			speed = 1
		}
		return start + (out-in)/speed
	}
	d := 0.0
	for _, c := range p.VideoTrack {
		d = math.Max(d, end(c.StartSec, c.InSec, c.OutSec, c.Speed))
	}
	for _, c := range p.AudioTrack {
		d = math.Max(d, end(c.StartSec, c.InSec, c.OutSec, c.Speed))
	}
	return d
}

// LoadProject 读取工程。素材丢失不报错，放进 MissingPaths（sources 与 clip 路径的并集，按首次出现顺序）。
func (s *Service) LoadProject(ctx context.Context, projectID string) (LoadedProject, error) {
	if s.cfg.Projects == nil {
		return LoadedProject{}, apperr.New(apperr.Internal, "工程存储不可用")
	}
	row, err := s.cfg.Projects.GetEditProject(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return LoadedProject{}, apperr.New(apperr.NotFound, "工程不存在")
	}
	if err != nil {
		return LoadedProject{}, canceledOr(ctx, apperr.Wrap(apperr.IOError, "读取工程失败", err))
	}
	var p EditProject
	if err := json.Unmarshal([]byte(row.Project), &p); err != nil {
		return LoadedProject{}, apperr.Wrap(apperr.IOError, "工程数据已损坏", err)
	}
	if p.SchemaVersion > 1 {
		return LoadedProject{}, apperr.New(apperr.Unsupported, "工程版本过新，请升级应用后再打开").WithDetail(fmt.Sprintf("schemaVersion=%d", p.SchemaVersion))
	}
	if p.SchemaVersion == 0 {
		p.SchemaVersion = 1
	}
	p.ID, p.Name, p.UpdatedAt = row.ID, row.Name, row.UpdatedAt
	if p.Sources == nil {
		p.Sources = []string{}
	}
	if p.VideoTrack == nil {
		p.VideoTrack = []VideoClip{}
	}
	if p.AudioTrack == nil {
		p.AudioTrack = []AudioClip{}
	}
	missing := []string{}
	seen := map[string]bool{}
	check := func(path string) {
		if seen[path] {
			return
		}
		seen[path] = true
		if path == "" || !filepath.IsAbs(path) {
			missing = append(missing, path) // 相对路径不可能对应磁盘上的素材（也不能按当前目录解析）
			return
		}
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			missing = append(missing, path)
		}
	}
	for _, x := range p.Sources {
		check(x)
	}
	for _, c := range p.VideoTrack {
		check(c.Path)
	}
	for _, c := range p.AudioTrack {
		check(c.Path)
	}
	return LoadedProject{Project: p, MissingPaths: missing}, nil
}

// ListProjects 按 updatedAt 倒序返回工程摘要（默认 50，最大 200）。
func (s *Service) ListProjects(ctx context.Context, limit int) ([]EditProjectMeta, error) {
	if s.cfg.Projects == nil {
		return nil, apperr.New(apperr.Internal, "工程存储不可用")
	}
	rows, err := s.cfg.Projects.ListEditProjects(ctx, limit)
	if err != nil {
		return nil, canceledOr(ctx, apperr.Wrap(apperr.IOError, "读取工程列表失败", err))
	}
	out := make([]EditProjectMeta, len(rows))
	for i, r := range rows {
		out[i] = EditProjectMeta{ID: r.ID, Name: r.Name, DurationSec: r.DurationSec, ClipCount: r.ClipCount, UpdatedAt: r.UpdatedAt}
	}
	return out, nil
}

// DeleteProject 删除工程（不删素材和导出文件）。不存在返回 NOT_FOUND。
func (s *Service) DeleteProject(ctx context.Context, projectID string) error {
	if s.cfg.Projects == nil {
		return apperr.New(apperr.Internal, "工程存储不可用")
	}
	if err := s.cfg.Projects.DeleteEditProject(ctx, projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apperr.New(apperr.NotFound, "工程不存在")
		}
		return canceledOr(ctx, apperr.Wrap(apperr.IOError, "删除工程失败", err))
	}
	return nil
}

// ---------- 启动清理 ----------

// CleanupInterruptedParts 启动时调用：删除 interrupted 的 edit_export 任务遗留的 .part 文件
// （<name>.part.<ext>，以及重名时可能占用的 <name>(1..99).part.<ext>）。只删普通文件（Lstat，不跟随符号链接）且修改时间早于本次启动的。返回删除个数。
func (s *Service) CleanupInterruptedParts(ctx context.Context) int {
	if s.cfg.Lister == nil {
		return 0
	}
	removed := 0
	for offset := 0; offset < 5000; {
		page, err := s.cfg.Lister.ListTasks(ctx, store.TaskFilter{
			Types: []store.TaskType{store.TypeEditExport}, Statuses: []store.TaskStatus{store.StatusInterrupted}, Limit: 200, Offset: offset})
		if err != nil || len(page.Items) == 0 {
			break
		}
		for _, t := range page.Items {
			if t.OutputPath == "" || !filepath.IsAbs(t.OutputPath) {
				continue
			}
			ext := filepath.Ext(t.OutputPath)
			base := strings.TrimSuffix(t.OutputPath, ext)
			cands := []string{base + ".part" + ext}
			for i := 1; i <= 99; i++ {
				cands = append(cands, fmt.Sprintf("%s(%d).part%s", base, i, ext))
			}
			for _, c := range cands {
				if fi, err := os.Lstat(c); err == nil && fi.Mode().IsRegular() && fi.ModTime().Before(s.started) {
					if os.Remove(c) == nil {
						removed++
					}
				}
			}
		}
		offset += len(page.Items)
	}
	return removed
}

// ---------- ffmpeg 能力探测 ----------

// 从文件读 filtergraph 的两个 ffmpeg 选项。不按版本号判断，只做功能探测：先探新的（7.0 起），不支持再探旧的（6.x；9.0 已移除）。
const (
	OptFilterFile   = "-/filter_complex"       // 后面跟文件路径；ffmpeg 7.0 起
	OptFilterScript = "-filter_complex_script" // ffmpeg 6.x；7.x 仍可用但打印 deprecated，9.0 已移除
)

// probeFilterScript 功能探测（不解析帮助文本、不看版本号），跑一个小样本：
//
//	ffmpeg -hide_banner -nostdin -loglevel error -f lavfi -i nullsrc=s=32x32:r=5:d=0.4
//	       <选项> <file:[0:v]scale=16:16[v]> -map [v] -f null -
//
// 先用 OptFilterFile，退出码 0 = 可用；stderr 含 `Unrecognized option` 才继续探 OptFilterScript；两个都不认识返回 UNSUPPORTED，
// detail 第一行 `project`、第二行 `missing=filter_complex`。其他失败（超时、崩溃、别的错误）返回 PROCESS_FAILED，不冒充 UNSUPPORTED。
// 成功结果（选中的选项）按 ffmpeg 路径 + 文件大小 + 修改时间缓存到进程内。
func (s *Service) probeFilterScript(ctx context.Context, exe string) (string, error) {
	key := exe
	if fi, err := os.Stat(exe); err == nil {
		key = fmt.Sprintf("%s|%d|%d", exe, fi.Size(), fi.ModTime().UnixNano())
	}
	if v, ok := s.scriptOK.Load(key); ok {
		return v.(string), nil
	}
	tmp, err := os.MkdirTemp(s.cfg.TempDir, "edit-probe-")
	if err != nil {
		return "", apperr.Wrap(apperr.IOError, "创建临时目录失败", err)
	}
	defer os.RemoveAll(tmp)
	script := filepath.Join(tmp, "probe.txt")
	if err := os.WriteFile(script, []byte("[0:v]scale=16:16[v]\n"), 0o600); err != nil {
		return "", apperr.Wrap(apperr.IOError, "写入临时文件失败", err)
	}
	for _, opt := range []string{OptFilterFile, OptFilterScript} {
		ok, err := s.tryFilterOption(ctx, exe, opt, script)
		if err != nil {
			return "", err
		}
		if ok {
			s.scriptOK.Store(key, opt)
			return opt, nil
		}
	}
	return "", projectErr(apperr.Unsupported, "当前 ffmpeg 版本不支持从文件读取滤镜图（-/filter_complex、-filter_complex_script 都不可用），无法导出多轨剪辑，请安装应用推荐的 ffmpeg 版本", "missing=filter_complex")
}

// tryFilterOption 用 opt 跑一次探测样本：可用 (true, nil)；ffmpeg 不认识该选项 (false, nil)；其他失败返回错误。
func (s *Service) tryFilterOption(ctx context.Context, exe, opt, script string) (bool, error) {
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := ffmpeg.NewCommand(pctx, exe, "-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", "nullsrc=s=32x32:r=5:d=0.4",
		opt, script, "-map", "[v]", "-f", "null", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := proc.Run(cmd)
	if err == nil {
		return true, nil
	}
	if ctx.Err() != nil {
		return false, apperr.Wrap(apperr.Canceled, "操作已取消", ctx.Err())
	}
	if strings.Contains(strings.ToLower(stderr.String()), "unrecognized option") {
		return false, nil
	}
	return false, apperr.Wrap(apperr.ProcessFailed, "检测 ffmpeg 的滤镜脚本能力失败", err).WithDetail(tailLines(stderr.String(), 10))
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
