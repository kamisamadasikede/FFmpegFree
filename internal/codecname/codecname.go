// Package codecname 是所有给人看的编码显示名的唯一来源（契约 6.14.5 / v0.23.4）：
// 参数摘要（paramsSummary）、MediaInfo.videoCodecName / audioCodecName 都从这里取，
// 不再各自“首字母大写”兜底（走查 X3：FFV1 被写成 “Ffv1”）。
package codecname

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// videoNames 是视频编码的显示名（UI 规范：H.264 / H.265 / ProRes……，不用 HEVC、PRORES 这类原样大写）。
// 键是小写的 ffmpeg 编码名（ffprobe codec_name）、编码器名或 ConvertOptions.VideoCodec 的取值。
var videoNames = map[string]string{
	"copy": "原画质",
	"":     "无画面",
	// ConvertOptions.VideoCodec 允许的值（ffmpeg.ValidateConvertOptions：copy h264 h265 vp9 ""）
	"h264": "H.264",
	"h265": "H.265",
	"vp9":  "VP9",
	// 常见的 ffmpeg 编码器 / 编码名
	"libx264":     "H.264",
	"libx264rgb":  "H.264",
	"avc":         "H.264",
	"hevc":        "H.265",
	"libx265":     "H.265",
	"libvpx-vp9":  "VP9",
	"vp8":         "VP8",
	"libvpx":      "VP8",
	"av1":         "AV1",
	"libaom-av1":  "AV1",
	"libsvtav1":   "AV1",
	"librav1e":    "AV1",
	"libdav1d":    "AV1",
	"vvc":         "H.266",
	"libvvenc":    "H.266",
	"h266":        "H.266",
	"mpeg4":       "MPEG-4",
	"libxvid":     "Xvid",
	"msmpeg4v1":   "MPEG-4",
	"msmpeg4v2":   "MPEG-4",
	"msmpeg4v3":   "MPEG-4",
	"msmpeg4":     "MPEG-4",
	"mpeg2video":  "MPEG-2",
	"mpeg1video":  "MPEG-1",
	"h263":        "H.263",
	"h263p":       "H.263",
	"flv1":        "FLV",
	"wmv1":        "WMV",
	"wmv2":        "WMV",
	"wmv3":        "WMV",
	"vc1":         "VC-1",
	"mjpeg":       "MJPEG",
	"jpeg2000":    "JPEG 2000",
	"libopenjpeg": "JPEG 2000",
	"dnxhd":       "DNxHD",
	"dnxhr":       "DNxHR",
	"prores":      "ProRes", // 其他 prores* 变体（prores_ks、prores_aw、prores_videotoolbox）按前缀匹配
	"cfhd":        "CineForm",
	"hap":         "HAP",
	"dvvideo":     "DV",
	"ffv1":        "FFV1",
	"huffyuv":     "HuffYUV",
	"ffvhuff":     "HuffYUV",
	"utvideo":     "Ut Video",
	"magicyuv":    "MagicYUV",
	"rawvideo":    "无压缩",
	"v210":        "V210",
	"qtrle":       "QuickTime RLE",
	"cinepak":     "Cinepak",
	"svq1":        "Sorenson",
	"svq3":        "Sorenson",
	"rv10":        "RealVideo",
	"rv20":        "RealVideo",
	"rv30":        "RealVideo",
	"rv40":        "RealVideo",
	"theora":      "Theora",
	"libtheora":   "Theora",
	"gif":         "GIF",
	"apng":        "APNG",
	"png":         "PNG",
	"bmp":         "BMP",
	"tiff":        "TIFF",
	"webp":        "WebP",
	"libwebp":     "WebP",
}

