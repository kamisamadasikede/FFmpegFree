//go:build !windows

package live

import "FFmpegFree/internal/apperr"

// enumTopLevelWindows：只有 Windows 有窗口来源，其他系统不会调用它（GOOS 不是 windows 时 ListCaptureSources 不列窗口）。
func enumTopLevelWindows() ([]RawWindow, error) {
	return nil, apperr.New(apperr.UnsupportedPlatform, "不是 Windows")
}
