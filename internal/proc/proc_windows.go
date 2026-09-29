//go:build windows

package proc

import (
	"os/exec"
	"strconv"
	"syscall"
)

func configure(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}

// kill 结束整个进程树。Windows 没有进程组信号，用系统自带的 taskkill /T /F 连同子孙进程一起结束
// （ffmpeg 通过 shell 包装或滤镜派生子进程时，只 Kill 主进程会留下孤儿）。
// taskkill 不可用或失败时退回只结束主进程。
func kill(cmd *exec.Cmd) error {
	pid := cmd.Process.Pid
	tk := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	tk.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := tk.Run(); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func interrupt(cmd *exec.Cmd) error { return ErrInterruptUnsupported }
