//go:build !windows

package live

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/task"
)

func TestCaptureCapabilities(t *testing.T) {
	cases := []struct {
		name    string
		goos    string
		env     map[string]string
		ok      bool
		backend string
		session string
		perm    string
	}{
		{"windows", "windows", nil, true, "gdigrab", "", "notRequired"},
		{"darwin", "darwin", nil, true, "avfoundation", "", "unknown"},
		{"x11", "linux", map[string]string{"XDG_SESSION_TYPE": "x11", "DISPLAY": ":0"}, true, "x11grab", "x11", "notRequired"},
		{"x11 无 XDG 但有 DISPLAY", "linux", map[string]string{"DISPLAY": ":99"}, true, "x11grab", "unknown", "notRequired"},
		{"wayland 即使有 XWayland", "linux", map[string]string{"XDG_SESSION_TYPE": "wayland", "DISPLAY": ":0", "WAYLAND_DISPLAY": "wayland-0"}, false, "", "wayland", "notRequired"},
		{"无 DISPLAY", "linux", map[string]string{"XDG_SESSION_TYPE": "x11"}, false, "", "x11", "notRequired"},
		{"其他系统", "freebsd", nil, false, "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, func(c *Config) {
				c.GOOS = tc.goos
				c.Getenv = func(k string) string { return tc.env[k] }
			})
			c, err := f.svc.GetCaptureCapabilities()
			if err != nil {
				t.Fatal(err)
			}
			if c.Supported != tc.ok || c.Backend != tc.backend || c.SessionType != tc.session || c.Permission != tc.perm || c.AudioCapture {
				t.Fatalf("%+v", c)
			}
			if !tc.ok && c.Reason == "" {
				t.Fatal("不支持时必须给 reason")
			}
			if !tc.ok {
				_, err := f.svc.ListScreens(context.Background())
				mustAppErr(t, err, apperr.UnsupportedPlatform)
			}
		})
	}
}

func TestParseXrandr(t *testing.T) {
	out := `Screen 0: minimum 320 x 200, current 3840 x 1080, maximum 16384 x 16384
HDMI-1 connected primary 1920x1080+0+0 (normal left inverted right x axis y axis) 527mm x 296mm
   1920x1080     60.00*+
DP-1 connected 1920x1080+1920+0 (normal left inverted right x axis y axis) 527mm x 296mm
   1920x1080     60.00*+
DP-2 disconnected (normal left inverted right x axis y axis)
VIRTUAL1 connected (normal left inverted right x axis y axis)
`
	got := parseXrandr(out)
	if len(got) != 2 || got[0].ID != "x11:HDMI-1" || !got[0].Primary || got[1].ID != "x11:DP-1" || got[1].Primary || got[1].X != 1920 || got[1].Width != 1920 {
		t.Fatalf("%+v", got)
	}
	// 没有 primary：第一个当主显示器
	got = parseXrandr("screen connected 1280x800+0+0 (normal)\n")
	if len(got) != 1 || !got[0].Primary || got[0].Height != 800 {
		t.Fatalf("%+v", got)
	}
	if len(parseXrandr("")) != 0 {
		t.Fatal("空输入应为空")
	}
}

func TestParseAVFoundationScreens(t *testing.T) {
	out := `[AVFoundation indev @ 0x7f8] AVFoundation video devices:
[AVFoundation indev @ 0x7f8] [0] FaceTime HD Camera
[AVFoundation indev @ 0x7f8] [1] Capture screen 0
[AVFoundation indev @ 0x7f8] [2] Capture screen 1
[AVFoundation indev @ 0x7f8] AVFoundation audio devices:
[AVFoundation indev @ 0x7f8] [0] MacBook Pro Microphone
`
	got := parseAVFoundationScreens(out)
	if len(got) != 2 || got[0].ID != "avf:1" || !got[0].Primary || got[1].ID != "avf:2" || got[1].Primary {
		t.Fatalf("%+v", got)
	}
}

func TestResolveScreen(t *testing.T) {
	list := []ScreenInfo{{ID: "a"}, {ID: "b", Primary: true}}
	if s, err := resolveScreen(list, ""); err != nil || s.ID != "b" {
		t.Fatalf("%+v %v", s, err)
	}
	if s, err := resolveScreen(list, "a"); err != nil || s.ID != "a" {
		t.Fatalf("%+v %v", s, err)
	}
	_, err := resolveScreen(list, "zzz")
	mustAppErr(t, err, apperr.InvalidArgument)
	_, err = resolveScreen(nil, "")
	mustAppErr(t, err, apperr.UnsupportedPlatform)
}

func x11Fixture(t *testing.T, mut func(*Config)) *fixture {
	return newFixture(t, func(c *Config) {
		c.GOOS = "linux"
		c.Getenv = func(k string) string { return map[string]string{"DISPLAY": ":99", "XDG_SESSION_TYPE": "x11"}[k] }
		c.Run = func(context.Context, string, ...string) (string, error) {
			return "HDMI-1 connected primary 1920x1080+0+0 (normal)\nDP-1 connected 1280x720+1920+0 (normal)\n", nil
		}
		if mut != nil {
			mut(c)
		}
	})
}

