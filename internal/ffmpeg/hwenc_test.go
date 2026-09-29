package ffmpeg

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestEquivCRF(t *testing.T) {
	cases := []struct {
		codec string
		crf   int
		want  int
	}{
		{"h264", 23, 23}, {"h264", 0, 1}, {"h264", 51, 51}, {"h264", 60, 51},
		{"hevc", 28, 23}, {"h265", 28, 23}, {"hevc", 3, 1}, {"hevc", 51, 46},
	}
	for _, c := range cases {
		if got := EquivCRF(c.codec, c.crf); got != c.want {
			t.Errorf("EquivCRF(%s,%d)=%d want %d", c.codec, c.crf, got, c.want)
		}
	}
}

func TestVTQuality(t *testing.T) {
	for eq, want := range map[int]int{18: 72, 23: 62, 28: 52, 51: 6, 1: 100} {
		if got := VTQuality(eq); got != want {
			t.Errorf("VTQuality(%d)=%d want %d", eq, got, want)
		}
	}
	if VTQuality(-100) != 100 || VTQuality(100) != 1 {
		t.Error("边界应限制在 1~100")
	}
}

func TestHWRateArgs(t *testing.T) {
	cases := []struct {
		name    string
		enc     string
		codec   string
		crf     int
		bitrate int64
		want    string
	}{
		{"nvenc h264 crf23", "h264_nvenc", "h264", 23, 0, "-c:v h264_nvenc -preset p4 -rc vbr -cq 23 -b:v 0 -pix_fmt yuv420p"},
		{"nvenc hevc crf28", "hevc_nvenc", "hevc", 28, 0, "-c:v hevc_nvenc -preset p4 -rc vbr -cq 23 -b:v 0 -pix_fmt yuv420p"},
		{"nvenc 码率", "h264_nvenc", "h264", 23, 4000000, "-c:v h264_nvenc -preset p4 -rc vbr -b:v 4000000 -pix_fmt yuv420p"},
		{"qsv h264", "h264_qsv", "h264", 20, 0, "-c:v h264_qsv -preset medium -global_quality 20 -pix_fmt nv12"},
		{"qsv 码率", "hevc_qsv", "hevc", 28, 2000000, "-c:v hevc_qsv -preset medium -b:v 2000000 -pix_fmt nv12"},
		{"amf h264", "h264_amf", "h264", 23, 0, "-c:v h264_amf -quality balanced -rc cqp -qp_i 23 -qp_p 23 -pix_fmt yuv420p"},
		{"amf hevc", "hevc_amf", "hevc", 28, 0, "-c:v hevc_amf -quality balanced -rc cqp -qp_i 23 -qp_p 23 -pix_fmt yuv420p"},
		{"amf 码率", "h264_amf", "h264", 23, 3000000, "-c:v h264_amf -quality balanced -rc vbr_peak -b:v 3000000 -pix_fmt yuv420p"},
		{"vt h264", "h264_videotoolbox", "h264", 23, 0, "-c:v h264_videotoolbox -q:v 62 -pix_fmt yuv420p"},
		{"vt hevc", "hevc_videotoolbox", "hevc", 28, 0, "-c:v hevc_videotoolbox -q:v 62 -pix_fmt yuv420p"},
		{"vt 码率", "h264_videotoolbox", "h264", 23, 5000000, "-c:v h264_videotoolbox -b:v 5000000 -pix_fmt yuv420p"},
	}
	for _, c := range cases {
		got := strings.Join(HWRateArgs(c.enc, c.codec, c.crf, c.bitrate), " ")
		if got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestHWLiveArgs(t *testing.T) {
	cases := map[string]string{
		"h264_nvenc":        "-c:v h264_nvenc -preset p4 -tune ll -rc cbr -pix_fmt yuv420p -b:v 2500k -maxrate 2500k -bufsize 5000k -g 60 -bf 0",
		"h264_qsv":          "-c:v h264_qsv -preset veryfast -async_depth 1 -pix_fmt nv12 -b:v 2500k -maxrate 2500k -bufsize 5000k -g 60 -bf 0",
		"h264_amf":          "-c:v h264_amf -usage lowlatency -rc cbr -pix_fmt yuv420p -b:v 2500k -maxrate 2500k -bufsize 5000k -g 60 -bf 0",
		"h264_videotoolbox": "-c:v h264_videotoolbox -realtime 1 -pix_fmt yuv420p -b:v 2500k -maxrate 2500k -bufsize 5000k -g 60 -bf 0",
	}
	for enc, want := range cases {
		if got := strings.Join(HWLiveArgs(enc, 2500, 60), " "); got != want {
			t.Errorf("%s:\n got %s\nwant %s", enc, got, want)
		}
	}
}

func TestHWVendorAndCPUName(t *testing.T) {
	for enc, v := range map[string]string{"h264_nvenc": "nvidia", "hevc_qsv": "intel", "h264_amf": "amd", "hevc_videotoolbox": "apple", "libx264": "", "libvpx-vp9": "", "": ""} {
		if HWVendor(enc) != v || IsHardwareEncoder(enc) != (v != "") {
			t.Errorf("HWVendor(%q)=%q want %q", enc, HWVendor(enc), v)
		}
	}
	if CPUEncoderName("h264") != "libx264" || CPUEncoderName("hevc") != "libx265" || CPUEncoderName("h265") != "libx265" || CPUEncoderName("vp9") != "" {
		t.Error("CPUEncoderName")
	}
}

func TestDecideEncoding(t *testing.T) {
	calls := 0
	mk := func(c EncoderChoice) EncoderResolver {
		return func(context.Context, string) EncoderChoice { calls++; return c }
	}
	cases := []struct {
		name     string
		r        EncoderResolver
		codec    string
		eligible bool
		wantHW   string
		want     EncoderInfo
		wantCall int
	}{
		{"nvidia", mk(EncoderChoice{Encoder: "h264_nvenc", Device: "nvidia"}), "h264", true, "h264_nvenc", EncoderInfo{Encoder: "h264_nvenc", Device: "nvidia"}, 1},
		{"cpu 偏好", mk(EncoderChoice{Encoder: "libx264", Device: "cpu"}), "h264", true, "", EncoderInfo{Encoder: "libx264", Device: "cpu"}, 1},
		{"设备不可用", mk(EncoderChoice{Encoder: "libx265", Device: "cpu", Fallback: true}), "hevc", true, "", EncoderInfo{Encoder: "libx265", Device: "cpu", HWFallback: true, HWFallbackReason: ReasonDeviceUnavailable}, 1},
		{"不适合硬件", mk(EncoderChoice{Encoder: "h264_nvenc", Device: "nvidia"}), "h264", false, "", EncoderInfo{Encoder: "libx264", Device: "cpu"}, 0},
		{"无解析器", nil, "h264", true, "", EncoderInfo{Encoder: "libx264", Device: "cpu"}, 0},
	}
	for _, c := range cases {
		calls = 0
		hw, info := DecideEncoding(context.Background(), c.r, c.codec, c.eligible)
		if hw != c.wantHW || info != c.want || calls != c.wantCall {
			t.Errorf("%s: hw=%q info=%+v calls=%d", c.name, hw, info, calls)
		}
	}
}

func TestHWInitFailure(t *testing.T) {
	cases := []struct {
		enc, tail, reason string
		ok                bool
	}{
		{"h264_nvenc", "[h264_nvenc @ 0x1] OpenEncodeSessionEx failed: unsupported device (2): (no details)", ReasonNVENCInit, true},
		{"h264_nvenc", "Cannot load libcuda.so.1", ReasonNVENCInit, true},
		{"hevc_nvenc", "[hevc_nvenc @ 0x1] Driver does not support the required nvenc API version. Required: 12.1 Found: 11.0", ReasonNVENCInit, true},
		{"h264_nvenc", "No NVENC capable devices found", ReasonNVENCInit, true},
		{"h264_qsv", "Error initializing an MFX session: -9", ReasonQSVInit, true},
		{"hevc_qsv", "[hevc_qsv @ 0x1] Selected ratecontrol mode is unsupported\nqsv device failure", ReasonQSVInit, true},
		{"h264_amf", "[h264_amf @ 0x1] DLL amfrt64.dll failed to open", ReasonAMFInit, true},
		{"h264_amf", "AMF failed to initialise", ReasonAMFInit, true},
		{"h264_videotoolbox", "Cannot prepare encoder: videotoolbox error", ReasonVTInit, true},
		{"h264_nvenc", "Unknown encoder 'h264_nvenc'", ReasonEncoderMissing, true},
		{"h264_nvenc", "Error while opening encoder for output stream #0:0 - maybe incorrect parameters", ReasonEncoderStart, true},
		// 与硬件无关的失败：不能回退
		{"h264_nvenc", "Invalid data found when processing input", "", false},
		{"h264_nvenc", "No space left on device", "", false},
		{"h264_nvenc", "Connection refused", "", false},
		// 别的厂商的特征不误伤
		{"h264_amf", "No NVENC capable devices found", "", false},
		{"libx264", "Cannot load libcuda.so.1", "", false},
	}
	for _, c := range cases {
		r, ok := HWInitFailure(c.enc, c.tail)
		if r != c.reason || ok != c.ok {
			t.Errorf("HWInitFailure(%s,%q)=(%q,%v) want (%q,%v)", c.enc, c.tail, r, ok, c.reason, c.ok)
		}
	}
}

func TestHWStartCrash(t *testing.T) {
	tail := "Stream #0:0 -> #0:0 (h264 (native) -> h264 (h264_nvenc))\n[h264_nvenc @ 0x1] Error setting option foo\nConversion failed!"
	if !HWStartCrash("h264_nvenc", tail, false, false) {
		t.Error("起始阶段崩溃且提到编码器应回退")
	}
	if HWStartCrash("h264_nvenc", tail, true, false) || HWStartCrash("h264_nvenc", tail, false, true) {
		t.Error("已有进度或已有输出不回退")
	}
	if HWStartCrash("libx264", "libx264 error", false, false) {
		t.Error("CPU 编码器不回退")
	}
	only := "Stream #0:0 -> #0:0 (h264 (native) -> h264 (h264_nvenc))\nConnection refused"
	if HWStartCrash("h264_nvenc", only, false, false) {
		t.Error("只有流映射行提到编码器（如连接被拒绝）不能算硬件失败")
	}
}

func TestConvertHWCodec(t *testing.T) {
	cases := []struct {
		name string
		o    ConvertOptions
		want string
	}{
		{"mp4 h264", ConvertOptions{Container: "mp4", VideoCodec: "h264"}, "h264"},
		{"mkv h265", ConvertOptions{Container: "mkv", VideoCodec: "h265"}, "hevc"},
		{"copy", ConvertOptions{Container: "mp4", VideoCodec: "copy"}, ""},
		{"vp9", ConvertOptions{Container: "webm", VideoCodec: "vp9"}, ""},
		{"gif", ConvertOptions{Container: "gif"}, ""},
		{"音频", ConvertOptions{Container: "mp3"}, ""},
		{"无视频", ConvertOptions{Container: "mp4"}, ""},
		{"两遍编码", ConvertOptions{Container: "mp4", VideoCodec: "h264", TargetSizeMB: 10}, ""},
		{"h264 超 4096", ConvertOptions{Container: "mp4", VideoCodec: "h264", Width: 7680, Height: 4320}, ""},
		{"h265 8K 仍可", ConvertOptions{Container: "mp4", VideoCodec: "h265", Width: 7680, Height: 4320}, "hevc"},
	}
	for _, c := range cases {
		if got := ConvertHWCodec(c.o); got != c.want {
			t.Errorf("%s: %q want %q", c.name, got, c.want)
		}
	}
}

func TestConvertEncoderName(t *testing.T) {
	for _, c := range []struct {
		o    ConvertOptions
		want string
	}{
		{ConvertOptions{Container: "mp4", VideoCodec: "h264"}, "libx264"},
		{ConvertOptions{Container: "mp4", VideoCodec: "h265"}, "libx265"},
		{ConvertOptions{Container: "webm", VideoCodec: "vp9"}, "libvpx-vp9"},
		{ConvertOptions{Container: "mp4", VideoCodec: "copy"}, "copy"},
		{ConvertOptions{Container: "gif"}, "gif"},
		{ConvertOptions{Container: "mp3"}, ""},
		{ConvertOptions{Container: "mp4"}, ""},
	} {
		if got := ConvertEncoderName(c.o); got != c.want {
			t.Errorf("%+v: %q want %q", c.o, got, c.want)
		}
	}
}

// 转换的 argv：各厂商 × h264 / h265，并确认 CPU 路径与原来逐字一致。
func TestPlanConvertHWArgv(t *testing.T) {
	src := ConvertSource{DurationSec: 10, HasVideo: true, HasAudio: true}
	plan := func(o ConvertOptions, hw string) string {
		p, err := PlanConvertHW("/in/a.mov", "/out/a.part.mp4", o, src, hw)
		if err != nil {
			t.Fatal(err)
		}
		return " " + strings.Join(p.Final, " ") + " "
	}
	h264 := ConvertOptions{Container: "mp4", VideoCodec: "h264"}
	h265 := ConvertOptions{Container: "mp4", VideoCodec: "h265", Crf: 30}
	cases := []struct {
		name string
		o    ConvertOptions
		hw   string
		want string
		not  string
	}{
		{"cpu h264", h264, "", " -c:v libx264 -preset medium -crf 23 -pix_fmt yuv420p ", "nvenc"},
		{"nvidia h264", h264, "h264_nvenc", " -c:v h264_nvenc -preset p4 -rc vbr -cq 23 -b:v 0 -pix_fmt yuv420p ", "libx264"},
		{"intel h264", h264, "h264_qsv", " -c:v h264_qsv -preset medium -global_quality 23 -pix_fmt nv12 ", "libx264"},
		{"amd h264", h264, "h264_amf", " -c:v h264_amf -quality balanced -rc cqp -qp_i 23 -qp_p 23 -pix_fmt yuv420p ", "libx264"},
		{"apple h264", h264, "h264_videotoolbox", " -c:v h264_videotoolbox -q:v 62 -pix_fmt yuv420p ", "libx264"},
		{"cpu h265", h265, "", " -c:v libx265 -preset medium -crf 30 -pix_fmt yuv420p -x265-params log-level=error -tag:v hvc1 ", "hevc_"},
		{"nvidia h265", h265, "hevc_nvenc", " -c:v hevc_nvenc -preset p4 -rc vbr -cq 25 -b:v 0 -pix_fmt yuv420p -tag:v hvc1 ", "libx265"},
		{"apple h265", h265, "hevc_videotoolbox", " -c:v hevc_videotoolbox -q:v 58 -pix_fmt yuv420p -tag:v hvc1 ", "libx265"},
		{"码率优先", ConvertOptions{Container: "mp4", VideoCodec: "h264", VideoBitrate: 4000000, Crf: 18}, "h264_nvenc", " -c:v h264_nvenc -preset p4 -rc vbr -b:v 4000000 -pix_fmt yuv420p ", "-cq"},
		{"缩放滤镜保留", ConvertOptions{Container: "mp4", VideoCodec: "h264", Width: 1280, Height: 720}, "h264_amf", " -vf scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2 -c:v h264_amf ", "libx264"},
		{"无缩放补偶数", h264, "h264_nvenc", " -vf crop=trunc(iw/2)*2:trunc(ih/2)*2 -c:v h264_nvenc ", ""},
		// 这些即使传了 hw 也必须走 CPU
		{"copy 走 CPU 路径", ConvertOptions{Container: "mp4", VideoCodec: "copy"}, "h264_nvenc", " -c:v copy ", "nvenc"},
		{"vp9 走 CPU", ConvertOptions{Container: "webm", VideoCodec: "vp9"}, "h264_nvenc", " -c:v libvpx-vp9 ", "nvenc"},
		{"gif 走 CPU", ConvertOptions{Container: "gif"}, "h264_nvenc", "[s0]palettegen", "nvenc"},
		{"h264 8K 走 CPU", ConvertOptions{Container: "mp4", VideoCodec: "h264", Width: 7680, Height: 4320}, "h264_nvenc", " -c:v libx264 ", "nvenc"},
	}
	for _, c := range cases {
		got := plan(c.o, c.hw)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: 缺少 %q\n%s", c.name, c.want, got)
		}
		if c.not != "" && strings.Contains(got, c.not) {
			t.Errorf("%s: 不应包含 %q\n%s", c.name, c.not, got)
		}
	}
	// PlanConvert（无硬件）与 PlanConvertHW(…, "") 一致。
	a, _ := PlanConvert("/in/a.mov", "/out/a.part.mp4", h264, src)
	b, _ := PlanConvertHW("/in/a.mov", "/out/a.part.mp4", h264, src, "")
	if !reflect.DeepEqual(a.Final, b.Final) {
		t.Error("PlanConvert 与 PlanConvertHW(\"\") 应一致")
	}
}

func TestLiveArgsHW(t *testing.T) {
	base := LiveEncode{Width: 1280, Height: 720, GOPFps: 30, VideoKbps: 2500, AudioKbps: 128}
	cpu := strings.Join(liveEncodeArgs(base, true), " ")
	if !strings.Contains(cpu, "-c:v libx264 -preset veryfast -tune zerolatency -pix_fmt yuv420p -b:v 2500k -maxrate 2500k -bufsize 5000k -g 60") {
		t.Errorf("CPU 参数不应变化: %s", cpu)
	}
	for _, hw := range []string{"h264_nvenc", "h264_qsv", "h264_amf", "h264_videotoolbox"} {
		e := base
		e.HW = hw
		got := strings.Join(liveEncodeArgs(e, true), " ")
		for _, w := range []string{"-c:v " + hw, "-b:v 2500k", "-maxrate 2500k", "-bufsize 5000k", "-g 60", "-bf 0", "-c:a aac"} {
			if !strings.Contains(got, w) {
				t.Errorf("%s 缺少 %q: %s", hw, w, got)
			}
		}
		if strings.Contains(got, "libx264") {
			t.Errorf("%s 不应含 libx264: %s", hw, got)
		}
	}
}
