package system

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/proc"
)

// launcher 启动一个不需要等待结果的外部命令（打开文件管理器）。测试里替换。
type launcher func(name string, args ...string) error

func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	proc.Configure(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // explorer 选中文件时退出码常常是 1，不当作失败
	return nil
}

// revealCommand 返回在文件管理器里显示 path 的命令。isDir 为 true 时打开这个文件夹本身。
//
//	Windows: explorer /select,<path>（文件夹直接打开）
//	macOS:   open -R <path>（文件夹直接 open）
//	Linux:   xdg-open <文件所在文件夹>（文件夹直接打开）
func revealCommand(goos, path string, isDir bool) (string, []string) {
	switch goos {
	case "windows":
		if isDir {
			return "explorer", []string{path}
		}
		return "explorer", []string{"/select," + path}
	case "darwin":
		if isDir {
			return "open", []string{path}
		}
		return "open", []string{"-R", path}
	default:
		if isDir {
			return "xdg-open", []string{path}
		}
		return "xdg-open", []string{filepath.Dir(path)}
	}
}

// RevealInFolder 在系统文件管理器里显示 path（"打开输出位置"）。
// path 必须是绝对路径且存在：空或相对路径 INVALID_ARGUMENT，不存在 NOT_FOUND，
// 找不到文件管理器命令或启动失败 PROCESS_FAILED。命令启动后立即返回，不等待窗口。
func (m *Manager) RevealInFolder(path string) error {
	return revealIn(runtime.GOOS, startDetached, path)
}

func revealIn(goos string, start launcher, path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return apperr.New(apperr.InvalidArgument, "路径必须是绝对路径").WithDetail(path)
	}
	path = filepath.Clean(path)
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return apperr.New(apperr.NotFound, "文件或文件夹不存在").WithDetail(path)
		}
		return apperr.Wrap(apperr.IOError, "无法读取文件信息", err)
	}
	name, args := revealCommand(goos, path, fi.IsDir())
	if err := start(name, args...); err != nil {
		return apperr.Wrap(apperr.ProcessFailed, "无法打开文件管理器", err)
	}
	return nil
}

// AppContext 返回 Start 传入的应用 ctx（Wails 对话框需要它）；Start 之前为 nil。
func (m *Manager) AppContext() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appCtx
}
