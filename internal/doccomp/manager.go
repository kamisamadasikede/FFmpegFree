package doccomp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
)

// 状态（契约 6.12.13）。
const (
	StateChecking    = "checking"
	StateReady       = "ready"
	StateMissing     = "missing"
	StateOutdated    = "outdated"
	StateDownloading = "downloading"
	StatePreparing   = "preparing"
	StateFailed      = "failed"
)

// 来源。
const (
	SourceSystem     = "system"
	SourceDownloaded = "downloaded"
)

// 事件名（契约 6.12.15）。
const (
	EventComponent = "doc:component"
	EventProgress  = "doc:component-progress"
)

// EngineInfo 是 DocEngineInfo（契约 v0.27，6.12.28）。本 PR 只有文档组件这一个引擎；Office / WPS 在下一个 PR。
type EngineInfo struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Source    string   `json:"source,omitempty"`
	Installed bool     `json:"installed"`
	Families  []string `json:"families"`
	Available bool     `json:"available"`
}

// Status 是 DocComponentStatus（契约 6.12.13；v0.26.1 加 installBytes；v0.27 加 componentState / engines，state 改为“整体可用”）。
// 只有文档组件一个引擎时 state 恒等于 componentState。
type Status struct {
	State          string           `json:"state"`
	ComponentState string           `json:"componentState"`
	Engines        []EngineInfo     `json:"engines"`
	Version        string           `json:"version"`
	Source         string           `json:"source"`
	CanDownload    bool             `json:"canDownload"`
	DownloadBytes  int64            `json:"downloadBytes"`
	InstallBytes   int64            `json:"installBytes"` // v0.26.1：解包后约占用的磁盘空间；没有下载源时为 0
	Phase          string           `json:"phase,omitempty"`
	ReceivedBytes  int64            `json:"receivedBytes,omitempty"`
	Error          *apperr.AppError `json:"error,omitempty"`
	Path           string           `json:"-"`
}

// Progress 是 doc:component-progress 的 payload。preparing 时不带 progress（不确定进度）。
type Progress struct {
	Phase         string   `json:"phase"`
	ReceivedBytes int64    `json:"receivedBytes"`
	TotalBytes    int64    `json:"totalBytes"`
	Progress      *float64 `json:"progress,omitempty"`
}

// Config 配置 Manager。
type Config struct {
	Dir  string                          // 组件目录（DefaultDir）
	Emit func(event string, payload any) // 可空
	Logf func(format string, args ...any)
	// 以下给测试用；空时用默认值。
	Package    *Package // 覆盖清单里当前平台的包；Linux 测试下载用
	Version    string   // 组件版本目录名，默认清单 version
	Client     *http.Client
	RetryWait  time.Duration
	Candidates func() []string                                           // 系统安装候选
	Prepare    func(ctx context.Context, pkg, staging, tmp string) error // 平台准备
	Validate   func(ctx context.Context, exe, tmp string) (string, error)
	DiskFree   func(dir string) (int64, error)
}

// DefaultDir 返回组件目录：Windows %LocalAppData%\FFmpegFree\components（不放漫游的 %AppData%），其他平台 <数据目录>/components。
func DefaultDir(dataRoot string) string {
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, "FFmpegFree", "components")
		}
	}
	return filepath.Join(dataRoot, "components")
}

// Manager 管理文档组件的状态机。所有方法并发安全。
type Manager struct {
	cfg     Config
	pkg     *Package
	version string

	mu        sync.Mutex
	st        Status
	checked   chan struct{} // 当前这轮检测结束时关闭
	cancel    context.CancelFunc
	busy      bool // 正在下载 / 准备
	lastEmit  time.Time
	installWG sync.WaitGroup
}

// New 创建 Manager（不检测，调 Start）。
func New(cfg Config) *Manager {
	m := &Manager{cfg: cfg}
	man, err := LoadManifest()
	if err == nil {
		m.version = man.Version
		if p, ok := man.Platforms[PlatformKey()]; ok {
			m.pkg = &p
		}
	}
	if cfg.Package != nil {
		m.pkg = cfg.Package
	}
	if cfg.Version != "" {
		m.version = cfg.Version
	}
	if m.cfg.Client == nil {
		m.cfg.Client = &http.Client{}
	}
	if m.cfg.RetryWait == 0 {
		m.cfg.RetryWait = 2 * time.Second
	}
	if m.cfg.Candidates == nil {
		m.cfg.Candidates = systemCandidates
	}
	if m.cfg.Prepare == nil {
		m.cfg.Prepare = preparePackage
	}
	if m.cfg.Validate == nil {
		m.cfg.Validate = Validate
	}
	if m.cfg.DiskFree == nil {
		m.cfg.DiskFree = diskFree
	}
	if m.cfg.Logf == nil {
		m.cfg.Logf = func(string, ...any) {}
	}
	m.st = m.base(StateChecking)
	m.checked = make(chan struct{})
	return m
}

