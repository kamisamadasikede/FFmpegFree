package live

import (
	"context"
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
	// previewHeaderWait：播放器在 FLV 头到达之前就连上来时，HTTP 请求最多挂这么久等头（契约 v0.25.1）。
	// 会话失败 / 预览分支没连上时分发器会关闭，请求立即以 503 结束，不会白等满。
	previewHeaderWait = 30 * time.Second
	previewAcceptWait = 15 * time.Second
	previewDropLimit  = 5 * time.Second
	// previewWriteStall（契约 v0.25.2）：一次写（含 Flush）这么久还没写进连接，就当这个播放器已经不读了（页面切走、WebView 挂起、
	// 半开连接），断开它、释放名额。
	previewWriteStall = 2 * time.Second
	// previewSendBuffer：每个播放器连接在本机这一侧的发送缓冲。默认会自动长到几 MB，播放器不读以后还能吞下几十秒的数据，
	// 写不会卡住，也就发现不了它已经不读了。
	previewSendBuffer = 256 << 10
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
	sawVideo  bool // 出现过视频 tag：新客户端要从关键帧开始；纯音频的流没有关键帧
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

// headerTracks 返回 FLV 头里的音视频标志（第 5 字节：0x04 有音频，0x01 有视频）。还没有头时 ok 为 false。
// 这是实际送出的流（ffmpeg 按映射到的流写这两个标志），比探测结果可靠（探测不到时两路都映射了“可选”）。
func (h *flvHub) headerTracks() (video, audio, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.header) < 5 {
		return false, false, false
	}
	return h.header[4]&0x01 != 0, h.header[4]&0x04 != 0, true
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
	if t.video && !t.seq {
		h.sawVideo = true
	}
	if t.video && t.keyframe {
		h.gop = nil
		h.gopBytes = 0
	}
	// GOP 缓存总是从视频关键帧开始（第一个关键帧之前、被清空之后都不缓存半个 GOP）；纯音频的流照常缓存。
	if (t.video || t.audio) && !t.seq && (len(h.gop) > 0 || (t.video && t.keyframe) || !h.sawVideo) {
		h.gop = append(h.gop, t)
		h.gopBytes += len(raw)
		h.trimGOP()
	}
	resume := (t.video && t.keyframe) || (!h.sawVideo && t.audio)
	dead := h.offerLocked(raw, resume)
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

// dropClient 让客户端离开分发器（正常离开、太慢、被新连接挤掉）。被挤掉 / 太慢的连接同时中止，不再把队列发完。
func (h *flvHub) dropClient(c *flvClient) {
	for i, x := range h.clients {
		if x == c {
			h.clients = append(h.clients[:i], h.clients[i+1:]...)
			c.kickOff()
			return
		}
	}
}

// preamble 是后加入客户端先收到的字节：FLV 头、metadata、序列头、缓存的 GOP（从关键帧开始）。
func (h *flvHub) preamble() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.preambleLocked()
}

func (h *flvHub) preambleLocked() []byte {
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
	c, _, ok := h.joinWithPreamble()
	return c, ok
}

// joinWithPreamble 在同一把锁里取开头字节并加入客户端：锁之前到的 tag 在开头字节里，之后到的进队列，
// 既不会重复（时间戳倒退），也不会漏掉（缺参考帧花屏）。
func (h *flvHub) joinWithPreamble() (*flvClient, []byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, nil, false
	}
	// 满了：挤掉最早加入的那个（契约 v0.25.2）。连上来的只有应用自己（Origin 白名单），最新的连接就是界面上正在显示的播放器；
	// 切页面时旧播放器的连接可能没有正常关闭（WebView 挂起、半开），系统缓冲还能吞几十秒数据，看起来仍“在读”，
	// 等它超时再放行新连接就是界面冻住（原来是 429）。
	for len(h.clients) >= previewMaxClients {
		h.dropClient(h.clients[0])
	}
	c := newFLVClient()
	// 缓存里没有从关键帧开始的 GOP（还没到第一个关键帧，或者缓存刚被清空）：等下一个关键帧再开始发，不发半个 GOP。
	if h.sawVideo && len(h.gop) == 0 {
		c.dropping = true
	}
	h.clients = append(h.clients, c)
	return c, h.preambleLocked(), true
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
		c.closeQueue() // 会话结束：把队列里的数据发完再正常收尾
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
	// kick 关闭 = 被挤掉或太慢，HTTP 处理立即中止；abort 由 HTTP 处理登记，让正卡在写上的那次写立刻失败。
	kick     chan struct{}
	kickOnce sync.Once
	abort    func()
}

func newFLVClient() *flvClient {
	return &flvClient{ch: make(chan []byte, 256), kick: make(chan struct{})}
}

func (c *flvClient) kickOff() {
	c.kickOnce.Do(func() {
		close(c.kick)
		c.mu.Lock()
		abort := c.abort
		c.mu.Unlock()
		if abort != nil {
			abort()
		}
	})
}

