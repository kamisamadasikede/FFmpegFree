package main

import (
	"FFmpegFree/backend/contollers"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/store"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx   context.Context
	dirs  paths.Dirs
	store *store.Store
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.initStore(ctx); err != nil {
		// v2 迁移期间旧的 gin 接口仍在工作，存储层初始化失败先记录日志，不阻止应用启动。
		log.Printf("初始化本地存储失败: %v", err)
	}
}

func (a *App) initStore(ctx context.Context) error {
	dirs, err := paths.Resolve("")
	if err != nil {
		return err
	}
	if err := dirs.Ensure(); err != nil {
		return err
	}
	s, err := store.Open(ctx, dirs.DB)
	if err != nil {
		return err
	}
	if n, err := s.MarkInterrupted(ctx, time.Now()); err != nil {
		log.Printf("标记中断任务失败: %v", err)
	} else if n > 0 {
		log.Printf("上次退出时有 %d 个任务未完成，已标记为 interrupted", n)
	}
	a.dirs, a.store = dirs, s
	return nil
}

// shutdown 在窗口关闭时由 Wails 调用：结束所有 ffmpeg 子进程并关闭数据库。
func (a *App) shutdown(ctx context.Context) {
	contollers.KillAllFFmpegProcesses()
	contollers.KillLiveOpsProcesses()
	if a.store != nil {
		if err := a.store.Close(); err != nil {
			log.Printf("关闭数据库失败: %v", err)
		}
	}
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}
func (a *App) Steamdom(inputPath string, outputPath string) error {
	// 执行转换逻辑...
	// 转换完成，通知前端
	runtime.EventsEmit(a.ctx, "conversion_complete", outputPath)
	return nil
}
