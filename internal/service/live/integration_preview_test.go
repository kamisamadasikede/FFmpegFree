//go:build !windows

package live

// 预览画面的端到端验证：真实 ffmpeg + 本地 MediaMTX（+ Xvfb 屏幕采集），经 LiveService 推流 / 拉流，取 GetPreview 的 JPEG，
// 检查宽 640、不是黑屏 / 纯色（亮度方差），并把图片存到 FFMPEGFREE_PREVIEW_OUT（默认测试临时目录）。
//   FFMPEGFREE_FFMPEG=/workspace/tools/ffmpeg-9.0.2/ffmpeg go test -run IntegrationPreview -v ./internal/service/live

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/task"
)

type previewEnv struct {
	*realFx
	out string // 图片保存目录
	tag string
}

func newPreviewEnv(t *testing.T) *previewEnv {
	r := newRealFixture(t, 0)
	r.svc.cfg.PreviewDir = filepath.Join(r.dir, "tmp", PreviewDirName)
	r.svc.cfg.Logf = func(f string, a ...any) { t.Logf("[live] "+f, a...) }
	out := os.Getenv("FFMPEGFREE_PREVIEW_OUT")
	if out == "" {
		out = t.TempDir()
	}
	os.MkdirAll(out, 0o755)
	tag := "ffmpeg"
	if v, err := exec.Command(r.ffmpeg, "-version").Output(); err == nil {
		f := strings.Fields(string(v))
		if len(f) >= 3 {
			tag = strings.SplitN(f[2], "-", 2)[0]
		}
	}
	return &previewEnv{realFx: r, out: out, tag: tag}
}

// waitFrame 轮询 GetPreview 直到拿到画面，返回解码后的图、原始 JPEG 和等待时长。
func (e *previewEnv) waitFrame(t *testing.T, sid string, within time.Duration) (image.Image, []byte, time.Duration) {
	t.Helper()
	begin := time.Now()
	for time.Since(begin) < within {
		p, err := e.svc.GetPreview(sid)
		if err != nil {
			t.Fatal(err)
		}
		if p.Data != "" {
			raw, err := base64.StdEncoding.DecodeString(p.Data)
			if err != nil {
				t.Fatalf("base64: %v", err)
			}
			img, err := jpeg.Decode(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("GetPreview 返回的不是合法 JPEG: %v", err)
			}
			if p.TS <= 0 || !p.Active {
				t.Fatalf("有画面时 ts 应大于 0 且 active: %+v", p)
			}
			return img, raw, time.Since(begin)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%v 内没有拿到预览画面", within)
	return nil, nil, 0
}

// lumaStats 返回亮度均值和标准差（0~255）。全黑 / 纯色的标准差接近 0。
func lumaStats(img image.Image) (mean, std float64) {
	b := img.Bounds()
	var sum, sum2, n float64
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bl, _ := img.At(x, y).RGBA()
			l := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)) / 257
			sum += l
			sum2 += l * l
			n++
		}
	}
	mean = sum / n
	v := sum2/n - mean*mean
	if v < 0 {
		v = 0
	}
	return mean, sqrt(v)
}

func sqrt(v float64) float64 {
	z := v
	if z == 0 {
		return 0
	}
	for i := 0; i < 40; i++ {
		z = (z + v/z) / 2
	}
	return z
}

// checkReal 断言宽 640、不是黑屏 / 纯色，并存盘。
func (e *previewEnv) checkReal(t *testing.T, name string, img image.Image, raw []byte, took time.Duration, minStd float64) {
	t.Helper()
	b := img.Bounds()
	mean, std := lumaStats(img)
	path := filepath.Join(e.out, e.tag+"-"+name+".jpg")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("[%s] %s: %dx%d，%d 字节，亮度均值 %.1f 标准差 %.1f，首帧耗时 %v，已保存 %s", e.tag, name, b.Dx(), b.Dy(), len(raw), mean, std, took.Round(10*time.Millisecond), path)
	if b.Dx() != 640 || b.Dy() <= 0 || b.Dy()%2 != 0 {
		t.Fatalf("预览宽应为 640、高为偶数: %dx%d", b.Dx(), b.Dy())
	}
	if mean < 5 || std < minStd {
		t.Fatalf("预览像黑屏 / 纯色占位: 均值 %.1f 标准差 %.1f", mean, std)
	}
}

func (e *previewEnv) previewFiles() []string {
	m, _ := filepath.Glob(filepath.Join(e.svc.cfg.PreviewDir, "*"))
	return m
}

func (e *previewEnv) waitNoPreviewFiles(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(e.previewFiles()) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("停止后预览临时文件应被清理: %v", e.previewFiles())
}

