//go:build windows

package proc

import (
	"os/exec"
	"syscall"
)

func configure(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}

func kill(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

func interrupt(cmd *exec.Cmd) error { return ErrInterruptUnsupported }
