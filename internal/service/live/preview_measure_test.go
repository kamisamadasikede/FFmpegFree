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

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

type arrivedTag struct {
	kind     byte
	ts       int64 // 毫秒
	keyframe bool
	seq      bool
	size     int
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
	tags, err := parseFLVArrivals(resp.Body)
	return tags, resp.StatusCode, err
}

// parseFLVArrivals 读一路 FLV，记录每个 tag 的到达时间，直到出错或 EOF。
func parseFLVArrivals(r io.Reader) ([]arrivedTag, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	hdr := make([]byte, 13)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return nil, err
	}
	var tags []arrivedTag
	h := make([]byte, 11)
	for {
		if _, err := io.ReadFull(br, h); err != nil {
			return tags, err
		}
		size := int(h[1])<<16 | int(h[2])<<8 | int(h[3])
		body := make([]byte, size+4)
		if _, err := io.ReadFull(br, body); err != nil {
			return tags, err
		}
		ts := int64(h[4])<<16 | int64(h[5])<<8 | int64(h[6]) | int64(h[7])<<24
		at := arrivedTag{kind: h[0], ts: ts, size: size, at: time.Now()}
		if h[0] == 9 && size >= 2 {
			at.seq = body[1] == 0 && body[0]&0x0f == 7
			at.keyframe = body[0]>>4 == 1 && !at.seq
		}
		tags = append(tags, at)
	}
}

