//go:build !windows

package live

// 实测（契约 6.10.3.9）：真实 ffmpeg + MediaMTX。量推流输出、预览流、拉流预览的帧率，预览延迟，
// 卡住的观看者不影响推流，后加入的客户端从关键帧开始，结束和中断时 HTTP 正常收尾，编码不支持时报 codec。
// 数字用 t.Logf 打出来（go test -v -run Measure）。

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

type arrivedTag struct {
	kind     byte
	ts       int64 // 毫秒
	keyframe bool
	seq      bool
	at       time.Time
}

// readPreview 读 HTTP-FLV，记录每个 tag 的到达时间，直到 EOF 或 ctx 结束。返回 tag、读完的原因。
func readPreview(ctx context.Context, rawURL, origin string) ([]arrivedTag, int, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	req.Header.Set("Origin", origin)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	br := bufio.NewReaderSize(resp.Body, 1<<20)
	hdr := make([]byte, 13)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return nil, resp.StatusCode, err
	}
	var tags []arrivedTag
	h := make([]byte, 11)
	for {
		if _, err := io.ReadFull(br, h); err != nil {
			return tags, resp.StatusCode, err
		}
		size := int(h[1])<<16 | int(h[2])<<8 | int(h[3])
		body := make([]byte, size+4)
		if _, err := io.ReadFull(br, body); err != nil {
			return tags, resp.StatusCode, err
		}
		ts := int64(h[4])<<16 | int64(h[5])<<8 | int64(h[6]) | int64(h[7])<<24
		at := arrivedTag{kind: h[0], ts: ts, at: time.Now()}
		if h[0] == 9 && size >= 2 {
			at.seq = body[1] == 0 && body[0]&0x0f == 7
			at.keyframe = body[0]>>4 == 1 && !at.seq
		}
		tags = append(tags, at)
	}
}

func videoFPS(tags []arrivedTag, from time.Time) (n int, fps float64) {
	var lo, hi int64 = -1, -1
	for _, t := range tags {
		if t.kind != 9 || t.seq || t.at.Before(from) {
			continue
		}
		if lo < 0 || t.ts < lo {
			lo = t.ts
		}
		if t.ts > hi {
			hi = t.ts
		}
		n++
	}
	if n < 2 || hi <= lo {
		return n, 0
	}
	return n, float64(n-1) * 1000 / float64(hi-lo)
}

// latency 估计端到端延迟：推流用 -re，源里时间戳 p 的帧在 t0+p 时被读入；到达预览客户端的时间减去它就是延迟。
// t0 取 StartFilePush 返回的时刻（比 ffmpeg 真正开始早，所以是偏大的估计）。只看客户端连上 1 秒以后的帧（避开 GOP 缓存一次性补发）。
func latency(tags []arrivedTag, t0, from time.Time, first int64) (median, p95 time.Duration) {
	var d []time.Duration
	for _, t := range tags {
		if t.kind != 9 || t.seq || t.at.Before(from) {
			continue
		}
		d = append(d, t.at.Sub(t0.Add(time.Duration(t.ts-first)*time.Millisecond)))
	}
	if len(d) == 0 {
		return 0, 0
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[len(d)/2], d[len(d)*95/100]
}

// pushFPSFromServer 从 MediaMTX 拉 N 秒，按时间戳数帧。
func pushFPSFromServer(t *testing.T, ff, ffp, u string, secs int) (int, float64) {
	t.Helper()
	got := filepath.Join(t.TempDir(), "got.flv")
	if b, err := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-y", "-i", u, "-t", strconv.Itoa(secs), "-c", "copy", "-f", "flv", got).CombinedOutput(); err != nil {
		t.Fatalf("从服务器拉流失败: %v %s", err, b)
	}
	return countFPS(t, ffp, got)
}

func countFPS(t *testing.T, ffprobe, path string) (int, float64) {
	t.Helper()
	out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	var n int
	var lo, hi float64
	for _, l := range strings.Fields(string(out)) {
		v, err := strconv.ParseFloat(strings.TrimSuffix(l, ","), 64)
		if err != nil {
			continue
		}
		if n == 0 || v < lo {
			lo = v
		}
		if n == 0 || v > hi {
			hi = v
		}
		n++
	}
	if n < 2 || hi <= lo {
		return n, 0
	}
	return n, float64(n-1) / (hi - lo)
}

// stallClient 发一个预览请求但永远不读。
func stallClient(t *testing.T, rawURL string) net.Conn {
	t.Helper()
	u, _ := url.Parse(rawURL)
	c, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	if tc, ok := c.(*net.TCPConn); ok {
		tc.SetReadBuffer(4096)
	}
	fmt.Fprintf(c, "GET %s HTTP/1.1\r\nHost: %s\r\nOrigin: http://wails.localhost\r\n\r\n", u.RequestURI(), u.Host)
	return c
}

