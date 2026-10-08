package convert

import (
	"math"
	"strconv"
	"strings"

	"FFmpegFree/internal/codecname"
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

// VideoCodecDisplayName 返回视频编码的显示名，规则和表都在 internal/codecname（全应用唯一的一张表，契约 v0.23.4）。
func VideoCodecDisplayName(codec string) string { return codecname.Video(codec) }
