package main

import (
	"FFmpegFree/app"
	"FFmpegFree/backend/contollers"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/system"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx   context.Context
	dirs  paths.Dirs
	store *store.Store
	sys   *system.Manager
	tasks atomic.Pointer[task.Manager]
}

// taskManager 返回任务管理器；OnStartup 完成前（或存储初始化失败时）为 nil。
// 首字母小写，不会被 Wails 当作绑定方法暴露给前端。
func (a *App) taskManager() *task.Manager { return a.tasks.Load() }

// NewApp creates a new App application struct
func NewApp(sys *system.Manager) *App {
	return &App{sys: sys}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.initStore(ctx); err != nil {
		// v2 迁移期间旧的 gin 接口仍在工作，存储层初始化失败先记录日志，不阻止应用启动。
		log.Printf("初始化本地存储失败: %v", err)
	}
	a.startTasks(ctx)
	a.startFFmpegDetect(ctx)
}

// startTasks 创建任务管理器。initStore 已经把上次未结束的任务标记为 interrupted，
// 所以这里启动时内存里没有任何活动任务，也不会自动恢复执行。
func (a *App) startTasks(ctx context.Context) {
	if a.store == nil {
		log.Printf("本地存储不可用，任务管理器未启动")
		return
	}
	a.tasks.Store(task.NewManager(task.Config{
		Store:   a.store,
		Emitter: app.NewWailsEmitter(ctx),
		LogDir:  a.dirs.Logs,
		Logf:    log.Printf,
	}))
}

// startFFmpegDetect 在后台检测 ffmpeg，不阻塞界面；状态变化通过 ffmpeg:status 事件推送。
func (a *App) startFFmpegDetect(ctx context.Context) {
	binDir := a.dirs.Bin
	if binDir == "" {
		// 存储初始化失败时 a.dirs 可能为空，仍然按默认位置检测。
		if d, err := paths.Resolve(""); err == nil {
			binDir = d.Bin
		}
	}
	loc := ffmpeg.NewLocator(binDir)
	cfg := system.Config{
		Locator: loc,
		Emitter: app.NewWailsEmitter(ctx),
	}
	if manifest, err := ffmpeg.DefaultManifest(); err != nil {
		log.Printf("加载 ffmpeg 下载清单失败: %v", err)
	} else {
		tmpDir := a.dirs.Temp
		if tmpDir == "" {
			if d, err := paths.Resolve(""); err == nil {
				tmpDir = d.Temp
			}
		}
		cfg.Installer = ffmpeg.NewInstaller(manifest, loc, binDir, tmpDir)
	}
	if a.store != nil { // 避免把 nil *Store 装进接口
		cfg.Settings = a.store
	}
	cfg.Tasks = a.taskManager()
	a.sys.Start(ctx, cfg)
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
	if m := a.taskManager(); m != nil {
		// 先停任务再关数据库：运行中的任务被取消并落库为 interrupted。
		m.Shutdown(8 * time.Second)
	}
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
