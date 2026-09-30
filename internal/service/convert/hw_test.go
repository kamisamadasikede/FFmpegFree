package convert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/task"
)

func fixedResolver(c ffmpeg.EncoderChoice, calls *int) ffmpeg.EncoderResolver {
	return func(context.Context, string) ffmpeg.EncoderChoice {
		if calls != nil {
			*calls++
		}
		return c
	}
}

// 各厂商 × 各场景的 argv 与编码器字段（假设备列表 = 注入的解析器）。
func TestRunnerEncoderSelection(t *testing.T) {
	src := ffmpeg.ConvertSource{DurationSec: 10, HasVideo: true, HasAudio: true}
	bin := ffmpeg.Binaries{FFmpeg: "/x/ffmpeg"}
	h264 := ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264"}
	h265 := ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "h265"}
	nv264 := ffmpeg.EncoderChoice{Encoder: "h264_nvenc", Device: "nvidia"}
	cases := []struct {
		name       string
		o          ffmpeg.ConvertOptions
		choice     ffmpeg.EncoderChoice
		wantEnc    string
		wantDev    string
		wantFB     bool
		wantReason string
		wantArg    string // argv 里应出现
		wantNoArg  string
		wantCalls  int
		wantCPURun bool // 是否有 CPU 回退参数
	}{
		{"nvidia h264", h264, nv264, "h264_nvenc", "nvidia", false, "", "-c:v h264_nvenc -preset p4 -rc vbr -cq 23", "libx264", 1, true},
		{"intel h264", h264, ffmpeg.EncoderChoice{Encoder: "h264_qsv", Device: "intel"}, "h264_qsv", "intel", false, "", "-c:v h264_qsv -preset medium -global_quality 23 -pix_fmt nv12", "libx264", 1, true},
		{"amd h264", h264, ffmpeg.EncoderChoice{Encoder: "h264_amf", Device: "amd"}, "h264_amf", "amd", false, "", "-c:v h264_amf -quality balanced -rc cqp -qp_i 23 -qp_p 23", "libx264", 1, true},
		{"apple h264", h264, ffmpeg.EncoderChoice{Encoder: "h264_videotoolbox", Device: "apple"}, "h264_videotoolbox", "apple", false, "", "-c:v h264_videotoolbox -q:v 62", "libx264", 1, true},
		{"nvidia h265", h265, ffmpeg.EncoderChoice{Encoder: "hevc_nvenc", Device: "nvidia"}, "hevc_nvenc", "nvidia", false, "", "-c:v hevc_nvenc -preset p4 -rc vbr -cq 23", "libx265", 1, true},
		{"cpu 偏好", h264, ffmpeg.EncoderChoice{Encoder: "libx264", Device: "cpu"}, "libx264", "cpu", false, "", "-c:v libx264 -preset medium -crf 23", "nvenc", 1, false},
		{"设备不可用回退", h265, ffmpeg.EncoderChoice{Encoder: "libx265", Device: "cpu", Fallback: true}, "libx265", "cpu", true, ffmpeg.ReasonDeviceUnavailable, "-c:v libx265", "hevc_", 1, false},
		{"copy 不解析", ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "copy"}, nv264, "copy", "", false, "", "-c:v copy", "nvenc", 0, false},
		{"vp9 走 CPU", ffmpeg.ConvertOptions{Container: "webm", VideoCodec: "vp9"}, nv264, "libvpx-vp9", "cpu", false, "", "-c:v libvpx-vp9", "nvenc", 0, false},
		{"gif 走 CPU", ffmpeg.ConvertOptions{Container: "gif"}, nv264, "gif", "cpu", false, "", "palettegen", "nvenc", 0, false},
		{"音频转换 无视频编码器", ffmpeg.ConvertOptions{Container: "mp3"}, nv264, "", "", false, "", "libmp3lame", "nvenc", 0, false},
	}
	for _, c := range cases {
		calls := 0
		s := &Service{cfg: Config{Encoder: fixedResolver(c.choice, &calls)}}
		s2 := src
		if c.o.Container == "mp3" {
			s2.HasVideo = false
		}
		r := s.newRunner(bin, "/in/a.mov", "/out/a.mp4", c.o, 10, s2)
		info := r.EncoderInfo()
		if info.Encoder != c.wantEnc || info.Device != c.wantDev || info.HWFallback != c.wantFB || info.HWFallbackReason != c.wantReason {
			t.Errorf("%s: info=%+v", c.name, info)
		}
		if calls != c.wantCalls {
			t.Errorf("%s: 解析器调用 %d 次，想要 %d", c.name, calls, c.wantCalls)
		}
		argv := " " + strings.Join(r.BuildArgs("/out/a.part.mp4"), " ")
		if !strings.Contains(argv, c.wantArg) {
			t.Errorf("%s: argv 缺少 %q:\n%s", c.name, c.wantArg, argv)
		}
		if strings.Contains(argv, c.wantNoArg) {
			t.Errorf("%s: argv 不应含 %q:\n%s", c.name, c.wantNoArg, argv)
		}
		if (r.BuildCPUArgs != nil) != c.wantCPURun {
			t.Errorf("%s: BuildCPUArgs != nil = %v", c.name, r.BuildCPUArgs != nil)
		}
		if c.wantCPURun {
			cpu := strings.Join(r.BuildCPUArgs("/out/a.part.mp4"), " ")
			if !strings.Contains(cpu, "-c:v libx26") || IsHW(cpu) {
				t.Errorf("%s: CPU 回退参数: %s", c.name, cpu)
			}
		}
	}
	// 没有解析器 = 一律 CPU。
	r := (&Service{}).newRunner(bin, "/in/a.mov", "/out/a.mp4", h264, 10, src)
	if r.EncoderInfo() != (ffmpeg.EncoderInfo{Encoder: "libx264", Device: "cpu"}) || r.HWEncoder != "" {
		t.Errorf("无解析器: %+v", r.EncoderInfo())
	}
}

