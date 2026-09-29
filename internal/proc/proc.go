// Package proc 统一管理子进程的平台相关属性。
//
// 所有启动 ffmpeg 等外部程序的地方都应调用 Configure，
// 不要在业务代码里直接写 syscall.SysProcAttr，否则只能在某一个平台上编译。
package proc

import (
	"errors"
	"os/exec"
)

// Configure 在 cmd.Start 之前调用：
// Windows 上隐藏控制台窗口；macOS 和 Linux 上让子进程单独成组，便于整组结束。
func Configure(cmd *exec.Cmd) {
	configure(cmd)
}

// Start 等价于 cmd.Start()，Windows 上还会把子进程放进 Job Object（KILL_ON_JOB_CLOSE），
// 应用崩溃时系统会连同 ffmpeg 子孙进程一起回收；其他平台与 cmd.Start() 完全相同。
// 需要"应用崩溃也不留孤儿"的长时间子进程（转换、推流、探测）应用它启动；Configure 仍然要先调用。
func Start(cmd *exec.Cmd) error { return start(cmd) }

// Run 等价于 cmd.Run()，启动方式同 Start。
func Run(cmd *exec.Cmd) error {
	if err := start(cmd); err != nil {
		return err
	}
	return cmd.Wait()
}

// Kill 结束子进程。macOS 和 Linux 上会连同它派生的进程一起结束；Windows 上先终结 Job Object，
// 失败退回 taskkill /T /F，再退回只结束主进程。
func Kill(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return kill(cmd)
}

// Interrupt 请求子进程优雅退出：macOS 和 Linux 上向进程组发 SIGINT（ffmpeg 收到后会写完文件尾再退出）；
// Windows 没有对应机制，返回 ErrInterruptUnsupported，调用方应改用 stdin 发 q 或直接 Kill。
func Interrupt(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return interrupt(cmd)
}

// ErrInterruptUnsupported 表示当前平台不支持 Interrupt。
var ErrInterruptUnsupported = errors.New("当前平台不支持信号中断")
