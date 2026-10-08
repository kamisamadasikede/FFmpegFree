//go:build !windows

package live

import (
	"FFmpegFree/internal/apperr"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"FFmpegFree/internal/task"
)

// startXvfb 起一个 Xvfb 虚拟显示，返回 DISPLAY（如 ":97"）；没有 Xvfb 就 Skip。
// 用 -displayfd 让 Xvfb 自己挑空闲的显示号（不依赖 /tmp/.X11-unix 里可能残留的旧 socket）；结束时先 SIGTERM 让它清理 socket 和锁文件。
func startXvfb(t *testing.T) string {
	bin := findBin(t, "FFMPEGFREE_XVFB", "Xvfb")
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-displayfd", "3", "-screen", "0", "800x600x24", "-nolisten", "tcp")
	cmd.ExtraFiles = []*os.File{pw}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("启动 Xvfb 失败: %v", err)
	}
	pw.Close()
	t.Cleanup(func() {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
	})
	type res struct {
		n   string
		err error
	}
	ch := make(chan res, 1)
	go func() {
		var b [32]byte
		n, err := pr.Read(b[:])
		ch <- res{strings.TrimSpace(string(b[:n])), err}
	}()
	select {
	case r := <-ch:
		if r.err != nil || r.n == "" {
			t.Skipf("Xvfb 没有报告显示号: %v", r.err)
		}
		return ":" + r.n
	case <-time.After(10 * time.Second):
		t.Skip("Xvfb 启动超时")
	}
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

// ---------- 存档（Xvfb + x11grab + tee → MediaMTX，真实 ffmpeg / ffprobe） ----------

type archiveEnv struct {
	*realFx
	m   *mtx
	dir string
}

func newArchiveEnv(t *testing.T, grace time.Duration) *archiveEnv {
	display := startXvfb(t)
	m := startMediaMTX(t)
	r := newRealFixture(t, grace)
	r.svc.cfg.GOOS = "linux"
	r.svc.cfg.Getenv = func(k string) string { return map[string]string{"DISPLAY": display, "XDG_SESSION_TYPE": "x11"}[k] }
	out, _ := exec.Command(r.ffmpeg, "-hide_banner", "-protocols").Output()
	if !strings.Contains(string(out), "tee") {
		t.Skip("ffmpeg 没有 tee")
	}
	return &archiveEnv{realFx: r, m: m, dir: filepath.Join(r.dir, "archives")}
}

