// Package genoffice hosts the pinned GenOffice engines and canvas inside ffc.
package genoffice

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct{ Dir string }
type ToolState struct {
	State   string `json:"state"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version"`
	Error   string `json:"error,omitempty"`
}
type Preview struct {
	ID         string `json:"id"`
	DocumentID string `json:"documentId"`
	Version    uint64 `json:"version"`
	State      string `json:"state"`
	Mode       string `json:"mode"`
	URL        string `json:"url,omitempty"`
	Token      string `json:"token,omitempty"`
	Error      string `json:"error,omitempty"`
	Slide      int    `json:"slide"`
}
type document struct {
	id, path, root, token string
	guard                 func() error
}
type Manager struct {
	cfg       Config
	startMu   sync.Mutex
	mu        sync.Mutex
	process   *engineProcess
	server    *http.Server
	url       string
	documents map[string]*document
	views     map[string]string
	closed    bool
}

func New(cfg Config) *Manager {
	return &Manager{cfg: cfg, documents: make(map[string]*document), views: make(map[string]string)}
}
func secret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (m *Manager) Tool(retry bool) ToolState {
	node, err := nodePath(m.cfg.Dir, retry)
	if err != nil {
		return ToolState{State: "failed", Version: "GenOffice 2a607bd49bc3", Error: err.Error()}
	}
	return ToolState{State: "ready", Path: node, Version: "GenOffice 2a607bd49bc3"}
}
func (m *Manager) ensure(ctx context.Context) error {
	if !filepath.IsAbs(m.cfg.Dir) {
		return fmt.Errorf("办公组件目录尚未就绪")
	}
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return fmt.Errorf("办公服务已经关闭")
	}
	if m.process != nil {
		select {
		case <-m.process.done:
			m.process = nil
			m.documents = make(map[string]*document)
			m.views = make(map[string]string)
		default:
			m.mu.Unlock()
			return nil
		}
	}
	m.mu.Unlock()
	node, err := nodePath(m.cfg.Dir, true)
	if err != nil {
		return err
	}
	file, err := writeRuntime(m.cfg.Dir)
	if err != nil {
		return err
	}
	p, err := startEngine(node, file, filepath.Join(m.cfg.Dir, "recovery"))
	if err != nil {
		return err
	}
	var hello struct {
		Protocol int `json:"protocol"`
	}
	if err = p.request(ctx, map[string]any{"action": "hello"}, &hello); err != nil || hello.Protocol != 1 {
		p.close()
		return fmt.Errorf("办公引擎握手失败：%v", err)
	}
	m.mu.Lock()
	m.process = p
	m.mu.Unlock()
	if m.server == nil {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			p.close()
			return err
		}
		m.url = "http://" + listener.Addr().String()
		m.server = &http.Server{Handler: http.HandlerFunc(m.serve), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 30 * time.Second}
		go func() { _ = m.server.Serve(listener) }()
	}
	return nil
}
func (m *Manager) Open(ctx context.Context, file, root string, create bool, guard func() error) (map[string]any, error) {
	if guard == nil {
		return nil, fmt.Errorf("文档路径授权缺失")
	}
	if err := guard(); err != nil {
		return nil, err
	}
	if err := m.ensure(ctx); err != nil {
		return nil, err
	}
	var state map[string]any
	m.mu.Lock()
	p := m.process
	m.mu.Unlock()
	if err := p.request(ctx, map[string]any{"action": "open", "path": file, "root": root, "create": create}, &state); err != nil {
		return nil, err
	}
	id, _ := state["documentId"].(string)
	if id == "" {
		return nil, fmt.Errorf("文档会话没有返回标识")
	}
	m.mu.Lock()
	if m.documents[id] == nil {
		m.documents[id] = &document{id: id, path: file, root: root, guard: guard, token: secret()}
	}
	m.mu.Unlock()
	return state, nil
}
func (m *Manager) Start(ctx context.Context, file, root string, guard func() error) (Preview, error) {
	state, err := m.Open(ctx, file, root, false, guard)
	if err != nil {
		return Preview{}, err
	}
	id := state["documentId"].(string)
	view := secret()
	m.mu.Lock()
	m.views[view] = id
	d := m.documents[id]
	base := m.url
	m.mu.Unlock()
	return Preview{ID: view, DocumentID: id, State: fmt.Sprint(state["state"]), Mode: "genoffice", URL: base + "/editor.html", Token: d.token, Version: uint64(state["sequence"].(float64))}, nil
}
func (m *Manager) Call(ctx context.Context, id string, args map[string]any) (json.RawMessage, error) {
	m.mu.Lock()
	d := m.documents[id]
	p := m.process
	m.mu.Unlock()
	if d == nil || p == nil {
		return nil, fmt.Errorf("文档会话已结束，请重新打开")
	}
	if err := d.guard(); err != nil {
		return nil, err
	}
	action, _ := args["action"].(string)
	switch action {
	case "state", "read", "page", "apply", "save", "undo", "redo", "guide", "audit", "editor":
	default:
		return nil, fmt.Errorf("未知办公操作")
	}
	request := make(map[string]any, len(args)+1)
	for k, v := range args {
		if k != "path" && k != "root" && k != "id" {
			request[k] = v
		}
	}
	request["documentId"] = id
	var result json.RawMessage
	if err := p.request(ctx, request, &result); err != nil {
		return nil, err
	}
	return result, nil
}
func (m *Manager) Read(id string, version uint64) (Preview, error) {
	m.mu.Lock()
	documentID := m.views[id]
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	raw, err := m.Call(ctx, documentID, map[string]any{"action": "state", "since": version})
	if err != nil {
		return Preview{}, err
	}
	var state struct {
		Sequence uint64 `json:"sequence"`
		State    string `json:"state"`
		Error    string `json:"error"`
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		return Preview{}, err
	}
	return Preview{ID: id, DocumentID: documentID, Version: state.Sequence, State: state.State, Mode: "genoffice", Error: state.Error}, nil
}
func (m *Manager) Release(id string) { m.mu.Lock(); delete(m.views, id); m.mu.Unlock() }
func (m *Manager) Close() {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	m.closed = true
	p := m.process
	server := m.server
	m.views = make(map[string]string)
	m.mu.Unlock()
	if server != nil {
		_ = server.Close()
	}
	p.close()
}
func (m *Manager) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	m.mu.Lock()
	base := m.url
	m.mu.Unlock()
	parsed, _ := url.Parse(base)
	if parsed == nil || r.Host != parsed.Host {
		http.Error(w, "Forbidden", 403)
		return
	}
	if r.URL.Path == "/editor.html" || r.URL.Path == "/editor.js" || r.URL.Path == "/editor.css" {
		if r.Method != "GET" {
			http.Error(w, "Method", 405)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		raw, err := runtimeFiles.ReadFile("runtime/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src data: blob:; font-src 'self' data: blob:; connect-src 'self'; base-uri 'none'; object-src 'none'")
		if name == "editor.js" {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		} else if name == "editor.css" {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		} else {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		_, _ = w.Write(raw)
		return
	}
	if r.URL.Path != "/api" || r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Origin") != base || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "Forbidden", 403)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	var args map[string]any
	if json.NewDecoder(r.Body).Decode(&args) != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	id, _ := args["id"].(string)
	m.mu.Lock()
	d := m.documents[id]
	m.mu.Unlock()
	if d == nil || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+d.token)) != 1 {
		http.Error(w, "Unauthorized", 401)
		return
	}
	result, err := m.Call(r.Context(), id, args)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err != nil {
		w.WriteHeader(409)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": err.Error()}})
		return
	}
	_, _ = w.Write(result)
}
