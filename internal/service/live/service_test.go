//go:build !windows

package live

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

type evRec struct {
	mu   sync.Mutex
	evts []struct {
		name    string
		payload any
	}
}

func (r *evRec) Emit(name string, p any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evts = append(r.evts, struct {
		name    string
		payload any
	}{name, p})
}

// jsonAll 把全部事件 payload 序列化，用来搜秘密片段。
func (r *evRec) allStatusEvents() []task.StatusEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []task.StatusEvent
	for _, e := range r.evts {
		if se, ok := e.payload.(task.StatusEvent); ok {
			out = append(out, se)
		}
	}
	return out
}

func (r *evRec) jsonAll() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var b strings.Builder
	for _, e := range r.evts {
		j, _ := json.Marshal(e.payload)
		b.WriteString(e.name + " " + string(j) + "\n")
	}
	return b.String()
}

func (r *evRec) statusEvents(id string) []task.StatusEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []task.StatusEvent
	for _, e := range r.evts {
		if s, ok := e.payload.(task.StatusEvent); ok && s.ID == id {
			out = append(out, s)
		}
	}
	return out
}

type fakeMedia struct {
	info store.MediaInfo
	err  error
}

func (f fakeMedia) Inspect(_ context.Context, p string) (store.MediaInfo, error) {
	i := f.info
	i.Path = p
	return i, f.err
}

type fixture struct {
	svc  *Service
	mgr  *task.Manager
	em   *evRec
	dir  string
	exe  string
	mode string // 假 ffmpeg 的模式文件
}

const secretURL = "rtmp://user:pw123456@127.0.0.1:1935/live/SECRETKEYabc?token=TOKENxyz"

var secretFragments = []string{"pw123456", "SECRETKEYabc", "TOKENxyz", "user:pw"}

// 假 ffmpeg：模式写在 <exe>.mode 里，收到的参数写在 <exe>.args 里。
func fakeFFmpeg(t *testing.T, dir string) string {
	p := filepath.Join(dir, "ffmpeg")
	script := `#!/bin/sh
mode=$(cat "$0.mode")
printf '%s\n' "$@" > "$0.args"
for a in "$@"; do last="$a"; done
prog() { echo "fps=25.00"; echo "drop_frames=0"; echo "total_size=$1"; echo "out_time_us=$2"; echo "speed=1.00x"; echo "progress=continue"; }
case "$mode" in
  live)        # 已开始推流，等 q，优雅退出 0
    prog 5000 1000000
    read -r x
    echo "trailer written" >&2
    exit 0 ;;
  live2)       # 两条 progress（有 total_size 数值），等 q
    prog 5000 1000000
    prog 15000 2000000
    read -r x
    exit 0 ;;
  live_slow_q) # 已开始，收到 q 后 0.5 秒才退
    prog 5000 1000000
    read -r x
    sleep 0.5
    exit 0 ;;
  q_then_fail) # 已开始，收到 q 之后带着 Broken pipe 以 224 退出
    prog 5000 1000000
    read -r x
    echo "[out#0/flv @ 0x1] Error writing trailer: Broken pipe" >&2
    exit 224 ;;
  refused)     # 连接被拒，把完整 URL 打到 stderr
    echo "Input #0, mov,mp4, from '/x/a.mp4':" >&2
    echo "[tcp @ 0x1] Connection to tcp://127.0.0.1:1935?tcp_nodelay=0 failed: Connection refused" >&2
    echo "[out#0/flv @ 0x2] Error opening output $last: Connection refused" >&2
    echo "Error opening output file $last." >&2
    echo "frame=0 total_size=0 out_time_us=N/A" >&2
    echo "total_size=0"; echo "out_time_us=N/A"; echo "progress=end"
    exit 1 ;;
  rejected)
    echo "[rtmp @ 0x1] Server error: authentication failed" >&2
    echo "[out#0/flv @ 0x2] Error opening output $last: Operation not permitted" >&2
    exit 1 ;;
  broken)      # 已开始，0.3 秒后被服务器断开
    prog 5000 1000000
    sleep 0.3
    echo "[out#0/flv @ 0x1] Error muxing a packet" >&2
    echo "[out#0/flv @ 0x1] Error writing trailer: Broken pipe $last" >&2
    exit 224 ;;
  connecting)  # 卡在连接里：没有 progress，不读 stdin
    trap '' INT
    sleep 30 ;;
  ignoreq)     # 已开始，但不响应 q
    prog 5000 1000000
    exec sleep 30 ;;
  natural)     # 播完自然结束
    prog 5000 1000000
    prog 9000 2000000
    echo "total_size=9000"; echo "out_time_us=2000000"; echo "progress=end"
    exit 0 ;;
  unknown)
    echo "some unexpected failure $last" >&2
    exit 2 ;;
esac
`
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func newFixture(t *testing.T, mut func(*Config)) *fixture {
	t.Helper()
	dir := t.TempDir()
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		dir = r
	}
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	em := &evRec{}
	mgr := task.NewManager(task.Config{Store: st, Emitter: em, LogDir: filepath.Join(dir, "logs"), ProgressInterval: -1, Logf: func(string, ...any) {}})
	t.Cleanup(func() { mgr.Shutdown(3 * time.Second) })
	exe := fakeFFmpeg(t, dir)
	f := &fixture{mgr: mgr, em: em, dir: dir, exe: exe, mode: exe + ".mode"}
	f.setMode("live")
	cfg := Config{
		Tasks:   mgr,
		Media:   fakeMedia{info: store.MediaInfo{HasVideo: true, HasAudio: true, Fps: 25}},
		Require: func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{FFmpeg: exe}, nil },
		Protocols: &ffmpeg.ProtocolProbe{Run: func(context.Context, string, ...string) (string, error) {
			return "Supported file protocols:\nInput:\n  file\nOutput:\n  file\n  rtmp\n  rtmps\n  srt\n  tee\n", nil
		}},
		Grace: 1500 * time.Millisecond,
	}
	if mut != nil {
		mut(&cfg)
	}
	f.svc = New(cfg)
	return f
}

