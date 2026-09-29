// Package localassets 是 `/local/<token>` 本地文件预览的登记表与 HTTP Handler（契约 6.11.4），
// EditService（视频 / 音频预览）和 DocService（PDF 预览）共用。
//
// 用法：
//
//	reg := localassets.New(localassets.Config{})
//	e, err := reg.Register("/abs/path/a.mp4") // e.URL == "/local/<token>"
//	options.App.AssetServer.Handler = reg.Handler()
//	reg.Revoke(e.Token)
//
// 安全约束：
//   - Handler 只按 token 查表，不接受任何路径参数；token 是 crypto/rand 的 128 位随机数（32 个十六进制字符）；
//   - 登记时和每次请求都对路径 EvalSymlinks，并要求结果与登记时相同、仍是同一个普通文件（os.SameFile），
//     文件被换成符号链接、目录、被别的文件替换后一律 404（前端应重新向服务要一个 token）；
//   - 只允许 GET / HEAD；HEAD 只回头部（用于探测 token 是否仍有效，token 失效 / 文件变化返回 404），不受 32 MiB 限制；Range 只取第一段，多段拒绝（416）；
//   - Windows 上 Wails AssetServer 把响应整个缓冲进内存、不支持流式：每个 Range 响应最多 MaxRangeBytes（4 MiB），
//     没有 Range 的请求文件不超过 MaxWholeBytes（32 MiB）才返回，更大返回 413。
package localassets

import (
	"container/list"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	// Prefix 是 URL 前缀。
	Prefix = "/local/"
	// DefaultMaxEntries 是登记表容量，超过时淘汰最旧的。
	DefaultMaxEntries = 1024
	// DefaultMaxRangeBytes 是单个 Range 响应的最大字节数（4 MiB）。
	DefaultMaxRangeBytes int64 = 4 << 20
	// DefaultMaxWholeBytes 是不带 Range 的请求允许返回的最大文件（32 MiB）。
	DefaultMaxWholeBytes int64 = 32 << 20

	tokenBytes = 16
)

// Config 可调参数，零值用默认值（测试里调小）。
type Config struct {
	MaxEntries    int
	MaxRangeBytes int64
	MaxWholeBytes int64
}

// Entry 是一次登记的结果。
type Entry struct {
	Token string `json:"token"`
	URL   string `json:"url"`  // /local/<token>
	Mime  string `json:"mime"` // 按扩展名，未知为 application/octet-stream
	Size  int64  `json:"size"`
}

type item struct {
	token    string
	path     string // 登记时传入的路径（Clean 后的绝对路径）
	resolved string // EvalSymlinks 之后的真实路径
	info     os.FileInfo
	elem     *list.Element
}

// Registry 是 token → 文件的登记表，并发安全。
type Registry struct {
	cfg    Config
	mu     sync.Mutex
	byTok  map[string]*item
	byPath map[string]*item // resolved → item，同一文件复用 token
	order  *list.List       // 前面最旧
}

// New 创建登记表。
func New(cfg Config) *Registry {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = DefaultMaxEntries
	}
	if cfg.MaxRangeBytes <= 0 {
		cfg.MaxRangeBytes = DefaultMaxRangeBytes
	}
	if cfg.MaxWholeBytes <= 0 {
		cfg.MaxWholeBytes = DefaultMaxWholeBytes
	}
	return &Registry{cfg: cfg, byTok: map[string]*item{}, byPath: map[string]*item{}, order: list.New()}
}

// ErrNotRegular 表示路径不是普通文件（目录、FIFO、设备……）。
var ErrNotRegular = errors.New("不是普通文件")

// ErrNotAbs 表示路径不是绝对路径。
var ErrNotAbs = errors.New("路径必须是绝对路径")

// resolve 校验 path：绝对、EvalSymlinks 成功、目标是普通文件。返回真实路径和 FileInfo。
func resolve(path string) (string, os.FileInfo, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", nil, ErrNotAbs
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", nil, err
	}
	fi, err := os.Stat(real)
	if err != nil {
		return "", nil, err
	}
	if !fi.Mode().IsRegular() {
		return "", nil, ErrNotRegular
	}
	return real, fi, nil
}

