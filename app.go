package main

import (
	"FFmpegFree/app"
	"FFmpegFree/internal/about"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/doccomp"
	"FFmpegFree/internal/doceng"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/langasr"
	"FFmpegFree/internal/localassets"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/service/convert"
	"FFmpegFree/internal/service/doc"
	"FFmpegFree/internal/service/lang"
	"FFmpegFree/internal/service/live"
	"FFmpegFree/internal/service/media"
	"FFmpegFree/internal/service/system"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	// storage 是启动时确定一次的存储根目录 <base>（契约 v0.24，6.15.1）。
	storage paths.Storage
	// interruptedReconverts 是启动时恢复的“上次退出时被中断的重转”条数（v0.24.1，交给 ConvertService.TakeInterruptedReconverts）。
	interruptedReconverts int
	store                 *store.Store
	sys                   *system.Manager
	tasks                 atomic.Pointer[task.Manager]
	media                 atomic.Pointer[media.Service]
	conv                  atomic.Pointer[convert.Service]
	docs                  atomic.Pointer[doc.Service]
	// docComp 是文档组件（契约 v0.26，6.12.12）：检测、下载、准备；不依赖转换组件。
	docComp atomic.Pointer[doccomp.Manager]
	// /local/<token> 预览登记表（契约 6.13）：doc、convert 分表，各 512 项，互不挤占；main.go 用 localHandler 挂到 AssetServer。
	// v0.23.5：剪辑已移除，edit 登记表随之删除。
	docLocal     *localassets.Registry
	convertLocal *localassets.Registry // 转换页 / 任务中心的预览（契约 v0.23，6.14.7）
	live         atomic.Pointer[live.Service]
	langAsr      atomic.Pointer[langasr.Manager]
	lang         atomic.Pointer[lang.Service]
	catReg       atomic.Pointer[catagent.Registry]
	cat          atomic.Pointer[cat.Service]
}

// docAssets 返回文档的 /local/<token> 登记表（NewApp 时创建，永不为 nil）。小写，不会被 Wails 暴露。
// DocService 用 docAssets().Register(path)。
func (a *App) docAssets() *localassets.Registry { return a.docLocal }

// localHandler 是挂在 Wails AssetServer.Handler 上的处理器，按 token 在两张表里查。
func (a *App) localHandler() http.Handler {
	return localassets.MultiHandler(a.docLocal, a.convertLocal)
}

// docComponent 返回文档组件管理器；OnStartup 完成前为 nil。小写，不会被 Wails 暴露。
func (a *App) docComponent() *doccomp.Manager { return a.docComp.Load() }

// docService 返回文档服务；OnStartup 完成前为 nil。小写，不会被 Wails 暴露。
func (a *App) docService() *doc.Service { return a.docs.Load() }

// taskManager 返回任务管理器；OnStartup 完成前（或存储初始化失败时）为 nil。
// 首字母小写，不会被 Wails 当作绑定方法暴露给前端。
func (a *App) taskManager() *task.Manager { return a.tasks.Load() }

// mediaService 返回媒体服务；OnStartup 完成前为 nil。小写，不会被 Wails 暴露。
func (a *App) mediaService() *media.Service { return a.media.Load() }

// convertService 返回转换服务；OnStartup 完成前（或存储 / 任务管理器不可用时）为 nil。小写，不会被 Wails 暴露。
func (a *App) convertService() *convert.Service { return a.conv.Load() }

// liveService 返回直播服务；OnStartup 完成前（或任务管理器 / 媒体服务不可用时）为 nil。小写，不会被 Wails 暴露。
func (a *App) liveService() *live.Service { return a.live.Load() }

func (a *App) langService() *lang.Service { return a.lang.Load() }

func (a *App) langAsrManager() *langasr.Manager { return a.langAsr.Load() }

func (a *App) catService() *cat.Service { return a.cat.Load() }

func (a *App) catRegistry() *catagent.Registry { return a.catReg.Load() }

// NewApp creates a new App application struct
func NewApp(sys *system.Manager) *App {
	// 根 ctx 在构造时就创建，保证绑定方法在 OnStartup 之前被调用也拿到有效的 ctx。
	ctx, cancel := context.WithCancel(context.Background())
	return &App{sys: sys, rootCtx: ctx, rootCancel: cancel, docLocal: localassets.New(localassets.Config{}),
		convertLocal: localassets.New(localassets.Config{})}
}