// 文件推流（testsrc2 + 正弦音频）→ MediaMTX：GetPreview 2 秒内出画面；主流不受影响；再用拉流预览会话拉回；停止后清理。
func TestIntegrationPreviewFilePushAndPull(t *testing.T) {
	m := startMediaMTX(t)
	e := newPreviewEnv(t)
	url := m.rtmpURL("live/pv")
	tk, err := e.push(t, e.withAudio, url, true, true)
	if err != nil {
		t.Fatal(err)
	}
	e.waitProgress(t, tk.ID)
	img, raw, took := e.waitFrame(t, tk.ID, 2*time.Second)
	e.checkReal(t, "push-file", img, raw, took, 20)
	// 同一个会话后续帧仍在更新（ts 前进）。
	p1, _ := e.svc.GetPreview(tk.ID)
	time.Sleep(1200 * time.Millisecond)
	p2, _ := e.svc.GetPreview(tk.ID)
	if p2.TS <= p1.TS {
		t.Fatalf("预览应持续更新: %d -> %d", p1.TS, p2.TS)
	}
	// 主流不受影响：服务器上仍能读到音视频两路。
	if n := e.probeStream(t, url); n < 2 {
		t.Fatalf("预览不应影响主流，服务器上应有音视频两路，实际 %d", n)
	}

	// 拉流预览：同一个 MediaMTX 地址拉回来。
	ps, err := e.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: url})
	if err != nil || !ps.Preview {
		t.Fatalf("%+v %v", ps, err)
	}
	if strings.Contains(ps.Redacted, "127.0.0.1:1935/live/pv") && strings.Contains(ps.Redacted, "pv") && !strings.Contains(ps.Redacted, "***") {
		t.Fatalf("脱敏地址不应带流名: %s", ps.Redacted)
	}
	pimg, praw, ptook := e.waitFrame(t, ps.ID, 8*time.Second)
	e.checkReal(t, "pull-rtmp", pimg, praw, ptook, 20)
	if err := e.svc.StopPullPreview(ps.ID); err != nil {
		t.Fatal(err)
	}
	if p, _ := e.svc.GetPreview(ps.ID); p.Active || p.Data != "" {
		t.Fatalf("停止后: %+v", p)
	}

	// 停止推流：succeeded（优雅停止不受预览输出影响），临时文件清理。
	begin := time.Now()
	e.mgr.Cancel(tk.ID)
	d := e.wait(t, tk.ID)
	if d.Status != task.StatusSucceeded || d.Error != nil {
		t.Fatalf("优雅停止应 succeeded: %+v %+v\n%s", d, d.Error, tailLines(e.logText(t, tk.ID), 8))
	}
	if el := time.Since(begin); el > 4*time.Second {
		t.Fatalf("停止耗时 %v", el)
	}
	e.waitNoPreviewFiles(t)
	if p, _ := e.svc.GetPreview(tk.ID); p.Active || p.Data != "" {
		t.Fatalf("结束后 GetPreview 应为空: %+v", p)
	}
}

// SRT 推流 + SRT 拉流预览。
func TestIntegrationPreviewSRT(t *testing.T) {
	m := startMediaMTX(t)
	e := newPreviewEnv(t)
	tk, err := e.push(t, e.withAudio, m.srtURL("streamid=publish:live/srtpv"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	e.waitProgress(t, tk.ID)
	img, raw, took := e.waitFrame(t, tk.ID, 2*time.Second)
	e.checkReal(t, "push-srt", img, raw, took, 20)
	ps, err := e.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: m.srtURL("streamid=read:live/srtpv")})
	if err != nil {
		t.Fatal(err)
	}
	pimg, praw, ptook := e.waitFrame(t, ps.ID, 10*time.Second)
	e.checkReal(t, "pull-srt", pimg, praw, ptook, 20)
	e.svc.StopPullPreview(ps.ID)
	e.mgr.Cancel(tk.ID)
	if d := e.wait(t, tk.ID); d.Status != task.StatusSucceeded || d.Error != nil {
		t.Fatalf("%+v %+v", d, d.Error)
	}
	e.waitNoPreviewFiles(t)
}

// preview=false：不加预览输出，GetPreview 恒为空，推流正常。
func TestIntegrationPreviewOffNoPreviewOutput(t *testing.T) {
	m := startMediaMTX(t)
	e := newPreviewEnv(t)
	no := false
	e.svc.cfg.Media = fakeMedia{info: mediaInfo(true)}
	tk, err := e.svc.StartFilePush(context.Background(), FilePushRequest{InputPath: e.withAudio, URL: m.rtmpURL("live/off"), Loop: true, Preview: &no,
		Options: PushOptions{VideoBitrateKbps: 500, Fps: 25}})
	if err != nil {
		t.Fatal(err)
	}
	e.waitProgress(t, tk.ID)
	time.Sleep(1500 * time.Millisecond)
	if p, _ := e.svc.GetPreview(tk.ID); p.Data != "" || !p.Active {
		t.Fatalf("关闭预览时应为空: %+v", p)
	}
	if fs := e.previewFiles(); len(fs) != 0 {
		t.Fatalf("不应产生预览文件: %v", fs)
	}
	if n := e.probeStream(t, m.rtmpURL("live/off")); n < 2 {
		t.Fatalf("主流应正常: %d", n)
	}
	e.mgr.Cancel(tk.ID)
	if d := e.wait(t, tk.ID); d.Status != task.StatusSucceeded {
		t.Fatalf("%+v", d)
	}
}

