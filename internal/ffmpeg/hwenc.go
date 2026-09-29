package ffmpeg

import (
	"context"
	"strconv"
	"strings"
)

// 本文件集中放"硬件编码接入"的纯函数（契约 v0.18 的 9.7）：编码器决策、CRF → 各硬件质量参数的映射、
// 转换 / 剪辑 / 直播的硬件编码参数、硬件初始化失败的特征匹配。不碰系统，方便表驱动测试。

// EncoderInfo 是一个任务实际使用的视频编码器信息，会写进 Task 的 encoder / encoderDevice / hwFallback / hwFallbackReason。
type EncoderInfo struct {
	Encoder          string // 如 h264_nvenc / libx264 / libx265 / libvpx-vp9 / copy；没有视频编码的任务（纯音频转换）为空
	Device           string // 设备 id（如 nvidia、intel）；CPU 编码为 "cpu"；Encoder 为空时也为空
	HWFallback       bool   // 想用硬件但实际用了 CPU（设备不可用，或硬件编码启动失败后自动改用 CPU 重试）
	HWFallbackReason string // 一行短原因（固定枚举，不含路径），HWFallback 为 false 时为空
}

// 回退原因枚举（HWFallbackReason 的取值）。
const (
	ReasonDeviceUnavailable = "device_unavailable"   // 选中的设备不可用（解析时就回退 CPU）
	ReasonNVENCInit         = "nvenc_init_failed"    // NVENC 初始化失败
	ReasonQSVInit           = "qsv_init_failed"      // Quick Sync 初始化失败
	ReasonAMFInit           = "amf_init_failed"      // AMF 初始化失败
	ReasonVTInit            = "videotoolbox_failed"  // VideoToolbox 初始化失败
	ReasonEncoderMissing    = "encoder_unavailable"  // 当前 ffmpeg 没有该硬件编码器
	ReasonEncoderStart      = "encoder_start_failed" // 硬件编码器启动阶段失败（未归入以上厂商特征）
)

// EncoderChoice 是编码器解析结果（生产实现 = system.ResolveEncoder + 设备缓存）。
type EncoderChoice struct {
	Encoder  string // h264_nvenc / libx264 …
	Device   string // 设备 id；CPU 为 "cpu"
	Fallback bool   // 用户选了具体设备但它不可用，已回退 CPU
}

// EncoderResolver 按当前偏好与设备缓存解析编码器。codec 为 "h264" 或 "hevc"。任务提交 / 启动时调用一次。
// 为 nil 或返回空 Encoder 表示 CPU。
type EncoderResolver func(ctx context.Context, codec string) EncoderChoice

// hwSuffixVendor 是硬件编码器名后缀 → 厂商（与 system 包的 VendorXxx 取值一致）。
var hwSuffixVendor = []struct{ suffix, vendor string }{
	{"_nvenc", "nvidia"}, {"_qsv", "intel"}, {"_amf", "amd"}, {"_videotoolbox", "apple"},
}

// HWVendor 返回硬件编码器的厂商（nvidia | intel | amd | apple），不是硬件编码器返回 ""。
func HWVendor(encoder string) string {
	for _, s := range hwSuffixVendor {
		if strings.HasPrefix(encoder, "h264") || strings.HasPrefix(encoder, "hevc") {
			if strings.HasSuffix(encoder, s.suffix) {
				return s.vendor
			}
		}
	}
	return ""
}

// IsHardwareEncoder 判断编码器名是不是硬件编码器。
func IsHardwareEncoder(encoder string) bool { return HWVendor(encoder) != "" }

// CPUEncoderName 返回 CPU 编码器名："h264" → libx264，"hevc" / "h265" → libx265。
func CPUEncoderName(codec string) string {
	switch codec {
	case "h264":
		return "libx264"
	case "hevc", "h265":
		return "libx265"
	}
	return ""
}

// DecideEncoding 决定一次 H.264 / HEVC 重编码用什么编码器。eligible 为 false（-c copy、非 H.264/HEVC 目标、
// 两遍编码、硬件不兼容的参数）时直接 CPU，且不算回退、不调用解析器。
// 返回 hw（硬件编码器名，CPU 为 ""）和初始的 EncoderInfo。用户选的设备不可用时 CPU 且 HWFallback=true、原因 device_unavailable。
func DecideEncoding(ctx context.Context, r EncoderResolver, codec string, eligible bool) (hw string, info EncoderInfo) {
	cpu := CPUEncoderName(codec)
	info = EncoderInfo{Encoder: cpu, Device: "cpu"}
	if !eligible || r == nil {
		return "", info
	}
	c := r(ctx, codec)
	if IsHardwareEncoder(c.Encoder) {
		return c.Encoder, EncoderInfo{Encoder: c.Encoder, Device: c.Device}
	}
	if c.Fallback {
		info.HWFallback, info.HWFallbackReason = true, ReasonDeviceUnavailable
	}
	return "", info
}