// appContext 返回应用根 ctx，shutdown 时被取消。小写，不会被 Wails 暴露。
func (a *App) appContext() context.Context { return a.rootCtx }

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// 应用日志 <数据目录>/logs/app.log（契约 v0.24.2）：Windows 的 GUI 程序没有控制台，不接文件的话 log.Printf 全部丢失。
	if d, err := paths.Resolve(""); err == nil {
		setupAppLog(d.Logs)
	}
	a.initStorage()
	if err := a.initStore(ctx); err != nil {
		// 存储层初始化失败先记录日志，不阻止应用启动（依赖存储的服务会返回 INTERNAL）。
		log.Printf("初始化本地存储失败: %v", err)
	}
	a.startTasks(ctx)
	a.startMedia()
	a.startConvert(ctx)
	a.startDoc()
	a.startLang()
	a.startCat()
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
		st := a.store
		// 契约 v0.23.4：Probe 成功后顺带刷新转换页同一文件的源文件行（convert_sources.media），失败只记日志。
		cfg.OnProbed = func(ctx context.Context, key string, m store.MediaInfo, fi os.FileInfo) {
			if err := st.SetConvertSourceMediaByKey(ctx, key, store.FileFingerprint(fi), &m); err != nil {
				log.Printf("刷新源文件行的媒体信息失败: %v", err)
			}
		}
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
		DefaultOutputDir: a.sys.ActualOutputDir, // v0.24：自定义优先，否则 <base>/output
		DataDir:          a.dirs.Root,
		Encoder:          a.sys.EncoderResolver(),
		AcquireHWEncode: func(ctx context.Context) (func(), error) {
			if s := a.langService(); s != nil {
				return s.HWAcquire(ctx)
			}
			return func() {}, nil
		},
		Sources: a.store,
		Thumbs:  med,
		Preview: a.convertLocal,
		Open:    a.sys.OpenWithDefaultApp,
		Reveal:  a.sys.RevealRegisteredPath,
		// v0.24：源文件副本、convert:copy 事件、启动时中断的重转条数
		UploadsDir:            a.sys.ActualUploadsDir,
		Emitter:               app.NewWailsEmitter(ctx),
		Logf:                  log.Printf,
		InterruptedReconverts: a.interruptedReconverts,
	})
	if err != nil {
		log.Printf("启动转换服务失败: %v", err)
		return
	}
	a.conv.Store(svc)
}

// startDoc 创建文档服务（Office 转 PDF、PDF 预览）：不依赖 ffmpeg；需要任务管理器才能提交转换，
// 存储不可用时 OpenPDF 仍可用，只是不记录最近打开。
func (a *App) startDoc() {
	cfg := doc.Config{
		Local:            a.docAssets(),
		DefaultOutputDir: a.sys.ActualOutputDir, // v0.24：Office 转 PDF 默认输出到 <base>/output
		DataDir:          a.dirs.Root,
	}
	// v0.26：文档组件在后台检测（不阻塞界面），状态走 doc:component 事件。
	root := a.dirs.Root
	if root == "" {
		if d, err := paths.Resolve(""); err == nil {
			root = d.Root
		}
	}
	emit := app.NewWailsEmitter(a.ctx).Emit
	// doc:component 一律发合并后的状态（含 Office / WPS，同 GetDocComponentStatus）：组件自己发的那份只有组件，
	// Registry 建好后改由 Registry 发；之前（启动瞬间）退回组件自己的状态。
	var regRef atomic.Pointer[doceng.Registry]
	compEmit := doceng.MergedComponentEmit(emit, regRef.Load)
	comp := doccomp.New(doccomp.Config{Dir: doccomp.DefaultDir(root), Emit: compEmit, Logf: log.Printf})
	a.docComp.Store(comp)
	comp.Start()
	cfg.Component = comp
	if a.dirs.Temp != "" {
		cfg.TempRoot = filepath.Join(a.dirs.Temp, "doc")
	}
	cfg.UploadsDir = a.sys.ActualUploadsDir
	if c := a.convertService(); c != nil {
		cfg.Sources = c
	}
	if a.store != nil { // 避免把 nil *Store 装进接口
		cfg.Recent = a.store
		cfg.Lister = a.store
		storeRef := a.store
		cfg.TaskGet = func(ctx context.Context, id string) (task.Task, error) {
			return storeRef.GetTask(ctx, id)
		}
	}
	if tm := a.taskManager(); tm != nil {
		cfg.Tasks = tm
	}
	cfg.Emit = emit
	cfg.DocEngine = a.sys.DocEngine
	reg := doceng.NewRegistry(doceng.Config{Component: comp, DocEngine: a.sys.DocEngine, Emit: emit, Logf: log.Printf})
	regRef.Store(reg)
	reg.StartDetect()
	cfg.Engines = reg
	a.sys.DocComponentDir = reg.DownloadedComponentDir
	a.sys.SetDocEngineHook(func(string) { reg.EmitStatus() })
	svc := doc.New(cfg)
	reg.SetCompConv(svc.NewComponentConverter())
	svc.CleanupDocTemp()
	if n := svc.CleanupInterruptedParts(a.rootCtx); n > 0 {
		log.Printf("已清理 %d 个中断的 Office 转 PDF 临时文件", n)
	}
	a.docs.Store(svc)
}