// showSomethingOnDisplay 在 Xvfb 上放一个带文字、有颜色的窗口，让屏幕采集拿到有内容的画面（否则是纯色背景）。
func showSomethingOnDisplay(t *testing.T, display string) {
	t.Helper()
	env := append(os.Environ(), "DISPLAY="+display)
	if p, err := exec.LookPath("xsetroot"); err == nil {
		cmd := exec.Command(p, "-solid", "#204080")
		cmd.Env = env
		cmd.Run()
	}
	xm, err := exec.LookPath("xmessage")
	if err != nil {
		t.Skip("没有 xmessage，无法在虚拟显示器上放内容")
	}
	cmd := exec.Command(xm, "-geometry", "500x300+120+90", "-fg", "yellow", "-bg", "red", "FFmpegFree preview test\nreal screen content\n0123456789 ABCDEFG")
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		t.Skipf("启动 xmessage 失败: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	time.Sleep(600 * time.Millisecond)
}

// 屏幕推流（Xvfb + x11grab）无存档 / 带 tee 存档：预览在 tee 之外独立一路；存档完整、主流正常。
func TestIntegrationPreviewScreenPush(t *testing.T) {
	for _, archive := range []bool{false, true} {
		name := "screen"
		if archive {
			name = "screen-archive"
		}
		t.Run(name, func(t *testing.T) {
			display := startXvfb(t)
			showSomethingOnDisplay(t, display)
			m := startMediaMTX(t)
			e := newPreviewEnv(t)
			e.svc.cfg.GOOS = "linux"
			e.svc.cfg.Getenv = func(k string) string { return map[string]string{"DISPLAY": display, "XDG_SESSION_TYPE": "x11"}[k] }
			if dv, _ := exec.Command(e.ffmpeg, "-hide_banner", "-devices").Output(); !strings.Contains(string(dv), "x11grab") {
				t.Skip("这个 ffmpeg 没有 x11grab（静态构建常见）")
			}
			out, _ := exec.Command(e.ffmpeg, "-hide_banner", "-protocols").Output()
			if archive && !strings.Contains(string(out), "tee") {
				t.Skip("ffmpeg 没有 tee")
			}
			req := ScreenPushRequest{URL: m.rtmpURL("live/" + name), Audio: "silent", Options: PushOptions{Fps: 10, VideoBitrateKbps: 500}}
			if archive {
				req.ArchiveDir = filepath.Join(e.dir, "archives")
			}
			tk, err := e.svc.StartScreenPush(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			e.waitProgress(t, tk.ID)
			img, raw, took := e.waitFrame(t, tk.ID, 2*time.Second)
			e.checkReal(t, "push-"+name, img, raw, took, 15)
			if n := e.probeStream(t, m.rtmpURL("live/"+name)); n < 2 {
				t.Fatalf("主流应有视频 + 静音音轨: %d", n)
			}
			// 拉流预览拉回屏幕流。
			ps, err := e.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: m.rtmpURL("live/" + name)})
			if err != nil {
				t.Fatal(err)
			}
			pimg, praw, ptook := e.waitFrame(t, ps.ID, 8*time.Second)
			e.checkReal(t, "pull-"+name, pimg, praw, ptook, 15)
			e.svc.StopPullPreview(ps.ID)
			time.Sleep(1500 * time.Millisecond)
			e.mgr.Cancel(tk.ID)
			d := e.wait(t, tk.ID)
			if d.Status != task.StatusSucceeded || d.Error != nil {
				t.Fatalf("%+v %+v\n%s", d, d.Error, tailLines(e.logText(t, tk.ID), 8))
			}
			if archive {
				if d.OutputPath == "" {
					t.Fatal("存档应保留")
				}
				ae := &archiveEnv{realFx: e.realFx, m: m, dir: req.ArchiveDir}
				dur, errs := ae.inspect(t, d.OutputPath)
				if dur < 2 || errs != 0 {
					t.Fatalf("带预览的存档应完整: 时长 %.2f 解码错误行 %d", dur, errs)
				}
				t.Logf("存档 %s 时长 %.2f 秒，解码错误 0", filepath.Base(d.OutputPath), dur)
			}
			e.waitNoPreviewFiles(t)
		})
	}
}
