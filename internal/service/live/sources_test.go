//go:build !windows

package live

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/task"
)

func winFixture(t *testing.T, wins *[]RawWindow, mut func(*Config)) *fixture {
	return newFixture(t, func(c *Config) {
		c.GOOS = "windows"
		c.Monitors = func() ([]ScreenInfo, error) {
			return []ScreenInfo{
				{ID: "monitor:0", Name: "显示器 1（主）", Primary: true, X: 0, Y: 0, Width: 1920, Height: 1080, Scale: 1},
				{ID: "monitor:1", Name: "显示器 2", X: -1280, Y: 0, Width: 1280, Height: 720, Scale: 1},
			}, nil
		}
		c.EnumWindows = func() ([]RawWindow, error) { return append([]RawWindow(nil), *wins...), nil }
		if mut != nil {
			mut(c)
		}
	})
}

func rw(h uint64, title string) RawWindow {
	return RawWindow{HWND: h, Title: title, Class: "Notepad", PID: 1, Visible: true, Width: 640, Height: 480}
}

// Linux：只有屏幕，没有 window；id 是 screen:<序号>
func TestListCaptureSourcesLinuxOnlyScreens(t *testing.T) {
	f := x11Fixture(t, func(c *Config) {
		// 即使注入了窗口枚举，非 Windows 也不列窗口
		c.EnumWindows = func() ([]RawWindow, error) { return []RawWindow{rw(1, "x")}, nil }
	})
	got, err := f.svc.ListCaptureSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []CaptureSource{
		{ID: "screen:0", Kind: "screen", Title: "显示器 1（主）", Width: 1920, Height: 1080},
		{ID: "screen:1", Kind: "screen", Title: "显示器 2", Width: 1280, Height: 720},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("%+v", got)
	}
	for _, s := range got {
		if s.Kind == "window" {
			t.Fatal("Linux 不应返回 window")
		}
	}
}

func TestListCaptureSourcesLinuxNoXrandrFallback(t *testing.T) {
	f := x11Fixture(t, func(c *Config) {
		c.Run = func(context.Context, string, ...string) (string, error) { return "", errors.New("no xrandr") }
	})
	got, err := f.svc.ListCaptureSources(context.Background())
	if err != nil || len(got) != 1 || got[0].ID != "screen:0" || got[0].Kind != "screen" {
		t.Fatalf("拿不到多显示器时只给一个默认: %+v %v", got, err)
	}
}

func TestListCaptureSourcesUnsupported(t *testing.T) {
	w := newFixture(t, func(c *Config) {
		c.GOOS = "linux"
		c.Getenv = func(k string) string { return map[string]string{"DISPLAY": ":0", "XDG_SESSION_TYPE": "wayland"}[k] }
	})
	_, err := w.svc.ListCaptureSources(context.Background())
	mustAppErr(t, err, apperr.UnsupportedPlatform)
}

