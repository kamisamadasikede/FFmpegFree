package system

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"FFmpegFree/internal/apperr"
)

// ErrNoApp 表示系统没有能打开这个文件的程序（契约 6.14.3 OpenWithSystem：NOT_FOUND reason=no_app）。
var ErrNoApp = errors.New("没有找到能打开这个文件的程序")

// opener 用系统默认程序打开 path。返回 ErrNoApp 表示没有关联程序，其他错误是启动失败。测试里替换。
type opener func(path string) error

// openWaitSeconds 是 macOS / Linux 等 open / xdg-open 退出码的最长时间，超过按成功（契约 6.14.3）。
const openWaitSeconds = 5

// classifyOpenExit 按平台把 open / xdg-open 的非 0 退出归类（契约 6.14.3）：
// macOS open 退出码非 0 且 stderr 含 "No application knows how to open" → ErrNoApp；Linux xdg-open 退出码 3 / 4 → ErrNoApp。
func classifyOpenExit(goos string, code int, stderr string) error {
	switch goos {
	case "darwin":
		if code != 0 && strings.Contains(stderr, "No application knows how to open") {
			return ErrNoApp
		}
	case "windows":
	default:
		if code == 3 || code == 4 {
			return ErrNoApp
		}
	}
	if code != 0 {
		return errors.New("打开文件的命令失败：" + strings.TrimSpace(lastStderrLine(stderr)))
	}
	return nil
}

func lastStderrLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// OpenWithDefaultApp 用系统默认程序打开 path（契约 6.14.3 OpenWithSystem 的启动部分）：Windows ShellExecuteW（open 动词，
// 路径作为文件参数，不经过命令行拼接）；macOS open <path>；Linux xdg-open <path>。路径由调用方从表里取并检查过扩展名白名单。
// 错误：不是绝对路径 INVALID_ARGUMENT；文件不在 NOT_FOUND（reason=file）；没有关联程序 NOT_FOUND（reason=no_app）；
// 命令找不到或其他启动失败 PROCESS_FAILED。
func (m *Manager) OpenWithDefaultApp(path string) error {
	open := m.open
	if open == nil {
		open = openDefault
	}
	return openWith(open, path)
}

func openWith(open opener, path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return apperr.New(apperr.InvalidArgument, "路径必须是绝对路径")
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return apperr.New(apperr.NotFound, "文件不存在").WithDetail("reason=file")
	}
	if err := open(path); err != nil {
		if errors.Is(err, ErrNoApp) {
			return apperr.New(apperr.NotFound, "没有找到能打开这个文件的程序").WithDetail("reason=no_app")
		}
		return apperr.Wrap(apperr.ProcessFailed, "无法用系统程序打开文件", err)
	}
	return nil
}

// RevealRegisteredPath 在文件管理器里显示 path（转换页的“打开所在文件夹”，契约 6.14.3 RevealSource / RevealRecord）：
// 平台命令、引号规则、启动失败 PROCESS_FAILED 全部同 RevealInFolder（Windows 同样走 reveal_windows.go 的 startDetached），
// 因为路径由调用方从表里取，不走 RevealInFolder 的范围白名单。Windows 路径含双引号 INVALID_ARGUMENT；不存在 NOT_FOUND。
func (m *Manager) RevealRegisteredPath(path string) error {
	start := m.launch
	if start == nil {
		start = startDetached
	}
	return revealIn(runtime.GOOS, start, path, nil)
}