func (e *archiveEnv) start(t *testing.T, url string) task.Task {
	t.Helper()
	tk, err := e.svc.StartScreenPush(context.Background(), ScreenPushRequest{URL: url, Audio: "silent", ArchiveDir: e.dir,
		Options: PushOptions{Fps: 10, VideoBitrateKbps: 500}})
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

// ffprobe 读时长并完整解码一遍，返回 (时长秒, 解码错误行数)。
func (e *archiveEnv) inspect(t *testing.T, path string) (float64, int) {
	t.Helper()
	d, err := probeDuration(context.Background(), e.ffprobe, path)
	if err != nil {
		t.Fatalf("存档应能读出时长: %v", err)
	}
	out, _ := exec.Command(e.ffmpeg, "-v", "error", "-i", "file:"+path, "-f", "null", "-").CombinedOutput()
	return d, len(strings.Fields(string(out)))
}

func (e *archiveEnv) files(t *testing.T) []string {
	ents, _ := os.ReadDir(e.dir)
	var n []string
	for _, x := range ents {
		n = append(n, x.Name())
	}
	return n
}

// 正常停止：succeeded，得到完整存档（可读时长、解码 0 个错误）。
func TestIntegrationArchiveGracefulStopIsComplete(t *testing.T) {
	e := newArchiveEnv(t, 0)
	tk := e.start(t, e.m.rtmpURL("live/arch1"))
	e.waitProgress(t, tk.ID)
	if n := e.probeStream(t, e.m.rtmpURL("live/arch1")); n < 2 {
		t.Fatalf("tee 的网络一路也要有数据: %d 路", n)
	}
	time.Sleep(3 * time.Second)
	cur, _ := e.mgr.Get(tk.ID)
	// v0.25：有预览分支时码率按预览分支收到的字节算，存档会话也有 bitrateKbps（契约 6.10.3.2）。
	if cur.BitrateKbps <= 0 {
		t.Fatalf("有存档 + 预览分支时应有 bitrateKbps: %v", cur.BitrateKbps)
	}
	e.mgr.Cancel(tk.ID)
	d := e.wait(t, tk.ID)
	if d.Status != task.StatusSucceeded || d.Error != nil || d.OutputPath != tk.OutputPath || d.OutputPath == "" {
		t.Fatalf("%+v %+v", d, d.Error)
	}
	dur, errs := e.inspect(t, d.OutputPath)
	if dur < 3 || errs != 0 {
		t.Fatalf("存档应完整: 时长 %.2f 解码错误行 %d", dur, errs)
	}
	t.Logf("存档 %s 时长 %.2f 秒，%s", filepath.Base(d.OutputPath), dur, e.files(t))
}

// 强杀（宽限期 1ms）：存档保留，canceled + outputPath 非空，仍可播放。
func TestIntegrationArchiveKeptAfterForceKill(t *testing.T) {
	e := newArchiveEnv(t, time.Millisecond)
	tk := e.start(t, e.m.rtmpURL("live/arch2"))
	e.waitProgress(t, tk.ID)
	time.Sleep(4 * time.Second)
	e.mgr.Cancel(tk.ID)
	d := e.wait(t, tk.ID)
	if d.Status != task.StatusCanceled || d.Error != nil || d.OutputPath == "" {
		t.Fatalf("强杀且存档保留应 canceled + outputPath 非空: %+v %+v", d, d.Error)
	}
	dur, _ := e.inspect(t, d.OutputPath)
	if dur < 1.5 {
		t.Fatalf("强杀后存档至少有前几秒: %.2f", dur)
	}
	ev := e.em.statusEvents(tk.ID)
	if last := ev[len(ev)-1]; last.Status != task.StatusCanceled || last.OutputPath != d.OutputPath {
		t.Fatalf("终态事件应带 outputPath: %+v", last)
	}
	t.Logf("强杀后存档时长 %.2f 秒", dur)
}

// 强杀过早（只有 moov、没有完整分片）：空壳被删，outputPath 清空。
func TestIntegrationArchiveEarlyKillShellDeleted(t *testing.T) {
	e := newArchiveEnv(t, time.Millisecond)
	tk := e.start(t, e.m.rtmpURL("live/arch3"))
	e.waitProgress(t, tk.ID)
	e.mgr.Cancel(tk.ID)
	d := e.wait(t, tk.ID)
	if d.Status != task.StatusCanceled || d.Error != nil {
		t.Fatalf("%+v %+v", d, d.Error)
	}
	if d.OutputPath == "" {
		if names := e.files(t); len(names) != 0 {
			t.Fatalf("空壳应已删除: %v", names)
		}
		ev := e.em.statusEvents(tk.ID)
		if ev[len(ev)-1].OutputPath != "" {
			t.Fatal("终态事件也应为空")
		}
		return
	}
	// 极少数情况下 ffmpeg 在被杀前已经落了一个完整分片：那就必须能读出时长（保留的是有效存档）
	if _, err := probeDuration(context.Background(), e.ffprobe, d.OutputPath); err != nil {
		t.Fatalf("保留的存档必须可读: %v", err)
	}
	t.Log("被杀前已落盘一个完整分片，存档保留（可读）")
}

// 连接被拒（onfail=abort）：LIVE_CONNECT_FAILED，存档是 0 字节空壳，被删，outputPath 清空。
func TestIntegrationArchiveConnectRefusedShellDeleted(t *testing.T) {
	e := newArchiveEnv(t, 0)
	e.m.stop()
	tk := e.start(t, e.m.rtmpURL("live/SECRETKEYarch"))
	d := e.wait(t, tk.ID)
	if d.Status != task.StatusFailed || d.Error == nil || d.Error.Code != apperr.LiveConnectFailed || firstLine(d.Error.Detail) != "scheme=rtmp" {
		t.Fatalf("%+v %+v", d, d.Error)
	}
	if d.OutputPath != "" || len(e.files(t)) != 0 {
		t.Fatalf("空壳应删除、outputPath 清空: %q %v", d.OutputPath, e.files(t))
	}
	assertNoSecrets(t, d.Error.Detail+d.Title+d.Params+e.logText(t, tk.ID)+e.em.jsonAll(), "SECRETKEYarch")
	t.Logf("detail:\n%s", tailLines(d.Error.Detail, 6))
}

// 同一秒内两次会话重名 → (1)，第一个存档不被覆盖。
func TestIntegrationArchiveSameNameGetsSuffix(t *testing.T) {
	e := newArchiveEnv(t, 0)
	e.svc.cfg.Now = func() time.Time { return fixedNow }
	run := func(path string) task.Task {
		tk := e.start(t, e.m.rtmpURL(path))
		e.waitProgress(t, tk.ID)
		time.Sleep(2500 * time.Millisecond)
		e.mgr.Cancel(tk.ID)
		return e.wait(t, tk.ID)
	}
	d1 := run("live/n1")
	d2 := run("live/n2")
	if filepath.Base(d1.OutputPath) != "screen-20260929-200000.mp4" || filepath.Base(d2.OutputPath) != "screen-20260929-200000(1).mp4" {
		t.Fatalf("%q %q", d1.OutputPath, d2.OutputPath)
	}
	for _, p := range []string{d1.OutputPath, d2.OutputPath} {
		if dur, _ := e.inspect(t, p); dur <= 0 {
			t.Fatal(p)
		}
	}
	// 已有的文件（哪怕内容是别的）不会被 -y 覆盖：预放一个，第三次得 (2)
	os.WriteFile(filepath.Join(e.dir, "screen-20260929-200000(2).mp4"), []byte("precious"), 0o644)
	d3 := run("live/n3")
	if filepath.Base(d3.OutputPath) != "screen-20260929-200000(3).mp4" {
		t.Fatalf("%q", d3.OutputPath)
	}
	if b, _ := os.ReadFile(filepath.Join(e.dir, "screen-20260929-200000(2).mp4")); string(b) != "precious" {
		t.Fatal("已有文件被覆盖")
	}
}

// 存档目录含所有特殊字符：真实 ffmpeg 把文件写在期望位置，目录里没有多余文件。
func TestIntegrationArchiveSpecialCharDir(t *testing.T) {
	e := newArchiveEnv(t, 0)
	e.dir = filepath.Join(e.dir, `录 屏'|[x],y=z;c:d#f?g%h&i(1)`)
	tk := e.start(t, e.m.rtmpURL("live/esc"))
	e.waitProgress(t, tk.ID)
	time.Sleep(2500 * time.Millisecond)
	e.mgr.Cancel(tk.ID)
	d := e.wait(t, tk.ID)
	if d.Status != task.StatusSucceeded || d.OutputPath == "" || filepath.Dir(d.OutputPath) != e.dir {
		t.Fatalf("%+v %+v", d, d.Error)
	}
	if names := e.files(t); len(names) != 1 || names[0] != filepath.Base(d.OutputPath) {
		t.Fatalf("目录里应只有存档一个文件: %v", names)
	}
	if dur, errs := e.inspect(t, d.OutputPath); dur < 2 || errs != 0 {
		t.Fatalf("%.2f %d", dur, errs)
	}
	parent, _ := os.ReadDir(filepath.Dir(e.dir))
	if len(parent) != 1 {
		t.Fatalf("特殊字符没有把路径拆成别的目录 / 文件: %v", parent)
	}
}

// SRT 也能带存档（网络一路是 mpegts）。
func TestIntegrationArchiveSRT(t *testing.T) {
	e := newArchiveEnv(t, 0)
	tk := e.start(t, e.m.srtURL("streamid=publish:live/asrt"))
	e.waitProgress(t, tk.ID)
	if n := e.probeStream(t, e.m.srtURL("streamid=read:live/asrt")); n < 1 {
		t.Fatalf("SRT 拉流应有数据: %d", n)
	}
	time.Sleep(2500 * time.Millisecond)
	e.mgr.Cancel(tk.ID)
	d := e.wait(t, tk.ID)
	if d.Status != task.StatusSucceeded || d.OutputPath == "" {
		t.Fatalf("%+v %+v", d, d.Error)
	}
	if dur, errs := e.inspect(t, d.OutputPath); dur < 2 || errs != 0 {
		t.Fatalf("%.2f %d", dur, errs)
	}
}