func (c *flvClient) setAbort(f func()) {
	c.mu.Lock()
	c.abort = f
	c.mu.Unlock()
}

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
	token string
	port  int
	url   string
	// hasVideo / hasAudio 是开始时按探测结果设的值，读写都经 tracks / setTracks（拉流的探测在另一个 goroutine 里改它们）；
	// 收到 FLV 头以后以头里的标志为准。
	trackMu  sync.Mutex
	hasVideo bool
	hasAudio bool
	push     bool // 推流会话（文案用）
	// acceptWait：等 ffmpeg 连上本机 TCP 的时间，0 = previewAcceptWait。拉流要先探测、再打开远端输入才会连上来，用更长的时间。
	acceptWait time.Duration
	// paced：HLS 拉流，tag 经 flvPacer 按时间戳匀速送进分发器（要在 ffmpeg 连上之前设好）。
	paced atomic.Bool
	hub   *flvHub
	ln    *net.TCPListener
	bytes atomic.Int64
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
// add 收切好的 tag（nil 时直接 h.add）。
func readFLV(r io.Reader, count func(int), h *flvHub, add func(flvTag)) error {
	if add == nil {
		add = h.add
	}
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
			add(parseFLVTag(buf[:total]))
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
	wait := f.acceptWait
	if wait <= 0 {
		wait = previewAcceptWait
	}
	deadline := time.Now().Add(wait)
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
			f.hub.closeHub() // 等头的 HTTP 请求立即结束（503），不挂满 previewHeaderWait
			return
		}
		var pacer *flvPacer
		var add func(flvTag)
		if f.paced.Load() {
			pacer = newFLVPacer(f.hub.add)
			add = pacer.push
		}
		err = readFLV(c, func(n int) { f.bytes.Add(int64(n)) }, f.hub, add)
		c.Close()
		if pacer != nil {
			pacer.close()
		}
		if f.hub.hasHeader() {
			// 预览分支断了：推流不受影响，但不再报码率，之后的 GetPreviewStream 是 preview_unavailable。
			if f.state.Load() == 1 {
				f.state.Store(2)
			}
			f.hub.closeHub()
			return
		}
		if time.Now().After(deadline) || f.state.Load() == 3 {
			f.state.CompareAndSwap(0, 2)
			f.hub.closeHub()
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
	p.srv = &http.Server{Handler: p, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, ConnContext: p.connContext}
	go p.srv.Serve(ln)
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// connContext 把每个播放器连接的发送缓冲限制在 previewSendBuffer（见常量说明）。
func (p *previewHTTP) connContext(ctx context.Context, c net.Conn) context.Context {
	if tc, ok := c.(*net.TCPConn); ok {
		tc.SetWriteBuffer(previewSendBuffer)
	}
	return ctx
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
	c, pre, ok := f.hub.joinWithPreamble()
	if !ok {
		http.Error(w, "too many preview clients", http.StatusTooManyRequests)
		return
	}
	defer f.hub.leave(c)
	h.Set("Content-Type", "video/x-flv")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	// 被挤掉 / 太慢时让正卡着的写立刻失败（deadline 设成现在）。处理返回后不再碰这个连接。
	var abortMu sync.Mutex
	finished := false
	c.setAbort(func() {
		abortMu.Lock()
		defer abortMu.Unlock()
		if !finished {
			rc.SetWriteDeadline(time.Now())
		}
	})
	defer func() {
		abortMu.Lock()
		finished = true
		abortMu.Unlock()
		c.setAbort(nil)
	}()
	write := func(b []byte) error {
		if len(b) == 0 {
			return nil
		}
		select {
		case <-c.kick:
			return http.ErrAbortHandler
		default:
		}
		// 每次写最多等 previewWriteStall：写不进去 = 播放器不读了。
		abortMu.Lock()
		rc.SetWriteDeadline(time.Now().Add(previewWriteStall))
		abortMu.Unlock()
		if _, err := w.Write(b); err != nil {
			return err
		}
		if err := rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		return nil
	}
	// 写失败、被挤掉：中止连接（不发分块结束标记，客户端看到的是连接断开，不是“直播结束”）。
	fail := func() { panic(http.ErrAbortHandler) }
	if write(pre) != nil {
		f.hub.leave(c)
		fail()
	}
	// 客户端断开（写失败或请求的 context 结束）时立即退出并离开分发器；
	// 否则断开的客户端一直占着名额，重连几次后就是 429。
	done := r.Context().Done()
	for {
		select {
		case <-c.kick:
			fail()
		case b, ok := <-c.ch:
			if !ok {
				return // 会话结束：队列已发完，正常收尾
			}
			if write(b) != nil {
				f.hub.leave(c)
				fail()
			}
			c.wrote(len(b))
		case <-done:
			return
		}
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

// tracks 返回这一路预览里有没有视频 / 音频（契约 v0.25.3）：收到 FLV 头以后按头里的标志，之前按探测结果。
func (f *previewFeed) tracks() (video, audio bool) {
	if v, a, ok := f.hub.headerTracks(); ok {
		return v, a
	}
	f.trackMu.Lock()
	defer f.trackMu.Unlock()
	return f.hasVideo, f.hasAudio
}

func (f *previewFeed) setTracks(video, audio bool) {
	f.trackMu.Lock()
	f.hasVideo, f.hasAudio = video, audio
	f.trackMu.Unlock()
}
