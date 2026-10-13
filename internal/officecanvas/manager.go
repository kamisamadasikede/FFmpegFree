package officecanvas

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
)

// Config 配置 Manager。
type Config struct {
	Dir      string // 组件父目录：…/components（其下 office/canvas/<version>/）
	Emit     func(event string, payload any)
	Logf     func(format string, args ...any)
	DiskFree func(dir string) (int64, error)
	// 测试可覆盖：
	Manifest Manifest
	Now      func() time.Time
}

// DefaultDir 返回组件父目录（与语音识别 / 文档组件同级）：
// Windows %LocalAppData%\FFmpegFree\components，其他 <dataRoot>/components。
func DefaultDir(dataRoot string) string {
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, "FFmpegFree", "components")
		}
	}
	return filepath.Join(dataRoot, "components")
}

// Manager 管理办公画布组件状态机。并发安全。
type Manager struct {
	cfg Config
	man Manifest

	mu      sync.Mutex
	st      Status
	checked chan struct{}
	cancel  context.CancelFunc
	busy    bool
}

// New 创建 Manager（不检测；调 Start）。
func New(cfg Config) *Manager {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.DiskFree == nil {
		cfg.DiskFree = diskFree
	}
	man := cfg.Manifest
	if man.Version == "" {
		man = LoadManifest()
	}
	m := &Manager{cfg: cfg, man: man, checked: make(chan struct{})}
	m.st = m.base(StateChecking)
	return m
}

func (m *Manager) base(state string) Status {
	pkg := m.man.SpecFor()
	s := Status{
		State:         state,
		DownloadBytes: GuideDownloadBytes,
		InstallBytes:  GuideInstallBytes,
		CanDownload:   pkg.Configured(),
	}
	if pkg.Size > 0 {
		s.DownloadBytes = pkg.EffectiveSize()
		s.InstallBytes = pkg.EffectiveSize()
	}
	return s
}

func (m *Manager) componentRoot(version string) string {
	return filepath.Join(m.cfg.Dir, "office", "canvas", version)
}

func (m *Manager) tmpDir() string {
	return filepath.Join(m.cfg.Dir, "office", "canvas", "tmp")
}

// Status 返回当前状态拷贝。
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.view(m.st)
}

func (m *Manager) view(s Status) Status {
	if s.State != StateReady {
		s.Source = ""
	}
	return s
}

// ComponentRoot 返回就绪时的组件根；未就绪返回 ""。
func (m *Manager) ComponentRoot() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.st.State != StateReady {
		return ""
	}
	return m.st.Path
}

func (m *Manager) emit(event string, payload any) {
	if m.cfg.Emit != nil {
		m.cfg.Emit(event, payload)
	}
}

func (m *Manager) set(s Status) {
	m.mu.Lock()
	m.st = s
	m.mu.Unlock()
	m.emit(EventCanvas, m.view(s))
}

// Start 后台检测一次。
func (m *Manager) Start() { go m.detect(context.Background()) }

// Recheck 重新检测；下载中不打断。
func (m *Manager) Recheck() Status {
	m.mu.Lock()
	if m.busy || m.st.State == StateChecking {
		s := m.view(m.st)
		m.mu.Unlock()
		return s
	}
	m.st = m.base(StateChecking)
	m.checked = make(chan struct{})
	s := m.view(m.st)
	m.mu.Unlock()
	m.emit(EventCanvas, s)
	go m.detect(context.Background())
	return s
}

func (m *Manager) detect(ctx context.Context) {
	s := m.runDetect(ctx)
	m.mu.Lock()
	if m.busy {
		close(m.checked)
		m.mu.Unlock()
		return
	}
	m.st = s
	close(m.checked)
	m.mu.Unlock()
	m.emit(EventCanvas, m.view(s))
}

