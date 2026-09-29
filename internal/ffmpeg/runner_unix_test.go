//go:build !windows

package ffmpeg

import (
	"context"
	"testing"
)

// 与 windows 版对应：证明 NewCommand 确实经过了 proc.Configure（unix 上表现为单独进程组）。
func TestNewCommandUsesProcConfigure(t *testing.T) {
	cmd := NewCommand(context.Background(), "ffmpeg", "-version")
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatalf("应经过 proc.Configure: %+v", cmd.SysProcAttr)
	}
	if cmd.WaitDelay == 0 {
		t.Fatal("应设置 WaitDelay")
	}
}
