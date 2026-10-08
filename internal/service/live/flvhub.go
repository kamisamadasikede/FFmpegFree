package live

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// 预览视频流（契约 v0.25 / 6.10.3）：ffmpeg 把 FLV 写到一次性的本机 TCP，这里切成 tag、缓存 GOP，
// 再由只监听 127.0.0.1 的 HTTP 服务分发给前端。慢客户端只影响它自己。

const (
	previewQueueBytes = 4 << 20 // 单个客户端大约 2 秒的上限
	previewGOPBytes   = 8 << 20
	previewGOPWindow  = 10 * time.Second
	previewMaxClients = 4
	previewHeaderWait = 10 * time.Second
	previewAcceptWait = 15 * time.Second
	previewDropLimit  = 5 * time.Second
)

type flvTag struct {
	raw      []byte
	ts       int32
	video    bool
	audio    bool
	keyframe bool
	seq      bool // AVC / AAC 序列头
	script   bool
}

// flvHub 是一个会话的分发器。
type flvHub struct {
	mu        sync.Mutex
	header    []byte
	meta      []byte
	avcSeq    []byte
	aacSeq    []byte
	gop       []flvTag
	gopBytes  int
	clients   []*flvClient
	closed    bool
	ready     chan struct{}
	readyOnce sync.Once
}

func newFLVHub() *flvHub { return &flvHub{ready: make(chan struct{})} }

func (h *flvHub) isClosed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

func (h *flvHub) hasHeader() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.header != nil
}

func (h *flvHub) waitHeader(d time.Duration) bool {
	select {
	case <-h.ready:
		return true
	case <-time.After(d):
		return h.hasHeader()
	}
}

func (h *flvHub) setHeader(b []byte) {
	h.mu.Lock()
	if h.header == nil && !h.closed {
		h.header = append([]byte(nil), b...)
	}
	h.mu.Unlock()
	h.readyOnce.Do(func() { close(h.ready) })
}

func (h *flvHub) add(t flvTag) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	raw := append([]byte(nil), t.raw...)
	t.raw = raw
	switch {
	case t.script:
		h.meta = raw
	case t.seq && t.video:
		h.avcSeq = raw
	case t.seq && t.audio:
		h.aacSeq = raw
	}
	if t.video && t.keyframe {
		h.gop = nil
		h.gopBytes = 0
	}
	if t.video || t.audio {
		h.gop = append(h.gop, t)
		h.gopBytes += len(raw)
		h.trimGOP()
	}
	dead := h.offerLocked(raw, t.video && t.keyframe)
	for _, c := range dead {
		h.dropClient(c)
	}
}

// trimGOP 超过 10 秒或 8 MiB 就清空，等下一个关键帧再开始缓存。
func (h *flvHub) trimGOP() {
	if len(h.gop) == 0 {
		return
	}
	span := time.Duration(h.gop[len(h.gop)-1].ts-h.gop[0].ts) * time.Millisecond
	if span <= previewGOPWindow && h.gopBytes <= previewGOPBytes {
		return
	}
	h.gop = nil
	h.gopBytes = 0
}

func (h *flvHub) offerLocked(raw []byte, keyframe bool) (dead []*flvClient) {
	for _, c := range h.clients {
		if !c.offer(raw, keyframe) {
			dead = append(dead, c)
		}
	}
	return dead
}

func (h *flvHub) dropClient(c *flvClient) {
	for i, x := range h.clients {
		if x == c {
			h.clients = append(h.clients[:i], h.clients[i+1:]...)
			c.closeQueue()
			return
		}
	}
}

// preamble 是后加入客户端先收到的字节：FLV 头、metadata、序列头、缓存的 GOP（从关键帧开始）。
func (h *flvHub) preamble() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []byte
	out = append(out, h.header...)
	out = append(out, h.meta...)
	out = append(out, h.avcSeq...)
	out = append(out, h.aacSeq...)
	for _, t := range h.gop {
		out = append(out, t.raw...)
	}
	return out
}

// join 加入一个客户端。满员返回 false。会话已结束返回 false。
func (h *flvHub) join() (*flvClient, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.clients) >= previewMaxClients {
		return nil, false
	}
	c := newFLVClient()
	h.clients = append(h.clients, c)
	return c, true
}

func (h *flvHub) leave(c *flvClient) {
	h.mu.Lock()
	h.dropClient(c)
	h.mu.Unlock()
}

// closeHub 结束会话：客户端把队列里的数据发完后返回。
func (h *flvHub) closeHub() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	cs := h.clients
	h.clients = nil
	h.mu.Unlock()
	for _, c := range cs {
		c.closeQueue()
	}
	h.readyOnce.Do(func() { close(h.ready) })
}

type flvClient struct {
	ch       chan []byte
	mu       sync.Mutex
	queued   int
	dropping bool
	dropFrom time.Time
	once     sync.Once
}

