//go:build !windows

package proc

import (
	"os/exec"
	"testing"
	"time"
)

// 非 Windows 上 Start / Run 就是 cmd.Start / cmd.Run，Kill 结束整个进程组（行为不变）。
func TestStartRunKillUnix(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30 & wait")
	Configure(cmd)
	if err := Start(cmd); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	time.Sleep(100 * time.Millisecond)
	if err := Kill(cmd); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Kill 后进程没有退出")
	}

	ok := exec.Command("sh", "-c", "exit 0")
	Configure(ok)
	if err := Run(ok); err != nil {
		t.Fatalf("Run: %v", err)
	}
	bad := exec.Command("sh", "-c", "exit 3")
	Configure(bad)
	if err := Run(bad); err == nil {
		t.Fatal("非零退出应返回错误")
	}
	if err := Start(exec.Command("/nonexistent/binary")); err == nil {
		t.Fatal("启动失败应返回错误")
	}
}
