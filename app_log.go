package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

// 应用日志（契约 v0.23.6）：<数据目录>/logs/app.log，Windows 上是 %AppData%\FFmpegFree\logs\app.log。
// Windows 包是 GUI 程序，没有控制台，log.Printf 默认写的 stderr 等于丢弃：包 19 的缩略图在 Windows 上全部失败，
// 却一行记录都没有。现在所有 log.Printf 同时写进这个文件（控制台照旧，写不进去也不影响文件）。
// 超过 appLogMaxBytes 时启动时轮转一次：app.log → app.log.1（只留一份旧的）。
const (
	appLogName     = "app.log"
	appLogMaxBytes = 5 << 20
)

// setupAppLog 打开应用日志并把标准库 log 的输出接上去，返回日志文件路径；打不开时只写控制台。
func setupAppLog(dir string) string {
	f, p, err := openAppLog(dir)
	if err != nil {
		log.Printf("打开应用日志失败，日志只输出到控制台: %v", err)
		return ""
	}
	log.SetOutput(&appLogWriter{file: f, console: os.Stderr})
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	exe, _ := os.Executable()
	log.Printf("应用启动 %s/%s exe=%q 日志=%q", runtime.GOOS, runtime.GOARCH, exe, p)
	return p
}

// openAppLog 在 dir 下以追加方式打开 app.log；已有的文件超过上限时先改名成 app.log.1（覆盖更旧的那份）。
// 改名失败（例如 Windows 上另一个实例正开着它）就继续往原文件追加。
func openAppLog(dir string) (*os.File, string, error) {
	if dir == "" {
		return nil, "", fmt.Errorf("日志目录为空")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	p := filepath.Join(dir, appLogName)
	if fi, err := os.Stat(p); err == nil && fi.Size() > appLogMaxBytes {
		_ = os.Remove(p + ".1")
		_ = os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, "", err
	}
	return f, p, nil
}

// appLogWriter 先写控制台（失败忽略：Windows GUI 程序的 stderr 是无效句柄），再写文件，返回文件的结果。
type appLogWriter struct {
	file    io.Writer
	console io.Writer
}

func (w *appLogWriter) Write(p []byte) (int, error) {
	if w.console != nil {
		_, _ = w.console.Write(p)
	}
	return w.file.Write(p)
}
