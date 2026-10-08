//go:build !windows

package live

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
)

func makeTag(kind byte, ts int, data []byte) []byte {
	n := len(data)
	b := []byte{kind, byte(n >> 16), byte(n >> 8), byte(n), byte(ts >> 16), byte(ts >> 8), byte(ts), byte(ts >> 24), 0, 0, 0}
	b = append(b, data...)
	prev := len(b)
	return append(b, byte(prev>>24), byte(prev>>16), byte(prev>>8), byte(prev))
}

func TestPreviewHTTPOriginTokenAndLateJoiner(t *testing.T) {
	h := &previewHTTP{feeds: map[string]*previewFeed{}, dev: true}
	f := &previewFeed{token: "tok", hub: newFLVHub(), hasVideo: true, hasAudio: true, done: make(chan struct{})}
	f.state.Store(1)
	h.register(f)
	f.hub.setHeader([]byte("FLV\x01\x05\x00\x00\x00\x09\x00\x00\x00\x00"))
	f.hub.add(parseFLVTag(makeTag(9, 0, []byte{0x17, 0x00, 0, 0, 0, 0x01}))) // 序列头
	f.hub.add(parseFLVTag(makeTag(9, 0, []byte{0x17, 0x01, 0, 0, 0, 0xAA}))) // 关键帧
	f.hub.add(parseFLVTag(makeTag(9, 40, []byte{0x27, 0x01, 0, 0, 0, 0xBB})))

	no := httptest.NewRequest(http.MethodGet, "/live/tok.flv", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, no)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("没有 Origin 应是 403: %d", rec.Code)
	}
	bad := httptest.NewRequest(http.MethodGet, "/live/tok.flv", nil)
	bad.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, bad)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("别的 Origin 应是 403: %d", rec.Code)
	}
	opt := httptest.NewRequest(http.MethodOptions, "/live/tok.flv", nil)
	opt.Header.Set("Origin", "http://wails.localhost")
	opt.Header.Set("Access-Control-Request-Private-Network", "true")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, opt)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "http://wails.localhost" || rec.Header().Get("Access-Control-Allow-Private-Network") != "true" {
		t.Fatalf("预检: %d %v", rec.Code, rec.Header())
	}
	miss := httptest.NewRequest(http.MethodGet, "/live/nope.flv", nil)
	miss.Header.Set("Origin", "wails://wails")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, miss)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("错误 token 应是 404: %d", rec.Code)
	}
	post := httptest.NewRequest(http.MethodPost, "/live/tok.flv", nil)
	post.Header.Set("Origin", "http://wails.localhost:34115")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, post)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST 应是 405: %d", rec.Code)
	}

	// 后加入：先收到的视频是关键帧，不是中间帧。
	body := h.preambleOf(t, f)
	if !strings.HasPrefix(body, "FLV") {
		t.Fatalf("应以 FLV 头开始: %q", body[:4])
	}
	if i := strings.Index(body, string([]byte{0x17, 0x01})); i < 0 {
		t.Fatal("缓存里应有关键帧")
	} else if j := strings.Index(body, string([]byte{0x27, 0x01})); j >= 0 && j < i {
		t.Fatal("后加入不能从中间帧开始")
	}

	// 慢客户端不拖住分发：队列满了 offer 立即返回。
	c, ok := f.hub.join()
	if !ok {
		t.Fatal("join")
	}
	start := time.Now()
	payload := bytesRepeat(200000)
	for i := 0; i < 40; i++ {
		f.hub.add(parseFLVTag(makeTag(9, 80+i*40, append([]byte{0x27, 0x01, 0, 0, 0}, payload...))))
	}
	if time.Since(start) > time.Second {
		t.Fatalf("慢客户端把分发堵住了: %v", time.Since(start))
	}
	// 8 MB 灌给一个从不读的客户端（队列上限 4 MB）：先进入丢帧（等下一个关键帧），连续丢 5 秒以上就被踢掉。
	c.mu.Lock()
	dropping := c.dropping
	c.dropFrom = time.Now().Add(-previewDropLimit - time.Second) // 模拟已经连续丢了 6 秒
	c.mu.Unlock()
	if !dropping {
		t.Fatal("队列满了应进入丢帧")
	}
	f.hub.add(parseFLVTag(makeTag(9, 2000, []byte{0x27, 0x01, 0, 0, 0, 1})))
	f.hub.mu.Lock()
	still := false
	for _, x := range f.hub.clients {
		still = still || x == c
	}
	f.hub.mu.Unlock()
	if still {
		t.Fatal("一直不读的客户端应被踢掉")
	}
	f.hub.leave(c)

	// 同时最多 previewMaxClients 个，再来一个 join 失败（HTTP 429）。
	var cs []*flvClient
	for i := 0; i < previewMaxClients; i++ {
		x, ok := f.hub.join()
		if !ok {
			t.Fatalf("第 %d 个客户端应能加入", i+1)
		}
		cs = append(cs, x)
	}
	extra := httptest.NewRequest(http.MethodGet, "/live/tok.flv", nil)
	extra.Header.Set("Origin", "http://wails.localhost")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, extra)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("第 %d 个客户端应 429: %d", previewMaxClients+1, rec.Code)
	}
	for _, x := range cs {
		f.hub.leave(x)
	}

	// 会话结束：正在读的客户端正常读完（handler 返回，不是半截挂起）。
	done := make(chan struct{})
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/live/tok.flv", nil)
		req.Header.Set("Origin", "http://localhost:34115")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "FLV") {
			t.Errorf("结束时应正常返回已有数据: %d %d", rec.Code, rec.Body.Len())
		}
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	f.hub.closeHub()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("会话结束没有把 HTTP 响应收尾")
	}
	h.revoke(f.token)
	gone := httptest.NewRequest(http.MethodGet, "/live/tok.flv", nil)
	gone.Header.Set("Origin", "http://wails.localhost")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, gone)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("作废后应是 404: %d", rec.Code)
	}
}

