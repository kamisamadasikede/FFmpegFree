package edit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/task"
)

func choice(enc, dev string, fb bool) ffmpeg.EncoderResolver {
	return func(context.Context, string) ffmpeg.EncoderChoice {
		return ffmpeg.EncoderChoice{Encoder: enc, Device: dev, Fallback: fb}
	}
}

// 各厂商 / CPU / webm 的导出 argv 与编码器字段。
func TestExportEncoderSelection(t *testing.T) {
	s := fakeSvc(base)
	pl, _ := s.build(context.Background(), proj(vclip("c1", pV, "V1", 0, 0, 2)))
	bin := ffmpeg.Binaries{FFmpeg: "/x/ffmpeg"}
	cases := []struct {
		name    string
		format  string
		r       ffmpeg.EncoderResolver
		wantEnc string
		wantDev string
		wantFB  bool
		want    string
		not     string
	}{
		{"nvidia mp4", "mp4", choice("h264_nvenc", "nvidia", false), "h264_nvenc", "nvidia", false, "-c:v h264_nvenc -preset p4 -rc vbr -cq 20 -b:v 0 -pix_fmt yuv420p -c:a aac -b:a 192k -movflags +faststart", "libx264"},
		{"intel mov", "mov", choice("h264_qsv", "intel", false), "h264_qsv", "intel", false, "-c:v h264_qsv -preset medium -global_quality 20 -pix_fmt nv12", "libx264"},
		{"amd mkv", "mkv", choice("h264_amf", "amd", false), "h264_amf", "amd", false, "-c:v h264_amf -quality balanced -rc cqp -qp_i 20 -qp_p 20", "libx264"},
		{"apple mp4", "mp4", choice("h264_videotoolbox", "apple", false), "h264_videotoolbox", "apple", false, "-c:v h264_videotoolbox -q:v 68", "libx264"},
		{"cpu mp4", "mp4", choice("libx264", "cpu", false), "libx264", "cpu", false, "-c:v libx264 -preset medium -crf 20 -pix_fmt yuv420p -c:a aac -b:a 192k", "nvenc"},
		{"设备不可用", "mp4", choice("libx264", "cpu", true), "libx264", "cpu", true, "-c:v libx264", "nvenc"},
		{"webm 恒 CPU", "webm", choice("h264_nvenc", "nvidia", false), "libvpx-vp9", "cpu", false, "-c:v libvpx-vp9", "nvenc"},
		{"无解析器", "mp4", nil, "libx264", "cpu", false, "-c:v libx264", "nvenc"},
	}
	for _, c := range cases {
		s.cfg.Encoder = c.r
		p := *pl
		p.format = c.format
		r := s.newRunner(bin, &p, "/o/x."+c.format).(*exportRunner)
		info := r.EncoderInfo()
		if info.Encoder != c.wantEnc || info.Device != c.wantDev || info.HWFallback != c.wantFB {
			t.Errorf("%s: %+v", c.name, info)
		}
		if c.wantFB && info.HWFallbackReason != ffmpeg.ReasonDeviceUnavailable {
			t.Errorf("%s: reason %q", c.name, info.HWFallbackReason)
		}
		j := " " + strings.Join(exportArgsHW(&p, "/tmp/g.txt", "/o/x.part."+c.format, r.hw), " ") + " "
		if !strings.Contains(j, c.want) || strings.Contains(j, c.not) {
			t.Errorf("%s:\n%s", c.name, j)
		}
		if !strings.Contains(j, " -/filter_complex /tmp/g.txt ") || !strings.Contains(j, " -map [vout] -map [aout] ") {
			t.Errorf("%s: 滤镜图 / map 不应变化: %s", c.name, j)
		}
	}
}

// 超出 H.264 硬件尺寸（> 4096）：走 CPU，不算回退。
func TestExportHugeOutputUsesCPU(t *testing.T) {
	s := fakeSvc(base)
	pl, _ := s.build(context.Background(), proj(vclip("c1", pV, "V1", 0, 0, 2)))
	p := *pl
	p.format, p.w, p.h = "mp4", 7680, 4320
	s.cfg.Encoder = choice("h264_nvenc", "nvidia", false)
	r := s.newRunner(ffmpeg.Binaries{FFmpeg: "/x"}, &p, "/o/x.mp4").(*exportRunner)
	if r.hw != "" || r.info.Encoder != "libx264" || r.info.HWFallback {
		t.Fatalf("%+v", r.info)
	}
}

// 真实 ffmpeg（无 GPU）：CPU 偏好端到端；选 NVIDIA 时硬件失败 → CPU 重试成功，输出 h264。
func TestExportRealCPUAndFallback(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "a.mp4", 2, "320x240")
	p := proj(vclip("c1", v, "V1", 0, 0, 2))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	out := EditExportOptions{OutputDir: filepath.Join(e.dir, "o")}

	e.svc.cfg.Encoder = choice("libx264", "cpu", false)
	d := e.wait(t, e.export(t, p, out).ID)
	if d.Status != task.StatusSucceeded || d.Encoder != "libx264" || d.EncoderDevice != "cpu" || d.HWFallback {
		t.Fatalf("CPU: %+v %+v", d, d.Error)
	}

	e.svc.cfg.Encoder = choice("h264_nvenc", "nvidia", false)
	tk := e.export(t, p, EditExportOptions{OutputDir: filepath.Join(e.dir, "o2")})
	if tk.Encoder != "h264_nvenc" {
		t.Fatalf("提交时: %+v", tk)
	}
	d = e.wait(t, tk.ID)
	if d.Status != task.StatusSucceeded || d.Encoder != "libx264" || d.EncoderDevice != "cpu" || !d.HWFallback || d.HWFallbackReason == "" {
		t.Fatalf("回退: %+v %+v", d, d.Error)
	}
	res := e.probe(t, d.OutputPath)
	if res.count("video") != 1 || res.Streams[0].CodecName != "h264" {
		t.Fatalf("%+v", res.Streams)
	}
}
