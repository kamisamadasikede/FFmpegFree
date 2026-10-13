//go:build !windows

package cat

import (
	"io"

	"FFmpegFree/internal/apperr"
)

func consoleAvailable() bool { return false }

func openConsole(string) (*termProc, []io.Reader, error) {
	return nil, nil, apperr.New(apperr.UnsupportedPlatform, "终端没有启动。")
}
