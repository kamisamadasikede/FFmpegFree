//go:build !windows

package live

import (
	"os"
	"testing"

	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/task"
)

// 真实 ffmpeg + MediaMTX，CPU 偏好：文件推流走 libx264，任务带 libx264 / cpu，能从服务器拉到音视频。
func TestIntegrationHWCPUPathPush(t *testing.T) {
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	r.svc.cfg.Encoder = resolverOf("libx264", "cpu", false)
	tk, err := r.push(t, r.withAudio, m.rtmpURL("live/cpu"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Encoder != "libx264" || tk.EncoderDevice != "cpu" || tk.HWFallback {
		t.Fatalf("%+v", tk)
	}
	r.waitProgress(t, tk.ID)
	if n := r.probeStream(t, m.rtmpURL("live/cpu")); n < 2 {
		b, _ := os.ReadFile(m.logPath)
		t.Fatalf("应读到音视频两路，实际 %d：%s", n, b)
	}
	r.mgr.Cancel(tk.ID)
	d := r.wait(t, tk.ID)
	if d.Status != task.StatusSucceeded || d.Error != nil || d.Encoder != "libx264" || d.HWFallback {
		t.Fatalf("%+v %+v", d, d.Error)
	}
}

// 真实 ffmpeg（有 h264_nvenc 编码器但没有 GPU）+ MediaMTX：选 NVIDIA → NVENC 初始化失败（推流前）→ 自动 CPU 重试，推流建立。
func TestIntegrationHWFailureFallsBackAndPushes(t *testing.T) {
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	r.svc.cfg.Encoder = resolverOf("h264_nvenc", "nvidia", false)
	tk, err := r.push(t, r.withAudio, m.rtmpURL("live/fb"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Encoder != "h264_nvenc" {
		t.Fatalf("提交时: %+v", tk)
	}
	r.waitProgress(t, tk.ID)
	cur, _ := r.mgr.Get(tk.ID)
	if cur.Encoder != "libx264" || cur.EncoderDevice != "cpu" || !cur.HWFallback || cur.HWFallbackReason == "" {
		t.Skipf("本机 ffmpeg 没有 h264_nvenc 编码器或该机器真有 GPU，跳过：%+v", cur)
	}
	if n := r.probeStream(t, m.rtmpURL("live/fb")); n < 2 {
		t.Fatalf("回退后应能拉到音视频，实际 %d", n)
	}
	r.mgr.Cancel(tk.ID)
	d := r.wait(t, tk.ID)
	if d.Status != task.StatusSucceeded || d.Error != nil {
		t.Fatalf("%+v %+v", d, d.Error)
	}
	_ = ffmpeg.ReasonNVENCInit
}