func TestListCaptureSourcesWindows(t *testing.T) {
	wins := []RawWindow{rw(100, "记事本"), func() RawWindow { w := rw(101, "最小化的"); w.Minimized = true; return w }(), rw(102, "计算器")}
	f := winFixture(t, &wins, nil)
	got, err := f.svc.ListCaptureSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "screen:0,screen:1,window:100,window:102" {
		t.Fatalf("%v", ids)
	}
	// 枚举窗口失败：只给屏幕
	f2 := winFixture(t, &wins, func(c *Config) { c.EnumWindows = func() ([]RawWindow, error) { return nil, errors.New("boom") } })
	got, err = f2.svc.ListCaptureSources(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
}

func argsLine(t *testing.T, f *fixture) string {
	t.Helper()
	b, _ := os.ReadFile(f.exe + ".args")
	return strings.ReplaceAll(strings.TrimSpace(string(b)), "\n", " ")
}

func sourceGoneDetail(t *testing.T, err error, kind string) {
	t.Helper()
	ae := mustAppErr(t, err, apperr.LiveSourceGone)
	if firstLine(ae.Detail) != "kind="+kind {
		t.Fatalf("detail=%q", ae.Detail)
	}
}

// LIVE_SOURCE_GONE：通过注入的窗口枚举函数模拟窗口最小化 / 关闭；失败不占用会话。
func TestStartScreenPushSourceGone(t *testing.T) {
	wins := []RawWindow{rw(100, "记事本")}
	f := winFixture(t, &wins, nil)
	ctx := context.Background()
	const u = "rtmp://127.0.0.1:1935/live/src1"

	// 窗口已关闭
	_, err := f.svc.StartScreenPush(ctx, ScreenPushRequest{URL: u, CaptureSourceID: "window:999"})
	sourceGoneDetail(t, err, "window")
	ae := apperr.From(err)
	if ae.Message != "所选窗口已不可用，请重新选择" {
		t.Fatal(ae.Message)
	}
	// 窗口最小化
	wins[0].Minimized = true
	_, err = f.svc.StartScreenPush(ctx, ScreenPushRequest{URL: u, CaptureSourceID: "window:100"})
	sourceGoneDetail(t, err, "window")
	// 不可见
	wins[0].Minimized, wins[0].Visible = false, false
	_, err = f.svc.StartScreenPush(ctx, ScreenPushRequest{URL: u, CaptureSourceID: "window:100"})
	sourceGoneDetail(t, err, "window")
	// 屏幕序号不存在
	_, err = f.svc.StartScreenPush(ctx, ScreenPushRequest{URL: u, CaptureSourceID: "screen:7"})
	sourceGoneDetail(t, err, "screen")
	if n, _ := f.svc.ActiveSessions(); n != 0 {
		t.Fatalf("失败不应占用会话: %d", n)
	}
	// id 格式不对：INVALID_ARGUMENT
	_, err = f.svc.StartScreenPush(ctx, ScreenPushRequest{URL: u, CaptureSourceID: "window:abc"})
	mustAppErr(t, err, apperr.InvalidArgument)
	// 枚举函数本身失败：INTERNAL
	f2 := winFixture(t, &wins, func(c *Config) { c.EnumWindows = func() ([]RawWindow, error) { return nil, errors.New("boom") } })
	_, err = f2.svc.StartScreenPush(ctx, ScreenPushRequest{URL: u, CaptureSourceID: "window:100"})
	mustAppErr(t, err, apperr.Internal)
	// 非 Windows 传 window:… → INVALID_ARGUMENT（它从来没被列出过）
	x := x11Fixture(t, nil)
	_, err = x.svc.StartScreenPush(ctx, ScreenPushRequest{URL: u, CaptureSourceID: "window:1"})
	mustAppErr(t, err, apperr.InvalidArgument)
	_, err = x.svc.StartScreenPush(ctx, ScreenPushRequest{URL: u, CaptureSourceID: "screen:5"})
	sourceGoneDetail(t, err, "screen")
}

// 窗口来源：argv 里是 gdigrab -i title=<标题>（标题原样一个参数）；屏幕来源：desktop + offset（含负数）。
func TestStartScreenPushWithSources(t *testing.T) {
	title := `He said "hi" & 100% | a=b; 中文 [x]`
	wins := []RawWindow{rw(100, title)}
	f := winFixture(t, &wins, nil)
	ctx := context.Background()

	tk, err := f.svc.StartScreenPush(ctx, ScreenPushRequest{URL: "rtmp://127.0.0.1:1935/live/w1", CaptureSourceID: "window:100", HideCursor: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tk.Title, title) || tk.Type != task.TypeLiveScreenPush {
		t.Fatalf("%+v", tk)
	}
	f.waitProgress(t, tk.ID)
	line := argsLine(t, f)
	if !strings.Contains(line, "-f gdigrab") || !strings.Contains(line, "-draw_mouse 0 -i title="+title+" ") || strings.Contains(line, "desktop") || strings.Contains(line, "offset_") {
		t.Fatalf("窗口采集参数: %s", line)
	}
	if !strings.Contains(tk.Params, `"captureSourceId":"window:100"`) || strings.Contains(tk.Params, "/live/w1") {
		t.Fatalf("params: %s", tk.Params)
	}
	f.mgr.Cancel(tk.ID)
	f.wait(t, tk.ID)

	tk, err = f.svc.StartScreenPush(ctx, ScreenPushRequest{URL: "rtmp://127.0.0.1:1935/live/w2", CaptureSourceID: "screen:1"})
	if err != nil {
		t.Fatal(err)
	}
	f.waitProgress(t, tk.ID)
	line = argsLine(t, f)
	if !strings.Contains(line, "-offset_x -1280 -offset_y 0 -video_size 1280x720 -i desktop") {
		t.Fatalf("屏幕采集参数: %s", line)
	}
	f.mgr.Cancel(tk.ID)
	f.wait(t, tk.ID)

	// 不传 captureSourceId：沿用 ScreenID（向后兼容），params 不带 captureSourceId
	tk, err = f.svc.StartScreenPush(ctx, ScreenPushRequest{URL: "rtmp://127.0.0.1:1935/live/w3", ScreenID: "monitor:1"})
	if err != nil {
		t.Fatal(err)
	}
	f.waitProgress(t, tk.ID)
	if line = argsLine(t, f); !strings.Contains(line, "-offset_x -1280 -offset_y 0 -video_size 1280x720 -i desktop") || strings.Contains(tk.Params, "captureSourceId") {
		t.Fatalf("%s %s", line, tk.Params)
	}
	f.mgr.Cancel(tk.ID)
	f.wait(t, tk.ID)
}
