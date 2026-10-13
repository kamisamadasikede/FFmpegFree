package main

import (
	"log"

	"FFmpegFree/internal/service/cat"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// initCatNotifications 打开系统通知（Wails v2.16 runtime：Windows 用 toast，macOS 用通知中心，Linux 走 D-Bus）。
// 点通知：把窗口拉到前面，推 cat:schedule { openConvId } 让界面打开那条对话。失败只记日志，不影响启动。
func (a *App) initCatNotifications() {
	if a.ctx == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("cat notifications init panic: %v", r)
		}
	}()
	if err := wruntime.InitializeNotifications(a.ctx); err != nil {
		log.Printf("cat notifications init: %v", err)
		return
	}
	a.catNotifyOK.Store(true)
	if ok, err := wruntime.CheckNotificationAuthorization(a.ctx); err == nil && !ok {
		_, _ = wruntime.RequestNotificationAuthorization(a.ctx)
	}
	wruntime.OnNotificationResponse(a.ctx, func(res wruntime.NotificationResult) {
		if res.Error != nil {
			return
		}
		info := res.Response.UserInfo
		convID, _ := info["convId"].(string)
		schedID, _ := info["scheduleId"].(string)
		wruntime.WindowUnminimise(a.ctx)
		wruntime.WindowShow(a.ctx)
		if svc := a.catService(); svc != nil {
			svc.OpenFromNotification(schedID, convID)
		}
	})
}

// sendCatNotification 发一条系统通知；通知不可用时什么也不做。
func (a *App) sendCatNotification(n cat.Notification) {
	if a.ctx == nil || !a.catNotifyOK.Load() {
		return
	}
	err := wruntime.SendNotification(a.ctx, wruntime.NotificationOptions{
		ID: n.ID, Title: n.Title, Body: n.Body,
		Data: map[string]interface{}{"convId": n.ConvID, "scheduleId": n.ScheduleID},
	})
	if err != nil {
		log.Printf("cat notification: %v", err)
	}
}

// catWindowForeground 近似「窗口在前台」：没有最小化、也没有藏起来。Wails v2 没有焦点查询接口。
func (a *App) catWindowForeground() bool {
	if a.ctx == nil {
		return false
	}
	return !wruntime.WindowIsMinimised(a.ctx)
}
