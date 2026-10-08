package live

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	f.hub.leave(c)

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
	var ev PullEvent
	f.svc.cfg.Emit = func(name string, payload any) {
		if name != "live:pull" {
			t.Errorf("事件名 %s", name)
		}
		ev = payload.(PullEvent)
	}
	ps, err := f.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: "rtmp://127.0.0.1:1935/live/k"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for ev.State == "" && time.Now().Before(deadline) {
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
