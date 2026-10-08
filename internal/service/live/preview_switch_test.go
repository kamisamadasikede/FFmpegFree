//go:build !windows

package live

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/store"
)

// switchClient 模拟前端的一个播放器：连上 HTTP-FLV 后一直读；abandon 之后不再读、也不关 socket
// （页面切走时 WebView 挂起、连接没有正常关闭的最坏情况）。
type switchClient struct {
	conn   net.Conn
	start  time.Time
	status int
	err    error

	mu     sync.Mutex
	tags   []arrivedTag
	header bool
	stop   chan struct{}
	ready  chan struct{} // 收到响应头（或失败）
}

func dialSwitchClient(rawURL string) *switchClient {
	c := &switchClient{start: time.Now(), stop: make(chan struct{}), ready: make(chan struct{})}
	u, _ := url.Parse(rawURL)
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		c.err = err
		close(c.ready)
		return c
	}
	c.conn = conn
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nOrigin: wails://wails\r\n\r\n", u.RequestURI(), u.Host)
	go c.read()
	return c
}

func (c *switchClient) read() {
	br := bufio.NewReader(c.conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		c.err = err
		close(c.ready)
		return
	}
	c.status = resp.StatusCode
	close(c.ready)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return
	}
	body := resp.Body
	hdr := make([]byte, 13)
	if _, err := io.ReadFull(body, hdr); err != nil {
		c.err = err
		return
	}
	c.mu.Lock()
	c.header = string(hdr[:3]) == "FLV"
	c.mu.Unlock()
	th := make([]byte, 11)
	for {
		select {
		case <-c.stop:
			select {} // 不再读，socket 保持打开（测试结束时由 conn.Close 结束）
		default:
		}
		if _, err := io.ReadFull(body, th); err != nil {
			c.mu.Lock()
			c.err = err
			c.mu.Unlock()
			return
		}
		size := int(th[1])<<16 | int(th[2])<<8 | int(th[3])
		data := make([]byte, size+4)
		if _, err := io.ReadFull(body, data); err != nil {
			c.mu.Lock()
			c.err = err
			c.mu.Unlock()
			return
		}
		tg := arrivedTag{kind: th[0], ts: int64(th[4])<<16 | int64(th[5])<<8 | int64(th[6]) | int64(th[7])<<24, size: size, at: time.Now()}
		if th[0] == 9 && size >= 2 {
			tg.seq = data[1] == 0 && data[0]&0x0f == 7
			tg.keyframe = data[0]>>4 == 1 && !tg.seq
		}
		if th[0] == 8 && size >= 2 && data[0]>>4 == 10 {
			tg.seq = data[1] == 0
		}
		c.mu.Lock()
		c.tags = append(c.tags, tg)
		c.mu.Unlock()
	}
}

func (c *switchClient) abandon() {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
}

// result：首个视频帧是否是关键帧（之前有 FLV 头和 AVC 序列头）、到首个关键帧的时间、到“连续出帧”的时间
// （从这一帧起之后 window 内视频帧的到达间隔都不超过 300 ms）、窗口内视频帧数。
func (c *switchClient) result(window time.Duration) (startOK bool, firstKey, continuous time.Duration, frames int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var v []arrivedTag
	avcSeq := false
	for _, tg := range c.tags {
		if tg.kind == 9 && tg.seq {
			avcSeq = true
		}
		if tg.kind == 9 && !tg.seq {
			v = append(v, tg)
		}
	}
	if len(v) == 0 {
		return false, -1, -1, 0
	}
	startOK = c.header && avcSeq && v[0].keyframe
	firstKey = v[0].at.Sub(c.start)
	continuous = -1
	for i := range v {
		end := v[i].at.Add(window)
		if v[len(v)-1].at.Before(end) {
			break // 后面不够一个窗口
		}
		ok := true
		for j := i + 1; j < len(v) && !v[j-1].at.After(end); j++ {
			if v[j].at.Sub(v[j-1].at) > 300*time.Millisecond {
				ok = false
				break
			}
		}
		if ok {
			continuous = v[i].at.Sub(c.start)
			break
		}
	}
	return startOK, firstKey, continuous, len(v)
}

func hubClients(f *previewFeed) int {
	f.hub.mu.Lock()
	defer f.hub.mu.Unlock()
	return len(f.hub.clients)
}

