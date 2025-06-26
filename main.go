package main

import (
	"FFmpegFree/backend/contollers"
	"FFmpegFree/backend/router"
	"embed"
	"log"
	"os"
	"os/signal"
	"syscall"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure

	go router.InitRouter()
	// 信号监听
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		log.Println("收到退出信号，正在清理所有转码/推流进程...")
		contollers.KillAllFFmpegProcesses()
		os.Exit(0)
	}()
	app := NewApp()

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
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}

}