// ---------- CRF → 硬件质量参数 ----------

// EquivCRF 把 CRF 换算成"x264 等价 CRF"：libx265 的 CRF 比 libx264 大约高 5 才是相近画质（默认 28 ≈ 23）。
// 各硬件编码器的质量刻度按 x264 等价值给。结果限制在 1~51（0 在 NVENC 里表示"不启用恒定质量"）。
func EquivCRF(codec string, crf int) int {
	v := crf
	if codec == "hevc" || codec == "h265" {
		v = crf - 5
	}
	if v < 1 {
		v = 1
	}
	if v > 51 {
		v = 51
	}
	return v
}

// VTQuality 把 x264 等价 CRF 换算成 VideoToolbox 的 -q:v（1~100，越大越好）：q = 108 - 2×crf（18→72，23→62，28→52）。
// 经验映射，没有在真机上校准。
func VTQuality(eq int) int {
	q := 108 - 2*eq
	if q < 1 {
		q = 1
	}
	if q > 100 {
		q = 100
	}
	return q
}

// HWPixFmt 是各硬件编码器接受的软件像素格式：QSV 不接受 yuv420p（要 nv12），其余用 yuv420p。都是 8bit 4:2:0，
// 10bit 输入会被 -pix_fmt 降成 8bit（转换 / 剪辑本来就是这么做的），所以不存在"位深不兼容"。
func HWPixFmt(encoder string) string {
	if HWVendor(encoder) == "intel" {
		return "nv12"
	}
	return "yuv420p"
}

// hwMaxH264Dim 是 H.264 硬件编码的宽 / 高上限（NVENC、AMF、QSV 的 H.264 都是 4096），超过时该任务走 CPU。
const hwMaxH264Dim = 4096

// HWDimsOK 判断输出尺寸是否在硬件编码器能力内；0 表示"沿用输入"，无法预判，视为可以（启动失败时会自动回退 CPU）。
func HWDimsOK(codec string, w, h int) bool {
	if codec == "h264" {
		return w <= hwMaxH264Dim && h <= hwMaxH264Dim
	}
	return true
}

// HWRateArgs 生成转换 / 剪辑导出用的硬件编码参数（含 -c:v 与 -pix_fmt，不含 hvc1 标签）。
// codec 是 "h264" 或 "hevc"；crf 是转换里已确定的 CRF（默认值也要传，h264 23 / h265 28 / 剪辑 20）；bitrate > 0 时用码率（bit/s）而不是质量。
//
//	nvenc:        -c:v X -preset p4 -rc vbr -cq <eq> -b:v 0          | 码率： -rc vbr -b:v <b>
//	qsv:          -c:v X -preset medium -global_quality <eq>          | 码率： -b:v <b>
//	amf:          -c:v X -quality balanced -rc cqp -qp_i <eq> -qp_p <eq> | 码率： -rc vbr_peak -b:v <b>
//	videotoolbox: -c:v X -q:v <VTQuality(eq)>                         | 码率： -b:v <b>
func HWRateArgs(encoder, codec string, crf int, bitrate int64) []string {
	eq := strconv.Itoa(EquivCRF(codec, crf))
	a := []string{"-c:v", encoder}
	br := strconv.FormatInt(bitrate, 10)
	switch HWVendor(encoder) {
	case "nvidia":
		a = append(a, "-preset", "p4", "-rc", "vbr")
		if bitrate > 0 {
			a = append(a, "-b:v", br)
		} else {
			a = append(a, "-cq", eq, "-b:v", "0")
		}
	case "intel":
		a = append(a, "-preset", "medium")
		if bitrate > 0 {
			a = append(a, "-b:v", br)
		} else {
			a = append(a, "-global_quality", eq)
		}
	case "amd":
		a = append(a, "-quality", "balanced")
		if bitrate > 0 {
			a = append(a, "-rc", "vbr_peak", "-b:v", br)
		} else {
			a = append(a, "-rc", "cqp", "-qp_i", eq, "-qp_p", eq)
		}
	case "apple":
		if bitrate > 0 {
			a = append(a, "-b:v", br)
		} else {
			a = append(a, "-q:v", strconv.Itoa(VTQuality(EquivCRF(codec, crf))))
		}
	}
	return append(a, "-pix_fmt", HWPixFmt(encoder))
}

