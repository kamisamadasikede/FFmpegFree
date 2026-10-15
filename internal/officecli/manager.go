package officecli

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/proc"
)

type ToolState struct {
	State   string `json:"state"`
	Path    string `json:"path,omitempty"`
	Error   string `json:"error,omitempty"`
	Version string `json:"version"`
}

type Preview struct {
	ID       string `json:"id"`
	Version  uint64 `json:"version"`
	State    string `json:"state"`
	Mode     string `json:"mode"`
	HTML     string `json:"html,omitempty"`
	Slide    int    `json:"slide"`
	ScrollTo string `json:"scrollTo,omitempty"`
	Error    string `json:"error,omitempty"`
}

type Config struct {
	// Executable and Install are injectable for deterministic subprocess tests.
	Executable string
	Install    func(context.Context) (string, error)
}

// Manager owns Watch processes and caches. Only trusted backend bindings expose
// snapshots; the webview never connects to OfficeCLI's editing HTTP endpoints.
type Manager struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	cfg      Config
	tool     ToolState
	toolDone chan struct{}
	sessions map[string]*session
	leases   map[string]lease
	closed   bool
}
type lease struct {
	session  *session
	validate func() error
}
type session struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	path     string
	validate func() error
	refs     int // manager lock
	preview  Preview
}