func (m *Manager) base(state string) Status {
	s := Status{State: state}
	if m.pkg != nil {
		s.CanDownload, s.DownloadBytes, s.InstallBytes = true, m.pkg.Size, InstallBytesApprox
	}
	return s
}

func (m *Manager) tmpDir() string     { return filepath.Join(m.cfg.Dir, "tmp") }
func (m *Manager) finalDir() string   { return filepath.Join(m.cfg.Dir, "doc", m.version) }
func (m *Manager) stagingDir() string { return m.finalDir() + ".staging" }
func (m *Manager) partPath() string {
	return filepath.Join(m.tmpDir(), "doc-"+m.version+"-"+m.pkg.SHA256[:12]+".part")
}

// pkgPath 是校验通过后的安装包（准备阶段取消时保留，下次直接从准备开始）。
func (m *Manager) pkgPath() string {
	return filepath.Join(m.tmpDir(), "doc-"+m.version+"-"+m.pkg.SHA256[:12]+"."+m.pkg.Type)
}

// view 填上派生字段：componentState、engines（v0.27，6.12.28）。
func (m *Manager) view(s Status) Status {
	s.ComponentState = s.State
	s.Engines = []EngineInfo{}
	ready := s.State == StateReady
	// Windows / macOS 始终列出文档组件（没下载时 installed=false）；Linux 没装时不列。
	if ready || m.pkg != nil {
		e := EngineInfo{ID: "component", Name: "文档组件", Installed: ready, Available: ready,
			Families: []string{"text", "sheet", "slide"}}
		if ready {
			e.Version, e.Source = s.Version, s.Source
		}
		s.Engines = append(s.Engines, e)
	}
	return s
}

// Status 返回当前状态（拷贝）。
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.view(m.st)
}

// ExePath 返回就绪组件的可执行文件路径；没就绪返回 ""。
func (m *Manager) ExePath() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.st.State != StateReady {
		return ""
	}
	return m.st.Path
}

// Wait 在 checking 时最多等 d，返回等到的状态（GetFormatMatrix 用，契约 6.12.14）。
func (m *Manager) Wait(ctx context.Context, d time.Duration) Status {
	m.mu.Lock()
	ch, state := m.checked, m.st.State
	m.mu.Unlock()
	if state == StateChecking {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ch:
		case <-t.C:
		case <-ctx.Done():
		}
	}
	return m.Status()
}

func (m *Manager) emit(event string, payload any) {
	if m.cfg.Emit != nil {
		m.cfg.Emit(event, payload)
	}
}

// set 更新状态并发 doc:component。
func (m *Manager) set(s Status) {
	m.mu.Lock()
	m.st = s
	m.mu.Unlock()
	m.emit(EventComponent, m.view(s))
}

// Start 在后台做一次检测（启动时调用，不阻塞界面）。
func (m *Manager) Start() { go m.detect(context.Background()) }

// Recheck 重新检测；下载 / 准备中返回当前状态，不打断。立即返回（checking），结果走 doc:component。
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
	m.emit(EventComponent, s)
	go m.detect(context.Background())
	return s
}

// detect 按契约 6.12.12 的顺序检测：应用下载的优先，然后系统安装；第一个通过校验的就是当前组件。
func (m *Manager) detect(ctx context.Context) {
	s := m.runDetect(ctx)
	m.mu.Lock()
	if m.busy { // 检测期间开始了安装：以安装为准
		close(m.checked)
		m.mu.Unlock()
		return
	}
	m.st = s
	close(m.checked)
	m.mu.Unlock()
	m.emit(EventComponent, m.view(s))
}

