// Package system 实现 SystemService 背后的业务逻辑：ffmpeg 环境检测状态机（契约第 9 节）。
// 不依赖 Wails：事件通过 Emitter 接口发出，配置通过 SettingsStore 读写，便于单元测试。
package system

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/task"
)

// EventFFmpegStatus 是 ffmpeg 状态变化事件名，payload 为完整的 FFmpegStatus。
const EventFFmpegStatus = "ffmpeg:status"

// settings 表里的键，与契约 Settings 字段同名，日后 GetSettings / UpdateSettings 沿用。
const (
	SettingFFmpegPath            = "ffmpegPath"
	SettingFFmpegPromptDismissed = "ffmpegPromptDismissed"
)

// FFmpegStatus 见契约第 9.4 节。
//
// Error 在 missing / outdated 时为 FFMPEG_NOT_FOUND（detail 里列出每个候选失败的原因），
// failed 时为 INTERNAL；ready / checking 时为 nil。
type FFmpegStatus struct {
	State   string `json:"state"`            // checking | ready | missing | outdated | installing | failed
	Path    string `json:"path"`             // ffmpeg 可执行文件的绝对路径
	Version string `json:"version"`          // 如 "6.1.1-3ubuntu5"
	Source  string `json:"source"`           // custom | bundled | system | legacy
	TaskID  string `json:"taskId,omitempty"` // installing 时对应的安装任务
	// FFprobeMissing 为 true 表示 ffmpeg 可用但没有 ffprobe（v1 的 ffmpeg/ 目录只带 ffmpeg）。
	// state 仍是 ready，转换等只依赖 ffmpeg 的功能可用；媒体探测、缩略图需要 ffprobe，
	// 前端据此提示"补全 ffprobe"，引导一键安装。
	FFprobeMissing bool             `json:"ffprobeMissing"`
	Error          *apperr.AppError `json:"error,omitempty"`
}

// Emitter 向前端发事件。生产实现在 app 包（封装 Wails runtime.EventsEmit）。
type Emitter interface {
	Emit(event string, payload any)
}

// SettingsStore 是 Manager 需要的设置读写能力，*store.Store 满足该接口。
type SettingsStore interface {
	GetSetting(ctx context.Context, key string, dst any) (found bool, err error)
	SetSetting(ctx context.Context, key string, v any) error
}

// Config 是 Manager.Start 的依赖。
type Config struct {
	Locator   *ffmpeg.Locator
	Settings  SettingsStore     // 可为 nil（存储初始化失败时降级为不持久化）
	Emitter   Emitter           // 可为 nil
	Installer *ffmpeg.Installer // 可为 nil（此时 InstallFFmpeg 返回 INTERNAL）
	Tasks     *task.Manager     // 可为 nil（此时 InstallFFmpeg 返回 INTERNAL）
}

// Manager 持有 ffmpeg 检测状态。所有方法并发安全。
type Manager struct {
	mu      sync.Mutex // 保护 status、cfg，以及"改状态 + 发事件"的整体顺序
	status  FFmpegStatus
	cfg     Config
	started bool
	appCtx  context.Context // Start 传入的应用 ctx，安装 goroutine 由它派生
	install *installRun     // 进行中的安装，nil 表示没有

	detectMu sync.Mutex // 串行化检测，避免并发 Recheck 互相覆盖

	memMu   sync.Mutex // 保护下面两个内存兜底值（Settings 为 nil 时使用）
	memPath string
	memDism bool
}

// NewManager 创建 Manager，初始状态 checking。真正的检测由 Start 触发。
func NewManager() *Manager {
	return &Manager{status: FFmpegStatus{State: ffmpeg.StateChecking}}
}

// Start 注入依赖并在后台 goroutine 执行首次检测，立即返回，不阻塞界面。
func (m *Manager) Start(ctx context.Context, cfg Config) {
	m.mu.Lock()
	m.cfg = cfg
	m.appCtx = ctx
	m.started = true
	if cfg.Tasks != nil && cfg.Installer != nil {
		m.registerInstallFactory(cfg)
	}
	m.mu.Unlock()
	go func() {
		if _, err := m.Recheck(ctx); err != nil {
			// 只有 ctx 取消（应用退出）才会走到这里，状态已由 Recheck 处理。
			return
		}
	}()
}

// Status 返回当前状态的副本。
func (m *Manager) Status() FFmpegStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// set 更新状态并发出 ffmpeg:status 事件；ready 时同步全局门控并重置"稍后"标记。
func (m *Manager) set(ctx context.Context, s FFmpegStatus, bins *ffmpeg.Binaries) {
	m.setIf(ctx, s, bins, false)
}