func (h *previewHTTP) preambleOf(t *testing.T, f *previewFeed) string {
	t.Helper()
	return string(f.hub.preamble())
}

func bytesRepeat(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = 0x11
	}
	return b
}

func TestGetPreviewStreamErrorsAndCodecGate(t *testing.T) {
	f := newFixture(t, func(c *Config) {
		c.Protocols = protoWithTCP()
		old := c.Require
		c.Require = func() (ffmpeg.Binaries, error) {
			b, err := old()
			if err == nil {
				b.FFprobe = b.FFmpeg
			}
			return b, err
		}
		c.ProbeStreams = func(ctx context.Context, _, _, _ string) (StreamProbe, error) {
			return StreamProbe{Video: "hevc", Audio: "aac"}, nil
		}
	})
	if _, err := f.svc.GetPreviewStream("nope"); err == nil || !apperr.Is(err, apperr.NotFound) || !strings.Contains(err.(*apperr.AppError).Detail, "reason=session") {
		t.Fatalf("不存在: %v", err)
	}
	var evMu sync.Mutex
	var got PullEvent
	f.svc.cfg.Emit = func(name string, payload any) {
		if name != "live:pull" {
			t.Errorf("事件名 %s", name)
		}
		evMu.Lock()
		got = payload.(PullEvent)
		evMu.Unlock()
	}
	ps, err := f.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: "rtmp://127.0.0.1:1935/live/k"})
	if err != nil {
		t.Fatal(err)
	}
	var ev PullEvent
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		evMu.Lock()
		ev = got
		evMu.Unlock()
		if ev.State != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ev.State != "unsupported" || ev.Error == nil || !strings.Contains(ev.Error.Detail, "reason=codec") || !strings.Contains(ev.Error.Detail, "video=hevc") || ev.Error.Message != "这路视频无法在应用内播放。" {
		t.Fatalf("编码不支持: %+v", ev)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := f.svc.GetPreviewStream(ps.ID); err == nil || !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("结束后应是 NOT_FOUND: %v", err)
	}
	_ = io.EOF
}