func (m *Manager) runDetect(ctx context.Context) Status {
	_ = os.MkdirAll(m.tmpDir(), 0o755)
	var cands []struct{ path, source string }
	if m.version != "" {
		if p := findInTree(m.finalDir(), 4); p != "" {
			cands = append(cands, struct{ path, source string }{p, SourceDownloaded})
		}
	}
	for _, p := range m.cfg.Candidates() {
		cands = append(cands, struct{ path, source string }{p, SourceSystem})
	}
	outdated := ""
	for _, c := range cands {
		if !regular(c.path) {
			continue
		}
		start := time.Now()
		v, err := m.cfg.Validate(ctx, c.path, m.tmpDir())
		m.cfg.Logf("文档组件检测 %s（%s）：版本 %q，%v，用时 %s", c.path, c.source, v, err, time.Since(start).Round(time.Millisecond))
		if err == nil {
			s := m.base(StateReady)
			s.Version, s.Source, s.Path = v, c.source, c.path
			return s
		}
		if ae := apperr.From(err); ae != nil && ae.Detail == "check=outdated" && outdated == "" {
			outdated = v
		}
	}
	if outdated != "" {
		s := m.base(StateOutdated)
		s.Version = outdated
		e := m.notReadyError()
		if m.pkg == nil {
			e = apperr.New(apperr.DocComponentNotReady, LinuxOutdatedHint) // v0.27（6.12.24）：Linux 上版本太旧的那一句
		}
		// 架构师确认（v0.26 实现 PR）：Win/mac 上 componentState=outdated、canDownload=true，提交转换报 DOC_COMPONENT_NOT_READY + reason=outdated
		s.Error = e.WithDetail("reason=outdated\nversion=" + outdated)
		return s
	}
	s := m.base(StateMissing)
	if m.pkg == nil {
		s.Error = m.notReadyError()
	}
	return s
}

// notReadyError 是 DOC_COMPONENT_NOT_READY（Linux 用唯一允许出现组件名字的那句话）。
func (m *Manager) notReadyError() *apperr.AppError {
	if m.pkg == nil {
		return apperr.New(apperr.DocComponentNotReady, LinuxHint)
	}
	return apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。")
}

// NotReadyError 是提交 / 开始转换时组件不可用的错误：版本太旧时带 reason=outdated（Linux 用“版本太旧”那句），否则见 notReadyError。
func (m *Manager) NotReadyError() *apperr.AppError {
	m.mu.Lock()
	st := m.st
	m.mu.Unlock()
	if st.State == StateOutdated && st.Error != nil {
		e := *st.Error
		return &e
	}
	return m.notReadyError()
}

// Install 开始或继续下载 + 准备，立即返回（downloading / preparing）；幂等。
func (m *Manager) Install(mirror string) (Status, error) {
	if mirror != MirrorDefault && mirror != MirrorCN {
		return Status{}, apperr.New(apperr.InvalidArgument, "下载源不正确").WithDetail("mirror=" + mirror)
	}
	if m.pkg == nil {
		return Status{}, apperr.New(apperr.UnsupportedPlatform, LinuxHint)
	}
	m.mu.Lock()
	if m.busy || m.st.State == StateReady {
		s := m.view(m.st)
		m.mu.Unlock()
		return s, nil
	}
	m.mu.Unlock()
	if err := os.MkdirAll(m.tmpDir(), 0o755); err != nil {
		return Status{}, apperr.Wrap(apperr.IOError, "无法创建文件夹", err)
	}
	var have int64
	if fi, err := os.Stat(m.partPath()); err == nil {
		have = fi.Size()
	}
	pkgReady := regular(m.pkgPath())
	if !pkgReady {
		need := m.pkg.Size - have + DiskReserve
		if free, err := m.cfg.DiskFree(m.cfg.Dir); err == nil && free < need {
			return Status{}, diskFullError(need, free)
		}
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
	s.Phase, s.ReceivedBytes = StateDownloading, have
	if pkgReady {
		s.State, s.Phase, s.ReceivedBytes = StatePreparing, StatePreparing, 0
	}
	m.st = s
	m.lastEmit = time.Time{}
	m.installWG.Add(1)
	m.mu.Unlock()
	s = m.view(s)
	m.emit(EventComponent, s)
	go m.install(ctx, mirror, pkgReady)
	return s, nil
}

// Cancel 取消下载 / 准备；没有在进行时无操作。取消后重新检测。
func (m *Manager) Cancel() {
	m.mu.Lock()
	cancel := m.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
		m.installWG.Wait()
	}
}

// Close 在应用退出时取消安装（不重新检测）。
func (m *Manager) Close() {
	m.Cancel()
}

func (m *Manager) install(ctx context.Context, mirror string, pkgReady bool) {
	err := m.doInstall(ctx, mirror, pkgReady)
	m.mu.Lock()
	m.cancel, m.busy = nil, false
	m.mu.Unlock()
	switch {
	case err == nil:
		m.installWG.Done()
	case ctx.Err() != nil: // 取消：重新检测（有系统安装就是 ready，否则 missing）
		m.mu.Lock()
		m.st = m.base(StateChecking)
		m.checked = make(chan struct{})
		m.mu.Unlock()
		m.emit(EventComponent, m.Status())
		m.installWG.Done()
		m.detect(context.Background())
	default:
		s := m.base(StateFailed)
		s.Error = apperr.From(err)
		m.cfg.Logf("文档组件安装失败: %v", err)
		m.set(s)
		m.installWG.Done()
	}
}