// Register 登记文件并返回 token。同一个（真实）文件复用已有 token；登记表满时淘汰最旧的。
// 路径必须是绝对路径、存在且是普通文件（符号链接会被解析到真实文件，真实目标必须是普通文件）。
func (r *Registry) Register(path string) (Entry, error) {
	real, fi, err := resolve(path)
	if err != nil {
		return Entry{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if it, ok := r.byPath[real]; ok && os.SameFile(it.info, fi) {
		it.info = fi // 同一个文件，刷新大小 / 修改时间
		it.path = filepath.Clean(path)
		r.order.MoveToBack(it.elem)
		return entryOf(it), nil
	} else if ok {
		r.removeLocked(it) // 同一路径上的文件已被换掉，旧 token 作废
	}
	tok, err := newToken()
	if err != nil {
		return Entry{}, err
	}
	it := &item{token: tok, path: filepath.Clean(path), resolved: real, info: fi}
	it.elem = r.order.PushBack(it)
	r.byTok[tok] = it
	r.byPath[real] = it
	for len(r.byTok) > r.cfg.MaxEntries {
		r.removeLocked(r.order.Front().Value.(*item))
	}
	return entryOf(it), nil
}

// Revoke 使 token 失效；不存在时无操作。
func (r *Registry) Revoke(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if it, ok := r.byTok[token]; ok {
		r.removeLocked(it)
	}
}

// Len 返回当前登记数（测试 / 诊断用）。
func (r *Registry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byTok)
}

func (r *Registry) removeLocked(it *item) {
	r.order.Remove(it.elem)
	delete(r.byTok, it.token)
	if r.byPath[it.resolved] == it {
		delete(r.byPath, it.resolved)
	}
}

func entryOf(it *item) Entry {
	return Entry{Token: it.token, URL: Prefix + it.token, Mime: ContentType(it.path), Size: it.info.Size()}
}

func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成 token 失败: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func validToken(s string) bool {
	if len(s) != tokenBytes*2 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

var mimeByExt = map[string]string{
	".mp4": "video/mp4", ".m4v": "video/mp4", ".webm": "video/webm", ".mov": "video/quicktime",
	".mkv": "video/x-matroska", ".avi": "video/x-msvideo", ".flv": "video/x-flv",
	".mp3": "audio/mpeg", ".wav": "audio/wav", ".aac": "audio/aac", ".m4a": "audio/mp4",
	".flac": "audio/flac", ".ogg": "audio/ogg", ".opus": "audio/ogg",
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
	".pdf": "application/pdf",
}

// ContentType 按扩展名（不区分大小写）返回 Content-Type，未知为 application/octet-stream。
func ContentType(path string) string {
	if m, ok := mimeByExt[strings.ToLower(filepath.Ext(path))]; ok {
		return m
	}
	return "application/octet-stream"
}

// Handler 返回给 Wails AssetServer 的 http.Handler（options.AssetServer.Handler）。
func (r *Registry) Handler() http.Handler { return http.HandlerFunc(r.serve) }

func (r *Registry) serve(w http.ResponseWriter, req *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		h.Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p := req.URL.Path
	if !strings.HasPrefix(p, Prefix) {
		http.NotFound(w, req)
		return
	}
	tok := p[len(Prefix):]
	if !validToken(tok) { // 同时排除了 /、..、多余的路径段
		http.NotFound(w, req)
		return
	}
	r.mu.Lock()
	it, ok := r.byTok[tok]
	var path, resolved string
	var want os.FileInfo
	if ok {
		path, resolved, want = it.path, it.resolved, it.info
	}
	r.mu.Unlock()
	if !ok {
		http.NotFound(w, req)
		return
	}
	// 每次请求重新校验：路径解析结果没变、是普通文件、且打开的就是登记时的那个文件。
	real, err := filepath.EvalSymlinks(path)
	if err != nil || real != resolved {
		http.NotFound(w, req)
		return
	}
	f, err := os.Open(real)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || !os.SameFile(want, fi) {
		http.NotFound(w, req)
		return
	}
	size := fi.Size()
	h.Set("Content-Type", ContentType(path))
	h.Set("Accept-Ranges", "bytes")

	rangeHdr := req.Header.Get("Range")
	start, length := int64(0), size
	status := http.StatusOK
	if rangeHdr != "" {
		s, l, kind := parseRange(rangeHdr, size, r.cfg.MaxRangeBytes)
		switch kind {
		case rangeIgnore:
			// 不是 bytes 单位：按没有 Range 处理
		case rangeInvalid:
			h.Set("Content-Range", "bytes */"+strconv.FormatInt(size, 10))
			http.Error(w, "range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
			return
		case rangeOK:
			start, length, status = s, l, http.StatusPartialContent
		}
	}
	// HEAD 用来探测 token 是否仍有效（前端 404 后重新 GetPreviewURL）：不受 32 MiB 限制，只回头部不回 body。
	if status == http.StatusOK && size > r.cfg.MaxWholeBytes && req.Method != http.MethodHead {
		http.Error(w, "file too large without Range", http.StatusRequestEntityTooLarge)
		return
	}
	h.Set("Content-Length", strconv.FormatInt(length, 10))
	if status == http.StatusPartialContent {
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+length-1, size))
	}
	w.WriteHeader(status)
	if req.Method == http.MethodHead || length == 0 {
		return
	}
	io.Copy(w, io.NewSectionReader(f, start, length))
}

type rangeKind int

const (
	rangeOK rangeKind = iota
	rangeIgnore
	rangeInvalid
)

// parseRange 解析 Range 头：只取第一段，多段返回 rangeInvalid；后缀（-N）和开区间（N-）按 maxLen 截断；
// 起点越界、格式错误返回 rangeInvalid（416）；非 bytes 单位返回 rangeIgnore。
func parseRange(h string, size, maxLen int64) (start, length int64, kind rangeKind) {
	const unit = "bytes="
	if !strings.HasPrefix(strings.ToLower(h), unit) {
		return 0, 0, rangeIgnore
	}
	spec := strings.TrimSpace(h[len(unit):])
	if spec == "" || strings.Contains(spec, ",") {
		return 0, 0, rangeInvalid // 多段一律拒绝
	}
	a, b, ok := strings.Cut(spec, "-")
	if !ok {
		return 0, 0, rangeInvalid
	}
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	var end int64
	switch {
	case a == "": // 后缀：最后 N 字节
		n, err := parseUint(b)
		if err != nil || n == 0 || size == 0 {
			return 0, 0, rangeInvalid
		}
		if n > size {
			n = size
		}
		start, end = size-n, size-1
	default:
		s, err := parseUint(a)
		if err != nil || s >= size {
			return 0, 0, rangeInvalid
		}
		start = s
		if b == "" {
			end = size - 1
		} else {
			e, err := parseUint(b)
			if err != nil || e < s {
				return 0, 0, rangeInvalid
			}
			if e > size-1 {
				e = size - 1
			}
			end = e
		}
	}
	length = end - start + 1
	if length > maxLen {
		length = maxLen
	}
	return start, length, rangeOK
}

func parseUint(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, errors.New("bad digit")
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err // 溢出
	}
	return n, nil
}