// startLang 创建语音工具（转字幕）服务：语音识别组件 + speech_to_subtitle（契约 6.18）。
func (a *App) startLang() {
	root := a.dirs.Root
	if root == "" {
		if d, err := paths.Resolve(""); err == nil {
			root = d.Root
		}
	}
	emit := app.NewWailsEmitter(a.ctx).Emit
	comp := langasr.New(langasr.Config{
		Dir:  langasr.DefaultDir(root),
		Tier: func() string { return a.sys.AsrTier(a.rootCtx) },
		Emit: emit,
		Logf: log.Printf,
	})
	a.langAsr.Store(comp)
	comp.Start()
	a.sys.LangAsrDir = func() (string, error) {
		if s := a.langService(); s != nil {
			return s.ComponentDir()
		}
		return "", nil
	}
	a.sys.SetAsrTierHook(func(string) {
		if s := a.langService(); s != nil {
			s.OnAsrTierChanged()
		} else {
			comp.RefreshTierGuide()
		}
	})
	temp := ""
	if a.dirs.Temp != "" {
		temp = filepath.Join(a.dirs.Temp, "lang")
	}
	cfg := lang.Config{
		Asr:             comp,
		TempRoot:        temp,
		Tier:            a.sys.AsrTier,
		Emit:            emit,
		Logf:            log.Printf,
		ActualOutputDir: a.sys.ActualOutputDir,
	}
	if tm := a.taskManager(); tm != nil {
		cfg.Tasks = tm
	}
	svc := lang.New(cfg)
	a.lang.Store(svc)
}

// startCat 创建 Cat 助手服务（契约 6.19）：一期仅注册 cat_build 适配器。
func (a *App) startCat() {
	root := a.dirs.Root
	if root == "" {
		if d, err := paths.Resolve(""); err == nil {
			root = d.Root
		}
	}
	emit := app.NewWailsEmitter(a.ctx).Emit
	temp := ""
	if a.dirs.Temp != "" {
		temp = a.dirs.Temp
	}
	reg := catagent.NewRegistry()
	build := catagent.NewBuildAdapter(catagent.BuildConfig{
		ComponentDir: catagent.DefaultComponentDir(root),
		DataTemp:     temp,
		Emit:         emit,
		Logf:         log.Printf,
	})
	reg.Register(build)
	reg.SetDefaultKind(a.sys.CatDefaultAgentKind(a.rootCtx))
	a.catReg.Store(reg)
	build.Start()

	cfg := cat.Config{
		Registry: reg,
		Emit:     emit,
		Logf:     log.Printf,
		DefaultAgentKind: func(ctx context.Context) string {
			return a.sys.CatDefaultAgentKind(ctx)
		},
		OpenFolder: a.sys.OpenFolder, // v0.31.1 RevealCatProject：同 OpenStorageFolder 的打开目录分支
	}
	if a.store != nil {
		cfg.Store = a.store
	}
	svc := cat.New(cfg)
	a.cat.Store(svc)
}