// setUnlessInstalling 仅在没有安装进行时更新状态，返回是否更新。
func (m *Manager) setUnlessInstalling(ctx context.Context, s FFmpegStatus, bins *ffmpeg.Binaries) bool {
	return m.setIf(ctx, s, bins, true)
}

func (m *Manager) setIf(ctx context.Context, s FFmpegStatus, bins *ffmpeg.Binaries, skipIfInstalling bool) bool {
	m.mu.Lock()
	if skipIfInstalling && m.install != nil {
		m.mu.Unlock()
		return false
	}
	m.status = s
	if s.State == ffmpeg.StateReady {
		ffmpeg.SetCurrent(bins)
	} else {
		ffmpeg.SetCurrent(nil)
	}
	em := m.cfg.Emitter
	if em != nil {
		em.Emit(EventFFmpegStatus, s)
	}
	m.mu.Unlock()

	if s.State == ffmpeg.StateReady && m.PromptDismissed(ctx) {
		_ = m.setDismissed(ctx, false) // 契约 9.5：变为 ready 后重置
	}
	return true
}

func (m *Manager) ready() (Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started || m.cfg.Locator == nil {
		return Config{}, apperr.New(apperr.Internal, "系统服务尚未初始化")
	}
	return m.cfg, nil
}

// Recheck 重新检测：先推送 checking，再推送结果，并返回最终状态。
// 只有 ctx 被取消时返回 error。
func (m *Manager) Recheck(ctx context.Context) (FFmpegStatus, error) {
	cfg, err := m.ready()
	if err != nil {
		return FFmpegStatus{}, err
	}
	m.detectMu.Lock()
	defer m.detectMu.Unlock()

	if st, busy := m.installingStatus(); busy {
		return st, nil // 安装期间保持 installing，不被重检覆盖
	}
	if !m.setUnlessInstalling(ctx, FFmpegStatus{State: ffmpeg.StateChecking}, nil) {
		st, _ := m.installingStatus()
		return st, nil
	}
	res, err := cfg.Locator.Locate(ctx, m.customPath(ctx, cfg))
	if err != nil {
		st := FFmpegStatus{State: ffmpeg.StateFailed, Error: apperr.Wrap(apperr.Internal, "检测 ffmpeg 被中断", err)}
		m.setUnlessInstalling(ctx, st, nil)
		return st, err
	}
	st, bins := statusFromResult(res)
	if !m.setUnlessInstalling(ctx, st, bins) {
		st, _ = m.installingStatus() // 检测期间开始了安装，以安装状态为准
	}
	return st, nil
}

func statusFromResult(res ffmpeg.Result) (FFmpegStatus, *ffmpeg.Binaries) {
	switch res.State {
	case ffmpeg.StateReady:
		b := res.Info.Binaries
		return FFmpegStatus{State: ffmpeg.StateReady, Path: b.FFmpeg, Version: res.Info.Version, Source: res.Info.Source, FFprobeMissing: res.Info.FFprobeMissing}, &b
	case ffmpeg.StateOutdated:
		return FFmpegStatus{
			State: ffmpeg.StateOutdated, Path: res.Info.FFmpeg, Version: res.Info.Version, Source: res.Info.Source,
			Error: apperr.New(apperr.FFmpegNotFound, "ffmpeg 版本过低，需要 6 或更高").WithDetail(attemptsDetail(res.Attempts)),
		}, nil
	default:
		return FFmpegStatus{
			State: ffmpeg.StateMissing,
			Error: apperr.New(apperr.FFmpegNotFound, "未找到可用的 ffmpeg").WithDetail(attemptsDetail(res.Attempts)),
		}, nil
	}
}

func attemptsDetail(as []ffmpeg.Attempt) string {
	lines := make([]string, 0, len(as))
	for _, a := range as {
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", a.Source, a.Path, a.Reason))
	}
	return strings.Join(lines, "\n")
}

