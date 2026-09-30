//go:build windows

package system

import (
	"os/exec"
	"syscall"
)

// startDetached 启动 explorer 并立即返回。
//
// 这里不能用 proc.Configure：它给子进程设了 HideWindow（SW_HIDE），explorer 会沿用这个显示方式，
// 进程启动成功但窗口被隐藏，用户看到的就是"点了没反应"。另外 Go 默认会给含空格的参数整体加引号，
// "/select,C:\a b\x.mp4" 会变成 "\"/select,C:\a b\x.mp4\""，explorer 不认，所以用 CmdLine 手拼命令行。
func startDetached(name string, args ...string) error {
	cmd := exec.Command(name)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: rawCmdLine(name, args)}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // explorer 选中文件时退出码常常是 1，不当作失败
	return nil
}
