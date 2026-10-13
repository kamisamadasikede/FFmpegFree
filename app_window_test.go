package main

import (
	"context"
	"testing"
)

type fakeWin struct {
	minimised bool
	visible   bool
	calls     []string
}

func newWinApp(w *fakeWin) *App {
	a := &App{ctx: context.Background()}
	a.windowOps = &windowOps{
		hide:       func(context.Context) { w.visible = false; w.calls = append(w.calls, "hide") },
		show:       func(context.Context) { w.visible = true; w.calls = append(w.calls, "show") },
		unminimise: func(context.Context) { w.minimised = false; w.calls = append(w.calls, "unminimise") },
		minimised:  func(context.Context) bool { return w.minimised },
	}
	return a
}

// v0.35.5 ⑤：隐藏或最小化都算不在前台。
func TestCatWindowForegroundHiddenOrMinimised(t *testing.T) {
	w := &fakeWin{visible: true}
	a := newWinApp(w)
	if !a.catWindowForeground() {
		t.Fatal("visible should be foreground")
	}
	w.minimised = true
	if a.catWindowForeground() {
		t.Fatal("minimised")
	}
	w.minimised = false
	a.hideWindow()
	if a.catWindowForeground() || w.visible {
		t.Fatal("hidden")
	}
	a.showWindow()
	if !a.catWindowForeground() {
		t.Fatal("shown again")
	}
	if (&App{}).catWindowForeground() {
		t.Fatal("no ctx")
	}
}

// v0.35.5 ②：点通知，隐藏 + 最小化都恢复并置前，清掉隐藏状态。
func TestCatNotificationClickRestoresWindow(t *testing.T) {
	w := &fakeWin{visible: true}
	a := newWinApp(w)
	a.hideWindow()
	w.minimised = true
	w.calls = nil
	a.onCatNotificationClick("s", "c") // 没有 cat 服务时不推事件，也不 panic
	if w.minimised || !w.visible || a.win.hidden.Load() || len(w.calls) != 2 || w.calls[0] != "unminimise" || w.calls[1] != "show" {
		t.Fatalf("%+v hidden=%v", w, a.win.hidden.Load())
	}
	if !a.catWindowForeground() {
		t.Fatal("should be foreground")
	}
}