func TestStartScreenPushX11CommandLine(t *testing.T) {
	f := x11Fixture(t, nil)
	tk, err := f.svc.StartScreenPush(context.Background(), ScreenPushRequest{ScreenID: "x11:DP-1", URL: "rtmp://127.0.0.1:1935/live/s1", Audio: "silent", HideCursor: true,
		Options: PushOptions{Fps: 15, VideoBitrateKbps: 1200}})
	if err != nil {
		t.Fatal(err)
	}
	if tk.Type != task.TypeLiveScreenPush || len(tk.InputPaths) != 0 {
		t.Fatalf("%+v", tk)
	}
	f.waitProgress(t, tk.ID)
	b, _ := os.ReadFile(f.exe + ".args")
	line := strings.ReplaceAll(strings.TrimSpace(string(b)), "\n", " ")
	for _, want := range []string{"-f x11grab", "-framerate 15", "-video_size 1280x720", "-draw_mouse 0", "-i :99+1920,0",
		"anullsrc", "-b:v 1200k", "-protocol_whitelist rtmp,tcp -f flv", "rtmp://127.0.0.1:1935/live/s1"} {
		if !strings.Contains(line, want) {
			t.Errorf("缺少 %q:\n%s", want, line)
		}
	}
	if strings.Contains(line, "-shortest") {
		t.Errorf("屏幕采集是无限流，不能加 -shortest:\n%s", line)
	}
	f.mgr.Cancel(tk.ID)
	if d := f.wait(t, tk.ID); d.Status != task.StatusSucceeded || d.Error != nil {
		t.Fatalf("%+v %+v", d, d.Error)
	}
}

func TestStartScreenPushErrors(t *testing.T) {
	f := x11Fixture(t, nil)
	ctx := context.Background()
	u := "rtmp://127.0.0.1:1935/live/s2"
	_, err := f.svc.StartScreenPush(ctx, ScreenPushRequest{ScreenID: "x11:nope", URL: u})
	mustAppErr(t, err, apperr.InvalidArgument)
	_, err = f.svc.StartScreenPush(ctx, ScreenPushRequest{URL: u, Audio: "mic"})
	mustAppErr(t, err, apperr.InvalidArgument)
	_, err = f.svc.StartScreenPush(ctx, ScreenPushRequest{URL: "http://x/y"})
	mustAppErr(t, err, apperr.LiveURLInvalid)
	if n, _ := f.svc.ActiveSessions(); n != 0 {
		t.Fatalf("失败不应占用会话: %d", n)
	}

	w := newFixture(t, func(c *Config) {
		c.GOOS = "linux"
		c.Getenv = func(k string) string { return map[string]string{"DISPLAY": ":0", "XDG_SESSION_TYPE": "wayland"}[k] }
	})
	_, err = w.svc.StartScreenPush(ctx, ScreenPushRequest{URL: u})
	mustAppErr(t, err, apperr.UnsupportedPlatform)
}

func TestStartScreenPushSessionLimits(t *testing.T) {
	f := x11Fixture(t, nil)
	f.setMode("live")
	ctx := context.Background()
	tk, err := f.svc.StartScreenPush(ctx, ScreenPushRequest{URL: "rtmp://127.0.0.1:1935/live/dup"})
	if err != nil {
		t.Fatal(err)
	}
	// 屏幕推流与文件推流共用会话表：同地址冲突
	_, err = f.start(t, "rtmp://127.0.0.1:1935/live/dup")
	ae := mustAppErr(t, err, apperr.TaskConflict)
	if firstLine(ae.Detail) != "reason=duplicate_url" {
		t.Fatalf("%q", ae.Detail)
	}
	f.mgr.Cancel(tk.ID)
	f.wait(t, tk.ID)
}

func screenReq(url string) ScreenPushRequest { return ScreenPushRequest{URL: url} }