func mediaInfo(hasAudio bool) store.MediaInfo {
	return store.MediaInfo{HasVideo: true, HasAudio: hasAudio, Fps: 25}
}

func (f *fixture) setMode(m string) { os.WriteFile(f.mode, []byte(m), 0o644) }

func (f *fixture) start(t *testing.T, url string) (task.Task, error) {
	t.Helper()
	return f.svc.StartFilePush(context.Background(), FilePushRequest{InputPath: filepath.Join(f.dir, "a.mp4"), URL: url})
}

func (f *fixture) wait(t *testing.T, id string) task.Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	tk, err := f.mgr.Wait(ctx, id)
	if err != nil {
		t.Fatalf("等待任务: %v", err)
	}
	return tk
}

func (f *fixture) waitProgress(t *testing.T, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tk, _ := f.mgr.Get(id)
		if tk.Status == task.StatusRunning && f.progressed(id) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("没等到第一条 progress")
}

func (f *fixture) progressed(id string) bool {
	f.em.mu.Lock()
	defer f.em.mu.Unlock()
	for _, e := range f.em.evts {
		if p, ok := e.payload.(task.ProgressEvent); ok && p.ID == id {
			return true
		}
	}
	return false
}

func (f *fixture) logText(t *testing.T, id string) string {
	b, _ := os.ReadFile(filepath.Join(f.dir, "logs", id+".log"))
	return string(b)
}

func firstLine(s string) string { l, _, _ := strings.Cut(s, "\n"); return l }

func mustAppErr(t *testing.T, err error, code apperr.Code) *apperr.AppError {
	t.Helper()
	ae := apperr.From(err)
	if err == nil || ae.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
	return ae
}

// ---------- 地址与协议 ----------

func TestCheckPushURL(t *testing.T) {
	f := newFixture(t, nil)
	info, err := f.svc.CheckPushURL(secretURL)
	if err != nil || info.Scheme != "rtmp" || info.Host != "127.0.0.1" || info.Port != 1935 || info.Redacted != "rtmp://***@127.0.0.1:1935/live/***?token=***" {
		t.Fatalf("%+v %v", info, err)
	}
	tests := []struct{ url, reason string }{
		{"", "malformed"}, {"http://h/a/k", "scheme_unsupported"}, {"rtmp:///a/k", "missing_host"},
		{"srt://h:9000?foo=1", "param_not_allowed"}, {"srt://h:9000?passphrase=short", "malformed"},
		{"srt://h:9000?mode=listener", "param_not_allowed"}, {"rtmp://h:99999/a/k", "malformed"},
	}
	for _, tc := range tests {
		_, err := f.svc.CheckPushURL(tc.url)
		ae := mustAppErr(t, err, apperr.LiveURLInvalid)
		if firstLine(ae.Detail) != "reason="+tc.reason {
			t.Errorf("%q: detail 首行=%q want reason=%s", tc.url, firstLine(ae.Detail), tc.reason)
		}
	}
	// 带秘密的非法地址不回显。
	_, err = f.svc.CheckPushURL("http://user:pw123456@evil.example.com/live/SECRETKEYabc?token=TOKENxyz")
	ae := mustAppErr(t, err, apperr.LiveURLInvalid)
	for _, s := range append(secretFragments, "evil.example.com") {
		if strings.Contains(ae.Message, s) || strings.Contains(ae.Detail, s) {
			t.Fatalf("LIVE_URL_INVALID 回显了 %q: %+v", s, ae)
		}
	}
}