// directReader 用 ffmpeg 直接从服务器读同一路流（-c copy 成 FLV 到标准输出），记录每帧到达时间，作为对照。
func directReader(ctx context.Context, ff, u string) (func() []arrivedTag, error) {
	cmd := exec.CommandContext(ctx, ff, "-hide_banner", "-loglevel", "error", "-fflags", "+nobuffer", "-i", u, "-c", "copy", "-f", "flv", "pipe:1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var tags []arrivedTag
	done := make(chan struct{})
	go func() {
		defer close(done)
		var perr error
		tags, perr = parseFLVArrivals(out)
		cmd.Wait()
		if len(tags) == 0 {
			fmt.Printf("directReader: 没有读到数据: %v %s\n", perr, stderr.String())
		}
	}()
	return func() []arrivedTag { <-done; return tags }, nil
}

// relLatency：同一帧预览客户端比直接读服务器晚到多少（负数 = 预览更早）。
// 两边都经过 ffmpeg，输出时间戳各自从 0 起算，不能按时间戳配对；改按视频帧的字节数序列对齐（编码后每帧大小各不相同）。
func relLatency(preview, direct []arrivedTag, from time.Time) (n int, median, p95 time.Duration) {
	var p, d []arrivedTag
	for _, t := range preview {
		if t.kind == 9 && !t.seq && !t.at.Before(from) {
			p = append(p, t)
		}
	}
	for _, t := range direct {
		if t.kind == 9 && !t.seq {
			d = append(d, t)
		}
	}
	best, bestN := 0, 0
	for k := -len(p); k < len(d); k++ {
		c := 0
		for i := range p {
			if j := i + k; j >= 0 && j < len(d) && d[j].size == p[i].size {
				c++
			}
		}
		if c > bestN {
			best, bestN = k, c
		}
	}
	if bestN < len(p)/2 {
		return 0, 0, 0 // 对不上
	}
	var ds []time.Duration
	for i := range p {
		if j := i + best; j >= 0 && j < len(d) && d[j].size == p[i].size {
			ds = append(ds, p[i].at.Sub(d[j].at))
		}
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	return len(ds), ds[len(ds)/2], ds[len(ds)*95/100]
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

// latency 估计推流端到端延迟的上限：推流用 -re（ffmpeg 7.1 的 -readrate_initial_burst 默认 0.5 秒），
// 源里时间戳 p 的帧最早在 T+max(0,p-0.5s) 被读入（T 是 ffmpeg 开始读的时刻）。用 StartFilePush 返回的时刻 t0 代替 T（t0 < T），
// 到达预览客户端的时间减去 t0+max(0,p-0.5s) 就是延迟的上限。只看客户端连上 1 秒以后的帧（避开 GOP 缓存一次性补发）。
func latency(tags []arrivedTag, t0, from time.Time) (median, p95 time.Duration) {
	var d []time.Duration
	for _, t := range tags {
		if t.kind != 9 || t.seq || t.at.Before(from) {
			continue
		}
		p := time.Duration(t.ts)*time.Millisecond - 500*time.Millisecond
		if p < 0 {
			p = 0
		}
		d = append(d, t.at.Sub(t0.Add(p)))
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
	// 推流端的首个 progress 与服务器端登记流之间有竞态，重试几次（同 probeStream）。
	var err error
	var b []byte
	for i := 0; i < 8; i++ {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		if b, err = exec.Command(ff, "-hide_banner", "-loglevel", "error", "-y", "-i", u, "-t", strconv.Itoa(secs), "-c", "copy", "-f", "flv", got).CombinedOutput(); err == nil {
			break
		}
	}
	if err != nil {
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
				t.Fatalf("%v\n%s", err, tailLines(r.logText(t, tk.ID), 30))
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
			med, p95 := latency(tags, t0, joined.Add(time.Second))
			t.Logf("MEASURE push %dfps: 预览没人看 %d 帧 %.2f fps；有人看（另有 1 个卡住的观看者）%d 帧 %.2f fps；FLV 客户端收到 %d 帧 %.2f fps；"+
				"端到端延迟上限 中位数 %v、p95 %v；后加入首帧是关键帧；停止后 %v 收尾",
				rate, nOff, fpsOff, nOn, fpsOn, nPrev, fpsPrev, med.Round(time.Millisecond), p95.Round(time.Millisecond),
				closeAfter.Round(10*time.Millisecond))
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
	if d.Status != task.StatusInterrupted || d.Error == nil || d.Error.Code != apperr.LivePushInterrupted { // v0.25.3：开始后中断记为 interrupted
		t.Fatalf("中断应 interrupted: %+v", d)
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
	var logs strings.Builder
	var logMu sync.Mutex
	r.svc.cfg.Logf = func(f string, a ...any) { logMu.Lock(); fmt.Fprintf(&logs, f+"\n", a...); logMu.Unlock() }
	ps, err := r.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: m.rtmpURL("live/pull")})
	if err != nil || ps.PreviewURL == "" || !ps.Preview {
		t.Fatalf("StartPullPreview: %+v %v", ps, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	direct, err := directReader(ctx, r.ffmpeg, m.rtmpURL("live/pull"))
	if err != nil {
		t.Fatal(err)
	}
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
	// 拉流延迟：远端流本身的延迟由远端决定，这里量转封装 + 分发多出来的部分：
	// 同时用 ffmpeg 直接读同一个地址，同一帧（按帧大小序列对齐）预览客户端晚到多少。
	rn, rmed, rp95 := relLatency(tags, direct(), joined.Add(2*time.Second))
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
	t.Logf("MEASURE pull: 收到 %d 帧 %.2f fps，到达抖动 p95 %.0f ms；比直接读同一地址（%d 帧对上）晚 中位数 %v、p95 %v；事件 %+v", n, fps, spread,
		rn, rmed.Round(100*time.Microsecond), rp95.Round(100*time.Microsecond), events)
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
	// 日志脱敏：拉流地址的路径和预览 token 都不能出现在日志里。
	logMu.Lock()
	lt := logs.String()
	logMu.Unlock()
	token := strings.TrimSuffix(ps.PreviewURL[strings.LastIndex(ps.PreviewURL, "/")+1:], ".flv")
	if lt == "" || strings.Contains(lt, "live/pull") || strings.Contains(lt, token) {
		t.Fatalf("日志没有脱敏（或者没有日志）:\n%s", lt)
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
		if ev.State != "unsupported" || ev.Error == nil || ev.Error.Detail != "reason=codec\nvideo=hevc" {
			t.Fatalf("HEVC 应 unsupported reason=codec: %+v", ev)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("没有收到 live:pull")
	}
	_ = binary.BigEndian
}

// 拉流时服务器被杀：报告实际收到的结束状态（ended / interrupted），预览 HTTP 正常收尾。
func TestMeasurePullServerKilled(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	src := genSource(t, r.ffmpeg, 30, 20)
	pub := exec.Command(r.ffmpeg, "-hide_banner", "-loglevel", "error", "-re", "-stream_loop", "-1", "-i", src, "-c", "copy", "-f", "flv", m.rtmpURL("live/kill"))
	if err := pub.Start(); err != nil {
		t.Fatal(err)
	}
	defer pub.Process.Kill()
	time.Sleep(1500 * time.Millisecond)
	ch := make(chan PullEvent, 8)
	r.svc.cfg.Emit = func(name string, p any) { ch <- p.(PullEvent) }
	ps, err := r.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: m.rtmpURL("live/kill")})
	if err != nil {
		t.Fatal(err)
	}
	var readErr error
	done := make(chan struct{})
	go func() { _, _, readErr = readPreview(context.Background(), ps.PreviewURL, "wails://wails"); close(done) }()
	if ev := <-ch; ev.State != "playing" {
		t.Fatalf("第一个事件应是 playing: %+v", ev)
	}
	time.Sleep(time.Second)
	killed := time.Now()
	m.stop()
	var last PullEvent
	select {
	case last = <-ch:
	case <-time.After(20 * time.Second):
		t.Fatal("服务器被杀后没有 live:pull")
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("预览 HTTP 没有收尾")
	}
	t.Logf("MEASURE pull 服务器被杀: %v 后 live:pull %s %v，预览 HTTP 以 %v 收尾", time.Since(killed).Round(10*time.Millisecond), last.State, last.Error, readErr)
	if last.State != "ended" && last.State != "interrupted" {
		t.Fatalf("应 ended 或 interrupted: %+v", last)
	}
	if readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		t.Fatalf("应正常收尾: %v", readErr)
	}
}

// 设计走查 G3：拉流连不上时 live:pull 的 failed 带拉流自己的文字，不是“推流启动失败”。
func TestPullFailedEventUsesPullText(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	for _, u := range []string{m.rtmpURL("live/nobody"), fmt.Sprintf("rtmp://127.0.0.1:%d/live/x", freePort(t, false))} {
		ch := make(chan PullEvent, 4)
		r.svc.cfg.Emit = func(name string, p any) {
			if name == "live:pull" {
				ch <- p.(PullEvent)
			}
		}
		if _, err := r.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: u}); err != nil {
			t.Fatal(err)
		}
		select {
		case ev := <-ch:
			if ev.Error != nil {
				t.Logf("%s: %s %v", ev.State, ev.Error.Code, ev.Error.Message)
			}
			if ev.State != "failed" || ev.Error == nil || ev.Error.Message != ffmpeg.PullFailedMessage || ev.Error.Code != apperr.LiveConnectFailed {
				t.Fatalf("应是 failed + LIVE_CONNECT_FAILED + 拉流文字: %+v %+v", ev, ev.Error)
			}
			if strings.Contains(ev.Error.Detail, "live/nobody") || strings.Contains(ev.Error.Detail, "live/x") {
				t.Fatalf("detail 没有脱敏: %s", ev.Error.Detail)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("没有 live:pull")
		}
	}
}

// 回归（契约 v0.25.1）：HLS 拉流预览按原速输出。不限速时 FLV 每个分片时长（2 秒）一次性到 2 秒的数据，
// 播放器缓冲忽大忽小、追帧跳到 GOP 中间花屏；排期后匀速到达（最大到达间隔 < 300 毫秒、100 毫秒内不超过 15 帧；修之前约 2 秒、约 60 帧）。
func TestMeasurePullHLSIsPaced(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	for _, variant := range []string{"mpegts", "lowLatency", "fmp4"} {
		t.Run(variant, func(t *testing.T) {
			m := startMediaMTXHLS(t, variant)
			r := newRealFixture(t, 0)
			pub := exec.Command(r.ffmpeg, "-hide_banner", "-loglevel", "error", "-re", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30",
				"-f", "lavfi", "-i", "sine=f=440:r=44100", "-c:v", "libx264", "-preset", "veryfast", "-tune", "zerolatency", "-g", "60",
				"-pix_fmt", "yuv420p", "-c:a", "aac", "-f", "flv", m.rtmpURL("live/h"))
			if err := pub.Start(); err != nil {
				t.Fatal(err)
			}
			defer pub.Process.Kill()
			// 等 HLS 有足够的分片（lowLatency 至少 7 个分片才出播放列表）
			deadline := time.Now().Add(40 * time.Second)
			for {
				resp, err := http.Get(m.hlsURL("live/h"))
				if err == nil {
					ok := resp.StatusCode == 200
					resp.Body.Close()
					if ok {
						break
					}
				}
				if time.Now().After(deadline) {
					t.Skip("HLS 没有就绪")
				}
				time.Sleep(time.Second)
			}
			ps, err := r.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: m.hlsURL("live/h")})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			// 对照：同时直接读 RTMP（最新的画面）和直接读 HLS（ffmpeg 默认参数、不限速），看预览比它们晚多少。
			rtmpDirect, err := directReader(ctx, r.ffmpeg, m.rtmpURL("live/h"))
			if err != nil {
				t.Fatal(err)
			}
			hlsDirect, err := directReader(ctx, r.ffmpeg, m.hlsURL("live/h"))
			if err != nil {
				t.Fatal(err)
			}
			var tags []arrivedTag
			var code int
			for i := 0; i < 20; i++ { // 还没收到 FLV 头时是 503
				if tags, code, _ = readPreview(ctx, ps.PreviewURL, "wails://wails"); code != http.StatusServiceUnavailable {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
			if code != 200 {
				t.Fatalf("HTTP %d", code)
			}
			var v []arrivedTag
			for _, tg := range tags {
				if tg.kind == 9 && !tg.seq {
					v = append(v, tg)
				}
			}
			if len(v) < 300 {
				t.Fatalf("帧太少: %d", len(v))
			}
			// 跳过开头 5 秒（GOP 缓存补发、第一个分片），看到达时间和时间戳的偏差范围。
			start := v[0].at.Add(5 * time.Second)
			lo, hi := time.Duration(1<<62), time.Duration(-1<<62)
			back := 0
			for i, tg := range v {
				if i > 0 && tg.ts <= v[i-1].ts {
					back++
				}
				if tg.at.Before(start) {
					continue
				}
				d := time.Duration(tg.ts-v[0].ts)*time.Millisecond - tg.at.Sub(v[0].at)
				lo, hi = min(lo, d), max(hi, d)
			}
			// 一阵一阵的程度：相邻两帧的最大到达间隔、任意 100 毫秒内最多到几帧（修之前约 2 秒、约 60 帧）。
			maxGap, maxBurst := time.Duration(0), 0
			for i, tg := range v {
				if tg.at.Before(start) {
					continue
				}
				maxGap = max(maxGap, tg.at.Sub(v[i-1].at))
				k := i
				for k < len(v) && v[k].at.Sub(tg.at) < 100*time.Millisecond {
					k++
				}
				maxBurst = max(maxBurst, k-i)
			}
			n, fps := videoFPS(tags, start)
			cancel()
			_, lr, _ := relLatency(tags, rtmpDirect(), start)
			_, lh, _ := relLatency(tags, hlsDirect(), start)
			t.Logf("MEASURE HLS %s: %d 帧 %.2f fps，最大到达间隔 %v，100ms 内最多 %d 帧，到达节奏与时间戳的偏差范围 %v，时间戳倒退 %d 次；比直接读 RTMP 晚 %v（中位数），比 ffmpeg 直接读 HLS 晚 %v",
				variant, n, fps, maxGap.Round(time.Millisecond), maxBurst, (hi - lo).Round(time.Millisecond), back, lr.Round(time.Millisecond), lh.Round(time.Millisecond))
			if maxGap > 300*time.Millisecond || maxBurst > 15 || back > 0 {
				t.Fatalf("HLS 输出应匀速且时间戳不倒退：最大间隔 %v，100ms 内 %d 帧，倒退 %d", maxGap, maxBurst, back)
			}
		})
	}
}

// 回归（契约 v0.25.1，首次拉流停在“正在连接…”）：远端接受连接却一直不给数据时，ffmpeg 原来没有任何超时，
// 会话永远不出 playing / failed（实测旧参数 40 秒还没退出）。现在探测、转封装都有 -rw_timeout，另有 pullHeaderWait 兜底：
// 有限时间内以 failed + LIVE_CONNECT_FAILED + 拉流失败文字结束；期间先连上来的播放器请求也随之结束（503），不挂着。
func TestPullSilentServerFailsInBoundedTime(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var held []net.Conn
	var heldMu sync.Mutex
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			heldMu.Lock()
			held = append(held, c) // 接受连接，什么都不回
			heldMu.Unlock()
		}
	}()
	defer func() {
		heldMu.Lock()
		for _, c := range held {
			c.Close()
		}
		heldMu.Unlock()
	}()
	old := pullHeaderWait
	pullHeaderWait = 4 * time.Second
	defer func() { pullHeaderWait = old }()
	r := newRealFixture(t, 0)
	port := ln.Addr().(*net.TCPAddr).Port
	for _, u := range []string{fmt.Sprintf("rtmp://127.0.0.1:%d/live/x", port), fmt.Sprintf("http://127.0.0.1:%d/live/x.flv", port)} {
		ch := make(chan PullEvent, 4)
		r.svc.cfg.Emit = func(name string, p any) {
			if name == "live:pull" {
				ch <- p.(PullEvent)
			}
		}
		start := time.Now()
		ps, err := r.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: u})
		if err != nil || ps.PreviewURL == "" {
			t.Fatalf("%+v %v", ps, err)
		}
		// 播放器在 FLV 头之前就连上来：请求挂着等，会话失败时立即 503，不挂满 previewHeaderWait。
		httpDone := make(chan int, 1)
		go func() {
			_, code, _ := readPreview(context.Background(), ps.PreviewURL, "wails://wails")
			httpDone <- code
		}()
		var ev PullEvent
		select {
		case ev = <-ch:
		case <-time.After(pullProbeWait + 20*time.Second):
			t.Fatalf("%s: %v 内没有 live:pull（一直停在“正在连接…”）", u, pullProbeWait+20*time.Second)
		}
		took := time.Since(start)
		t.Logf("MEASURE 远端不给数据 %s: %v 后 %s（%v）", u, took.Round(100*time.Millisecond), ev.State, ev.Error)
		if ev.State != "failed" || ev.Error == nil || ev.Error.Code != apperr.LiveConnectFailed || ev.Error.Message != ffmpeg.PullFailedMessage {
			t.Fatalf("应是 failed + LIVE_CONNECT_FAILED + 拉流文字: %+v %+v", ev, ev.Error)
		}
		select {
		case code := <-httpDone:
			if code != http.StatusServiceUnavailable && code != http.StatusNotFound {
				t.Fatalf("等头的播放器请求应以 503 / 404 结束: %d", code)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("会话失败后，等头的播放器请求还挂着")
		}
	}
}