func genSource(t *testing.T, ff string, rate, secs int) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), fmt.Sprintf("src%d.mp4", rate))
	gen := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=640x360:rate=%d", rate),
		"-f", "lavfi", "-i", "sine=f=440:r=44100", "-t", strconv.Itoa(secs), "-c:v", "libx264", "-preset", "ultrafast",
		"-g", strconv.Itoa(rate*2), "-c:a", "aac", "-shortest", src)
	if b, err := gen.CombinedOutput(); err != nil {
		t.Skipf("生成源失败: %v %s", err, b)
	}
	return src
}

func TestMeasurePushPreview(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	for _, rate := range []int{30, 60} {
		t.Run(fmt.Sprintf("%dfps", rate), func(t *testing.T) {
			m := startMediaMTX(t)
			r := newRealFixture(t, 0)
			src := genSource(t, r.ffmpeg, rate, 20)
			r.svc.cfg.Media = fakeMedia{info: store.MediaInfo{HasVideo: true, HasAudio: true, Fps: float64(rate)}}
			path := fmt.Sprintf("live/m%d", rate)
			t0 := time.Now()
			tk, err := r.svc.StartFilePush(context.Background(), FilePushRequest{InputPath: src, URL: m.rtmpURL(path), Loop: true})
			if err != nil {
				t.Fatal(err)
			}
			r.waitProgress(t, tk.ID)
			ps, err := r.svc.GetPreviewStream(tk.ID)
			if err != nil {
				t.Fatal(err)
			}
			if ps.MIME != "video/x-flv" || !ps.HasVideo || !ps.HasAudio || !strings.HasPrefix(ps.URL, "http://127.0.0.1:") {
				t.Fatalf("PreviewStream: %+v", ps)
			}
			// 预览没人连：推流帧率。
			nOff, fpsOff := pushFPSFromServer(t, r.ffmpeg, r.ffprobe, m.rtmpURL(path), 4)
			// 一个卡住的观看者 + 一个正常观看者（后加入）。
			stalled := stallClient(t, ps.URL)
			defer stalled.Close()
			ctx, cancel := context.WithCancel(context.Background())
			var tags []arrivedTag
			var readErr error
			joined := time.Now()
			var wg sync.WaitGroup
			wg.Add(1)
			go func() { defer wg.Done(); tags, _, readErr = readPreview(ctx, ps.URL, "wails://wails") }()
			nOn, fpsOn := pushFPSFromServer(t, r.ffmpeg, r.ffprobe, m.rtmpURL(path), 6)

			// 正常结束：取消任务，预览 HTTP 应在几秒内正常收尾（EOF，不是被我们取消）。
			stopAt := time.Now()
			r.mgr.Cancel(tk.ID)
			done := make(chan struct{})
			go func() { wg.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				cancel()
				t.Fatal("任务结束后预览响应没有收尾")
			}
			cancel()
			closeAfter := time.Since(stopAt)
			if readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
				t.Fatalf("预览应以 EOF 结束，实际 %v", readErr)
			}
			d := r.wait(t, tk.ID)
			if d.Status != task.StatusSucceeded {
				t.Fatalf("停止后应 succeeded: %+v", d)
			}
			// 后加入的客户端：第一个视频帧（非序列头）必须是关键帧。
			firstVideo := -1
			for i, tg := range tags {
				if tg.kind == 9 && !tg.seq {
					firstVideo = i
					break
				}
			}
			if firstVideo < 0 || !tags[firstVideo].keyframe {
				t.Fatalf("后加入的客户端没有从关键帧开始")
			}
			nPrev, fpsPrev := videoFPS(tags, joined.Add(time.Second))
			med, p95 := latency(tags, t0, joined.Add(time.Second), 0)
			t.Logf("MEASURE push %dfps: 预览没人看 %d 帧 %.2f fps；有人看（另有 1 个卡住的观看者）%d 帧 %.2f fps；FLV 客户端收到 %d 帧 %.2f fps；"+
				"延迟中位数 %v、p95 %v（偏大的估计）；后加入首帧是关键帧；停止后 %v 收尾",
				rate, nOff, fpsOff, nOn, fpsOn, nPrev, fpsPrev, med.Round(time.Millisecond), p95.Round(time.Millisecond), closeAfter.Round(10*time.Millisecond))
			min := float64(rate) - 1.5
			if fpsOff < min || fpsOn < min || fpsPrev < min {
				t.Fatalf("帧率不够")
			}
			if p95 > 1500*time.Millisecond {
				t.Fatalf("预览延迟 p95 %v 超过 1.5s", p95)
			}
		})
	}
}

