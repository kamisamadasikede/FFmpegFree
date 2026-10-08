// Package live 是 LiveService 的实现（契约 v0.10 第 4 节 LiveService、6.10）：
// 文件推流、屏幕推流、采集能力检测、推流地址校验。推流本身是任务管理器里的 live 池任务，停止走 TaskService.Cancel。
package live

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/id"
	livepkg "FFmpegFree/internal/live"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

const (
	// MaxSessions 是同时进行的直播会话上限。
	MaxSessions = 4

	defaultVideoKbps = 2500
	defaultAudioKbps = 128
)

// TaskSubmitter 是任务管理器的能力（*task.Manager 实现）。
type TaskSubmitter interface {
	Submit(spec task.Spec, r task.Runner) (task.Task, error)
}

// Inspector 探测输入文件（*media.Service 实现）。
type Inspector interface {
	Inspect(ctx context.Context, path string) (store.MediaInfo, error)
}

// Config 是 Service 的依赖，测试可以替换其中任何一项。
type Config struct {
	Tasks TaskSubmitter
	Media Inspector
	// Require 返回当前 ffmpeg，默认 ffmpeg.Require（缺失返回 FFMPEG_NOT_FOUND）。
	Require func() (ffmpeg.Binaries, error)
	// Protocols 检查 ffmpeg 支持的输出协议，默认真实执行 ffmpeg -protocols（按路径缓存）。
	Protocols *ffmpeg.ProtocolProbe
	// GOOS 默认 runtime.GOOS；Getenv 默认 os.Getenv；Now 默认 time.Now（测试用）。
	GOOS   string
	Getenv func(string) string
	Now    func() time.Time
	// Run 运行外部命令取标准输出（xrandr、ffmpeg -list_devices），默认 ffmpeg.ExecRunner(10s)。
	Run ffmpeg.Runner
	// ProbeDuration 读存档时长，读不出返回错误（空壳判定）；默认用 ffprobe。Remove 删除空壳存档，默认 os.Remove（测试用）。
	ProbeDuration func(ctx context.Context, ffprobe, path string) (float64, error)
	Remove        func(path string) error
	// EnumWindows 枚举顶层窗口（未过滤），Monitors 枚举 Windows 显示器（测试注入）。默认只在 GOOS 等于真实系统时用平台实现：
	// EnumWindows 默认 enumTopLevelWindows（仅 Windows 有意义），Monitors 默认 listWindowsMonitors。
	EnumWindows func() ([]RawWindow, error)
	Monitors    func() ([]ScreenInfo, error)
	// Grace 覆盖优雅停止的等待时间（测试用）；0 用默认（无存档 5 秒、有存档 15 秒）。
	Grace time.Duration
	// Encoder 按用户偏好与设备缓存解析 H.264 编码器（契约 9.7）；nil = 一律 CPU（libx264）。每次开始推流解析一次。
	Encoder ffmpeg.EncoderResolver
	// Logf 记录内部信息；只会收到脱敏内容。为空时不记录。
	Logf func(format string, args ...any)
	// PreviewDir 是预览 JPEG 的临时目录（<数据目录>/tmp/live-preview）；空 = 不出预览（推流照常，拉流预览返回 UNSUPPORTED）。
	PreviewDir string
	// Preview 检查 ffmpeg 能否出预览，默认真实执行 ffmpeg（按路径缓存）；ProbeStreams 探测拉流地址里有没有视频，默认用 ffprobe。
	Preview      *ffmpeg.PreviewProbe
	ProbeStreams func(ctx context.Context, ffprobe, url, whitelist string) (hasVideo bool, err error)
}

// Service 实现直播推流。会话登记在内存里（上限 4、同地址 1 个）。
type Service struct {
	cfg Config

	mu       sync.Mutex
	sessions map[string]session      // 任务 ID → 会话
	pulls    map[string]*pullSession // 拉流预览会话 ID → 会话（不占推流会话名额）
}

type session struct {
	key     string // 标准化地址（只在内存里比较，不展示）
	archive bool   // 有本地存档（优雅停止等 15 秒）
	screen  bool   // 屏幕推流（同一时间最多 1 路）
	preview string // 预览 JPEG 路径；空 = 没有预览。会话结束时删除
}

