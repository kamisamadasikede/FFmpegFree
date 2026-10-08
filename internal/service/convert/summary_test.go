package convert

import (
	"strings"
	"testing"

	"FFmpegFree/internal/ffmpeg"
)

// 契约 6.14.5（v0.23.1）：paramsSummary 不含容器名；只给宽度时常见宽度显示成 "<高>p"。
func TestParamsSummary(t *testing.T) {
	type O = ffmpeg.ConvertOptions
	cases := []struct {
		name string
		o    O
		want string
	}{
		{"宽 3840", O{Container: "mp4", VideoCodec: "h264", Width: 3840}, "H.264 · 2160p"},
		{"宽 2560", O{Container: "mp4", VideoCodec: "h264", Width: 2560}, "H.264 · 1440p"},
		{"宽 1920", O{Container: "mp4", VideoCodec: "h264", Width: 1920}, "H.264 · 1080p"},
		{"宽 1280", O{Container: "mp4", VideoCodec: "h264", Width: 1280}, "H.264 · 720p"},
		{"宽 854", O{Container: "mp4", VideoCodec: "h265", Width: 854}, "H.265 · 480p"},
		{"不在映射里的宽度", O{Container: "mp4", VideoCodec: "h264", Width: 1000}, "H.264 · 宽 1000"},
		{"宽高都给", O{Container: "mp4", VideoCodec: "h264", Width: 1920, Height: 1080}, "H.264 · 1920×1080"},
		{"只给高", O{Container: "mkv", VideoCodec: "vp9", Height: 720}, "VP9 · 720p"},
		{"原画质", O{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}, "原画质"},
		{"音频码率", O{Container: "mp3", AudioCodec: "mp3", AudioBitrate: 192_000}, "192 kbps"},
		{"gif", O{Container: "gif", Width: 480, Fps: 10, TrimStart: 1}, "宽 480 · 10 fps · 已裁剪"},
		{"视频码率 + 无声", O{Container: "mp4", VideoCodec: "h264", VideoBitrate: 2_500_000, AudioCodec: "none"}, "H.264 · 2.5 Mbps · 无声"},
		{"什么都没设", O{Container: "wav", AudioCodec: "pcm"}, "默认参数"},
	}
	for _, c := range cases {
		got := ParamsSummary(c.o)
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
		up := strings.ToUpper(c.o.Container)
		for _, seg := range strings.Split(got, " · ") {
			if seg == up {
				t.Errorf("%s: 不应含容器名: %q", c.name, got)
			}
		}
	}
	// 内置 1080p / 720p 预设只设了宽度
	for _, p := range builtinPresets() {
		got := ParamsSummary(p.Options)
		switch p.ID {
		case "builtin-mp4-h264-1080p":
			if got != "H.264 · 1080p" {
				t.Errorf("%s: %q", p.ID, got)
			}
		case "builtin-mp4-h264-720p":
			if got != "H.264 · 720p" {
				t.Errorf("%s: %q", p.ID, got)
			}
		}
		if got == "" || strings.HasPrefix(got, strings.ToUpper(p.Options.Container)+" ") {
			t.Errorf("%s: %q", p.ID, got)
		}
	}
	// 最长 80 个字符
	long := ParamsSummary(O{Container: "mp4", VideoCodec: strings.Repeat("x", 200)})
	if n := len([]rune(long)); n != 80 {
		t.Fatalf("最长 80: %d", n)
	}
}

// UI 规范：视频编码显示名统一写法（H.265、ProRes……），不出现 HEVC / PRORES 这类原样大写。
func TestVideoCodecDisplayName(t *testing.T) {
	cases := map[string]string{
		// ConvertOptions.VideoCodec 允许的全部取值
		"": "无画面", "copy": "原画质", "h264": "H.264", "h265": "H.265", "vp9": "VP9",
		// H.264 / H.265
		"libx264": "H.264", "hevc": "H.265", "HEVC": "H.265", "libx265": "H.265", " h265 ": "H.265",
		"h264_nvenc": "H.264", "hevc_nvenc": "H.265", "hevc_qsv": "H.265", "hevc_amf": "H.265", "hevc_videotoolbox": "H.265",
		// ProRes
		"prores": "ProRes", "PRORES": "ProRes", "prores_ks": "ProRes", "prores_aw": "ProRes", "prores_videotoolbox": "ProRes",
		// AV1 / VP9
		"av1": "AV1", "libaom-av1": "AV1", "libsvtav1": "AV1", "av1_nvenc": "AV1", "libvpx-vp9": "VP9", "vp9_vaapi": "VP9",
		// 其他常见
		"mpeg4": "MPEG-4", "dnxhd": "DNxHD", "mjpeg": "MJPEG",
		// 未知值：可读的通用写法，不是原样大写
		"foocodec": "Foocodec", "libfoo": "Foo",
	}
	for in, want := range cases {
		if got := VideoCodecDisplayName(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	// ParamsSummary 里同样生效（视频容器）
	if got := ParamsSummary(ffmpeg.ConvertOptions{Container: "mov", VideoCodec: "prores_ks"}); got != "ProRes" {
		t.Fatalf("%q", got)
	}
	if got := ParamsSummary(ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "hevc", Height: 1080}); got != "H.265 · 1080p" {
		t.Fatalf("%q", got)
	}
	// 所有内置预设的摘要都不含原样大写的编码名
	for _, p := range builtinPresets() {
		got := ParamsSummary(p.Options)
		for _, bad := range []string{"HEVC", "PRORES", "H264", "H265", "LIBX"} {
			if strings.Contains(got, bad) {
				t.Errorf("%s: %q", p.ID, got)
			}
		}
	}
}