// 推流被中断（服务器被杀）：任务 failed，预览 HTTP 正常收尾，之后 GetPreviewStream 是 NOT_FOUND。
func TestMeasurePushBreakClosesPreview(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	tk, err := r.push(t, r.withAudio, m.rtmpURL("live/brk"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	r.waitProgress(t, tk.ID)
	ps, err := r.svc.GetPreviewStream(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	var readErr error
	done := make(chan struct{})
	go func() {
		_, _, readErr = readPreview(context.Background(), ps.URL, "http://wails.localhost")
		close(done)
	}()
	time.Sleep(1500 * time.Millisecond)
	killed := time.Now()
	m.stop()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("中断后预览响应没有收尾")
	}
	d := r.wait(t, tk.ID)
	t.Logf("MEASURE break: 服务器被杀后 %v 预览收尾（%v），任务 %s %v", time.Since(killed).Round(10*time.Millisecond), readErr, d.Status, d.Error)
	if readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		t.Fatalf("应正常收尾: %v", readErr)
	}
	if d.Status != task.StatusFailed {
		t.Fatalf("中断应 failed: %+v", d)
	}
	if _, err := r.svc.GetPreviewStream(tk.ID); err == nil {
		t.Fatal("结束后应 NOT_FOUND")
	}
}

// 拉流预览：转封装后的帧率、延迟、live:pull 事件、远端结束。
func TestMeasurePullPreview(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	src := genSource(t, r.ffmpeg, 30, 20)
	pub := exec.Command(r.ffmpeg, "-hide_banner", "-loglevel", "error", "-re", "-stream_loop", "-1", "-i", src, "-c", "copy", "-f", "flv", m.rtmpURL("live/pull"))
	if err := pub.Start(); err != nil {
		t.Fatal(err)
	}
	defer pub.Process.Kill()
	time.Sleep(1500 * time.Millisecond)
	var mu sync.Mutex
	var events []PullEvent
	r.svc.cfg.Emit = func(name string, p any) {
		if name == "live:pull" {
			mu.Lock()
			events = append(events, p.(PullEvent))
			mu.Unlock()
		}
	}
	ps, err := r.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: m.rtmpURL("live/pull")})
	if err != nil || ps.PreviewURL == "" || !ps.Preview {
		t.Fatalf("StartPullPreview: %+v %v", ps, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	joined := time.Now()
	tags, code, _ := readPreview(ctx, ps.PreviewURL, "wails://wails")
	if code != http.StatusOK {
		t.Fatalf("拉流预览 HTTP %d", code)
	}
	n, fps := videoFPS(tags, joined.Add(2*time.Second))
	// 拉流延迟：相邻到达时间与时间戳的偏差（远端的 t0 不知道，只能看稳定后的抖动），以 p95 偏差衡量。
	var off []float64
	var base float64
	for i, tg := range tags {
		if tg.kind != 9 || tg.seq || tg.at.Before(joined.Add(2*time.Second)) {
			continue
		}
		v := float64(tg.at.UnixMilli()) - float64(tg.ts)
		if len(off) == 0 {
			base = v
		}
		off = append(off, v-base)
		_ = i
	}
	sort.Float64s(off)
	spread := 0.0
	if len(off) > 0 {
		spread = off[len(off)*95/100] - off[0]
	}
	// 远端停止：会话以 ended 或 interrupted 结束。
	pub.Process.Kill()
	pub.Wait()
	deadline := time.Now().Add(20 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		mu.Lock()
		if len(events) > 0 {
			last = events[len(events)-1].State
		}
		mu.Unlock()
		if last == "ended" || last == "interrupted" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	mu.Lock()
	t.Logf("MEASURE pull: 收到 %d 帧 %.2f fps，到达抖动 p95 %.0f ms；事件 %+v", n, fps, spread, events)
	mu.Unlock()
	if fps < 28.5 {
		t.Fatalf("拉流预览帧率不够: %.2f", fps)
	}
	if last != "ended" && last != "interrupted" {
		t.Fatalf("远端停止后应发 ended 或 interrupted，实际 %q", last)
	}
	if _, err := r.svc.GetPreviewStream(ps.ID); err == nil {
		t.Fatal("会话结束后应 NOT_FOUND")
	}
}

// 编码门：真实 HEVC 流的拉流预览报 unsupported（reason=codec），推流不受影响（这里没有推流）。
func TestMeasurePullCodecGate(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	enc, _ := exec.Command(r.ffmpeg, "-hide_banner", "-encoders").Output()
	if !strings.Contains(string(enc), "libx265") {
		t.Skip("没有 libx265")
	}
	src := filepath.Join(t.TempDir(), "hevc.mkv")
	if b, err := exec.Command(r.ffmpeg, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25",
		"-t", "20", "-c:v", "libx265", "-preset", "ultrafast", src).CombinedOutput(); err != nil {
		t.Skipf("生成 HEVC 失败: %v %s", err, b)
	}
	pub := exec.Command(r.ffmpeg, "-hide_banner", "-loglevel", "error", "-re", "-stream_loop", "-1", "-i", src, "-c", "copy", "-f", "mpegts",
		m.srtURL("streamid=publish:live/hevc"))
	if err := pub.Start(); err != nil {
		t.Fatal(err)
	}
	defer pub.Process.Kill()
	time.Sleep(1500 * time.Millisecond)
	ch := make(chan PullEvent, 4)
	r.svc.cfg.Emit = func(name string, p any) { ch <- p.(PullEvent) }
	if _, err := r.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: m.srtURL("streamid=read:live/hevc")}); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-ch:
		t.Logf("MEASURE codec gate: %s %v", ev.State, ev.Error)
		if ev.State != "unsupported" || ev.Error == nil || !strings.Contains(ev.Error.Detail, "reason=codec") {
			t.Fatalf("HEVC 应 unsupported reason=codec: %+v", ev)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("没有收到 live:pull")
	}
	_ = binary.BigEndian
}
