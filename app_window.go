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
