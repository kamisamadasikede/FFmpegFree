// Package proc 统一管理子进程的平台相关属性。
//
// 所有启动 ffmpeg 等外部程序的地方都应调用 Configure，
// 不要在业务代码里直接写 syscall.SysProcAttr，否则只能在某一个平台上编译。
package proc

import "os/exec"

// Configure 在 cmd.Start 之前调用：
// Windows 上隐藏控制台窗口；macOS 和 Linux 上让子进程单独成组，便于整组结束。
func Configure(cmd *exec.Cmd) {
	configure(cmd)
}

// Kill 结束子进程。macOS 和 Linux 上会连同它派生的进程一起结束。
func Kill(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return kill(cmd)
}
