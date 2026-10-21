package main

import (
	"context"
	"testing"
	"time"
)

// 关窗收尾：第一次关窗阻止关闭并在后台收尾，收尾完成后 Quit；Quit 触发的第二次关窗放行。
func TestBeforeCloseRunsShutdownThenQuit(t *testing.T) {
	a := &App{ctx: context.Background()}
	quit := make(chan struct{}, 1)
	emitted := make(chan string, 1)
	a.quitFn = func(context.Context) { quit <- struct{}{} }
	a.emitFn = func(_ context.Context, event string, _ ...any) { emitted <- event }

	if prevented := a.beforeClose(context.Background()); !prevented {
		t.Fatal("第一次关窗应阻止关闭，先做收尾")
	}
	if prevented := a.beforeClose(context.Background()); prevented {
		t.Fatal("收尾完成后的第二次关窗应放行")
	}
	select {
	case e := <-emitted:
		if e != "app:closing" {
			t.Fatalf("event = %q", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("没有发 app:closing")
	}
	select {
	case <-quit:
	case <-time.After(2 * time.Second):
		t.Fatal("收尾后没有 Quit")
	}
}

// 收尾只跑一次：beforeClose 已经跑过之后，OnShutdown 再调不能重复收尾。
func TestShutdownIsIdempotent(t *testing.T) {
	a := &App{ctx: context.Background()}
	a.shutdown(context.Background())
	a.shutdown(context.Background())
}