func (m *Manager) doInstall(ctx context.Context, mirror string, pkgReady bool) error {
	t0 := time.Now()
	if !pkgReady {
		d := &downloader{client: m.cfg.Client, idleTimeout: 60 * time.Second, retries: 3, retryWait: m.cfg.RetryWait}
		report := func(done, total int64) { m.progress(done, total, false) }
		reset := func() { m.progress(0, m.pkg.Size, true) }
		if err := d.fetch(ctx, m.pkg.urlsFor(mirror), m.partPath(), m.pkg.SHA256, m.pkg.Size, report, reset); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, errChecksum) {
				return apperr.Wrap(apperr.DocChecksumFailed, "下载的文档组件校验失败，请重试。", err)
			}
			return apperr.Wrap(apperr.DocDownloadFailed, "文档组件下载失败，请检查网络后重试。", err).WithDetail(trimDetail(err.Error()))
		}
		if err := os.Rename(m.partPath(), m.pkgPath()); err != nil {
			return apperr.Wrap(apperr.DocComponentInstallFailed, "文档组件准备失败，请重试。", err).WithDetail("check=rename")
		}
		m.cfg.Logf("文档组件下载完成，用时 %s", time.Since(t0).Round(time.Second))
	}
	// preparing：不给百分比，一直持续到检测通过。
	s := m.base(StatePreparing)
	s.Phase = StatePreparing
	m.set(s)
	m.emit(EventProgress, Progress{Phase: StatePreparing, ReceivedBytes: m.pkg.Size, TotalBytes: m.pkg.Size})
	t1 := time.Now()
	staging := m.stagingDir()
	os.RemoveAll(staging)
	if err := os.MkdirAll(filepath.Dir(staging), 0o755); err != nil {
		return installFailed("check=mkdir")
	}
	fail := func(err error) error {
		os.RemoveAll(staging)
		return err
	}
	if err := m.cfg.Prepare(ctx, m.pkgPath(), staging, m.tmpDir()); err != nil {
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		return fail(err)
	}
	exe := findInTree(staging, 4)
	if exe == "" {
		return fail(installFailed("check=not_found"))
	}
	ver, err := m.cfg.Validate(ctx, exe, m.tmpDir())
	if ctx.Err() != nil {
		return fail(ctx.Err())
	}
	if err != nil {
		return fail(err)
	}
	rel, _ := filepath.Rel(staging, exe)
	final := m.finalDir()
	old := ""
	if _, err := os.Stat(final); err == nil {
		old = final + ".old-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		if err := os.Rename(final, old); err != nil {
			return fail(installFailed("check=rename"))
		}
	}
	if err := os.Rename(staging, final); err != nil {
		if old != "" {
			_ = os.Rename(old, final)
		}
		return fail(installFailed("check=rename"))
	}
	if old != "" {
		os.RemoveAll(old)
	}
	os.Remove(m.pkgPath())
	os.Remove(m.partPath())
	m.cfg.Logf("文档组件准备完成，用时 %s", time.Since(t1).Round(time.Second))
	r := m.base(StateReady)
	r.Version, r.Source, r.Path = ver, SourceDownloaded, filepath.Join(final, rel)
	m.set(r)
	return nil
}

// progress 发 doc:component-progress（下载中最多 4 次/秒；归 0 时立即发）。
func (m *Manager) progress(done, total int64, force bool) {
	m.mu.Lock()
	if m.st.State == StateDownloading {
		m.st.ReceivedBytes = done
	}
	now := time.Now()
	if !force && done != total && now.Sub(m.lastEmit) < 250*time.Millisecond {
		m.mu.Unlock()
		return
	}
	m.lastEmit = now
	m.mu.Unlock()
	var f float64
	if total > 0 {
		f = math.Min(1, float64(done)/float64(total))
	}
	m.emit(EventProgress, Progress{Phase: StateDownloading, ReceivedBytes: done, TotalBytes: total, Progress: &f})
}

func trimDetail(s string) string {
	if len(s) > 300 {
		s = s[:300]
	}
	return strings.TrimSpace(s)
}

// diskFullError 同 convert/copier.go：CONVERT_DISK_FULL（reason=no_space，detail 三行）。
func diskFullError(need, free int64) *apperr.AppError {
	return apperr.New(apperr.ConvertDiskFull, fmt.Sprintf("磁盘空间不足，需要 %s，剩余 %s。", humanBytes(need), humanBytes(free))).
		WithDetail(fmt.Sprintf("reason=no_space\nneedBytes=%d\nfreeBytes=%d", need, free))
}

func humanBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	f, i := float64(n), 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	s := strconv.FormatFloat(math.Round(f*10)/10, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0") + " " + units[i]
}
