//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
)

func configure(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func kill(cmd *exec.Cmd) error {
	// 负数 pid 表示整个进程组。
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func interrupt(cmd *exec.Cmd) error {
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err == nil {
		return nil
	}
	return cmd.Process.Signal(syscall.SIGINT)
}
