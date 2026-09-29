//go:build !windows

package live

import "FFmpegFree/internal/apperr"

func listWindowsMonitors() ([]ScreenInfo, error) {
	return nil, apperr.New(apperr.UnsupportedPlatform, "不是 Windows")
}