// 屏幕推流同一时间最多 1 路。判断顺序：duplicate_url → screen_busy → max_sessions；文件推流不受屏幕推流影响。
func TestScreenBusyAndConflictOrder(t *testing.T) {
	f := x11Fixture(t, nil)
	f.setMode("live")
	ctx := context.Background()
	const u1 = "rtmp://127.0.0.1:1935/live/sb1"
	first, err := f.svc.StartScreenPush(ctx, screenReq(u1))
	if err != nil {
		t.Fatal(err)
	}
	reason := func(err error) string {
		ae := mustAppErr(t, err, apperr.TaskConflict)
		assertNoSecrets(t, ae.Message+"\n"+ae.Detail, "127.0.0.1", "1935", "sb1", "sb2")
		return firstLine(ae.Detail)
	}
	// 同地址且已有屏幕推流 → duplicate_url（先于 screen_busy）
	_, err = f.svc.StartScreenPush(ctx, screenReq(u1))
	if r := reason(err); r != "reason=duplicate_url" {
		t.Fatalf("同地址应 duplicate_url: %s", r)
	}
	// 不同地址、已有屏幕推流 → screen_busy
	_, err = f.svc.StartScreenPush(ctx, screenReq("rtmp://127.0.0.1:1935/live/sb2"))
	if r := reason(err); r != "reason=screen_busy" {
		t.Fatalf("不同地址应 screen_busy: %s", r)
	}
	if n, _ := f.svc.ActiveSessions(); n != 1 {
		t.Fatalf("失败不应占用会话: %d", n)
	}
	// 文件推流不受影响（不同地址可以开；同地址仍是 duplicate_url）
	fp, err := f.start(t, "rtmp://10.0.0.2/live/f2")
	if err != nil {
		t.Fatalf("文件推流不应受屏幕推流限制: %v", err)
	}
	_, err = f.start(t, u1)
	if r := reason(err); r != "reason=duplicate_url" {
		t.Fatal(r)
	}
	// screen_busy 先于 max_sessions：凑满 4 个会话后再开屏幕推流仍是 screen_busy；文件推流则是 max_sessions
	for i := 3; i <= 4; i++ {
		if _, err := f.start(t, fmt.Sprintf("rtmp://10.0.0.%d/live/f%d", i, i)); err != nil {
			t.Fatal(err)
		}
	}
	_, err = f.svc.StartScreenPush(ctx, screenReq("rtmp://127.0.0.1:1935/live/sb2"))
	if r := reason(err); r != "reason=screen_busy" {
		t.Fatalf("满额时屏幕推流应先 screen_busy: %s", r)
	}
	_, err = f.start(t, "rtmp://10.0.0.9/live/f9")
	if r := reason(err); r != "reason=max_sessions" {
		t.Fatalf("文件推流满额应 max_sessions: %s", r)
	}
	// 屏幕推流结束后释放，可以再开一路
	f.mgr.Cancel(first.ID)
	f.wait(t, first.ID)
	deadline := time.Now().Add(3 * time.Second)
	for {
		tk, err := f.svc.StartScreenPush(ctx, screenReq("rtmp://127.0.0.1:1935/live/sb2"))
		if err == nil {
			f.mgr.Cancel(tk.ID)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("屏幕推流结束后应释放: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = fp
	for _, a := range f.mgr.ListActive() {
		f.mgr.Cancel(a.ID)
	}
}

// 并发两次 StartScreenPush（不同地址）只成功一次，另一次 screen_busy；检查与登记在同一把锁里。
func TestConcurrentScreenStartsOnlyOneSucceeds(t *testing.T) {
	for round := 0; round < 5; round++ {
		f := x11Fixture(t, nil)
		f.setMode("live")
		const n = 8
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make([]error, n)
		ids := make([]string, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				tk, err := f.svc.StartScreenPush(context.Background(), screenReq(fmt.Sprintf("rtmp://127.0.0.1:1935/live/c%d", i)))
				errs[i], ids[i] = err, tk.ID
			}(i)
		}
		close(start)
		wg.Wait()
		ok := 0
		for i, err := range errs {
			if err == nil {
				ok++
				continue
			}
			ae := mustAppErr(t, err, apperr.TaskConflict)
			if firstLine(ae.Detail) != "reason=screen_busy" {
				t.Fatalf("第 %d 个失败应是 screen_busy: %q", i, ae.Detail)
			}
		}
		if ok != 1 {
			t.Fatalf("第 %d 轮：并发 %d 次 StartScreenPush 应只成功 1 次，实际 %d", round, n, ok)
		}
		if got, _ := f.svc.ActiveSessions(); got != 1 {
			t.Fatalf("会话数应为 1: %d", got)
		}
		if len(f.mgr.ListActive()) != 1 {
			t.Fatalf("任务表里应只有 1 个进行中的任务: %d", len(f.mgr.ListActive()))
		}
		for _, a := range f.mgr.ListActive() {
			f.mgr.Cancel(a.ID)
			f.wait(t, a.ID)
		}
	}
}

// 同地址并发的屏幕推流：只成功一次，另一次 duplicate_url（先于 screen_busy）。
func TestConcurrentScreenStartsSameURL(t *testing.T) {
	f := x11Fixture(t, nil)
	f.setMode("live")
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = f.svc.StartScreenPush(context.Background(), screenReq("rtmp://127.0.0.1:1935/live/same"))
		}(i)
	}
	close(start)
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if ae := mustAppErr(t, err, apperr.TaskConflict); firstLine(ae.Detail) != "reason=duplicate_url" {
			t.Fatalf("同地址并发应 duplicate_url: %q", ae.Detail)
		}
	}
	if ok != 1 {
		t.Fatalf("应只成功 1 次: %d", ok)
	}
	for _, a := range f.mgr.ListActive() {
		f.mgr.Cancel(a.ID)
		f.wait(t, a.ID)
	}
}
