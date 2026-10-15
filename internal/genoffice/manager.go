// Package genoffice hosts the pinned GenOffice engines and canvas inside ffc.
package genoffice

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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
	kind, page            string
	session               string
	mu                    sync.Mutex
	preview               []byte
}
type Manager struct {
	cfg       Config
	startMu   sync.Mutex
	mu        sync.Mutex
	process   *engineProcess
	sheets    *xlsxSidecar
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
			for id, d := range m.documents {
				if d.kind == "file" {
					continue
				}
				delete(m.documents, id)
				for view, docID := range m.views {
					if docID == id {
						delete(m.views, view)
					}
				}
			}
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
	if err = m.listen(); err != nil {
		p.close()
		return err
	}
	return nil
}
func (m *Manager) listen() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("办公服务已经关闭")
	}
	if m.server != nil {
		return nil
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	m.url = "http://" + listener.Addr().String()
	m.server = &http.Server{Handler: http.HandlerFunc(m.serve), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 30 * time.Second}
	go func() { _ = m.server.Serve(listener) }()
	return nil
}
func (m *Manager) ensureServer() error {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	return m.listen()
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
		m.documents[id] = &document{id: id, path: file, root: root, guard: guard, token: secret(), kind: "pptx"}
	}
	m.mu.Unlock()
	return state, nil
}
func (m *Manager) Start(ctx context.Context, file, root string, guard func() error) (Preview, error) {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".docx":
		return m.startFile(ctx, file, root, guard, "/docs.html")
	case ".pdf":
		return m.startFile(ctx, file, root, guard, "/pdf.html")
	case ".html", ".htm":
		return m.startFile(ctx, file, root, guard, "/html.html")
	case ".md", ".markdown":
		return m.startFile(ctx, file, root, guard, "/markdown.html")
	case ".xlsx":
		return m.startFile(ctx, file, root, guard, "/sheets.html")
	case ".pptx":
	default:
		return Preview{}, fmt.Errorf("当前页面不支持这个文件")
	}
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
func (m *Manager) startFile(ctx context.Context, file, root string, guard func() error, page string) (Preview, error) {
	if err := ctx.Err(); err != nil {
		return Preview{}, err
	}
	if guard == nil {
		return Preview{}, fmt.Errorf("文档路径授权缺失")
	}
	if err := guard(); err != nil {
		return Preview{}, err
	}
	fi, err := os.Stat(file)
	if err != nil || fi.IsDir() {
		return Preview{}, fmt.Errorf("文件还没有准备好")
	}
	if fi.Size() > fileByteLimit {
		return Preview{}, fmt.Errorf("文件太大")
	}
	if err = m.ensureServer(); err != nil {
		return Preview{}, err
	}
	id, token, view := secret(), secret(), secret()
	m.mu.Lock()
	m.documents[id] = &document{id: id, path: file, root: root, token: token, guard: guard, kind: "file", page: page}
	m.views[view] = id
	base := m.url
	m.mu.Unlock()
	return Preview{ID: view, DocumentID: id, State: "ready", Mode: "genoffice", URL: base + page, Token: token, Version: uint64(fi.ModTime().Unix())}, nil
}
func (m *Manager) Call(ctx context.Context, id string, args map[string]any) (json.RawMessage, error) {
	m.mu.Lock()
	d := m.documents[id]
	p := m.process
	m.mu.Unlock()
	if d == nil || (d.kind != "file" && p == nil) {
		return nil, fmt.Errorf("文档会话已结束，请重新打开")
	}
	if d.kind == "file" {
		return nil, fmt.Errorf("这个文件由官方页面直接读写")
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
	d := m.documents[documentID]
	m.mu.Unlock()
	if d != nil && d.kind == "file" {
		if err := d.guard(); err != nil {
			return Preview{}, err
		}
		fi, err := os.Stat(d.path)
		if err != nil {
			return Preview{}, err
		}
		return Preview{ID: id, DocumentID: documentID, Version: uint64(fi.ModTime().Unix()), State: "ready", Mode: "genoffice"}, nil
	}
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
	sheets := m.sheets
	server := m.server
	m.views = make(map[string]string)
	m.mu.Unlock()
	if server != nil {
		_ = server.Close()
	}
	p.close()
	sheets.close()
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
	if kind, ok := pageFiles[strings.TrimPrefix(r.URL.Path, "/")]; ok {
		m.servePage(w, r, kind)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/pdfjs/") && r.Method == "GET" {
		m.servePdfjs(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/fonts/") && r.Method == "GET" {
		m.serveFont(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/preview/") && r.Method == "GET" {
		m.servePreview(w, r)
		return
	}
	if r.URL.Path == "/meta" && r.Method == "GET" {
		m.serveMeta(w, r, base)
		return
	}
	if r.URL.Path == "/bytes" && (r.Method == "GET" || r.Method == "POST") {
		m.serveBytes(w, r, base)
		return
	}
	if r.URL.Path == "/preview" && r.Method == "POST" {
		m.servePreviewWrite(w, r, base)
		return
	}
	if r.URL.Path == "/sheets" && r.Method == "POST" {
		m.serveSheets(w, r, base)
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

const fileByteLimit = 64 << 20

var pageFiles = map[string]string{
	"editor.html": "text/html; charset=utf-8", "editor.js": "application/javascript; charset=utf-8", "editor.css": "text/css; charset=utf-8",
	"docs.html": "text/html; charset=utf-8", "docs.js": "application/javascript; charset=utf-8", "docs.css": "text/css; charset=utf-8",
	"pdf.html": "text/html; charset=utf-8", "pdf.js": "application/javascript; charset=utf-8", "pdf.css": "text/css; charset=utf-8",
	"pdf.worker.min.mjs": "text/javascript; charset=utf-8",
	"html.html":          "text/html; charset=utf-8", "html.js": "application/javascript; charset=utf-8", "html.css": "text/css; charset=utf-8",
	"markdown.html": "text/html; charset=utf-8", "markdown.js": "application/javascript; charset=utf-8", "markdown.css": "text/css; charset=utf-8",
	"sheets.html": "text/html; charset=utf-8", "sheets.js": "application/javascript; charset=utf-8", "sheets.css": "text/css; charset=utf-8",
}

func (m *Manager) servePdfjs(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if !strings.HasPrefix(name, "pdfjs/") || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	raw, err := runtimeFiles.ReadFile("runtime/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasSuffix(name, ".wasm"):
		w.Header().Set("Content-Type", "application/wasm")
	case strings.HasSuffix(name, ".js"), strings.HasSuffix(name, ".mjs"):
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case strings.HasSuffix(name, ".ttf"):
		w.Header().Set("Content-Type", "font/ttf")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	_, _ = w.Write(raw)
}

var fontFile = regexp.MustCompile(`^[a-f0-9]{16}\.(ttf|woff|woff2)$`)

func (m *Manager) serveFont(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/fonts/")
	if !fontFile.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	raw, err := runtimeFiles.ReadFile("runtime/fonts/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasSuffix(name, ".woff2"):
		w.Header().Set("Content-Type", "font/woff2")
	case strings.HasSuffix(name, ".woff"):
		w.Header().Set("Content-Type", "font/woff")
	default:
		w.Header().Set("Content-Type", "font/ttf")
	}
	_, _ = w.Write(raw)
}
func (m *Manager) servePage(w http.ResponseWriter, r *http.Request, kind string) {
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
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self' blob:; style-src 'self' 'unsafe-inline'; img-src data: blob:; font-src 'self' data: blob:; connect-src 'self' blob:; frame-src 'self'; media-src data: blob:; base-uri 'none'; object-src 'none'")
	w.Header().Set("Content-Type", kind)
	_, _ = w.Write(raw)
}
func validID(id string) bool {
	if len(id) != 64 {
		return false
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func (m *Manager) fileDocument(r *http.Request, base string, write bool) (*document, int, string) {
	id := r.URL.Query().Get("id")
	if !validID(id) {
		return nil, 401, "未授权"
	}
	m.mu.Lock()
	d := m.documents[id]
	m.mu.Unlock()
	if d == nil || d.kind != "file" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+d.token)) != 1 {
		return nil, 401, "未授权"
	}
	origin := r.Header.Get("Origin")
	if write {
		if origin != base {
			return nil, 403, "来源不被接受"
		}
	} else if origin != "" && origin != base {
		return nil, 403, "来源不被接受"
	}
	if err := d.guard(); err != nil {
		return nil, 403, "工作区路径已改变"
	}
	return d, 200, ""
}
func rejectFile(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
func fileRevision(fi os.FileInfo) string {
	return fmt.Sprintf("%d-%d", fi.ModTime().UnixNano(), fi.Size())
}
func (m *Manager) serveMeta(w http.ResponseWriter, r *http.Request, base string) {
	d, status, message := m.fileDocument(r, base, false)
	if d == nil {
		rejectFile(w, status, message)
		return
	}
	fi, err := os.Stat(d.path)
	if err != nil || fi.IsDir() || fi.Size() > fileByteLimit {
		rejectFile(w, 409, "文件读取失败")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"name": filepath.Base(d.path), "path": d.path, "revision": fileRevision(fi), "size": fi.Size()})
}
func (m *Manager) serveBytes(w http.ResponseWriter, r *http.Request, base string) {
	d, status, message := m.fileDocument(r, base, r.Method == "POST")
	if d == nil {
		rejectFile(w, status, message)
		return
	}
	if r.Method == "GET" {
		fi, err := os.Stat(d.path)
		if err != nil || fi.IsDir() || fi.Size() > fileByteLimit {
			rejectFile(w, 409, "文件读取失败")
			return
		}
		f, err := os.Open(d.path)
		if err != nil {
			rejectFile(w, 409, "文件读取失败")
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, "file", fi.ModTime(), f)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, fileByteLimit)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		rejectFile(w, 400, "文件太大")
		return
	}
	if err = os.WriteFile(d.path, data, 0o600); err != nil {
		rejectFile(w, 409, "保存失败")
		return
	}
	fi, err := os.Stat(d.path)
	if err != nil {
		rejectFile(w, 409, "保存失败")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "revision": fileRevision(fi)})
}
func (m *Manager) servePreview(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/preview/")
	if !validID(id) || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	m.mu.Lock()
	d := m.documents[id]
	m.mu.Unlock()
	if d == nil || d.kind != "file" || d.guard() != nil {
		http.NotFound(w, r)
		return
	}
	d.mu.Lock()
	body := d.preview
	d.mu.Unlock()
	if body == nil {
		body = []byte("<!doctype html><meta charset=\"utf-8\"><title></title>")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}
func (m *Manager) servePreviewWrite(w http.ResponseWriter, r *http.Request, base string) {
	d, status, message := m.fileDocument(r, base, true)
	if d == nil {
		rejectFile(w, status, message)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, fileByteLimit)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		rejectFile(w, 400, "文件太大")
		return
	}
	d.mu.Lock()
	d.preview = append([]byte(nil), data...)
	d.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