// HWLiveArgs 生成直播的硬件编码参数（H.264，低延迟、恒定码率）：-g 关键帧间隔（2 秒）、-bf 0 不用 B 帧、
// -b:v / -maxrate / -bufsize 控码率。gop 已换算成帧数，kbps 是视频码率。
//
//	nvenc:        -preset p4 -tune ll -rc cbr
//	qsv:          -preset veryfast -async_depth 1
//	amf:          -usage lowlatency -rc cbr
//	videotoolbox: -realtime 1
func HWLiveArgs(encoder string, kbps, gop int) []string {
	k := strconv.Itoa(kbps) + "k"
	buf := strconv.Itoa(kbps*2) + "k"
	a := []string{"-c:v", encoder}
	switch HWVendor(encoder) {
	case "nvidia":
		a = append(a, "-preset", "p4", "-tune", "ll", "-rc", "cbr")
	case "intel":
		a = append(a, "-preset", "veryfast", "-async_depth", "1")
	case "amd":
		a = append(a, "-usage", "lowlatency", "-rc", "cbr")
	case "apple":
		a = append(a, "-realtime", "1")
	}
	return append(a, "-pix_fmt", HWPixFmt(encoder), "-b:v", k, "-maxrate", k, "-bufsize", buf,
		"-g", strconv.Itoa(gop), "-bf", "0")
}

// ---------- 硬件初始化失败特征 ----------

// vendorSigs 是各厂商硬件初始化失败的 stderr 特征（小写子串，任意一条命中）。
// 一部分来自各厂商文档 / 常见报错，**没有在真机上逐一验证**。
var vendorSigs = map[string][]string{
	"nvidia": {"no nvenc capable devices", "cannot load libcuda", "cannot load nvcuda", "failed loading nvcuda", "driver does not support the required nvenc api",
		"minimum required nvidia driver", "openencodesessionex failed", "cuda_error_no_device", "no cuda-capable device", "incompatible client key",
		"cuinit failed", "nvenc api version"},
	"intel": {"error initializing an mfx session", "mfx session", "no device available for qsv", "failed to create a qsv", "unsupported qsv",
		"qsv: device", "init failed: qsv", "libmfx", "libvpl"},
	"amd":   {"failed to create amf", "amf failed", "amfrt", "amf dll", "dll amf", "amf: not found", "amfcontext", "init() failed", "createcomponent() failed"},
	"apple": {"vtcompressionsessioncreate", "cannot prepare encoder", "videotoolbox: error", "videotoolbox encoder", "videotoolbox failed"},
}

var vendorReason = map[string]string{
	"nvidia": ReasonNVENCInit, "intel": ReasonQSVInit, "amd": ReasonAMFInit, "apple": ReasonVTInit,
}

// HWInitFailure 判断 ffmpeg 的 stderr 尾部是不是硬件编码器初始化失败，是则返回固定枚举的原因。
// 只看 encoder 所属厂商的特征，加上"编码器不存在"和"打开编码器失败"两个通用特征。
func HWInitFailure(encoder, stderrTail string) (reason string, ok bool) {
	v := HWVendor(encoder)
	if v == "" {
		return "", false
	}
	low := strings.ToLower(stderrTail)
	for _, s := range vendorSigs[v] {
		if strings.Contains(low, s) {
			return vendorReason[v], true
		}
	}
	// 只在提到本编码器时才匹配 amf / qsv / videotoolbox 这类过短的词，避免误伤。
	if v == "amd" && strings.Contains(low, "amf") && containsAny(low, "fail", "not found", "dll") {
		return ReasonAMFInit, true
	}
	if v == "intel" && strings.Contains(low, "qsv") && containsAny(low, "device", "session", "unsupported", "fail") {
		return ReasonQSVInit, true
	}
	if v == "apple" && strings.Contains(low, "videotoolbox") && containsAny(low, "error", "fail", "cannot") {
		return ReasonVTInit, true
	}
	if strings.Contains(low, "unknown encoder") || (strings.Contains(low, "encoder") && strings.Contains(low, "not found")) {
		return ReasonEncoderMissing, true
	}
	if strings.Contains(low, "error while opening encoder") || strings.Contains(low, "error initializing output stream") ||
		strings.Contains(low, "could not open encoder") {
		return ReasonEncoderStart, true
	}
	return "", false
}

// HWStartCrash 判断"起始阶段崩溃且输出未产生"：还没有任何进度、输出文件不存在或为空，且 stderr 里有一行
// 同时提到这个硬件编码器和错误字样（不含 "Stream #" 的流映射行——那一行在任何失败里都会出现，例如推流连接被拒绝）。
// 要求提到编码器是为了不把输入损坏、磁盘满、连接失败之类与编码器无关的失败当成硬件问题。
func HWStartCrash(encoder, stderrTail string, started, outputProduced bool) bool {
	if started || outputProduced || !IsHardwareEncoder(encoder) {
		return false
	}
	for _, l := range strings.Split(strings.ToLower(stderrTail), "\n") {
		if !strings.Contains(l, encoder) || strings.Contains(l, "stream #") {
			continue
		}
		if containsAny(l, "error", "fail", "cannot", "can't", "unable", "not supported", "unsupported", "invalid") {
			return true
		}
	}
	return false
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}