// startLive 创建直播服务：需要任务管理器和媒体服务，缺一个就不启动（此时 LiveService 返回 INTERNAL）。
func (a *App) startLive() {
	tm, med := a.taskManager(), a.mediaService()
	if tm == nil || med == nil {
		log.Printf("直播服务未启动：任务管理器或媒体服务不可用")
		return
	}
	// v0.25 不再用 JPEG 预览目录。上次运行留下的 live-preview 删掉，不重建。
	if a.dirs.Temp != "" {
		if err := live.CleanupPreviewDir(filepath.Join(a.dirs.Temp, live.PreviewDirName)); err != nil {
			log.Printf("清理旧的直播预览目录失败: %v", err)
		}
	}
	a.live.Store(live.New(live.Config{Tasks: tm, Media: med, Encoder: a.sys.EncoderResolver(), Logf: log.Printf, Emit: app.NewWailsEmitter(a.ctx).Emit}))
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

// initStorage 确定存储根目录 <base>（契约 6.15.1，只在启动时判断一次），注入 system.Manager。
func (a *App) initStorage() {
	d, err := paths.Resolve("")
	if err != nil {
		log.Printf("定位应用数据目录失败: %v", err)
		return
	}
	a.storage = paths.ResolveStorage(d.Root)
	a.sys.SetStorage(a.storage, d.Root)
	log.Printf("存储位置：%s（%s，回退=%v）", a.storage.Base, a.storage.BaseKind, a.storage.FellBack)
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
	// 契约 v0.24 / v0.24.1（6.17.5）：先恢复上次退出时没结束的原地重转（记录回到 succeeded、删临时文件），再标记中断任务。
	// 孤儿临时文件扫描额外看实际输出目录（设置还没加载，直接读设置键）。
	outDir := a.storage.Output
	var custom string
	if _, err := s.GetSetting(ctx, system.SettingDefaultOutputDir, &custom); err == nil && custom != "" {
		outDir = custom
	}
	var extra []string
	if outDir != "" {
		extra = append(extra, outDir)
	}
	if n, err := task.RecoverReconverts(ctx, s, extra, log.Printf); err != nil {
		log.Printf("恢复中断的重转失败: %v", err)
	} else {
		a.interruptedReconverts = n
		if n > 0 {
			log.Printf("上次退出时有 %d 条重转被中断，已恢复原来的结果", n)
		}
	}
	if n, err := s.MarkInterrupted(ctx, time.Now()); err != nil {
		log.Printf("标记中断任务失败: %v", err)
	} else if n > 0 {
		log.Printf("上次退出时有 %d 个任务未完成，已标记为 interrupted", n)
	}
	// 契约 v0.23（6.14.8）：回填旧 convert 任务的源文件行和 output_name_key。幂等，失败只记日志、不阻止启动，下次启动再试。
	if ns, nn, err := s.BackfillConvertSources(ctx); err != nil {
		log.Printf("回填转换记录失败（下次启动再试）: %v", err)
	} else if ns > 0 || nn > 0 {
		log.Printf("已回填 %d 条转换记录的源文件行、%d 条记录的输出文件名", ns, nn)
	}
	a.dirs, a.store = dirs, s
	return nil
}

// shutdown 在窗口关闭时由 Wails 调用：取消根 ctx、停止所有任务（结束 ffmpeg 子进程）并关闭数据库。
func (a *App) shutdown(ctx context.Context) {
	if a.rootCancel != nil {
		a.rootCancel() // 先取消根 ctx：进行中的探测 / 缩略图立即结束 ffprobe / ffmpeg
	}
	if l := a.liveService(); l != nil {
		l.Close() // 停止拉流预览会话（推流任务由下面的任务管理器停止）
	}
	if dc := a.docComp.Load(); dc != nil {
		dc.Close() // 取消下载 / 准备（保留 .part / 安装包，下次接着来）
	}
	if c := a.convertService(); c != nil {
		c.Close(3 * time.Second) // 停复制队列：正在复制的副本下次启动标记为 failed（reason=interrupted）
	}
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

// GetLicenseText 返回内嵌的第三方许可全文。白名单："OFL"（Noto Sans SC 的 SIL Open Font License 1.1）、"OFL-Nunito"（Nunito 的 SIL OFL 1.1）；
// 其它名称（含空串、带路径、大小写不同）返回 INVALID_ARGUMENT。
func (a *App) GetLicenseText(name string) (string, error) {
	return about.LicenseText(name)
}

// GetAppVersion 返回应用版本号；构建时未用 -ldflags 注入则返回“开发版”。
func (a *App) GetAppVersion() string {
	return about.AppVersion()
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