func (m *Manager) runDetect(ctx context.Context) Status {
	_ = ctx
	_ = os.MkdirAll(m.tmpDir(), 0o755)
	ver := m.man.Version
	if ver == "" {
		ver = PlaceholderVersion
	}
	root := m.componentRoot(ver)
	if exe := EntryBinary(root); exe != "" {
		s := m.base(StateReady)
		s.Version, s.Source, s.Path = ver, SourceDownloaded, root
		return s
	}
	// 扫描其他版本目录
	base := filepath.Join(m.cfg.Dir, "office", "canvas")
	if ents, err := os.ReadDir(base); err == nil {
		for _, e := range ents {
			if !e.IsDir() || e.Name() == "tmp" {
				continue
			}
			cand := filepath.Join(base, e.Name())
			if EntryBinary(cand) != "" {
				s := m.base(StateReady)
				s.Version, s.Source, s.Path = e.Name(), SourceDownloaded, cand
				return s
			}
		}
	}
	s := m.base(StateMissing)
	if !s.CanDownload {
		s.Error = apperr.New(apperr.LangDownloadFailed, MsgMissingGuide).WithDetail("reason=missing")
	}
	return s
}

// WaitChecked 阻塞到首次检测结束（测试用）。
func (m *Manager) WaitChecked() {
	m.mu.Lock()
	ch := m.checked
	m.mu.Unlock()
	<-ch
}

// NotReadyError 导出等操作时组件不可用。
func (m *Manager) NotReadyError() *apperr.AppError {
	m.mu.Lock()
	st := m.st
	m.mu.Unlock()
	reason := st.State
	if reason == "" {
		reason = StateMissing
	}
	return apperr.New(apperr.NotFound, MsgNotReady).WithDetail("reason=" + reason)
}

// Install 开始下载；URL 未配置时立即返回 LANG_DOWNLOAD_FAILED（契约文案）。
func (m *Manager) Install() error {
	pkg := m.man.SpecFor()
	if !pkg.Configured() {
		return apperr.New(apperr.LangDownloadFailed, MsgNotPublished).WithDetail("reason=not_configured")
	}
	m.mu.Lock()
	if m.busy || m.st.State == StateReady {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	need := pkg.EffectiveSize() + DiskReserve
	if free, err := m.cfg.DiskFree(m.cfg.Dir); err == nil && free < need {
		return apperr.New(apperr.ConvertDiskFull, "磁盘空间不足，请清理后重试").
			WithDetail("reason=no_space")
	}
	if err := os.MkdirAll(m.tmpDir(), 0o755); err != nil {
		return apperr.Wrap(apperr.IOError, "无法创建文件夹", err)
	}

	m.mu.Lock()
	if m.busy || m.st.State == StateReady {
		m.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.busy = cancel, true
	s := m.base(StateDownloading)
	s.Phase = StateDownloading
	s.DownloadBytes = pkg.EffectiveSize()
	m.st = s
	m.mu.Unlock()
	m.emit(EventCanvas, m.view(s))
	go m.runInstall(ctx, pkg)
	return nil
}

func (m *Manager) runInstall(ctx context.Context, pkg PackageSpec) {
	defer func() {
		m.mu.Lock()
		m.busy = false
		m.cancel = nil
		m.mu.Unlock()
	}()
	err := downloadAndPrepare(ctx, downloadArgs{
		Dir: m.cfg.Dir, Tmp: m.tmpDir(), Version: m.man.Version,
		Pkg: pkg, Emit: m.emit, Logf: m.cfg.Logf,
	})
	if ctx.Err() != nil {
		s := m.base(StateMissing)
		m.set(s)
		return
	}
	if err != nil {
		s := m.base(StateFailed)
		s.Error = apperr.From(err)
		m.set(s)
		return
	}
	root := m.componentRoot(m.man.Version)
	s := m.base(StateReady)
	s.Version, s.Source, s.Path = m.man.Version, SourceDownloaded, root
	m.set(s)
}

// CancelInstall 取消下载 / 准备；空闲时无操作。
func (m *Manager) CancelInstall() {
	m.mu.Lock()
	c := m.cancel
	m.mu.Unlock()
	if c != nil {
		c()
	}
}

// MarkReadyForTest 将状态置为 ready（仅测试：写入假入口后调用或直接注入）。
func (m *Manager) MarkReadyForTest(root, version string) {
	m.mu.Lock()
	s := m.base(StateReady)
	s.Version, s.Source, s.Path = version, SourceDownloaded, root
	m.st = s
	m.mu.Unlock()
}