func TestStartRejectsBadInputsWithoutCreatingTask(t *testing.T) {
	f := newFixture(t, nil)
	_, err := f.start(t, "file:///etc/passwd")
	mustAppErr(t, err, apperr.LiveURLInvalid)
	_, err = f.svc.StartFilePush(context.Background(), FilePushRequest{InputPath: "rel.mp4", URL: "rtmp://h/a/k"})
	mustAppErr(t, err, apperr.InvalidArgument)
	_, err = f.svc.StartFilePush(context.Background(), FilePushRequest{InputPath: "/a.mp4", URL: "rtmp://h/a/k", Options: PushOptions{Fps: 61}})
	mustAppErr(t, err, apperr.InvalidArgument)
	_, err = f.svc.StartFilePush(context.Background(), FilePushRequest{InputPath: "/a.mp4", URL: "rtmp://h/a/k", Options: PushOptions{VideoBitrateKbps: 99}})
	mustAppErr(t, err, apperr.InvalidArgument)
	f2 := newFixture(t, func(c *Config) { c.Media = fakeMedia{info: store.MediaInfo{HasVideo: false, HasAudio: true}} })
	_, err = f2.start(t, "rtmp://h/a/k")
	mustAppErr(t, err, apperr.InvalidArgument)
	f3 := newFixture(t, func(c *Config) { c.Media = fakeMedia{err: apperr.New(apperr.NotFound, "x")} })
	_, err = f3.start(t, "rtmp://h/a/k")
	mustAppErr(t, err, apperr.NotFound)
	f4 := newFixture(t, func(c *Config) {
		c.Require = func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{}, apperr.New(apperr.FFmpegNotFound, "x") }
	})
	_, err = f4.start(t, "rtmp://h/a/k")
	mustAppErr(t, err, apperr.FFmpegNotFound)
	if n, _ := f.svc.ActiveSessions(); n != 0 || len(f.mgr.ListActive()) != 0 {
		t.Fatal("同步失败不应留下会话或任务")
	}
}

func TestMissingProtocolIsUnsupported(t *testing.T) {
	f := newFixture(t, func(c *Config) {
		c.Protocols = &ffmpeg.ProtocolProbe{Run: func(context.Context, string, ...string) (string, error) {
			return "Output:\n  file\n  rtmp\n  tee\n", nil
		}}
	})
	_, err := f.start(t, "srt://h:9000")
	if ae := mustAppErr(t, err, apperr.Unsupported); ae.Detail != "missing=srt" {
		t.Fatalf("%+v", ae)
	}
	_, err = f.start(t, "rtmps://h/a/k")
	if ae := mustAppErr(t, err, apperr.Unsupported); ae.Detail != "missing=rtmps" {
		t.Fatalf("%+v", ae)
	}
	f.setMode("live")
	tk, err := f.start(t, "rtmp://h/a/k") // rtmp 有
	if err != nil {
		t.Fatal(err)
	}
	f.mgr.Cancel(tk.ID)
	f.wait(t, tk.ID)
}

func TestSRTURLReassembledLowercaseKeys(t *testing.T) {
	f := newFixture(t, nil)
	f.setMode("live")
	tk, err := f.start(t, "srt://127.0.0.1:9000?PassPhrase=0123456789abc&StreamID=publish:live/xyz")
	if err != nil {
		t.Fatal(err)
	}
	f.waitProgress(t, tk.ID)
	args, _ := os.ReadFile(f.exe + ".args")
	a := string(args)
	if !strings.Contains(a, "srt://127.0.0.1:9000?passphrase=0123456789abc&streamid=publish:live/xyz") {
		t.Fatalf("传给 ffmpeg 的 srt URL 键必须是小写: %s", a)
	}
	if !strings.Contains(a, "srt,udp") || !strings.Contains(a, "mpegts") {
		t.Fatalf("%s", a)
	}
	f.mgr.Cancel(tk.ID)
	f.wait(t, tk.ID)
}