// New 创建 Service。
func New(cfg Config) *Service {
	if cfg.Require == nil {
		cfg.Require = ffmpeg.Require
	}
	if cfg.Protocols == nil {
		cfg.Protocols = &ffmpeg.ProtocolProbe{}
	}
	if cfg.GOOS == "" {
		cfg.GOOS = runtime.GOOS
	}
	if cfg.Getenv == nil {
		cfg.Getenv = os.Getenv
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.ProbeDuration == nil {
		cfg.ProbeDuration = probeDuration
	}
	if cfg.Remove == nil {
		cfg.Remove = os.Remove
	}
	if cfg.EnumWindows == nil && cfg.GOOS == runtime.GOOS {
		cfg.EnumWindows = enumTopLevelWindows
	}
	if cfg.Monitors == nil {
		cfg.Monitors = listWindowsMonitors
	}
	if cfg.Run == nil {
		cfg.Run = ffmpeg.ExecRunner(10 * time.Second)
	}
	if cfg.Preview == nil {
		cfg.Preview = &ffmpeg.PreviewProbe{}
	}
	if cfg.ProbeStreams == nil {
		cfg.ProbeStreams = probeStreams
	}
	return &Service{cfg: cfg, sessions: map[string]session{}, pulls: map[string]*pullSession{}}
}

func (s *Service) logf(format string, args ...any) {
	if s.cfg.Logf != nil {
		s.cfg.Logf(format, args...)
	}
}

// ---------- 选项 ----------

// PushOptions 是两个 Start* 共用的编码选项，零值 = 默认（契约 3 节 / LiveService）。
type PushOptions struct {
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	Fps              float64 `json:"fps"`
	VideoBitrateKbps int     `json:"videoBitrateKbps"`
	AudioBitrateKbps int     `json:"audioBitrateKbps"`
}

// FilePushRequest 是 StartFilePush 的参数。
type FilePushRequest struct {
	InputPath string      `json:"inputPath"`
	URL       string      `json:"url"`
	Loop      bool        `json:"loop"`
	Options   PushOptions `json:"options"`
	// Preview 为 nil（缺省）或 true 时会话带预览画面（GetPreview）；false 时不加预览输出。
	Preview *bool `json:"preview"`
}

// PushURLInfo 是 CheckPushURL 的返回。
type PushURLInfo struct {
	Scheme   string `json:"scheme"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Redacted string `json:"redacted"`
}

func invalidArg(format string, a ...any) *apperr.AppError {
	return apperr.New(apperr.InvalidArgument, fmt.Sprintf(format, a...))
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func (o PushOptions) validate() error {
	if o.Width < 0 || o.Width > 8192 || o.Height < 0 || o.Height > 8192 {
		return invalidArg("分辨率必须在 0~8192 之间")
	}
	if !finite(o.Fps) || (o.Fps != 0 && (o.Fps < 1 || o.Fps > 60)) {
		return invalidArg("帧率必须是 0（默认）或 1~60")
	}
	if o.VideoBitrateKbps != 0 && (o.VideoBitrateKbps < 100 || o.VideoBitrateKbps > 50000) {
		return invalidArg("视频码率必须是 0（默认 2500）或 100~50000 kbit/s")
	}
	if o.AudioBitrateKbps != 0 && (o.AudioBitrateKbps < 32 || o.AudioBitrateKbps > 512) {
		return invalidArg("音频码率必须是 0（默认 128）或 32~512 kbit/s")
	}
	return nil
}

func (o PushOptions) videoKbps() int {
	if o.VideoBitrateKbps == 0 {
		return defaultVideoKbps
	}
	return o.VideoBitrateKbps
}

func (o PushOptions) audioKbps() int {
	if o.AudioBitrateKbps == 0 {
		return defaultAudioKbps
	}
	return o.AudioBitrateKbps
}

// ---------- 地址 ----------

func urlError(e error) *apperr.AppError {
	ue, ok := e.(*livepkg.URLError)
	if !ok {
		return apperr.New(apperr.LiveURLInvalid, "推流地址不合法").WithDetail("reason=" + livepkg.ReasonMalformed)
	}
	// detail 第一行固定 reason=<值>，整个 detail 和 message 都不带地址、口令或它们的片段（契约 2.2）。
	return apperr.New(apperr.LiveURLInvalid, ue.Message).WithDetail("reason=" + ue.Reason)
}

func parseURL(raw string) (livepkg.PushURL, error) {
	u, err := livepkg.ParsePushURL(raw)
	if err != nil {
		return livepkg.PushURL{}, urlError(err)
	}
	return u, nil
}

// CheckPushURL 只校验地址并返回脱敏后的显示文本，不联网，不检查会话冲突。
func (s *Service) CheckPushURL(raw string) (PushURLInfo, error) {
	u, err := parseURL(raw)
	if err != nil {
		return PushURLInfo{}, err
	}
	return PushURLInfo{Scheme: u.Scheme, Host: u.Host, Port: u.Port, Redacted: u.Redacted}, nil
}

// checkProtocols 用 ffmpeg -protocols 确认 Output 段有需要的协议，缺哪个返回 UNSUPPORTED（detail 写 missing=<协议名>）。
func (s *Service) checkProtocols(ctx context.Context, bin ffmpeg.Binaries, scheme string, needTee bool) error {
	p, err := s.cfg.Protocols.OutputProtocols(ctx, bin.FFmpeg)
	if err != nil {
		if ctx.Err() != nil {
			return apperr.Wrap(apperr.Canceled, "操作已取消", ctx.Err())
		}
		return apperr.Wrap(apperr.Internal, "检查转换组件支持的协议失败", err)
	}
	var need []string
	switch scheme {
	case "srt":
		need = append(need, "srt")
	case "rtmps":
		need = append(need, "rtmps")
	default:
		need = append(need, "rtmp")
	}
	if needTee {
		need = append(need, "tee")
	}
	for _, n := range need {
		if !p[n] {
			return apperr.New(apperr.Unsupported, "当前转换组件不支持 "+n+"，请安装完整版转换组件").WithDetail("missing=" + n)
		}
	}
	return nil
}

// ---------- 会话登记 ----------

// reserve 登记一个会话。检查和登记在同一把锁里完成，并发的两次 Start 不会同时成功。
// 判断顺序固定（都是 TASK_CONFLICT，detail 首行固定 reason=…，不含任何地址信息，文案由前端负责）：
//  1. duplicate_url：同一标准化地址已有会话；
//  2. screen_busy：要开的是屏幕推流，且已有进行中（含已入队未结束）的屏幕推流会话（屏幕推流同一时间最多 1 路；文件推流不受影响）；
//  3. max_sessions：会话总数已达上限。
func (s *Service) reserve(taskID, key string, archive, screen bool, preview string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	busy := false
	for _, x := range s.sessions {
		if x.key == key {
			return apperr.New(apperr.TaskConflict, "这个推流地址已经在推流").
				WithDetail("reason=duplicate_url\n已有会话使用同一推流地址")
		}
		busy = busy || x.screen
	}
	if screen && busy {
		return apperr.New(apperr.TaskConflict, "已有屏幕推流正在进行").
			WithDetail("reason=screen_busy")
	}
	if len(s.sessions) >= MaxSessions {
		return apperr.New(apperr.TaskConflict, "同时进行的直播会话已达上限").
			WithDetail(fmt.Sprintf("reason=max_sessions\n最多同时推 %d 路", MaxSessions))
	}
	s.sessions[taskID] = session{key: key, archive: archive, screen: screen, preview: preview}
	return nil
}

func (s *Service) release(taskID string) {
	s.mu.Lock()
	x := s.sessions[taskID]
	delete(s.sessions, taskID)
	s.mu.Unlock()
	removePreviewFiles(x.preview) // 会话结束（含从未运行）清理预览文件
}

// ActiveSessions 返回进行中的会话数，HasArchiveSession 表示其中有带本地存档的（应用退出时要多等）。
func (s *Service) ActiveSessions() (n int, hasArchive bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.sessions {
		if x.archive {
			hasArchive = true
		}
	}
	return len(s.sessions), hasArchive
}

// ---------- Runner ----------

// runner 包装 task.FFmpegRunner：任务结束（含从未运行）时释放会话登记。
type runner struct {
	inner *task.FFmpegRunner
	s     *Service
	id    string
}

func (r *runner) Run(ctx context.Context, report func(task.Progress)) (string, error) {
	defer r.s.release(r.id)
	return r.inner.Run(ctx, report)
}

// EncoderInfo 实现 task.EncoderReporter。
func (r *runner) EncoderInfo() ffmpeg.EncoderInfo { return r.inner.Encoding }

// OnFinish 实现 task.Finalizer：排队中被取消的任务不会执行 Run，也要释放。
func (r *runner) OnFinish(task.Task) { r.s.release(r.id) }

const (
	graceNoArchive = 5 * time.Second
	graceArchive   = 15 * time.Second
)

// newRunner 构造直播 Runner。started 由 ReportGate 置位：第一个 progress=continue 且 out_time_us>0。
//
// enc 是这次推流的编码器决策（resolveLiveEncoder）：硬件编码时 args 已是硬件参数，enc.cpuArgs 是同一推流的 CPU 参数，
// 只在"推流尚未建立"（还没有第一个输出进度）时硬件编码启动失败才会自动用 CPU 重试一次；推流中途失败不重试。
func (s *Service) newRunner(taskID string, bin ffmpeg.Binaries, u livepkg.PushURL, rawURL string, args []string, enc liveEncoding, screen, archive bool) *runner {
	redact := chainRedact(livepkg.NewRedactor(rawURL), livepkg.NewRedactor(u.FFmpeg))
	var started atomic.Bool
	grace := graceNoArchive
	if archive {
		grace = graceArchive
	}
	if s.cfg.Grace > 0 {
		grace = s.cfg.Grace
	}
	inner := &task.FFmpegRunner{
		Exe:                       bin.FFmpeg,
		BuildArgs:                 func(string) []string { return args },
		Live:                      true,
		Encoding:                  enc.info,
		Redact:                    redact,
		GracePeriod:               grace,
		GracefulOnlyAfterProgress: true,
		StrictGracefulExit:        true,
		NoBitrate:                 archive, // tee 下 total_size 恒为 N/A，不算 bitrateKbps
		ReportGate: func(p ffmpeg.ProgressUpdate) bool {
			if !p.End && p.OutTimeSec > 0 {
				started.Store(true)
				return true
			}
			return false
		},
		Classify: func(tail string, _ error) *apperr.AppError {
			return ffmpeg.ClassifyLiveError(ffmpeg.LiveClassifyInput{Tail: tail, Scheme: u.Scheme, Started: started.Load(), Screen: screen})
		},
	}
	// v0.24.5：完整 argv 记进应用日志，排查帧率、预览这类问题时直接看日志。推流地址整段换成占位符（连主机和端口也不写），
	// 其余再过一遍脱敏。
	logArgs := func(a []string) string {
		line := strings.Join(a, " ")
		for _, raw := range []string{u.FFmpeg, ffmpeg.TeeEscape(u.FFmpeg), rawURL} {
			if raw != "" {
				line = strings.ReplaceAll(line, raw, "<推流地址>")
			}
		}
		return redact(line)
	}
	s.logf("直播 %s ffmpeg 参数: %s", taskID, logArgs(args))
	if enc.hw != "" {
		s.logf("直播 %s 硬件编码启动失败时的 CPU 参数: %s", taskID, logArgs(enc.cpuArgs))
		inner.HWEncoder, inner.CPUEncoding = enc.hw, enc.cpuInfo
		inner.BuildCPUArgs = func(string) []string { return enc.cpuArgs }
	}
	return &runner{inner: inner, s: s, id: taskID}
}

// liveEncoding 是一次直播推流的编码器决策。
type liveEncoding struct {
	hw      string             // 硬件 H.264 编码器名，CPU 为 ""
	info    ffmpeg.EncoderInfo // 开始时的编码器信息
	cpuInfo ffmpeg.EncoderInfo // 回退 CPU 后的编码器信息
	cpuArgs []string           // 硬件编码时的 CPU 参数（回退用）
}

// resolveLiveEncoder 解析直播编码器并生成参数：build 用给定的 LiveEncode 生成完整 ffmpeg 参数。
// 直播恒为 H.264 重编码，所以总是"可用硬件"（受尺寸限制）。
func (s *Service) resolveLiveEncoder(ctx context.Context, enc ffmpeg.LiveEncode, build func(ffmpeg.LiveEncode) []string) ([]string, liveEncoding) {
	hw, info := ffmpeg.DecideEncoding(ctx, s.cfg.Encoder, "h264", ffmpeg.HWDimsOK("h264", enc.Width, enc.Height))
	le := liveEncoding{hw: hw, info: info, cpuInfo: ffmpeg.EncoderInfo{Encoder: "libx264", Device: "cpu"}}
	enc.HW = ""
	cpuArgs := build(enc)
	if hw == "" {
		return cpuArgs, le
	}
	le.cpuArgs = cpuArgs
	enc.HW = hw
	return build(enc), le
}

func chainRedact(fs ...func(string) string) func(string) string {
	return func(line string) string {
		for _, f := range fs {
			line = f(line)
		}
		return line
	}
}

// ---------- StartFilePush ----------

type filePushParams struct {
	Kind    string      `json:"kind"`
	Input   string      `json:"input"`
	URL     string      `json:"url"`
	Loop    bool        `json:"loop"`
	Options PushOptions `json:"options"`
}

// StartFilePush 开始文件推流（可循环）。立即返回入队前的任务快照；连接 / 鉴权失败体现在任务的 failed + error 上。
// 同步错误见契约「Start* 同步返回的错误」。
func (s *Service) StartFilePush(ctx context.Context, req FilePushRequest) (task.Task, error) {
	t, err := s.startFilePush(ctx, req)
	if err != nil && ctx.Err() != nil && !apperr.Is(err, apperr.Canceled) {
		return task.Task{}, apperr.Wrap(apperr.Canceled, "操作已取消", ctx.Err())
	}
	return t, err
}

func (s *Service) startFilePush(ctx context.Context, req FilePushRequest) (task.Task, error) {
	if s.cfg.Tasks == nil || s.cfg.Media == nil {
		return task.Task{}, apperr.New(apperr.Internal, "直播服务尚未初始化")
	}
	bin, err := s.cfg.Require()
	if err != nil {
		return task.Task{}, err
	}
	u, err := parseURL(req.URL)
	if err != nil {
		return task.Task{}, err
	}
	if err := req.Options.validate(); err != nil {
		return task.Task{}, err
	}
	if !filepath.IsAbs(req.InputPath) {
		return task.Task{}, invalidArg("输入文件必须是绝对路径")
	}
	if err := s.checkProtocols(ctx, bin, u.Scheme, false); err != nil {
		return task.Task{}, err
	}
	if err := ctx.Err(); err != nil {
		return task.Task{}, apperr.Wrap(apperr.Canceled, "操作已取消", err)
	}
	info, err := s.cfg.Media.Inspect(ctx, req.InputPath)
	if err != nil {
		return task.Task{}, err
	}
	if !info.HasVideo {
		return task.Task{}, invalidArg("输入文件没有视频画面，无法推流")
	}
	in := info.Path
	if in == "" {
		in = filepath.Clean(req.InputPath)
	}
	gop := req.Options.Fps
	if gop <= 0 {
		gop = info.Fps
	}
	if gop <= 0 || gop > 240 {
		gop = 30
	}
	taskID := id.New()
	previewPath := s.planPreview(ctx, bin, taskID, req.Preview)
	args, encoding := s.resolveLiveEncoder(ctx, ffmpeg.LiveEncode{
		Width: req.Options.Width, Height: req.Options.Height, Fps: req.Options.Fps, GOPFps: gop,
		VideoKbps: req.Options.videoKbps(), AudioKbps: req.Options.audioKbps(),
	}, func(e ffmpeg.LiveEncode) []string {
		return ffmpeg.BuildFilePushArgs(ffmpeg.FilePushPlan{
			Input: in, Loop: req.Loop, HasAudio: info.HasAudio, Scheme: u.Scheme, URL: u.FFmpeg, PreviewPath: previewPath, Enc: e,
		})
	})
	if err := s.reserve(taskID, u.Key, false, false, previewPath); err != nil {
		return task.Task{}, err
	}
	pj, _ := json.Marshal(filePushParams{Kind: "file", Input: in, URL: u.Redacted, Loop: req.Loop, Options: req.Options})
	spec := task.Spec{
		ID:         taskID,
		Type:       task.TypeLiveFilePush,
		Title:      "文件推流：" + filepath.Base(in) + " → " + u.Redacted,
		InputPaths: []string{in},
		Params:     string(pj),
	}
	t, err := s.cfg.Tasks.Submit(spec, s.newRunner(taskID, bin, u, req.URL, args, encoding, false, false))
	if err != nil {
		s.release(taskID)
		return task.Task{}, err
	}
	return t, nil
}
