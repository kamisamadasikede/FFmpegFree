package langasr

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
	Dir      string // 组件根：…/components（其下 lang/asr/<tier>/<version>/）
	Tier     func() string
	Emit     func(event string, payload any)
	Logf     func(format string, args ...any)
	DiskFree func(dir string) (int64, error)
	// 测试可覆盖：
	Manifest Manifest
	Now      func() time.Time
}

// DefaultDir 返回组件父目录（与文档组件同级）：Windows %LocalAppData%\FFmpegFree\components，其他 <dataRoot>/components。
func DefaultDir(dataRoot string) string {
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, "FFmpegFree", "components")
		}
	}
	return filepath.Join(dataRoot, "components")
}

// Manager 管理语音识别组件状态机。并发安全。
type Manager struct {
	cfg Config
	man Manifest

	mu      sync.Mutex
	st      Status
	checked chan struct{}
	cancel  context.CancelFunc
	busy    bool
	gate    HWGate
}

// New 创建 Manager（不检测；调 Start）。
func New(cfg Config) *Manager {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.Tier == nil {
		cfg.Tier = func() string { return TierStandard }
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

func (m *Manager) tier() string {
	t := m.cfg.Tier()
	if t == TierHD {
		return TierHD
	}
	return TierStandard
}

func (m *Manager) base(state string) Status {
	tier := m.tier()
	pkg := m.man.SpecFor(tier)
	s := Status{
		State:         state,
		Tier:          tier,
		DownloadBytes: DownloadBytesFor(tier),
		InstallBytes:  InstallBytesFor(tier),
		CanDownload:   pkg.Configured(),
	}
	if pkg.Size > 0 {
		s.DownloadBytes = pkg.EffectiveSize(tier)
	}
	return s
}

func (m *Manager) componentRoot(tier, version string) string {
	return filepath.Join(m.cfg.Dir, "lang", "asr", tier, version)
}

func (m *Manager) tmpDir() string {
	return filepath.Join(m.cfg.Dir, "lang", "asr", "tmp")
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

// Gate 返回高清 ASR × 硬件编码闸门。
func (m *Manager) Gate() *HWGate { return &m.gate }

func (m *Manager) emit(event string, payload any) {
	if m.cfg.Emit != nil {
		m.cfg.Emit(event, payload)
	}
}

func (m *Manager) set(s Status) {
	m.mu.Lock()
	m.st = s
	m.mu.Unlock()
	m.emit(EventAsr, m.view(s))
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
	m.emit(EventAsr, s)
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
	m.emit(EventAsr, m.view(s))
}

func (m *Manager) runDetect(ctx context.Context) Status {
	_ = ctx
	_ = os.MkdirAll(m.tmpDir(), 0o755)
	tier := m.tier()
	ver := m.man.Version
	if ver == "" {
		ver = PlaceholderVersion
	}
	root := m.componentRoot(tier, ver)
	if exe := EntryBinary(root); exe != "" {
		s := m.base(StateReady)
		s.Version, s.Source, s.Path = ver, SourceDownloaded, root
		return s
	}
	// 扫描同档其他版本目录
	base := filepath.Join(m.cfg.Dir, "lang", "asr", tier)
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
		s.Error = apperr.New(apperr.LangAsrNotReady, MsgMissingGuide).WithDetail("reason=missing")
	}
	return s
}

// NotReadyError 提交识别时组件不可用。
func (m *Manager) NotReadyError() *apperr.AppError {
	m.mu.Lock()
	st := m.st
	m.mu.Unlock()
	reason := st.State
	if reason == "" {
		reason = StateMissing
	}
	return apperr.New(apperr.LangAsrNotReady, MsgNotReady).WithDetail("reason=" + reason)
}

// Install 开始下载；URL 未配置时立即返回 LANG_DOWNLOAD_FAILED。
func (m *Manager) Install(tier string) (Status, error) {
	if tier == "" {
		tier = m.tier()
	}
	if tier != TierStandard && tier != TierHD {
		return Status{}, apperr.New(apperr.InvalidArgument, "识别档位不正确").WithDetail("tier=" + tier)
	}
	pkg := m.man.SpecFor(tier)
	if !pkg.Configured() {
		return Status{}, apperr.New(apperr.LangDownloadFailed, MsgNotPublished).WithDetail("reason=not_configured\ntier=" + tier)
	}
	m.mu.Lock()
	if m.busy || m.st.State == StateReady {
		s := m.view(m.st)
		m.mu.Unlock()
		return s, nil
	}
	m.mu.Unlock()

	need := pkg.EffectiveSize(tier) + DiskReserve
	if free, err := m.cfg.DiskFree(m.cfg.Dir); err == nil && free < need {
		return Status{}, apperr.New(apperr.ConvertDiskFull, "磁盘空间不足，请清理后重试").
			WithDetail("reason=no_space")
	}
	if err := os.MkdirAll(m.tmpDir(), 0o755); err != nil {
		return Status{}, apperr.Wrap(apperr.IOError, "无法创建文件夹", err)
	}

	m.mu.Lock()
	if m.busy || m.st.State == StateReady {
		s := m.view(m.st)
		m.mu.Unlock()
		return s, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.busy = cancel, true
	s := m.base(StateDownloading)
	s.Phase, s.Tier = StateDownloading, tier
	s.DownloadBytes = pkg.EffectiveSize(tier)
	m.st = s
	m.mu.Unlock()
	m.emit(EventAsr, m.view(s))
	go m.runInstall(ctx, tier, pkg)
	return m.view(s), nil
}

func (m *Manager) runInstall(ctx context.Context, tier string, pkg PackageSpec) {
	defer func() {
		m.mu.Lock()
		m.busy = false
		m.cancel = nil
		m.mu.Unlock()
	}()
	err := downloadAndPrepare(ctx, downloadArgs{
		Dir: m.cfg.Dir, Tmp: m.tmpDir(), Tier: tier, Version: m.man.Version,
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
		s.Tier = tier
		m.set(s)
		return
	}
	root := m.componentRoot(tier, m.man.Version)
	s := m.base(StateReady)
	s.Version, s.Source, s.Path, s.Tier = m.man.Version, SourceDownloaded, root, tier
	m.set(s)
}

// CancelInstall 取消下载 / 准备。
func (m *Manager) CancelInstall() {
	m.mu.Lock()
	c := m.cancel
	m.mu.Unlock()
	if c != nil {
		c()
	}
}

// RefreshTierGuide 在 asrTier 切换后刷新 DownloadBytes / CanDownload（不触发下载、不改 state，除非仍是 missing）。
func (m *Manager) RefreshTierGuide() Status {
	m.mu.Lock()
	st := m.st
	tier := m.tier()
	pkg := m.man.SpecFor(tier)
	st.Tier = tier
	st.DownloadBytes = pkg.EffectiveSize(tier)
	st.InstallBytes = InstallBytesFor(tier)
	st.CanDownload = pkg.Configured()
	// 未就绪时保持 missing；ready 时 Path 仍指向旧档，需用户下载新档——一期简化：档切换后若当前 ready 档与设置不一致，视为 missing。
	if st.State == StateReady && st.Path != "" {
		want := filepath.Join(m.cfg.Dir, "lang", "asr", tier)
		if filepath.Clean(filepath.Dir(filepath.Dir(st.Path))) != filepath.Clean(filepath.Join(m.cfg.Dir, "lang", "asr")) ||
			filepath.Base(filepath.Dir(st.Path)) != "" && !pathHasTier(st.Path, tier) {
			_ = want
		}
		if !pathHasTier(st.Path, tier) {
			st = m.base(StateMissing)
			if !st.CanDownload {
				st.Error = apperr.New(apperr.LangAsrNotReady, MsgMissingGuide).WithDetail("reason=missing")
			}
		}
	} else if st.State == StateMissing || st.State == StateFailed {
		st.CanDownload = pkg.Configured()
		st.DownloadBytes = pkg.EffectiveSize(tier)
		st.InstallBytes = InstallBytesFor(tier)
		st.Tier = tier
		if !st.CanDownload {
			st.Error = apperr.New(apperr.LangAsrNotReady, MsgMissingGuide).WithDetail("reason=missing")
		} else {
			st.Error = nil
		}
	}
	m.st = st
	m.mu.Unlock()
	out := m.view(st)
	m.emit(EventAsr, out)
	return out
}

func pathHasTier(componentRoot, tier string) bool {
	// …/lang/asr/<tier>/<version>
	return filepath.Base(filepath.Dir(componentRoot)) == tier
}
