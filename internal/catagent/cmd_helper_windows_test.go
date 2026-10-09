//go:build windows

package catagent

import (
	"context"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRunCmdHidesWindowOnWindows(t *testing.T) {
	a := NewBuildAdapter(BuildConfig{DataTemp: t.TempDir()})
	for _, args := range [][]string{{"-v"}, {"models"}, {"-p", "hi"}} {
		cmd := a.runCmd(context.Background(), "grok", args...)
		if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
			t.Fatalf("%v: HideWindow 必须为 true", args)
		}
		if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
			t.Fatalf("%v: 缺 CREATE_NO_WINDOW: %#x", args, cmd.SysProcAttr.CreationFlags)
		}
	}
}
