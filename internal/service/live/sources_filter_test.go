package live

import (
	"reflect"
	"testing"

	"FFmpegFree/internal/apperr"
)

func TestFilterCaptureWindows(t *testing.T) {
	ok := func(h uint64, title string) RawWindow {
		return RawWindow{HWND: h, Title: title, Class: "Chrome_WidgetWin_1", PID: 100, Visible: true, Width: 800, Height: 600}
	}
	mod := func(w RawWindow, f func(*RawWindow)) RawWindow { f(&w); return w }
	const self = 4242
	tests := []struct {
		name string
		in   RawWindow
		keep bool
	}{
		{"普通窗口", ok(1, "记事本"), true},
		{"空标题", ok(2, ""), false},
		{"全空白标题", ok(3, "  \t"), false},
		{"不可见", mod(ok(4, "x"), func(w *RawWindow) { w.Visible = false }), false},
		{"最小化", mod(ok(5, "x"), func(w *RawWindow) { w.Minimized = true }), false},
		{"DWM 隐藏（别的虚拟桌面）", mod(ok(6, "x"), func(w *RawWindow) { w.Cloaked = true }), false},
		{"工具窗口", mod(ok(7, "x"), func(w *RawWindow) { w.ExStyle = wsExToolWindow }), false},
		{"工具窗口但带 APPWINDOW", mod(ok(8, "x"), func(w *RawWindow) { w.ExStyle = wsExToolWindow | wsExAppWindow }), true},
		{"NOACTIVATE 浮层", mod(ok(9, "x"), func(w *RawWindow) { w.ExStyle = wsExNoActivate }), false},
		{"被拥有的窗口（对话框）", mod(ok(10, "x"), func(w *RawWindow) { w.Owner = 99 }), false},
		{"被拥有但带 APPWINDOW", mod(ok(11, "x"), func(w *RawWindow) { w.Owner = 99; w.ExStyle = wsExAppWindow }), true},
		{"Program Manager（标题）", ok(12, "Program Manager"), false},
		{"Progman 类", mod(ok(13, "桌面"), func(w *RawWindow) { w.Class = "Progman" }), false},
		{"任务栏", mod(ok(14, "任务栏"), func(w *RawWindow) { w.Class = "Shell_TrayWnd" }), false},
		{"副屏任务栏", mod(ok(15, "任务栏"), func(w *RawWindow) { w.Class = "Shell_SecondaryTrayWnd" }), false},
		{"本进程窗口", mod(ok(16, "FFmpegFree"), func(w *RawWindow) { w.PID = self }), false},
		{"零宽", mod(ok(17, "x"), func(w *RawWindow) { w.Width = 0 }), false},
		{"零高", mod(ok(18, "x"), func(w *RawWindow) { w.Height = 0 }), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterCaptureWindows([]RawWindow{tc.in}, self)
			if (len(got) == 1) != tc.keep {
				t.Fatalf("keep=%v got=%+v", tc.keep, got)
			}
		})
	}
	// 输出结构、id 是十进制 hwnd、保持枚举顺序、标题原样保留（含特殊字符）
	got := filterCaptureWindows([]RawWindow{
		ok(0xABCDEF, `He said "hi" = 1; 100%`), mod(ok(2, "x"), func(w *RawWindow) { w.Minimized = true }), ok(3, "b"),
	}, self)
	want := []CaptureSource{
		{ID: "window:11259375", Kind: "window", Title: `He said "hi" = 1; 100%`, Width: 800, Height: 600},
		{ID: "window:3", Kind: "window", Title: "b", Width: 800, Height: 600},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
	// selfPID=0 不过滤任何进程
	if g := filterCaptureWindows([]RawWindow{mod(ok(1, "a"), func(w *RawWindow) { w.PID = 0 })}, 0); len(g) != 1 {
		t.Fatal("selfPID=0 时不应按 PID 过滤")
	}
}

func TestParseSourceID(t *testing.T) {
	tests := []struct {
		id   string
		kind string
		n    uint64
		ok   bool
	}{
		{"screen:0", "screen", 0, true},
		{"screen:12", "screen", 12, true},
		{"window:265782", "window", 265782, true},
		{"window:18446744073709551615", "window", 18446744073709551615, true},
		{"", "", 0, false},
		{"screen", "", 0, false},
		{"screen:", "", 0, false},
		{"screen:-1", "", 0, false},
		{"screen:01", "", 0, false},
		{"window:0x10", "", 0, false},
		{"window:+5", "", 0, false},
		{"window:1 ", "", 0, false},
		{"monitor:0", "", 0, false},
		{"window:18446744073709551616", "", 0, false},
	}
	for _, tc := range tests {
		k, n, err := parseSourceID(tc.id)
		if tc.ok {
			if err != nil || k != tc.kind || n != tc.n {
				t.Errorf("%q: %s %d %v", tc.id, k, n, err)
			}
			continue
		}
		if err == nil || apperr.From(err).Code != apperr.InvalidArgument {
			t.Errorf("%q 应 INVALID_ARGUMENT: %v", tc.id, err)
		}
	}
}

func TestSourceGoneShape(t *testing.T) {
	for kind, msg := range map[string]string{"window": "所选窗口已不可用，请重新选择", "screen": "所选屏幕已不可用，请重新选择"} {
		e := sourceGone(kind)
		if e.Code != apperr.LiveSourceGone || e.Detail != "kind="+kind || e.Message != msg {
			t.Errorf("%+v", e)
		}
	}
}