func New(cfg Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	if cfg.Install == nil {
		cfg.Install = Install
	}
	return &Manager{ctx: ctx, cancel: cancel, cfg: cfg, sessions: map[string]*session{}, leases: map[string]lease{}}
}
func (m *Manager) Tool(retry bool) ToolState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ToolState{State: "failed", Error: "预览服务已关闭", Version: Version}
	}
	if m.toolDone != nil && !(retry && m.tool.State == "failed") {
		return m.tool
	}
	m.tool = ToolState{State: "installing", Version: Version}
	done := make(chan struct{})
	m.toolDone = done
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		path := m.cfg.Executable
		var err error
		if path == "" {
			path, err = m.cfg.Install(m.ctx)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.tool.State = "failed"
			m.tool.Error = err.Error()
		} else {
			m.tool.State = "ready"
			m.tool.Path = path
		}
		close(done)
	}()
	return m.tool
}
func (m *Manager) executable(ctx context.Context) (string, error) {
	m.Tool(false)
	m.mu.Lock()
	done := m.toolDone
	m.mu.Unlock()
	if done == nil {
		return "", fmt.Errorf("预览服务已关闭")
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-done:
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tool.State != "ready" {
		return "", fmt.Errorf("%s", m.tool.Error)
	}
	return m.tool.Path, nil
}
func key(path string) string {
	p := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}
func (m *Manager) Start(path string, validate func() error) (Preview, error) {
	if validate == nil {
		return Preview{}, fmt.Errorf("缺少工作区权限校验")
	}
	if err := validate(); err != nil {
		return Preview{}, err
	}
	if strings.ToLower(filepath.Ext(path)) != ".pptx" {
		return Preview{}, fmt.Errorf("仅支持 .pptx")
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return Preview{}, err
	}
	id := hex.EncodeToString(b)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Preview{}, fmt.Errorf("预览服务已关闭")
	}
	s := m.sessions[key(path)]
	if s == nil {
		if len(m.sessions) >= 8 {
			return Preview{}, fmt.Errorf("同时最多打开 8 份 PPT，请关闭一个预览标签")
		}
		ctx, cancel := context.WithCancel(m.ctx)
		s = &session{ctx: ctx, cancel: cancel, path: path, preview: Preview{State: "starting", Mode: "watch"}}
		s.validate = func() error { return m.authorizeSession(s) }
		m.sessions[key(path)] = s
		m.wg.Add(1)
		go func() { defer m.wg.Done(); s.run(m) }()
	}
	s.refs++
	m.leases[id] = lease{s, validate}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.preview
	p.ID = id
	return p, nil
}
func (m *Manager) Read(id string, version uint64) (Preview, error) {
	m.mu.Lock()
	l := m.leases[id]
	s := l.session
	m.mu.Unlock()
	if s == nil {
		return Preview{}, fmt.Errorf("预览标签已关闭")
	}
	if err := l.validate(); err != nil {
		m.Release(id)
		return Preview{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.preview
	p.ID = id
	if p.Version == version {
		p.HTML = ""
	}
	return p, nil
}
func (m *Manager) Release(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.leases[id].session
	if s == nil {
		return
	}
	delete(m.leases, id)
	s.refs--
	if s.refs == 0 {
		delete(m.sessions, key(s.path))
		s.cancel()
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.cancel()
	m.leases = map[string]lease{}
	m.sessions = map[string]*session{}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Manager) authorizeSession(s *session) error {
	m.mu.Lock()
	var checks []func() error
	for _, l := range m.leases {
		if l.session == s {
			checks = append(checks, l.validate)
		}
	}
	m.mu.Unlock()
	for _, check := range checks {
		if err := check(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("PPT 工作区权限已失效")
}

func (s *session) status(state, mode, err string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preview.State = state
	s.preview.Mode = mode
	s.preview.Error = err
}
func (s *session) publish(raw string, ev watchEvent) error {
	safe, err := SafeHTML(raw)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if safe != s.preview.HTML || ev.Action == "scroll" {
		s.preview.Version++
		s.preview.HTML = safe
		s.preview.Slide = ev.Slide
		s.preview.ScrollTo = ev.ScrollTo
	}
	s.preview.State = "ready"
	s.preview.Error = ""
	return nil
}

// stableRevision validates ZIP structure, not slide layout. It tolerates partial
// saves and enforces bounds before any native rendering process reads the file.
func stableRevision(file string) (string, error) {
	rev, err := statRevision(file)
	if err != nil {
		return "", err
	}
	z, err := zip.OpenReader(file)
	if err != nil {
		return "", fmt.Errorf("PPT 正在保存或文件不完整")
	}
	defer z.Close()
	found := false
	var total uint64
	if len(z.File) > 30000 {
		return "", fmt.Errorf("PPT 包含过多资源")
	}
	for _, f := range z.File {
		if f.UncompressedSize64 > 256<<20 {
			return "", fmt.Errorf("PPT 资源过大")
		}
		total += f.UncompressedSize64
		if total > 512<<20 {
			return "", fmt.Errorf("PPT 解压大小超过限制")
		}
		if f.Name == "ppt/presentation.xml" {
			found = true
		}
	}
	if !found {
		return "", fmt.Errorf("不是有效的 PPTX 文件")
	}
	return rev, nil
}

func statRevision(file string) (string, error) {
	fi, err := os.Stat(file)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() || fi.Size() > 100<<20 {
		return "", fmt.Errorf("PPT 必须为普通文件且不超过 100 MiB")
	}
	return fmt.Sprintf("%d:%d", fi.Size(), fi.ModTime().UnixNano()), nil
}

type watchEvent struct {
	Action   string `json:"action"`
	Slide    int    `json:"slide"`
	ScrollTo string `json:"scrollTo"`
}
type watcher struct {
	url    string
	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan error
	events chan watchEvent
}

var watchURL = regexp.MustCompile(`Watch: http://localhost:(\d+)`)

func startWatch(ctx context.Context, exe, path string) (*watcher, error) {
	ctx, cancel := context.WithCancel(ctx)
	c := Command(ctx, exe, "watch", path, "--port", "0")
	c.Cancel = func() error { return proc.Kill(c) }
	out, err := c.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	c.Stderr = io.Discard
	if err = proc.Start(c); err != nil {
		cancel()
		return nil, err
	}
	w := &watcher{cmd: c, cancel: cancel, done: make(chan error, 1), events: make(chan watchEvent, 1)}
	ready := make(chan string, 1)
	go func() {
		scan := bufio.NewScanner(out)
		for scan.Scan() {
			if match := watchURL.FindStringSubmatch(scan.Text()); len(match) == 2 {
				port, _ := strconv.Atoi(match[1])
				if port > 0 && port < 65536 {
					select {
					case ready <- "http://127.0.0.1:" + match[1]:
					default:
					}
				}
			}
		}
	}()
	go func() { w.done <- c.Wait() }()
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	select {
	case w.url = <-ready:
		go w.listen(ctx)
		return w, nil
	case err = <-w.done:
		cancel()
		return nil, fmt.Errorf("Watch 无法启动: %v", err)
	case <-timer.C:
		cancel()
		<-w.done
		return nil, fmt.Errorf("Watch 启动超时")
	case <-ctx.Done():
		cancel()
		<-w.done
		return nil, ctx.Err()
	}
}
func (w *watcher) stop() {
	w.cancel()
	select {
	case <-w.done:
	case <-time.After(5 * time.Second):
	}
}
func (w *watcher) listen(ctx context.Context) {
	for ctx.Err() == nil {
		req, _ := http.NewRequestWithContext(ctx, "GET", w.url+"/events", nil)
		resp, err := (&http.Client{}).Do(req)
		if err == nil {
			scan := bufio.NewScanner(resp.Body)
			scan.Buffer(make([]byte, 4096), MaxHTML)
			for scan.Scan() {
				line := scan.Text()
				if strings.HasPrefix(line, "data: ") {
					var ev watchEvent
					if json.Unmarshal([]byte(line[6:]), &ev) == nil && ev.Action != "mark-update" && ev.Action != "selection-update" {
						select {
						case w.events <- ev:
						default:
							select {
							case <-w.events:
							default:
							}
							select {
							case w.events <- ev:
							default:
							}
						}
					}
				}
			}
			resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
func (w *watcher) html(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", w.url+"/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Watch HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, MaxHTML+1))
	if len(b) > MaxHTML {
		return "", fmt.Errorf("PPT 预览超过 48 MiB")
	}
	return string(b), err
}

func (s *session) run(m *Manager) {
	exe, err := m.executable(s.ctx)
	if err != nil {
		s.status("failed", "static", err.Error())
		return
	}
	var w *watcher
	defer func() {
		if w != nil {
			w.stop()
		}
	}()
	var events <-chan watchEvent
	last, candidate := "", ""
	seen := 0
	watchAttempted := false
	lastEvent := time.Time{}
	nextRender := time.Time{}
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case ev := <-events:
			if err = s.validate(); err != nil {
				s.status("failed", "static", err.Error())
				return
			}
			if raw, e := w.html(s.ctx); e == nil {
				if s.publish(raw, ev) == nil {
					s.status("ready", "watch", "")
					lastEvent = time.Now()
					last, _ = statRevision(s.path)
				}
			}
		case <-ticker.C:
			if err = s.validate(); err != nil {
				s.status("failed", "static", err.Error())
				return
			}
			rev, e := statRevision(s.path)
			if e != nil {
				state := "writing"
				if os.IsNotExist(e) {
					state = "waiting"
				}
				s.status(state, "watch", e.Error())
				continue
			}
			if rev == candidate {
				seen++
			} else {
				candidate = rev
				seen = 1
			}
			if seen < 2 {
				continue
			}
			if rev != last {
				if _, e = stableRevision(s.path); e != nil {
					s.status("writing", "watch", e.Error())
					continue
				}
			}
			if !watchAttempted {
				watchAttempted = true
				w, err = startWatch(s.ctx, exe, s.path)
				if err == nil {
					events = w.events
					if raw, e := w.html(s.ctx); e == nil && s.publish(raw, watchEvent{}) == nil {
						s.status("ready", "watch", "")
						last = rev
					}
				}
			}
			if w != nil {
				select {
				case <-w.done:
					w.cancel()
					w = nil
					events = nil
					s.status("ready", "static", "Watch 已停止，使用 CLI 静态渲染跟踪文件")
				default:
				}
			}
			if rev == last {
				s.mu.Lock()
				if s.preview.State == "waiting" || s.preview.State == "writing" {
					s.preview.State = "ready"
					s.preview.Error = ""
				}
				s.mu.Unlock()
				continue
			}
			// Give SSE a short chance to render OfficeCLI mutations; external file
			// replacements have no SSE and use a fresh CLI disk render instead.
			if time.Since(lastEvent) < 500*time.Millisecond {
				continue
			}
			if time.Now().Before(nextRender) {
				continue
			}
			s.status("rendering", "static", "")
			raw, e := Render(s.ctx, exe, s.path)
			if e != nil {
				s.status("failed", "static", e.Error())
				nextRender = time.Now().Add(5 * time.Second)
				continue
			}
			if after, _ := stableRevision(s.path); after != rev {
				continue
			}
			if e = s.publish(raw, watchEvent{}); e != nil {
				s.status("failed", "static", e.Error())
				continue
			}
			last = rev
		}
	}
}