func protoWithTCP() *ffmpeg.ProtocolProbe {
	return &ffmpeg.ProtocolProbe{Run: func(context.Context, string, ...string) (string, error) {
		return "Output:\n  rtmp\n  tee\n  tcp\n", nil
	}}
}

func TestParseStreamProbe(t *testing.T) {
	pr, err := parseStreamProbe([]byte(`{"programs":[],"streams":[{"codec_name":"aac","codec_type":"audio"},{"codec_name":"h264","codec_type":"video"},{"codec_name":"mp3","codec_type":"audio"}],"format":{"format_name":"hls"}}`))
	if err != nil || pr.Video != "h264" || pr.Audio != "aac" || pr.Format != "hls" {
		t.Fatalf("%+v %v", pr, err)
	}
	if _, err := parseStreamProbe([]byte(`{"streams":[],"format":{"format_name":"flv"}}`)); err == nil {
		t.Fatal("没有流应报错")
	}
	if _, err := parseStreamProbe([]byte(`garbage`)); err == nil {
		t.Fatal("解析失败应报错")
	}
}

// 客户端断开后要立刻离开分发器：反复连上再断开不能把名额占满（修之前第 5 次就是 429）。
func TestPreviewClientDisconnectFreesSlot(t *testing.T) {
	h := &previewHTTP{feeds: map[string]*previewFeed{}, dev: true}
	f := &previewFeed{token: "tok", hub: newFLVHub(), hasVideo: true, hasAudio: true, done: make(chan struct{})}
	f.state.Store(1)
	h.register(f)
	f.hub.setHeader([]byte("FLV\x01\x05\x00\x00\x00\x09\x00\x00\x00\x00"))
	f.hub.add(parseFLVTag(makeTag(9, 0, []byte{0x17, 0x01, 0, 0, 0, 0xAA})))
	srv := httptest.NewServer(h)
	defer srv.Close()
	for i := 0; i < 3*previewMaxClients; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/live/tok.flv", nil)
		req.Header.Set("Origin", "wails://wails")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("第 %d 次连接: HTTP %d", i+1, resp.StatusCode)
		}
		buf := make([]byte, 13)
		io.ReadFull(resp.Body, buf)
		cancel()
		resp.Body.Close()
		deadline := time.Now().Add(2 * time.Second)
		for {
			f.hub.mu.Lock()
			n := len(f.hub.clients)
			f.hub.mu.Unlock()
			if n == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("第 %d 次断开后还占着 %d 个名额", i+1, n)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// 后加入的客户端：开头字节和之后的队列在同一把锁里切开，tag 不重复也不遗漏。
func TestJoinWithPreambleIsAtomic(t *testing.T) {
	hub := newFLVHub()
	hub.setHeader([]byte("FLV\x01\x05\x00\x00\x00\x09\x00\x00\x00\x00"))
	before := makeTag(9, 0, []byte{0x17, 0x01, 0, 0, 0, 0xAA})
	after := makeTag(9, 40, []byte{0x27, 0x01, 0, 0, 0, 0xBB})
	hub.add(parseFLVTag(before))
	c, pre, ok := hub.joinWithPreamble()
	if !ok {
		t.Fatal("join")
	}
	hub.add(parseFLVTag(after))
	if !strings.Contains(string(pre), string(before)) || strings.Contains(string(pre), string(after)) {
		t.Fatal("开头字节应只含加入前的 tag")
	}
	select {
	case b := <-c.ch:
		if string(b) != string(after) {
			t.Fatalf("队列里第一个应是加入后的 tag: % x", b)
		}
	default:
		t.Fatal("加入后的 tag 应进队列")
	}
	select {
	case b := <-c.ch:
		t.Fatalf("队列里不应有重复: % x", b)
	default:
	}
}
