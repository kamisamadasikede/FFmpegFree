//go:build windows

package proc

import (
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// 以下测试只有在 Windows 上才会运行（本仓库的 CI / 开发机是 Linux，只做了交叉编译，没有真机验证）。

func TestStartAssignsJobAndKillTerminatesTree(t *testing.T) {
	// cmd /c 派生一个 ping 子进程，Kill 应该连子孙一起结束
	cmd := exec.Command("cmd", "/c", "ping -n 30 127.0.0.1 >nul")
	Configure(cmd)
	if err := Start(cmd); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	if _, ok := jobs.get(pid); !ok {
		t.Fatal("进程应已放进 Job Object")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	time.Sleep(300 * time.Millisecond)
	if err := Kill(cmd); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Kill 后进程没有退出")
	}
	deadline := time.Now().Add(2 * time.Second)
	for jobs.len() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if _, ok := jobs.get(pid); ok {
		t.Fatal("进程退出后应释放 Job 登记")
	}
}

func TestRunNormalExitReleasesJob(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit 0")
	Configure(cmd)
	if err := Run(cmd); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for jobs.len() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if jobs.len() != 0 {
		t.Fatal("正常退出后应释放 Job 句柄")
	}
}

func TestConfigureHidesConsoleWindow(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit 0")
	Configure(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
		t.Fatalf("HideWindow 必须为 true: %+v", cmd.SysProcAttr)
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("CreationFlags 必须含 CREATE_NO_WINDOW: %#x", cmd.SysProcAttr.CreationFlags)
	}
}