// audioNames 是音频编码的显示名。pcm_* 一律 “PCM”（按前缀匹配）。
var audioNames = map[string]string{
	"copy":              "原音频",
	"none":              "无声",
	"":                  "无声音",
	"aac":               "AAC",
	"libfdk_aac":        "AAC",
	"aac_latm":          "AAC",
	"mp3":               "MP3",
	"libmp3lame":        "MP3",
	"mp3float":          "MP3",
	"mp2":               "MP2",
	"mp2float":          "MP2",
	"mp1":               "MP1",
	"opus":              "Opus",
	"libopus":           "Opus",
	"vorbis":            "Vorbis",
	"libvorbis":         "Vorbis",
	"flac":              "FLAC",
	"alac":              "ALAC",
	"ac3":               "AC-3",
	"eac3":              "E-AC-3",
	"dts":               "DTS",
	"truehd":            "TrueHD",
	"mlp":               "MLP",
	"wmav1":             "WMA",
	"wmav2":             "WMA",
	"wmapro":            "WMA Pro",
	"wmalossless":       "WMA Lossless",
	"wmavoice":          "WMA Voice",
	"amr_nb":            "AMR",
	"amr_wb":            "AMR-WB",
	"libopencore_amrnb": "AMR",
	"libopencore_amrwb": "AMR-WB",
	"ape":               "APE",
	"wavpack":           "WavPack",
	"tta":               "TTA",
	"speex":             "Speex",
	"libspeex":          "Speex",
	"gsm":               "GSM",
	"gsm_ms":            "GSM",
	"cook":              "RealAudio",
	"ra_144":            "RealAudio",
	"ra_288":            "RealAudio",
	"nellymoser":        "Nellymoser",
	"atrac3":            "ATRAC3",
	"qdm2":              "QDesign",
	"mmf":               "MMF",
	"adpcm_yamaha":      "ADPCM",
}

var hwSuffixes = []string{"_nvenc", "_qsv", "_amf", "_vaapi", "_videotoolbox", "_mf", "_v4l2m2m", "_omx", "_mediacodec", "_cuvid", "_vulkan", "_d3d12va"}

// Video 返回视频编码的显示名：已知编码用固定写法（H.264、H.265、VP9、AV1、ProRes、FFV1、DNxHD、MJPEG……）；
// 硬件编码器后缀（_nvenc / _qsv / _amf / _vaapi / _videotoolbox / _mf …）去掉后再查；prores* 一律 ProRes；
// 表里没有的值去掉 "lib" 前缀后首字母大写（"foo" → "Foo"，契约 v0.23.2 的兜底规则；常见编码都应进表）。
func Video(codec string) string {
	c := strings.ToLower(strings.TrimSpace(codec))
	if n, ok := videoNames[c]; ok {
		return n
	}
	if strings.HasPrefix(c, "prores") {
		return "ProRes"
	}
	for _, suf := range hwSuffixes {
		if base, ok := strings.CutSuffix(c, suf); ok {
			if n, ok := videoNames[base]; ok {
				return n
			}
		}
	}
	return fallback(codec, c)
}

// Audio 返回音频编码的显示名：已知编码用固定写法（AAC、MP3、FLAC、Opus、AC-3、WMA……），pcm_* 一律 PCM，adpcm_* 一律 ADPCM；
// 其他同 Video 的兜底规则。
func Audio(codec string) string {
	c := strings.ToLower(strings.TrimSpace(codec))
	if n, ok := audioNames[c]; ok {
		return n
	}
	switch {
	case strings.HasPrefix(c, "pcm_"):
		return "PCM"
	case strings.HasPrefix(c, "adpcm_"):
		return "ADPCM"
	case strings.HasPrefix(c, "dsd_"):
		return "DSD"
	}
	for _, suf := range []string{"_at", "_mf"} {
		if base, ok := strings.CutSuffix(c, suf); ok {
			if n, ok := audioNames[base]; ok {
				return n
			}
		}
	}
	return fallback(codec, c)
}

func fallback(orig, c string) string {
	c = strings.TrimPrefix(c, "lib")
	r, size := utf8.DecodeRuneInString(c)
	if size == 0 {
		return strings.TrimSpace(orig)
	}
	return string(unicode.ToUpper(r)) + c[size:]
}
