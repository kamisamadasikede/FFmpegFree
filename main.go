package main

import (
	"FFmpegFree/app"
	"FFmpegFree/backend/router"
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

	go router.InitRouter()
	sysManager := system.NewManager()
	mainApp := NewApp(sysManager)
	jsonService := app.NewJsonService()
	systemService := app.NewSystemService(sysManager)
	taskService := app.NewTaskService(mainApp.taskManager)
	mediaService := app.NewMediaService(mainApp.mediaService, mainApp.appContext)
	convertService := app.NewConvertService(mainApp.convertService, mainApp.appContext)

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "FFmpegFree",
		Width:     1600,
		Height:    850,
		MinWidth:  1600, // ✅ 设置为 0 表示无最小宽度
		MinHeight: 850,  // ✅ 设置为 0 表示无最小高度
		MaxWidth:  0,    // ✅ 0 表示无最大宽度
		MaxHeight: 0,    // ✅ 0 表示无最大高度
		AssetServer: &assetserver.Options{
			Assets: assets,
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
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}

}
