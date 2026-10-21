package main

import (
	"context"
	"sync/atomic"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// windowState 记录窗口是否被藏起来（WindowHide / 关到托盘）。Wails v2 没有「窗口是否可见」的查询，
// 所以所有隐藏 / 显示都必须走 hideWindow / showWindow，不要直接调 wruntime.WindowHide / WindowShow（契约 v0.35.5 ⑤）。
type windowState struct {
	hidden atomic.Bool
}

// windowOps 是窗口运行时调用（测试替换）。
type windowOps struct {
	hide       func(ctx context.Context)
	show       func(ctx context.Context)
	unminimise func(ctx context.Context)
	minimised  func(ctx context.Context) bool
}

var defaultWindowOps = windowOps{
	hide:       wruntime.WindowHide,
	show:       wruntime.WindowShow,
	unminimise: wruntime.WindowUnminimise,
	minimised:  wruntime.WindowIsMinimised,
}

func (a *App) winOps() windowOps {
	if a.windowOps != nil {
		return *a.windowOps
	}
	return defaultWindowOps
}

// hideWindow 藏起窗口（关到后台 / 托盘时用）并记下隐藏状态。
func (a *App) hideWindow() {
	a.win.hidden.Store(true)
	if a.ctx != nil {
		a.winOps().hide(a.ctx)
	}
}

// showWindow 把窗口恢复并置前：取消最小化、显示，清掉隐藏状态。
func (a *App) showWindow() {
	if a.ctx != nil {
		ops := a.winOps()
		ops.unminimise(a.ctx)
		ops.show(a.ctx)
	}
	a.win.hidden.Store(false)
}

// catWindowForeground：窗口可见且未最小化才算在前台（不要求焦点）；隐藏或最小化 → 不在前台，发系统通知（v0.35.5 ⑤）。
func (a *App) catWindowForeground() bool {
	if a.ctx == nil || a.win.hidden.Load() {
		return false
	}
	return !a.winOps().minimised(a.ctx)
}

// emitEvent 发应用级事件（测试可替换 emitFn）。
func (a *App) emitEvent(ctx context.Context, event string, data ...any) {
	if ctx == nil {
		return
	}
	if a.emitFn != nil {
		a.emitFn(ctx, event, data...)
		return
	}
	wruntime.EventsEmit(ctx, event, data...)
}

// beforeClose 是 Wails 的关窗钩子。第一次关窗不立刻关窗口：先让前端显示「正在结束任务…」，
// 后台做收尾（最长 16 秒：带存档的直播要写完存档尾），完成后再调 Quit 真正退出。
// 返回 true = 阻止本次关闭；Quit 触发的第二次关窗会直接放行。
func (a *App) beforeClose(ctx context.Context) bool {
	if a.closing.Load() {
		return false
	}
	a.closing.Store(true)
	base := a.ctx
	if base == nil {
		base = ctx
	}
	a.emitEvent(base, "app:closing")
	quit := a.quitFn
	if quit == nil {
		quit = wruntime.Quit
	}
	go func() {
		a.shutdown(base)
		if base != nil {
			quit(base)
		}
	}()
	return true
}