func IsHW(argv string) bool {
	for _, w := range strings.Fields(argv) {
		if ffmpeg.IsHardwareEncoder(w) {
			return true
		}
	}
	return false
}

// 沙箱真实 ffmpeg（无 GPU）：选了 NVIDIA → 硬件编码启动失败 → 自动 CPU 重试成功，输出是 h264，任务带回退信息。
func TestRealHWFailureFallsBackToCPU(t *testing.T) {
	e := newEnv(t)
	e.svc.cfg.Encoder = fixedResolver(ffmpeg.EncoderChoice{Encoder: "h264_nvenc", Device: "nvidia"}, nil)
	in := e.genVideo(t, filepath.Join(e.dir, "a.mp4"), 2)
	ts := mustSubmit(t, e, []string{in}, ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264"}, filepath.Join(e.dir, "o"))
	if ts[0].Encoder != "h264_nvenc" || ts[0].EncoderDevice != "nvidia" {
		t.Fatalf("提交时: %+v", ts[0])
	}
	d := e.wait(t, ts[0].ID)
	if d.Status != task.StatusSucceeded {
		t.Fatalf("回退后应成功: %+v %+v", d, d.Error)
	}
	if d.Encoder != "libx264" || d.EncoderDevice != "cpu" || !d.HWFallback || d.HWFallbackReason == "" || strings.ContainsAny(d.HWFallbackReason, "/\\ ") {
		t.Fatalf("回退信息: %+v", d)
	}
	if got := codecs(e.probe(t, d.OutputPath)); len(got) == 0 || got[0] != "video:h264" {
		t.Fatalf("输出应是 h264: %v", got)
	}
	// 落库 + Get 一致。
	g, err := e.tm.Get(d.ID)
	if err != nil || g.Encoder != "libx264" || !g.HWFallback {
		t.Fatalf("Get: %+v %v", g, err)
	}
	log, _ := os.ReadFile(g.LogPath)
	if !strings.Contains(string(log), "[FFmpegFree] 显卡编码启动失败，已自动改用 CPU 重试一次") {
		t.Fatalf("日志应记录回退: %s", log)
	}
	// FFmpegFree 自己写的行不带编码器名（ffmpeg 自己的 stderr 里当然有）。
	for _, l := range strings.Split(string(log), "\n") {
		if !strings.Contains(l, "[FFmpegFree]") {
			continue
		}
		low := strings.ToLower(l)
		for _, bad := range []string{"nvenc", "qsv", "amf", "videotoolbox", "h264_", "硬件编码", "转码"} {
			if strings.Contains(low, bad) {
				t.Errorf("FFmpegFree 自己写的日志行不应含 %q: %q", bad, l)
			}
		}
	}
}

// 沙箱真实 ffmpeg：CPU 路径端到端（h264 / h265 / copy），字段正确且不回退。
func TestRealCPUPathEncoderFields(t *testing.T) {
	e := newEnv(t)
	calls := 0
	e.svc.cfg.Encoder = fixedResolver(ffmpeg.EncoderChoice{Encoder: "libx264", Device: "cpu"}, &calls)
	in := e.genVideo(t, filepath.Join(e.dir, "a.mp4"), 2)
	for _, c := range []struct {
		o        ffmpeg.ConvertOptions
		wantEnc  string
		wantDev  string
		wantCode string
	}{
		{ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264"}, "libx264", "cpu", "video:h264"},
		{ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy"}, "copy", "", "video:mpeg4"},
	} {
		ts := mustSubmit(t, e, []string{in}, c.o, filepath.Join(e.dir, "o"))
		d := e.wait(t, ts[0].ID)
		if d.Status != task.StatusSucceeded || d.Encoder != c.wantEnc || d.EncoderDevice != c.wantDev || d.HWFallback || d.HWFallbackReason != "" {
			t.Fatalf("%+v: %+v %+v", c.o, d, d.Error)
		}
		if got := codecs(e.probe(t, d.OutputPath)); got[0] != c.wantCode {
			t.Fatalf("%+v: %v", c.o, got)
		}
	}
}

// Retry：重新解析编码器（偏好变了，新任务用新的）。
func TestRetryResolvesEncoderAgain(t *testing.T) {
	e := newEnv(t)
	in := e.genVideo(t, filepath.Join(e.dir, "a.mp4"), 1)
	cpu := ffmpeg.EncoderChoice{Encoder: "libx264", Device: "cpu"}
	e.svc.cfg.Encoder = fixedResolver(cpu, nil)
	ts := mustSubmit(t, e, []string{in}, ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264"}, filepath.Join(e.dir, "o"))
	e.wait(t, ts[0].ID)
	e.svc.cfg.Encoder = fixedResolver(ffmpeg.EncoderChoice{Encoder: "h264_nvenc", Device: "nvidia"}, nil)
	nt, err := e.tm.Retry(ts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if nt.Encoder != "h264_nvenc" || nt.EncoderDevice != "nvidia" {
		t.Fatalf("Retry 应重新解析: %+v", nt)
	}
	d := e.wait(t, nt.ID)
	if d.Status != task.StatusSucceeded || !d.HWFallback {
		t.Fatalf("%+v", d)
	}
}
