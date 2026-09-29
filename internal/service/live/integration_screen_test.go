//go:build !windows

package live

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"FFmpegFree/internal/task"
)

// startXvfb 起一个 Xvfb 虚拟显示，返回 DISPLAY（如 ":97"）；没有 Xvfb 就 Skip。
func startXvfb(t *testing.T) string {
	bin := findBin(t, "FFMPEGFREE_XVFB", "Xvfb")
	for n := 90; n < 140; n++ {
		if _, err := os.Stat(fmt.Sprintf("/tmp/.X11-unix/X%d", n)); err == nil {
			continue
		}
		cmd := exec.Command(bin, fmt.Sprintf(":%d", n), "-screen", "0", "800x600x24", "-nolisten", "tcp")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Skipf("启动 Xvfb 失败: %v", err)
		}
		t.Cleanup(func() { syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); cmd.Wait() })
		for i := 0; i < 50; i++ {
			if _, err := os.Stat(fmt.Sprintf("/tmp/.X11-unix/X%d", n)); err == nil {
				return fmt.Sprintf(":%d", n)
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Skip("Xvfb 没有起来")
	}
	t.Skip("找不到空闲的 DISPLAY 号")
	return ""
}

// 屏幕推流（无存档）：Xvfb + x11grab → MediaMTX，拉流确认有视频数据。
func TestIntegrationScreenPushX11(t *testing.T) {
	if _, err := exec.LookPath("xrandr"); err != nil {
		t.Log("没有 xrandr，会退化成整个桌面")
	}
	display := startXvfb(t)
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	r.svc.cfg.GOOS = "linux"
	r.svc.cfg.Getenv = func(k string) string { return map[string]string{"DISPLAY": display, "XDG_SESSION_TYPE": "x11"}[k] }
	c, err := r.svc.GetCaptureCapabilities()
	if err != nil || !c.Supported {
		t.Fatalf("%+v %v", c, err)
	}
	screens, err := r.svc.ListScreens(context.Background())
	if err != nil || len(screens) == 0 {
		t.Fatalf("%+v %v", screens, err)
	}
	t.Logf("screens: %+v", screens)
	tk, err := r.svc.StartScreenPush(context.Background(), ScreenPushRequest{URL: m.rtmpURL("live/screen"), Audio: "silent",
		Options: PushOptions{Fps: 10, VideoBitrateKbps: 500}})
	if err != nil {
		t.Fatal(err)
	}
	r.waitProgress(t, tk.ID)
	if n := r.probeStream(t, m.rtmpURL("live/screen")); n < 2 {
		t.Fatalf("应读到视频 + 静音音轨，实际 %d 路", n)
	}
	cur, _ := r.mgr.Get(tk.ID)
	t.Logf("running snapshot: fps=%v bitrate=%v dropped=%v", cur.Fps, cur.BitrateKbps, cur.DroppedFrames)
	r.mgr.Cancel(tk.ID)
	if d := r.wait(t, tk.ID); d.Status != task.StatusSucceeded || d.Error != nil {
		t.Fatalf("%+v %+v", d, d.Error)
	}
}