// 实测（契约 v0.25.2）：推流 + 拉流同一路流，模拟 10 次切页面。每次切换把两路当前的播放器丢下
// （不再读、socket 不关），再各开一个新的。每次回来都要：200（不是 429）、从 FLV 头 + 序列头 + 关键帧开始、
// 3 秒内连续出帧；推流帧率不受影响；分发器里的名额不泄漏。
func TestMeasurePreviewPageSwitches(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	const rate, switches, dwell = 30, 10, 4 * time.Second
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	src := genSource(t, r.ffmpeg, rate, 20)
	r.svc.cfg.Media = fakeMedia{info: store.MediaInfo{HasVideo: true, HasAudio: true, Fps: rate}}
	tk, err := r.svc.StartFilePush(context.Background(), FilePushRequest{InputPath: src, URL: m.rtmpURL("live/sw"), Loop: true})
	if err != nil {
		t.Fatal(err)
	}
	r.waitProgress(t, tk.ID)
	pushPS, err := r.svc.GetPreviewStream(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	pull, err := r.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: m.rtmpURL("live/sw")})
	if err != nil || pull.PreviewURL == "" {
		t.Fatalf("%+v %v", pull, err)
	}
	// 推流帧率：整段时间从 MediaMTX 直接读推上去的流。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if r.probeStream(t, m.rtmpURL("live/sw")) == 0 {
		t.Fatal("服务器上没有推上去的流")
	}
	direct, err := directReader(ctx, r.ffmpeg, m.rtmpURL("live/sw"))
	if err != nil {
		t.Fatal(err)
	}
	r.svc.mu.Lock()
	pushFeed := r.svc.sessions[tk.ID].feed
	pullFeed := r.svc.pulls[pull.ID].feed
	r.svc.mu.Unlock()
	// 等拉流出画面
	deadline := time.Now().Add(20 * time.Second)
	for !pullFeed.hub.hasHeader() && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	var all []*switchClient
	defer func() {
		for _, c := range all {
			if c.conn != nil {
				c.conn.Close()
			}
		}
	}()
	type row struct {
		pushStatus, pullStatus           int
		pushOK, pullOK                   bool
		pushKey, pushCont, pullKey, pull time.Duration
		pushSlots, pullSlots             int
		start, end                       time.Time
	}
	var rows []row
	var cur [2]*switchClient
	fails := 0
	for i := 0; i <= switches; i++ { // 第 0 次是第一次进入页面，之后 10 次是切回来
		for _, c := range cur {
			if c != nil {
				c.abandon()
			}
		}
		// 每次回来：GetPreviewStream 再取一次推流地址（拉流沿用 previewUrl），地址不变。
		ps, err := r.svc.GetPreviewStream(tk.ID)
		if err != nil || ps.URL != pushPS.URL {
			t.Fatalf("第 %d 次 GetPreviewStream: %+v %v", i, ps, err)
		}
		cur[0], cur[1] = dialSwitchClient(ps.URL), dialSwitchClient(pull.PreviewURL)
		all = append(all, cur[0], cur[1])
		start := time.Now()
		time.Sleep(dwell)
		var rw row
		rw.start, rw.end = start, time.Now()
		for k, c := range cur {
			<-c.ready
			ok, key, cont, _ := c.result(time.Second)
			if k == 0 {
				rw.pushStatus, rw.pushOK, rw.pushKey, rw.pushCont = c.status, ok, key, cont
			} else {
				rw.pullStatus, rw.pullOK, rw.pullKey, rw.pull = c.status, ok, key, cont
			}
		}
		rw.pushSlots, rw.pullSlots = hubClients(pushFeed), hubClients(pullFeed)
		rows = append(rows, rw)
	}
	cancel()
	dtags := direct()
	t.Logf("直接读推流: %d 个 tag", len(dtags))
	minFPS := 1e9
	t.Logf("MEASURE | 次 | 推流预览 HTTP | 首个关键帧 | 连续出帧 | 拉流预览 HTTP | 首个关键帧 | 连续出帧 | 推流帧率 | 名额（推/拉） |")
	for i, rw := range rows {
		n := 0
		var lo, hi int64 = -1, -1
		for _, tg := range dtags {
			if tg.kind == 9 && !tg.seq && !tg.at.Before(rw.start) && tg.at.Before(rw.end) {
				n++
				if lo < 0 || tg.ts < lo {
					lo = tg.ts
				}
				hi = max(hi, tg.ts)
			}
		}
		fps := 0.0
		if hi > lo {
			fps = float64(n-1) / (float64(hi-lo) / 1000)
		}
		minFPS = min(minFPS, fps)
		label := fmt.Sprint(i)
		if i == 0 {
			label = "进入"
		}
		t.Logf("MEASURE | %s | %d | %v | %v | %d | %v | %v | %.2f | %d / %d |", label, rw.pushStatus, rw.pushKey.Round(time.Millisecond), rw.pushCont.Round(time.Millisecond),
			rw.pullStatus, rw.pullKey.Round(time.Millisecond), rw.pull.Round(time.Millisecond), fps, rw.pushSlots, rw.pullSlots)
		bad := []string{}
		if rw.pushStatus != 200 || rw.pullStatus != 200 {
			bad = append(bad, "HTTP 不是 200")
		}
		if !rw.pushOK || !rw.pullOK {
			bad = append(bad, "没有从头 + 序列头 + 关键帧开始")
		}
		if rw.pushCont < 0 || rw.pushCont > 3*time.Second || rw.pull < 0 || rw.pull > 3*time.Second {
			bad = append(bad, "3 秒内没有连续出帧")
		}
		if rw.pushSlots > previewMaxClients || rw.pullSlots > previewMaxClients {
			bad = append(bad, "名额超限")
		}
		if len(bad) > 0 {
			fails++
			t.Errorf("第 %s 次: %s", label, strings.Join(bad, "、"))
		}
	}
	// 丢下的播放器最终都离开分发器（系统缓冲吞满、写卡住 previewWriteStall 后断开）：只剩当前的一个。
	lastSwitch := rows[len(rows)-1].start
	var ps, pl int
	for {
		ps, pl = hubClients(pushFeed), hubClients(pullFeed)
		if ps <= 1 && pl <= 1 || time.Since(lastSwitch) > 30*time.Second {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("MEASURE 最低推流帧率 %.2f fps；失败 %d 次；最后一次切换后 %v 名额回到（推/拉）%d / %d", minFPS, fails, time.Since(lastSwitch).Round(100*time.Millisecond), ps, pl)
	if minFPS < rate-1.5 {
		t.Errorf("推流帧率下降: %.2f", minFPS)
	}
	if ps > 1 || pl > 1 {
		t.Errorf("名额泄漏：推 %d、拉 %d（应只剩当前的播放器）", ps, pl)
	}
}
