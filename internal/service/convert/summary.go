package convert

import (
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"FFmpegFree/internal/ffmpeg"
)

// summarySep 是 paramsSummary 各段之间的分隔（空格、U+00B7、空格）。
const summarySep = " · "

// defaultSummary 是没有任何参数段时的摘要。
const defaultSummary = "默认参数"

// maxSummaryRunes 是 paramsSummary 的最大长度（字符）。
const maxSummaryRunes = 80

// widthOnlyLabels：只给宽度时这些常见宽度显示成 "<高>p"（契约 v0.23.1，6.14.5）。
var widthOnlyLabels = map[int]string{3840: "2160p", 2560: "1440p", 1920: "1080p", 1280: "720p", 854: "480p"}

// ParamsSummary 按 options 生成给人看的参数摘要（契约 6.14.5，v0.23.1）：**不含容器名**（容器由预设名或输出扩展名体现），
// 依次是视频编码（只对视频容器；gif 不写）、尺寸、帧率、码率、无声、已裁剪，用 " · " 连接，最长 80 个字符。
// 例："H.264 · 1080p"、"原画质"、"192 kbps"、"宽 480 · 10 fps · 已裁剪"。没有任何段时为 "默认参数"（永远非空）。
func ParamsSummary(o ffmpeg.ConvertOptions) string {
	var segs []string
	audio := ffmpeg.IsAudioContainer(o.Container)
	if !audio && o.Container != "gif" {
		segs = append(segs, VideoCodecDisplayName(o.VideoCodec))
	}
	switch {
	case o.Width > 0 && o.Height > 0:
		segs = append(segs, strconv.Itoa(o.Width)+"×"+strconv.Itoa(o.Height))
	case o.Height > 0:
		segs = append(segs, strconv.Itoa(o.Height)+"p")
	case o.Width > 0:
		if l, ok := widthOnlyLabels[o.Width]; ok {
			segs = append(segs, l)
		} else {
			segs = append(segs, "宽 "+strconv.Itoa(o.Width))
		}
	}
	if o.Fps > 0 {
		segs = append(segs, strconv.FormatFloat(o.Fps, 'f', -1, 64)+" fps")
	}
	if !audio && o.VideoBitrate > 0 {
		segs = append(segs, strconv.FormatFloat(float64(o.VideoBitrate)/1e6, 'f', 1, 64)+" Mbps")
	}
	if audio && o.AudioBitrate > 0 {
		segs = append(segs, strconv.FormatInt(int64(math.Round(float64(o.AudioBitrate)/1000)), 10)+" kbps")
	}
	if o.AudioCodec == "none" {
		segs = append(segs, "无声")
	}
	if o.TrimStart > 0 || o.TrimEnd > 0 {
		segs = append(segs, "已裁剪")
	}
	if len(segs) == 0 {
		// 全部取默认值（如 MP3 / WAV / GIF 不设任何参数）：给一个固定文案，保证 paramsSummary 永远非空（契约 v0.23.1）。
		segs = append(segs, defaultSummary)
	}
	s := strings.Join(segs, summarySep)
	if r := []rune(s); len(r) > maxSummaryRunes {
		s = string(r[:maxSummaryRunes])
	}
	return s
}

// videoCodecNames 是视频编码的显示名（UI 规范：H.264 / H.265 / ProRes……，不用 HEVC、PRORES 这类原样大写）。
// 键是小写的 ffmpeg 编码名或 ConvertOptions.VideoCodec 的取值。
var videoCodecNames = map[string]string{
	"copy": "原画质",
	"":     "无画面",
	// ConvertOptions.VideoCodec 允许的值（ffmpeg.ValidateConvertOptions：copy h264 h265 vp9 ""）
	"h264": "H.264",
	"h265": "H.265",
	"vp9":  "VP9",
	// 常见的 ffmpeg 编码器 / 编码名
	"libx264":    "H.264",
	"avc":        "H.264",
	"hevc":       "H.265",
	"libx265":    "H.265",
	"libvpx-vp9": "VP9",
	"vp8":        "VP8",
	"libvpx":     "VP8",
	"av1":        "AV1",
	"libaom-av1": "AV1",
	"libsvtav1":  "AV1",
	"librav1e":   "AV1",
	"mpeg4":      "MPEG-4",
	"mpeg2video": "MPEG-2",
	"mpeg1video": "MPEG-1",
	"mjpeg":      "MJPEG",
	"dnxhd":      "DNxHD",
	"theora":     "Theora",
	"libtheora":  "Theora",
	"gif":        "GIF",
	"h263":       "H.263",
	"wmv2":       "WMV",
	"rawvideo":   "无压缩",
	"ffv1":       "FFV1",
	"libxvid":    "Xvid",
	"cinepak":    "Cinepak",
	"utvideo":    "Ut Video",
	"qtrle":      "QuickTime RLE",
	"png":        "PNG",
	"libwebp":    "WebP",
	"webp":       "WebP",
	"apng":       "APNG",
	"msmpeg4v3":  "MPEG-4",
	"flv1":       "FLV",
	"hap":        "HAP",
	"vvc":        "H.266",
	"libvvenc":   "H.266",
	"h266":       "H.266",
	"cfhd":       "CineForm",
	"dvvideo":    "DV",
	"prores":     "ProRes", // 其他 prores* 变体（prores_ks、prores_aw、prores_videotoolbox）按前缀匹配
}

// VideoCodecDisplayName 返回视频编码的显示名：已知编码用固定写法（H.264、H.265、VP9、AV1、ProRes……）；
// 硬件编码器后缀（_nvenc / _qsv / _amf / _vaapi / _videotoolbox / _mf / _v4l2m2m）去掉后再查；prores* 一律 ProRes；
// 其他未知值去掉 "lib" 前缀后首字母大写（如 "foo" → "Foo"），不再原样全大写。
func VideoCodecDisplayName(codec string) string {
	c := strings.ToLower(strings.TrimSpace(codec))
	if n, ok := videoCodecNames[c]; ok {
		return n
	}
	if strings.HasPrefix(c, "prores") {
		return "ProRes"
	}
	for _, suf := range []string{"_nvenc", "_qsv", "_amf", "_vaapi", "_videotoolbox", "_mf", "_v4l2m2m", "_omx", "_mediacodec"} {
		if base, ok := strings.CutSuffix(c, suf); ok {
			if n, ok := videoCodecNames[base]; ok {
				return n
			}
		}
	}
	c = strings.TrimPrefix(c, "lib")
	r, size := utf8.DecodeRuneInString(c)
	if size == 0 {
		return codec
	}
	return string(unicode.ToUpper(r)) + c[size:]
}