// SetPath 手动指定 ffmpeg 所在目录（也可以直接是 ffmpeg 可执行文件）。
// 校验失败返回 INVALID_ARGUMENT，且不改变当前状态、不写设置；成功后持久化并推送 ready。
// dir 为空表示清除手动指定，回到自动检测（等价于清除设置后 Recheck）。
func (m *Manager) SetPath(ctx context.Context, dir string) (FFmpegStatus, error) {
	cfg, err := m.ready()
	if err != nil {
		return FFmpegStatus{}, err
	}
	if strings.TrimSpace(dir) == "" {
		if err := m.setCustomPath(ctx, cfg, ""); err != nil {
			return FFmpegStatus{}, apperr.Wrap(apperr.IOError, "保存设置失败", err)
		}
		return m.Recheck(ctx)
	}

	if _, busy := m.installingStatus(); busy {
		return FFmpegStatus{}, apperr.New(apperr.TaskConflict, "正在安装 ffmpeg，请等待完成或先取消")
	}
	m.detectMu.Lock()
	defer m.detectMu.Unlock()
	info, state, reason := cfg.Locator.CheckCustom(ctx, dir)
	if state != ffmpeg.StateReady {
		msg := "所选位置不是可用的 ffmpeg"
		if state == ffmpeg.StateOutdated {
			msg = "所选 ffmpeg 版本过低，需要 6 或更高"
		}
		return FFmpegStatus{}, apperr.New(apperr.InvalidArgument, msg).WithDetail(reason)
	}
	if err := m.setCustomPath(ctx, cfg, dir); err != nil {
		return FFmpegStatus{}, apperr.Wrap(apperr.IOError, "保存设置失败", err)
	}
	b := info.Binaries
	st := FFmpegStatus{State: ffmpeg.StateReady, Path: b.FFmpeg, Version: info.Version, Source: ffmpeg.SourceCustom, FFprobeMissing: info.FFprobeMissing}
	m.set(ctx, st, &b)
	return st, nil
}

func (m *Manager) customPath(ctx context.Context, cfg Config) string {
	if cfg.Settings == nil {
		m.memMu.Lock()
		defer m.memMu.Unlock()
		return m.memPath
	}
	var p string
	if _, err := cfg.Settings.GetSetting(ctx, SettingFFmpegPath, &p); err != nil {
		return ""
	}
	return p
}

func (m *Manager) setCustomPath(ctx context.Context, cfg Config, p string) error {
	if cfg.Settings == nil {
		m.memMu.Lock()
		defer m.memMu.Unlock()
		m.memPath = p
		return nil
	}
	return cfg.Settings.SetSetting(ctx, SettingFFmpegPath, p)
}

// PromptDismissed 返回用户是否已选择"稍后"（契约 Settings.ffmpegPromptDismissed）。
func (m *Manager) PromptDismissed(ctx context.Context) bool {
	m.mu.Lock()
	st := m.cfg.Settings
	m.mu.Unlock()
	if st == nil {
		m.memMu.Lock()
		defer m.memMu.Unlock()
		return m.memDism
	}
	var b bool
	if _, err := st.GetSetting(ctx, SettingFFmpegPromptDismissed, &b); err != nil {
		return false
	}
	return b
}

func (m *Manager) setDismissed(ctx context.Context, v bool) error {
	m.mu.Lock()
	st := m.cfg.Settings
	m.mu.Unlock()
	if st == nil {
		m.memMu.Lock()
		defer m.memMu.Unlock()
		m.memDism = v
		return nil
	}
	return st.SetSetting(ctx, SettingFFmpegPromptDismissed, v)
}

// Settings 是契约 Settings 里与 ffmpeg 相关的部分。输出目录、并发数、主题、语言等字段
// 由后续 PR 补充到同一个结构里；这里先只承载这两项，前端据此决定是否弹安装确认框。
type Settings struct {
	FFmpegPath            string `json:"ffmpegPath"`
	FFmpegPromptDismissed bool   `json:"ffmpegPromptDismissed"`
}

// GetSettings 返回当前设置。
func (m *Manager) GetSettings(ctx context.Context) (Settings, error) {
	cfg, err := m.ready()
	if err != nil {
		return Settings{}, err
	}
	return Settings{FFmpegPath: m.customPath(ctx, cfg), FFmpegPromptDismissed: m.PromptDismissed(ctx)}, nil
}

// UpdateSettings 更新设置。ffmpegPath 变化时走 SetPath 校验（失败返回 INVALID_ARGUMENT，
// 且整个更新不生效）；ffmpegPromptDismissed 直接保存。
func (m *Manager) UpdateSettings(ctx context.Context, s Settings) error {
	cfg, err := m.ready()
	if err != nil {
		return err
	}
	if s.FFmpegPath != m.customPath(ctx, cfg) {
		if _, err := m.SetPath(ctx, s.FFmpegPath); err != nil {
			return err
		}
	}
	if err := m.setDismissed(ctx, s.FFmpegPromptDismissed); err != nil {
		return apperr.Wrap(apperr.IOError, "保存设置失败", err)
	}
	return nil
}