func newFLVClient() *flvClient { return &flvClient{ch: make(chan []byte, 256)} }

// offer 不阻塞。返回 false 表示这个客户端该断开（连续丢了 5 秒）。
func (c *flvClient) offer(b []byte, keyframe bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dropping {
		if !keyframe {
			if !c.dropFrom.IsZero() && time.Since(c.dropFrom) > previewDropLimit {
				return false
			}
			return true
		}
		c.dropping = false
		c.dropFrom = time.Time{}
	}
	if c.queued+len(b) > previewQueueBytes && c.queued > 0 {
		c.dropping = true
		if c.dropFrom.IsZero() {
			c.dropFrom = time.Now()
		}
		return time.Since(c.dropFrom) <= previewDropLimit
	}
	select {
	case c.ch <- b:
		c.queued += len(b)
		return true
	default:
		c.dropping = true
		if c.dropFrom.IsZero() {
			c.dropFrom = time.Now()
		}
		return time.Since(c.dropFrom) <= previewDropLimit
	}
}

func (c *flvClient) wrote(n int) {
	c.mu.Lock()
	c.queued -= n
	if c.queued < 0 {
		c.queued = 0
	}
	c.mu.Unlock()
}

func (c *flvClient) closeQueue() { c.once.Do(func() { close(c.ch) }) }

// previewFeed 是一个会话的预览：本机 TCP 接 ffmpeg，HTTP 用 token 对外。
type previewFeed struct {
	token    string
	port     int
	url      string
	hasVideo bool
	hasAudio bool
	push     bool // 推流会话（文案用）
	hub      *flvHub
	ln       *net.TCPListener
	bytes    atomic.Int64
	// state: 0 等待 ffmpeg，1 正在出流，2 预览分支没了（会话还在），3 会话结束。
	state atomic.Int32
	done  chan struct{}
	once  sync.Once
}

func (f *previewFeed) Port() int {
	if f == nil {
		return 0
	}
	return f.port
}

// Bytes 返回预览分支收到的字节数。预览不在出流时 ok=false（不再报码率）。
func (f *previewFeed) Bytes() (int64, bool) {
	if f == nil || f.state.Load() != 1 {
		return 0, false
	}
	return f.bytes.Load(), true
}

func (f *previewFeed) stop() {
	if f == nil {
		return
	}
	f.once.Do(func() {
		f.state.Store(3)
		if f.ln != nil {
			f.ln.Close()
		}
		f.hub.closeHub()
		close(f.done)
	})
}

// readFLV 从 ffmpeg 的连接里切 tag，送进 hub。header 之前断线返回 io.EOF 一类错误，调用方可以再等一次连接。
func readFLV(r io.Reader, count func(int), h *flvHub) error {
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 32*1024)
	gotHeader := false
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			count(n)
			buf = append(buf, tmp[:n]...)
		}
		if !gotHeader && len(buf) >= 13 {
			if string(buf[:3]) != "FLV" {
				return errors.New("不是 FLV")
			}
			h.setHeader(buf[:13])
			buf = append([]byte(nil), buf[13:]...)
			gotHeader = true
		}
		for gotHeader && len(buf) >= 11 {
			size := int(buf[1])<<16 | int(buf[2])<<8 | int(buf[3])
			if size < 0 || size > 16<<20 {
				return errors.New("FLV tag 大小不合法")
			}
			total := 11 + size + 4
			if len(buf) < total {
				break
			}
			h.add(parseFLVTag(buf[:total]))
			buf = append([]byte(nil), buf[total:]...)
		}
		if err != nil {
			return err
		}
	}
}

func parseFLVTag(raw []byte) flvTag {
	t := flvTag{raw: append([]byte(nil), raw...)}
	t.ts = int32(raw[4])<<16 | int32(raw[5])<<8 | int32(raw[6]) | int32(raw[7])<<24
	kind := raw[0]
	size := int(raw[1])<<16 | int(raw[2])<<8 | int(raw[3])
	data := raw[11 : 11+size]
	switch kind {
	case 18:
		t.script = true
	case 9: // video
		t.video = true
		if len(data) >= 2 && data[0]&0x0f == 7 {
			if data[1] == 0 {
				t.seq = true
			} else if data[1] == 1 && data[0]>>4 == 1 {
				t.keyframe = true
			}
		} else if len(data) >= 1 && data[0]>>4 == 1 {
			t.keyframe = true
		}
	case 8: // audio
		t.audio = true
		if len(data) >= 2 && data[0]>>4 == 10 && data[1] == 0 {
			t.seq = true
		}
	}
	return t
}

func newPreviewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// acceptIngest 在 15 秒内接受 ffmpeg 的连接。还没收到 FLV 头时允许再连一次（硬件编码失败后用 CPU 重试会再连一次）。
func (f *previewFeed) acceptIngest() {
	defer f.ln.Close()
	deadline := time.Now().Add(previewAcceptWait)
	go func() {
		<-f.hub.ready
		f.state.CompareAndSwap(0, 1)
	}()
	for {
		f.ln.SetDeadline(deadline)
		c, err := f.ln.Accept()
		if err != nil {
			if f.state.Load() == 0 {
				f.state.Store(2)
			}
			return
		}
		err = readFLV(c, func(n int) { f.bytes.Add(int64(n)) }, f.hub)
		c.Close()
		if f.hub.hasHeader() {
			// 预览分支断了：推流不受影响，但不再报码率，之后的 GetPreviewStream 是 preview_unavailable。
			if f.state.Load() == 1 {
				f.state.Store(2)
			}
			f.hub.closeHub()
			return
		}
		if time.Now().After(deadline) || f.state.Load() == 3 {
			f.state.Store(2)
			return
		}
		_ = err
	}
}

// previewHTTP 是全局的本机 HTTP 服务。
type previewHTTP struct {
	mu    sync.Mutex
	srv   *http.Server
	ln    net.Listener
	feeds map[string]*previewFeed
	dev   bool
}

func (p *previewHTTP) start() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ln != nil {
		return p.ln.Addr().(*net.TCPAddr).Port, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	p.ln = ln
	if p.feeds == nil {
		p.feeds = map[string]*previewFeed{}
	}
	p.srv = &http.Server{Handler: p, ReadHeaderTimeout: 5 * time.Second}
	go p.srv.Serve(ln)
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func (p *previewHTTP) register(f *previewFeed) {
	p.mu.Lock()
	p.feeds[f.token] = f
	p.mu.Unlock()
}

func (p *previewHTTP) revoke(token string) {
	p.mu.Lock()
	delete(p.feeds, token)
	p.mu.Unlock()
}

func (p *previewHTTP) feed(token string) *previewFeed {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.feeds[token]
}

func (p *previewHTTP) close() {
	p.mu.Lock()
	srv, ln := p.srv, p.ln
	p.srv, p.ln = nil, nil
	p.mu.Unlock()
	if srv != nil {
		srv.Close()
	}
	if ln != nil {
		ln.Close()
	}
}

func (p *previewHTTP) port() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ln == nil {
		return 0
	}
	return p.ln.Addr().(*net.TCPAddr).Port
}

func (p *previewHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if !previewOriginAllowed(origin, p.dev) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	h := w.Header()
	h.Set("Vary", "Origin")
	h.Set("Access-Control-Allow-Origin", origin)
	if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
		h.Set("Access-Control-Allow-Private-Network", "true")
	}
	if r.Method == http.MethodOptions {
		h.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Range")
		h.Set("Access-Control-Max-Age", "600")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	token := previewTokenFromPath(r.URL.Path)
	f := p.feed(token)
	if f == nil || f.state.Load() == 3 {
		http.NotFound(w, r)
		return
	}
	if !f.hub.waitHeader(previewHeaderWait) || !f.hub.hasHeader() || f.hub.isClosed() {
		http.Error(w, "preview not ready", http.StatusServiceUnavailable)
		return
	}
	c, ok := f.hub.join()
	if !ok {
		http.Error(w, "too many preview clients", http.StatusTooManyRequests)
		return
	}
	defer f.hub.leave(c)
	h.Set("Content-Type", "video/x-flv")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)
	write := func(b []byte) {
		if len(b) == 0 {
			return
		}
		w.Write(b)
		if fl != nil {
			fl.Flush()
		}
	}
	write(f.hub.preamble())
	for b := range c.ch {
		write(b)
		c.wrote(len(b))
	}
}

func previewTokenFromPath(path string) string {
	const prefix = "/live/"
	if len(path) <= len(prefix) || path[:len(prefix)] != prefix {
		return ""
	}
	rest := path[len(prefix):]
	if len(rest) < len(".flv") || rest[len(rest)-4:] != ".flv" {
		return ""
	}
	return rest[:len(rest)-4]
}

// previewOriginAllowed 只放行 Wails 自己的 origin（契约 6.10.3.5）。空 Origin 一律拒绝。
// dev 为 true 时另外放行开发服务器（wails dev 的 34115 和 Vite 默认的 5173；wails.json 里 serverUrl 是 auto）。
func previewOriginAllowed(origin string, dev bool) bool {
	switch origin {
	case "":
		return false
	case "http://wails.localhost", "wails://wails":
		return true
	}
	if len(origin) > len("http://wails.localhost:") && origin[:len("http://wails.localhost:")] == "http://wails.localhost:" {
		_, err := strconv.Atoi(origin[len("http://wails.localhost:"):])
		return err == nil
	}
	if dev && (origin == "http://localhost:34115" || origin == "http://localhost:5173") {
		return true
	}
	return false
}
