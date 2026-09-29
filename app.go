package main

import (
	"FFmpegFree/app"
	"FFmpegFree/backend/contollers"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/convert"
	"FFmpegFree/internal/service/live"
	"FFmpegFree/internal/service/media"
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
	ctx context.Context
	// rootCtx 是应用根 ctx，shutdown 时取消；探测、缩略图等长时间操作用它，退出时不会遗留子进程。
	rootCtx    context.Context
	rootCancel context.CancelFunc
	dirs       paths.Dirs
	store      *store.Store
	sys        *system.Manager
	tasks      atomic.Pointer[task.Manager]
	media      atomic.Pointer[media.Service]
	conv       atomic.Pointer[convert.Service]
	live       atomic.Pointer[live.Service]
}

// taskManager 返回任务管理器；OnStartup 完成前（或存储初始化失败时）为 nil。
// 首字母小写，不会被 Wails 当作绑定方法暴露给前端。
func (a *App) taskManager() *task.Manager { return a.tasks.Load() }

// mediaService 返回媒体服务；OnStartup 完成前为 nil。小写，不会被 Wails 暴露。
func (a *App) mediaService() *media.Service { return a.media.Load() }

// convertService 返回转换服务；OnStartup 完成前（或存储 / 任务管理器不可用时）为 nil。小写，不会被 Wails 暴露。
func (a *App) convertService() *convert.Service { return a.conv.Load() }

// liveService 返回直播服务；OnStartup 完成前（或任务管理器 / 媒体服务不可用时）为 nil。小写，不会被 Wails 暴露。
func (a *App) liveService() *live.Service { return a.live.Load() }

// NewApp creates a new App application struct
func NewApp(sys *system.Manager) *App {
	// 根 ctx 在构造时就创建，保证绑定方法在 OnStartup 之前被调用也拿到有效的 ctx。
	ctx, cancel := context.WithCancel(context.Background())
	return &App{sys: sys, rootCtx: ctx, rootCancel: cancel}
}

// appContext 返回应用根 ctx，shutdown 时被取消。小写，不会被 Wails 暴露。
func (a *App) appContext() context.Context { return a.rootCtx }

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.initStore(ctx); err != nil {
		// v2 迁移期间旧的 gin 接口仍在工作，存储层初始化失败先记录日志，不阻止应用启动。
		log.Printf("初始化本地存储失败: %v", err)
	}
	a.startTasks(ctx)
	a.startMedia()
	a.startConvert(ctx)
	a.startLive()
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

// startMedia 创建媒体服务（探测、缩略图）并清理一次缩略图缓存。存储不可用时仍可生成缩略图，只是不记录最近媒体。
func (a *App) startMedia() {
	thumbs := a.dirs.Thumbs
	if thumbs == "" {
		d, err := paths.Resolve("")
		if err != nil {
			log.Printf("定位缩略图目录失败，媒体服务未启动: %v", err)
			return
		}
		thumbs = d.Thumbs
	}
	cfg := media.Config{ThumbsDir: thumbs}
	if a.store != nil { // 避免把 nil *Store 装进接口
		cfg.Store = a.store
	}
	svc := media.New(cfg)
	go svc.CleanupCache()
	a.media.Store(svc)
}

// startConvert 创建转换服务：需要存储（预设）、任务管理器和媒体服务，缺一个就不启动（此时 ConvertService 返回 INTERNAL）。
func (a *App) startConvert(ctx context.Context) {
	tm, med := a.taskManager(), a.mediaService()
	if a.store == nil || tm == nil || med == nil {
		log.Printf("转换服务未启动：存储、任务管理器或媒体服务不可用")
		return
	}
	svc, err := convert.New(ctx, convert.Config{
		Presets:          a.store,
		Media:            med,
		Tasks:            tm,
		DefaultOutputDir: a.sys.DefaultOutputDir,
	})
	if err != nil {
		log.Printf("启动转换服务失败: %v", err)
		return
	}
	a.conv.Store(svc)
}

// startLive 创建直播服务：需要任务管理器和媒体服务，缺一个就不启动（此时 LiveService 返回 INTERNAL）。
func (a *App) startLive() {
	tm, med := a.taskManager(), a.mediaService()
	if tm == nil || med == nil {
		log.Printf("直播服务未启动：任务管理器或媒体服务不可用")
		return
	}
	a.live.Store(live.New(live.Config{Tasks: tm, Media: med}))
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
	if a.rootCancel != nil {
		a.rootCancel() // 先取消根 ctx：进行中的探测 / 缩略图立即结束 ffprobe / ffmpeg
	}
	contollers.KillAllFFmpegProcesses()
	contollers.KillLiveOpsProcesses()
	if m := a.taskManager(); m != nil {
		// 先停任务再关数据库：运行中的任务被取消并落库为 interrupted。
		// 有带存档的直播会话时要多等：优雅停止最多 15 秒写完存档尾（契约 6.10：总等待 16 秒，超时强杀）。
		wait := 8 * time.Second
		if l := a.liveService(); l != nil {
			if _, archive := l.ActiveSessions(); archive {
				wait = 16 * time.Second
			}
		}
		m.Shutdown(wait)
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
