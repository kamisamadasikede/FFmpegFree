package main

import (
	"FFmpegFree/app"
	"FFmpegFree/internal/service/system"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure

	sysManager := system.NewManager()
	mainApp := NewApp(sysManager)
	jsonService := app.NewJsonService()
	systemService := app.NewSystemService(sysManager)
	taskService := app.NewTaskService(mainApp.taskManager, mainApp.convertService, mainApp.appContext)
	mediaService := app.NewMediaService(mainApp.mediaService, mainApp.appContext)
	convertService := app.NewConvertService(mainApp.convertService, mainApp.appContext)
	docService := app.NewDocService(mainApp.docService, mainApp.appContext)
	liveService := app.NewLiveService(mainApp.liveService, mainApp.appContext)
	langService := app.NewLangService(mainApp.langService, mainApp.appContext)
	catService := app.NewCatService(mainApp.catService, mainApp.appContext)

	// /local/<token> 本地文件预览（契约 6.13）：doc / convert 两张登记表，Handler 挂在 AssetServer 上。

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "FFmpegFree",
		Width:     1280,
		Height:    800,
		MinWidth:  1024, // ✅ 设置为 0 表示无最小宽度
		MinHeight: 680,  // ✅ 设置为 0 表示无最小高度
		MaxWidth:  0,    // ✅ 0 表示无最大宽度
		MaxHeight: 0,    // ✅ 0 表示无最大高度
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: mainApp.localHandler(),
		},
		// 转换页等：拖入文件拿本地绝对路径（前端 OnFileDrop，落在带 --wails-drop-target: drop 的区域才回调）
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        mainApp.startup,
		OnShutdown:       mainApp.shutdown,
		Bind: []interface{}{
			mainApp,
			jsonService,
			systemService,
			taskService,
			mediaService,
			convertService,
			docService,
			liveService,
			langService,
			catService,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}

}
