//go:build windows

package ffmpeg

import (
	"context"
	"testing"
)

// Windows 上每条检测命令都必须隐藏控制台窗口，否则每次检测会闪黑框。
func TestNewCommandHidesWindow(t *testing.T) {
	cmd := NewCommand(context.Background(), "ffmpeg.exe", "-version")
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
		t.Fatalf("Windows 上必须设置 HideWindow: %+v", cmd.SysProcAttr)
	}
}