// ---------- 会话冲突 ----------

func TestTaskConflictReasons(t *testing.T) {
	f := newFixture(t, nil)
	f.setMode("live")
	first, err := f.start(t, secretURL)
	if err != nil {
		t.Fatal(err)
	}
	// 同一标准化地址（scheme/host 大小写、默认端口不同写法）→ duplicate_url
	for _, dup := range []string{secretURL, strings.Replace(secretURL, "rtmp://user:pw123456@127.0.0.1:1935", "RTMP://user:pw123456@127.0.0.1", 1)} {
		_, err = f.start(t, dup)
		ae := mustAppErr(t, err, apperr.TaskConflict)
		if firstLine(ae.Detail) != "reason=duplicate_url" {
			t.Fatalf("detail 首行=%q", firstLine(ae.Detail))
		}
		assertNoSecrets(t, ae.Message+"\n"+ae.Detail, "127.0.0.1", "1935")
	}
	// 再开 3 个不同地址（共 4 个），第 5 个 → max_sessions
	for i := 2; i <= 4; i++ {
		if _, err := f.start(t, fmt.Sprintf("rtmp://10.0.0.%d/live/key%d", i, i)); err != nil {
			t.Fatal(err)
		}
	}
	_, err = f.start(t, "rtmp://10.0.0.9/live/SECRETKEYabc?token=TOKENxyz")
	ae := mustAppErr(t, err, apperr.TaskConflict)
	if firstLine(ae.Detail) != "reason=max_sessions" {
		t.Fatalf("detail 首行=%q", firstLine(ae.Detail))
	}
	assertNoSecrets(t, ae.Message+"\n"+ae.Detail, "10.0.0.9", "SECRETKEYabc")
	// 判断顺序 duplicate_url → screen_busy → max_sessions：满额时重复地址仍是 duplicate_url
	_, err = f.start(t, secretURL)
	if ae := mustAppErr(t, err, apperr.TaskConflict); firstLine(ae.Detail) != "reason=duplicate_url" {
		t.Fatalf("%q", ae.Detail)
	}
	// 事件和日志里也没有冲突相关的秘密
	assertNoSecrets(t, f.em.jsonAll(), "SECRETKEYabc", "pw123456", "TOKENxyz")

	// 其他 TASK_CONFLICT（已结束的会话再 Cancel）没有 reason= 行。
	f.mgr.Cancel(first.ID)
	f.wait(t, first.ID)
	err = f.mgr.Cancel(first.ID)
	ae = mustAppErr(t, err, apperr.TaskConflict)
	if strings.Contains(ae.Detail, "reason=") || strings.Contains(ae.Message, "reason=") {
		t.Fatalf("其他 TASK_CONFLICT 不带 reason: %+v", ae)
	}
	// 释放后同一地址可以再推
	deadline := time.Now().Add(3 * time.Second)
	for {
		if n, _ := f.svc.ActiveSessions(); n < MaxSessions {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("会话没有释放")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if tk, err := f.start(t, secretURL); err != nil {
		t.Fatal(err)
	} else {
		f.mgr.Cancel(tk.ID)
	}
	for _, id := range f.mgr.ListActive() {
		f.mgr.Cancel(id.ID)
	}
}

func assertNoSecrets(t *testing.T, text string, extra ...string) {
	t.Helper()
	for _, s := range append(append([]string{}, secretFragments...), extra...) {
		if strings.Contains(text, s) {
			t.Fatalf("发现不该出现的片段 %q:\n%s", s, text)
		}
	}
}

// ---------- 结果语义 ----------

func TestGracefulStopIsSucceededWithoutError(t *testing.T) {
	f := newFixture(t, nil)
	f.setMode("live")
	tk, err := f.start(t, "rtmp://127.0.0.1:1935/live/k1")
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != task.StatusQueued || tk.Version != 1 || tk.Progress != -1 || tk.Type != task.TypeLiveFilePush {
		t.Fatalf("返回的是入队前快照: %+v", tk)
	}
	f.waitProgress(t, tk.ID)
	if err := f.mgr.Cancel(tk.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.mgr.Cancel(tk.ID); err != nil { // 停止中重复点击返回 nil
		t.Fatalf("重复 Cancel 应返回 nil: %v", err)
	}
	d := f.wait(t, tk.ID)
	if d.Status != task.StatusSucceeded || d.Error != nil {
		t.Fatalf("优雅停止应 succeeded 且 error 为空: %+v", d)
	}
	assertStatusPayloadsHaveNoError(t, f, tk.ID, task.StatusSucceeded)
}

func assertStatusPayloadsHaveNoError(t *testing.T, f *fixture, id string, final task.Status) {
	t.Helper()
	var last task.Status
	for _, e := range f.em.statusEvents(id) {
		if e.Error != nil {
			t.Fatalf("task:status 载荷不应带 error: %+v", e)
		}
		last = e.Status
	}
	if last != final {
		t.Fatalf("最后一个 task:status=%s want %s", last, final)
	}
	f.em.mu.Lock()
	defer f.em.mu.Unlock()
	for _, e := range f.em.evts {
		if s, ok := e.payload.(task.StatusEvent); ok && s.ID == id {
			if j, _ := json.Marshal(s); strings.Contains(string(j), `"error"`) {
				t.Fatalf("task:status JSON 里不能有 error 键: %s", j)
			}
		}
	}
}

func TestForceKillIsCanceledWithoutError(t *testing.T) {
	f := newFixture(t, nil)
	f.setMode("ignoreq") // 已开始但不响应 q：等满宽限期后强杀
	tk, _ := f.start(t, "rtmp://127.0.0.1:1935/live/k2")
	f.waitProgress(t, tk.ID)
	begin := time.Now()
	f.mgr.Cancel(tk.ID)
	d := f.wait(t, tk.ID)
	if d.Status != task.StatusCanceled || d.Error != nil {
		t.Fatalf("强杀应 canceled 且 error 为空: %+v", d)
	}
	if el := time.Since(begin); el < 1400*time.Millisecond {
		t.Fatalf("已开始的会话应先等宽限期再杀: %v", el)
	}
	assertStatusPayloadsHaveNoError(t, f, tk.ID, task.StatusCanceled)
}

func TestNonzeroExitAfterCancelIsCanceledNotInterrupted(t *testing.T) {
	f := newFixture(t, nil)
	for _, mode := range []string{"q_then_fail"} {
		f.setMode(mode)
		tk, _ := f.start(t, "rtmp://127.0.0.1:1935/live/k3")
		f.waitProgress(t, tk.ID)
		f.mgr.Cancel(tk.ID)
		d := f.wait(t, tk.ID)
		if d.Status != task.StatusCanceled || d.Error != nil {
			t.Fatalf("%s: 取消后非零退出（224 + Broken pipe）应 canceled、无错误码: %+v", mode, d)
		}
		assertStatusPayloadsHaveNoError(t, f, tk.ID, task.StatusCanceled)
	}
}

func TestCancelDuringConnectKillsImmediately(t *testing.T) {
	f := newFixture(t, func(c *Config) { c.Grace = 10 * time.Second })
	f.setMode("connecting")
	tk, _ := f.start(t, "rtmp://10.255.255.1/live/k4")
	deadline := time.Now().Add(3 * time.Second)
	for {
		if g, _ := f.mgr.Get(tk.ID); g.Status == task.StatusRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("没有进入 running")
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	if f.progressed(tk.ID) {
		t.Fatal("连接阶段不应有 task:progress")
	}
	begin := time.Now()
	f.mgr.Cancel(tk.ID)
	d := f.wait(t, tk.ID)
	if d.Status != task.StatusCanceled || d.Error != nil || time.Since(begin) > 2*time.Second {
		t.Fatalf("连接阶段取消应直接强杀（不等 10 秒宽限期）: %+v %v", d, time.Since(begin))
	}
}

func TestNaturalEndIsSucceeded(t *testing.T) {
	f := newFixture(t, nil)
	f.setMode("natural")
	tk, _ := f.start(t, "rtmp://127.0.0.1:1935/live/k5")
	d := f.wait(t, tk.ID)
	if d.Status != task.StatusSucceeded || d.Error != nil {
		t.Fatalf("自然播完也是 succeeded: %+v", d)
	}
	if n, _ := f.svc.ActiveSessions(); n != 0 {
		t.Fatal("结束后应释放会话")
	}
}

// ---------- 错误分类（假 ffmpeg，全部脱敏） ----------

func TestFailureClassificationAndRedaction(t *testing.T) {
	tests := []struct {
		mode, url string
		code      apperr.Code
		first     string
	}{
		{"refused", secretURL, apperr.LiveConnectFailed, "scheme=rtmp"},
		{"rejected", secretURL, apperr.LivePushRejected, ""},
		{"broken", secretURL, apperr.LivePushInterrupted, ""},
		{"unknown", secretURL, apperr.Internal, ""},
		{"refused", "srt://127.0.0.1:9000?streamid=publish:SECRETKEYabc&passphrase=pw123456xyz", apperr.LiveConnectFailed, "scheme=srt"},
	}
	for _, tc := range tests {
		t.Run(tc.mode+"/"+strings.SplitN(tc.url, ":", 2)[0], func(t *testing.T) {
			f := newFixture(t, nil)
			f.setMode(tc.mode)
			tk, err := f.start(t, tc.url)
			if err != nil {
				t.Fatal(err)
			}
			d := f.wait(t, tk.ID)
			if d.Status != task.StatusFailed || d.Error == nil || d.Error.Code != tc.code {
				t.Fatalf("want failed/%s got %+v err=%+v", tc.code, d, d.Error)
			}
			if tc.first != "" && firstLine(d.Error.Detail) != tc.first {
				t.Fatalf("detail 首行=%q", firstLine(d.Error.Detail))
			}
			// 秘密不得出现在任何地方：标题、params、error、日志、全部事件、任务记录。
			frags := append(append([]string{}, secretFragments...), "pw123456xyz")
			rec, _ := json.Marshal(d)
			for name, text := range map[string]string{
				"title": d.Title, "params": d.Params, "error": d.Error.Message + "\n" + d.Error.Detail,
				"log": f.logText(t, tk.ID), "events": f.em.jsonAll(), "record": string(rec),
			} {
				for _, s := range frags {
					if strings.Contains(text, s) {
						t.Fatalf("%s 里出现秘密片段 %q:\n%s", name, s, text)
					}
				}
			}
			if !strings.Contains(d.Title, "***") {
				t.Fatalf("标题应是脱敏地址: %s", d.Title)
			}
			var p map[string]any
			if json.Unmarshal([]byte(d.Params), &p) != nil || p["kind"] != "file" {
				t.Fatalf("params: %s", d.Params)
			}
			// 库里读出来的也一样
			if got, err := f.mgr.Get(tk.ID); err != nil || got.Error == nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRetryOfLiveTaskIsUnsupported(t *testing.T) {
	f := newFixture(t, nil)
	f.setMode("natural")
	tk, _ := f.start(t, "rtmp://127.0.0.1:1935/live/k6")
	f.wait(t, tk.ID)
	_, err := f.mgr.Retry(tk.ID)
	mustAppErr(t, err, apperr.Unsupported)
}

// ---------- 命令行 ----------

func TestFilePushCommandLine(t *testing.T) {
	f := newFixture(t, func(c *Config) { c.Media = fakeMedia{info: store.MediaInfo{HasVideo: true, HasAudio: false, Fps: 25}} })
	f.setMode("live")
	tk, err := f.svc.StartFilePush(context.Background(), FilePushRequest{InputPath: filepath.Join(f.dir, "a b.mp4"), URL: "rtmp://127.0.0.1:1935/live/k7", Loop: true,
		Options: PushOptions{Width: 1281, Fps: 30, VideoBitrateKbps: 1500}})
	if err != nil {
		t.Fatal(err)
	}
	f.waitProgress(t, tk.ID)
	b, _ := os.ReadFile(f.exe + ".args")
	line := strings.ReplaceAll(strings.TrimSpace(string(b)), "\n", " ")
	for _, want := range []string{"-protocol_whitelist file -re -stream_loop -1 -i file:" + filepath.Join(f.dir, "a b.mp4"),
		"-f lavfi -i anullsrc=r=44100:cl=stereo", "-map 0:v:0 -map 1:a", "fps=30,scale=1280:-2", "-b:v 1500k", "-g 60", "-shortest",
		"-protocol_whitelist rtmp,tcp -f flv", "rtmp://127.0.0.1:1935/live/k7"} {
		if !strings.Contains(line, want) {
			t.Errorf("命令行缺少 %q:\n%s", want, line)
		}
	}
	if strings.Contains(line, "-c copy") || strings.Contains(line, "-c:v copy") {
		t.Fatal("不能 copy")
	}
	if tk.InputPaths[0] != filepath.Join(f.dir, "a b.mp4") {
		t.Fatalf("%+v", tk.InputPaths)
	}
	f.mgr.Cancel(tk.ID)
	f.wait(t, tk.ID)
}
