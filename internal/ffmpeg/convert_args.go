package ffmpeg

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"FFmpegFree/internal/apperr"
)

// ConvertOptions 是契约第 3 节的 ConvertOptions（v0.9 增加 Crf）。
//
// 约定（详见契约 6.8）：
//   - VideoCodec：copy | h264 | h265 | vp9 | ""。""（无视频）表示丢弃视频流；音频容器（mp3 等）和 gif 忽略该字段。
//   - AudioCodec：copy | aac | mp3 | opus | vorbis | flac | pcm | ac3 | none | ""。
//     ""（默认）取容器的默认音频编码；none 丢弃音轨。
//   - Width / Height：0 保持；只给一个时按比例缩放；两个都给时等比缩进该框内并补黑边。
//   - VideoBitrate / AudioBitrate：比特率，单位 bit/s；0 自动。
//   - Crf：恒定质量，0 用编码器默认（h264 23、h265 28、vp9 32）；设置了 VideoBitrate 或 TargetSizeMb 时忽略。
//   - TargetSizeMb > 0：按目标大小反推视频码率并两遍编码（覆盖 VideoBitrate）。
//   - TrimStart / TrimEnd：秒，TrimEnd 为 0 表示到结尾。
type ConvertOptions struct {
	Container    string  `json:"container"`
	VideoCodec   string  `json:"videoCodec"`
	AudioCodec   string  `json:"audioCodec"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	Fps          float64 `json:"fps"`
	VideoBitrate int64   `json:"videoBitrate"`
	AudioBitrate int64   `json:"audioBitrate"`
	Crf          int     `json:"crf"`
	TargetSizeMB float64 `json:"targetSizeMb"`
	TrimStart    float64 `json:"trimStart"`
	TrimEnd      float64 `json:"trimEnd"`
}

// ConvertSource 是输入文件的探测结果（来自 MediaService）。
type ConvertSource struct {
	DurationSec float64 // 0 表示未知
	HasVideo    bool    // 有真正的视频画面（不含封面图）
	VideoIndex  int     // 主视频流在文件里的流序号，HasVideo 为 true 时有效
	HasAudio    bool
}

// ConvertPlan 是构造好的 ffmpeg 参数。参数里不含可执行文件、-hide_banner、-nostats、-progress
// （由 Run 统一添加），输出位置是调用方传入的 out（一般是 .part 临时文件）。
type ConvertPlan struct {
	Final []string
	// OutDurationSec 是输出时长（应用裁剪后），用来换算进度；0 表示未知。
	OutDurationSec float64
	// Fallback 非空时：Final 正常结束但没有产出文件（图片输出、时长未知时 -ss 1 越过结尾），用它再跑一次（契约 6.16.5）。
	Fallback []string
}

type containerSpec struct {
	audioOnly    bool
	image        bool     // 图片输出（单帧，契约 v0.24，6.16.5）
	video        []string // 允许的 VideoCodec（不含 ""）
	audio        []string // 允许的 AudioCodec（不含 "" 和 none；音频容器必须有音频）
	defaultAudio string
	faststart    bool
	muxer        string   // 显式的 -f（契约 v0.24：所有输出都加，不靠扩展名猜）
	fixedAudio   []string // 按格式固定的音频参数（如 amr 的 -ar 8000 -ac 1），用户改不了
	imageCodec   []string // 图片输出的编码参数
}

var containers = map[string]containerSpec{
	"mp4":  {video: []string{"copy", "h264", "h265", "vp9"}, audio: []string{"copy", "aac", "mp3", "opus", "ac3"}, defaultAudio: "aac", faststart: true, muxer: "mp4"},
	"mov":  {video: []string{"copy", "h264", "h265"}, audio: []string{"copy", "aac", "mp3", "ac3", "pcm"}, defaultAudio: "aac", faststart: true, muxer: "mov"},
	"mkv":  {video: []string{"copy", "h264", "h265", "vp9"}, audio: []string{"copy", "aac", "mp3", "opus", "vorbis", "flac", "ac3", "pcm"}, defaultAudio: "aac", muxer: "matroska"},
	"webm": {video: []string{"copy", "vp9"}, audio: []string{"copy", "opus", "vorbis"}, defaultAudio: "opus", muxer: "webm"},
	"avi":  {video: []string{"copy", "h264", "mpeg4"}, audio: []string{"copy", "mp3", "aac", "ac3", "pcm"}, defaultAudio: "mp3", muxer: "avi"},
	"flv":  {video: []string{"copy", "h264"}, audio: []string{"copy", "aac", "mp3"}, defaultAudio: "aac", muxer: "flv"},
	"gif":  {muxer: "gif"},
	"wmv":  {video: []string{"wmv2"}, audio: []string{"wma"}, defaultAudio: "wma", muxer: "asf"},
	"mpg":  {video: []string{"mpeg2"}, audio: []string{"mp2", "ac3"}, defaultAudio: "mp2", muxer: "mpeg"},
	"vob":  {video: []string{"mpeg2"}, audio: []string{"ac3", "mp2"}, defaultAudio: "ac3", muxer: "vob"},
	"3gp":  {video: []string{"h264"}, audio: []string{"aac", "amr_nb"}, defaultAudio: "aac", muxer: "3gp"},
	"swf":  {video: []string{"flv1"}, audio: []string{"mp3"}, defaultAudio: "mp3", muxer: "swf", fixedAudio: []string{"-ar", "44100"}},
	"ogv":  {video: []string{"theora"}, audio: []string{"vorbis", "opus"}, defaultAudio: "vorbis", muxer: "ogg"},

	"mp3":  {audioOnly: true, audio: []string{"mp3", "copy"}, defaultAudio: "mp3", muxer: "mp3"},
	"aac":  {audioOnly: true, audio: []string{"aac", "copy"}, defaultAudio: "aac", muxer: "adts"},
	"m4a":  {audioOnly: true, audio: []string{"aac", "copy"}, defaultAudio: "aac", faststart: true, muxer: "ipod"},
	"wav":  {audioOnly: true, audio: []string{"pcm"}, defaultAudio: "pcm", muxer: "wav"},
	"flac": {audioOnly: true, audio: []string{"flac"}, defaultAudio: "flac", muxer: "flac"},
	"ogg":  {audioOnly: true, audio: []string{"vorbis", "opus", "copy"}, defaultAudio: "vorbis", muxer: "ogg"},
	"opus": {audioOnly: true, audio: []string{"opus", "copy"}, defaultAudio: "opus", muxer: "opus"},
	"wma":  {audioOnly: true, audio: []string{"wma"}, defaultAudio: "wma", muxer: "asf"},
	"amr":  {audioOnly: true, audio: []string{"amr_nb"}, defaultAudio: "amr_nb", muxer: "amr", fixedAudio: []string{"-ar", "8000", "-ac", "1"}},
	"m4r":  {audioOnly: true, audio: []string{"aac", "copy"}, defaultAudio: "aac", faststart: true, muxer: "ipod"},
	"mp2":  {audioOnly: true, audio: []string{"mp2"}, defaultAudio: "mp2", muxer: "mp2"},
	"ape":  {audioOnly: true, audio: []string{"ape"}, defaultAudio: "ape", muxer: "ape"},
	"wv":   {audioOnly: true, audio: []string{"wavpack"}, defaultAudio: "wavpack", muxer: "wv"},
	"mmf":  {audioOnly: true, audio: []string{"adpcm_yamaha"}, defaultAudio: "adpcm_yamaha", muxer: "mmf", fixedAudio: []string{"-ar", "22050", "-ac", "1"}},

	"jpg":  {image: true, muxer: "image2", imageCodec: []string{"-c:v", "mjpeg", "-q:v", "2"}},
	"png":  {image: true, muxer: "image2", imageCodec: []string{"-c:v", "png"}},
	"webp": {image: true, muxer: "image2", imageCodec: []string{"-c:v", "libwebp", "-quality", "80"}},
	"ico":  {image: true, muxer: "ico", imageCodec: []string{"-c:v", "png"}},
	"bmp":  {image: true, muxer: "image2", imageCodec: []string{"-c:v", "bmp"}},
	"tif":  {image: true, muxer: "image2", imageCodec: []string{"-c:v", "tiff", "-compression_algo", "lzw"}},
	"tga":  {image: true, muxer: "image2", imageCodec: []string{"-c:v", "targa"}},
}

// containerOrder 是目标容器的固定顺序（错误提示、格式目录用）：视频、音频、图片，组内按契约 6.16.2 的表格顺序。
var containerOrder = []string{
	"mp4", "mkv", "mov", "webm", "avi", "flv", "gif", "wmv", "mpg", "vob", "3gp", "swf", "ogv",
	"mp3", "m4a", "aac", "wav", "flac", "ogg", "opus", "wma", "amr", "m4r", "mp2", "ape", "wv", "mmf",
	"jpg", "png", "webp", "ico", "bmp", "tif", "tga",
}

// ContainerNames 返回支持的目标容器（用于错误提示和测试）。
func ContainerNames() []string { return append([]string{}, containerOrder...) }

// IsAudioContainer 判断容器是否是纯音频输出。
func IsAudioContainer(c string) bool { return containers[c].audioOnly }

// IsImageContainer 判断容器是否是图片输出（单帧，契约 6.16.5）。
func IsImageContainer(c string) bool { return containers[c].image }

// ContainerMuxer 返回容器对应的 muxer 名（-f 的值）；未知容器返回 ""。
func ContainerMuxer(c string) string { return containers[c].muxer }

// videoEncoders / audioEncoders 是 ConvertOptions 的编码取值对应的 CPU 编码器名（契约 6.16.2 / 6.16.3）。
var videoEncoders = map[string]string{
	"h264": "libx264", "h265": "libx265", "vp9": "libvpx-vp9", "mpeg4": "mpeg4", "mpeg2": "mpeg2video",
	"wmv2": "wmv2", "flv1": "flv", "theora": "libtheora",
}

var audioEncoders = map[string]string{
	"aac": "aac", "mp3": "libmp3lame", "opus": "libopus", "vorbis": "libvorbis", "flac": "flac", "pcm": "pcm_s16le",
	"ac3": "ac3", "wma": "wmav2", "amr_nb": "libopencore_amrnb", "mp2": "mp2", "wavpack": "wavpack",
	"adpcm_yamaha": "adpcm_yamaha", "ape": "ape",
}

// RequiredEncoders 返回这组参数实际要用的 muxer 和编码器名（不含 copy；硬件编码器不在这里，契约 6.16.3 / 6.16.6）。
// 容器未知时 muxer 为 ""。
func RequiredEncoders(o ConvertOptions) (muxer string, encoders []string) {
	spec, ok := containers[o.Container]
	if !ok {
		return "", nil
	}
	muxer = spec.muxer
	switch {
	case spec.image:
		return muxer, []string{spec.imageCodec[1]}
	case o.Container == "gif":
		return muxer, []string{"gif"}
	}
	if !spec.audioOnly {
		if e := videoEncoders[o.VideoCodec]; e != "" {
			encoders = append(encoders, e)
		}
	}
	audio := o.AudioCodec
	if audio == "" {
		audio = spec.defaultAudio
	}
	if e := audioEncoders[audio]; e != "" {
		encoders = append(encoders, e)
	}
	return muxer, encoders
}

// imageInputExts 是“图片输入”的扩展名（契约 6.16.5）：转图片时取第 0 帧。
var imageInputExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".bmp": true, ".webp": true, ".tif": true, ".tiff": true,
	".tga": true, ".ico": true, ".gif": true,
}

// IsImageInput 判断输入文件按扩展名是不是图片（契约 6.16.5）。
func IsImageInput(p string) bool { return imageInputExts[strings.ToLower(filepath.Ext(p))] }

func invalid(format string, a ...any) error {
	return apperr.New(apperr.InvalidArgument, fmt.Sprintf(format, a...))
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// 选项上下限（契约 v0.9.1）。
const (
	maxDimension    = 8192
	minFps          = 0.1
	maxVideoBitrate = 1e9 // bit/s
	minAudioBitrate = 8000
	maxAudioBitrate = 1e6
	maxTrimSec      = 1e6
)

// ValidateConvertOptions 只检查选项本身（不需要输入文件），保存预设和提交任务前都会调用。
// 错误一律是 INVALID_ARGUMENT，message 直接可以展示给用户。
func ValidateConvertOptions(o ConvertOptions) error {
	if o.TargetSizeMB != 0 {
		return invalid("暂不支持按目标大小压缩")
	}
	spec, ok := containers[o.Container]
	if !ok {
		return invalid("不支持的输出格式 %q，可选：%s", o.Container, strings.Join(ContainerNames(), " "))
	}
	if o.VideoCodec != "" && o.VideoCodec != "copy" && videoEncoders[o.VideoCodec] == "" {
		return invalid("不支持的视频编码 %q，可选：copy h264 h265 vp9 mpeg4 mpeg2 wmv2 flv1 theora", o.VideoCodec)
	}
	if o.AudioCodec != "" && o.AudioCodec != "none" && o.AudioCodec != "copy" && audioEncoders[o.AudioCodec] == "" {
		return invalid("不支持的音频编码 %q，可选：copy aac mp3 opus vorbis flac pcm ac3 wma amr_nb mp2 wavpack adpcm_yamaha ape none", o.AudioCodec)
	}
	if o.Width < 0 || o.Height < 0 || o.Width > maxDimension || o.Height > maxDimension {
		return invalid("分辨率必须在 0~%d 之间", maxDimension)
	}
	if !finite(o.Fps) || o.Fps < 0 || o.Fps > 240 || (o.Fps > 0 && o.Fps < minFps) {
		return invalid("帧率必须是 0（保持）或 %.1f~240", minFps)
	}
	if o.VideoBitrate < 0 || o.VideoBitrate > maxVideoBitrate {
		return invalid("视频码率必须是 0（自动）或不超过 %d bit/s", int64(maxVideoBitrate))
	}
	if o.AudioBitrate != 0 && (o.AudioBitrate < minAudioBitrate || o.AudioBitrate > maxAudioBitrate) {
		return invalid("音频码率必须是 0（自动）或 %d~%d bit/s", int64(minAudioBitrate), int64(maxAudioBitrate))
	}
	if o.Crf < 0 || o.Crf > 63 {
		return invalid("CRF 必须在 0~63 之间")
	}
	if !finite(o.TargetSizeMB) || o.TargetSizeMB < 0 || o.TargetSizeMB > 1e6 {
		return invalid("目标大小不合法")
	}
	if !finite(o.TrimStart) || !finite(o.TrimEnd) || o.TrimStart < 0 || o.TrimEnd < 0 || o.TrimStart > maxTrimSec || o.TrimEnd > maxTrimSec {
		return invalid("裁剪时间必须在 0~%g 秒之间", maxTrimSec)
	}
	if o.TrimEnd > 0 && o.TrimEnd <= o.TrimStart {
		return invalid("裁剪结束时间必须大于开始时间")
	}

	audio := o.AudioCodec
	if audio == "" {
		audio = spec.defaultAudio
	}
	switch {
	case spec.image:
		// 图片输出只允许宽高（契约 6.16.5）；ICO 另有 256 上限。
		if o.TrimStart > 0 || o.TrimEnd > 0 || o.Fps > 0 || o.VideoBitrate > 0 || o.AudioBitrate > 0 || o.Crf > 0 ||
			o.VideoCodec != "" || (o.AudioCodec != "" && o.AudioCodec != "none") {
			return invalid("图片格式不能设置裁剪、帧率、码率或画质")
		}
		if o.Container == "ico" && (o.Width > 256 || o.Height > 256) {
			return invalid("ICO 图标的宽和高不能超过 256")
		}
		return nil
	case spec.audioOnly:
		if o.AudioCodec == "none" {
			return invalid("%s 是纯音频格式，不能去掉音轨", o.Container)
		}
		if !contains(spec.audio, audio) {
			return invalid("%s 格式不支持 %s 音频编码，可选：%s", o.Container, audio, strings.Join(spec.audio, " "))
		}
		if o.Width > 0 || o.Height > 0 || o.Fps > 0 {
			return invalid("%s 是纯音频格式，不能设置分辨率或帧率", o.Container)
		}
	case o.Container == "gif":
		if o.AudioCodec != "" && o.AudioCodec != "none" {
			return invalid("GIF 没有音轨")
		}
		if o.TargetSizeMB > 0 || o.VideoBitrate > 0 {
			return invalid("GIF 不支持按码率或目标大小压缩")
		}
	default:
		if o.VideoCodec != "" && !contains(spec.video, o.VideoCodec) {
			return invalid("%s 格式不支持 %s 视频编码，可选：%s", o.Container, o.VideoCodec, strings.Join(spec.video, " "))
		}
		if o.AudioCodec != "none" && !contains(spec.audio, audio) {
			return invalid("%s 格式不支持 %s 音频编码，可选：%s", o.Container, audio, strings.Join(spec.audio, " "))
		}
		if o.VideoCodec == "" && o.AudioCodec == "none" {
			return invalid("视频和音频都被去掉了，没有可输出的内容")
		}
	}
	if o.Crf > 0 && o.VideoCodec != "" && o.VideoCodec != "copy" && o.VideoCodec != "h264" && o.VideoCodec != "h265" && o.VideoCodec != "vp9" {
		return invalid("%s 编码不支持 CRF，请改用码率", o.VideoCodec)
	}
	if o.Crf > 51 && (o.VideoCodec == "h264" || o.VideoCodec == "h265") {
		return invalid("H.264 / H.265 的 CRF 必须在 0~51 之间")
	}
	if o.VideoCodec == "copy" && !spec.audioOnly {
		if o.Width > 0 || o.Height > 0 || o.Fps > 0 || o.VideoBitrate > 0 || o.Crf > 0 || o.TargetSizeMB > 0 {
			return invalid("直接复制视频流时不能设置分辨率、帧率、码率、CRF 或目标大小")
		}
	}
	if o.TargetSizeMB > 0 {
		switch {
		case spec.audioOnly && !contains([]string{"mp3", "aac", "opus", "vorbis", "ac3"}, audio):
			return invalid("%s 音频编码无法按目标大小压缩", audio)
		case !spec.audioOnly && o.VideoCodec == "":
			return invalid("按目标大小压缩需要选择视频编码")
		}
	}
	return nil
}

// defaultAudioBitrate 是没指定音频码率时各编码器使用的码率（bit/s），0 表示不使用 -b:a（VBR 质量档或无损）。
func defaultAudioBitrate(codec string) int64 {
	switch codec {
	case "aac", "ac3", "wma":
		return 192_000
	case "opus":
		return 128_000
	case "mp2":
		return 224_000
	}
	return 0
}

// PlanConvert 生成转换命令（单次 ffmpeg 调用；两遍编码暂缓，见契约 v0.7.2）。in 是输入文件，
// out 是输出文件（调用方传 .part 临时路径）。
// 输入输出都会加 `file:` 前缀，以 - 开头、含冒号或空格的路径都安全；
// 输入是文件名带 % 的图片时在 -i 前加 -pattern_type none，避免被当成序列模板。
func PlanConvert(in, out string, o ConvertOptions, src ConvertSource) (ConvertPlan, error) {
	return PlanConvertHW(in, out, o, src, "")
}

// ConvertHWCodec 返回这次转换里"可以用硬件编码"的编码：真正重编码 H.264 / H.265 的视频输出返回 "h264" / "hevc"，
// 其余（-c copy、VP9、GIF、纯音频、无视频、按目标大小的两遍编码）返回 ""，一律走 CPU（契约 9.7）。
func ConvertHWCodec(o ConvertOptions) string {
	spec, ok := containers[o.Container]
	if !ok || spec.audioOnly || spec.image || o.Container == "gif" || o.Container == "3gp" || o.TargetSizeMB > 0 {
		return "" // 3gp 固定 -profile:v baseline，一律 CPU（v0.24）
	}
	var c string
	switch o.VideoCodec {
	case "h264":
		c = "h264"
	case "h265":
		c = "hevc"
	default:
		return ""
	}
	if !HWDimsOK(c, o.Width, o.Height) {
		return ""
	}
	return c
}

// ConvertEncoderName 返回这次转换的 CPU 侧视频编码器名（写进任务的 encoder 字段）：libx264 / libx265 / libvpx-vp9 / gif / copy；
// 没有视频输出（纯音频、丢弃视频）返回 ""。
func ConvertEncoderName(o ConvertOptions) string {
	spec, ok := containers[o.Container]
	if !ok || spec.audioOnly {
		return ""
	}
	if spec.image {
		return spec.imageCodec[1]
	}
	if o.Container == "gif" {
		return "gif"
	}
	if o.VideoCodec == "copy" {
		return "copy"
	}
	return videoEncoders[o.VideoCodec]
}

// PlanConvertHW 同 PlanConvert，hw 非空（硬件编码器名，如 h264_nvenc）且 o 是 H.264 / H.265 重编码时，视频用该硬件编码器。
func PlanConvertHW(in, out string, o ConvertOptions, src ConvertSource, hw string) (ConvertPlan, error) {
	if err := ValidateConvertOptions(o); err != nil {
		return ConvertPlan{}, err
	}
	if in == "" || out == "" {
		return ConvertPlan{}, invalid("输入或输出路径为空")
	}
	spec := containers[o.Container]
	if spec.image {
		return planImage(in, out, o, src, spec, src.DurationSec > 0 && src.DurationSec < 1 || IsImageInput(in), src.DurationSec <= 0 && !IsImageInput(in))
	}
	audio := o.AudioCodec
	if audio == "" {
		audio = spec.defaultAudio
	}
	wantVideo := !spec.audioOnly && (o.Container == "gif" || o.VideoCodec != "")
	wantAudio := !spec.audioOnly && o.Container != "gif" && o.AudioCodec != "none" || spec.audioOnly

	if wantVideo && !src.HasVideo {
		return ConvertPlan{}, invalid("输入文件没有视频画面，无法转成 %s", o.Container)
	}
	if spec.audioOnly && !src.HasAudio {
		return ConvertPlan{}, invalid("输入文件没有音轨，无法转成 %s", o.Container)
	}
	if !src.HasAudio {
		wantAudio = false // 视频容器遇到无音轨输入：静默不带音频
	}
	if !wantVideo && !wantAudio {
		return ConvertPlan{}, invalid("输入文件里没有可输出的内容")
	}

	// 输出时长（裁剪后）。
	var outDur float64
	if src.DurationSec > 0 {
		if o.TrimStart >= src.DurationSec {
			return ConvertPlan{}, invalid("裁剪开始时间超过了视频时长（%.1f 秒）", src.DurationSec)
		}
		end := src.DurationSec
		if o.TrimEnd > 0 && o.TrimEnd < end {
			end = o.TrimEnd
		}
		outDur = end - o.TrimStart
	} else if o.TrimEnd > 0 {
		outDur = o.TrimEnd - o.TrimStart
	}

	plan := ConvertPlan{OutDurationSec: outDur}
	audioBitrate := o.AudioBitrate
	videoBitrate := o.VideoBitrate

	// 输入侧参数。
	pre := []string{"-y"}
	if o.TrimStart > 0 {
		pre = append(pre, "-ss", fnum(o.TrimStart))
	}
	pre = append(pre, ImagePatternArgs(in)...)
	pre = append(pre, "-i", "file:"+in)
	if outDur > 0 && (o.TrimEnd > 0) {
		pre = append(pre, "-t", fnum(outDur))
	}

	// 流选择。视频容器保留所有音轨；音频容器只取第一条。
	var maps []string
	if wantVideo {
		maps = append(maps, "-map", "0:"+strconv.Itoa(src.VideoIndex))
	}
	switch {
	case spec.audioOnly:
		maps = append(maps, "-map", "0:a:0")
	case wantAudio:
		maps = append(maps, "-map", "0:a?")
	}
	maps = append(maps, "-sn", "-dn")
	if spec.audioOnly {
		maps = append(maps, "-vn")
	} else if !wantVideo {
		maps = append(maps, "-vn")
	}
	if !wantAudio {
		maps = append(maps, "-an")
	}

	if ConvertHWCodec(o) == "" {
		hw = "" // copy / VP9 / GIF / 两遍编码 / 超出硬件尺寸：一律 CPU
	}
	vargs := videoArgs(o, videoBitrate, wantVideo, hw)
	if wantVideo && o.Container == "3gp" && o.VideoCodec == "h264" {
		vargs = append(vargs, "-profile:v", "baseline")
	}
	if o.Container == "amr" {
		audioBitrate = 12200 // AMR-NB 固定 12.2 kbps，用户设的码率忽略（契约 6.16.2）
	}
	aargs := audioArgs(audio, audioBitrate, wantAudio, spec.audioOnly)
	if wantAudio && audio != "copy" {
		aargs = append(aargs, spec.fixedAudio...)
	}

	tail := []string{"-map_metadata", "0"}
	if o.Container == "gif" {
		tail = []string{"-loop", "0", "-f", "gif"}
	}
	if spec.faststart {
		tail = append(tail, "-movflags", "+faststart")
	}
	if o.VideoCodec == "copy" && o.TrimStart > 0 && wantVideo {
		tail = append(tail, "-avoid_negative_ts", "make_zero")
	}
	if o.Container != "gif" && spec.muxer != "" {
		tail = append(tail, "-f", spec.muxer)
	}

	a := append([]string{}, pre...)
	a = append(a, maps...)
	a = append(a, vargs...)
	a = append(a, aargs...)
	a = append(a, tail...)
	a = append(a, "file:"+out)
	plan.Final = a
	return plan, nil
}

func fnum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func evenFloor(v int) int {
	if v < 2 {
		return 2
	}
	return v &^ 1
}

// videoFilters 返回 -vf 的滤镜链（不含 gif 的调色板部分）。
func videoFilters(o ConvertOptions) (filters []string, scaled bool) {
	if o.Fps > 0 {
		filters = append(filters, "fps="+fnum(o.Fps))
	}
	w, h := o.Width, o.Height
	switch {
	case w > 0 && h > 0:
		w, h = evenFloor(w), evenFloor(h)
		filters = append(filters,
			fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", w, h),
			fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2", w, h))
		scaled = true
	case w > 0:
		filters = append(filters, fmt.Sprintf("scale=%d:-2", evenFloor(w)))
		scaled = true
	case h > 0:
		filters = append(filters, fmt.Sprintf("scale=-2:%d", evenFloor(h)))
		scaled = true
	}
	return filters, scaled
}

func videoArgs(o ConvertOptions, bitrate int64, want bool, hw string) []string {
	if !want {
		return nil
	}
	if o.Container == "gif" {
		fps := o.Fps
		if fps <= 0 {
			fps = 12
		}
		o.Fps = fps
		f, scaled := videoFilters(o)
		if !scaled {
			f = append(f, "scale='min(480,iw)':-2:flags=lanczos")
		}
		chain := strings.Join(f, ",") + ",split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse"
		return []string{"-vf", chain}
	}
	if o.VideoCodec == "copy" {
		return []string{"-c:v", "copy"}
	}
	f, scaled := videoFilters(o)
	if !scaled && o.VideoCodec != "vp9" {
		f = append(f, "crop=trunc(iw/2)*2:trunc(ih/2)*2") // yuv420p 要求宽高为偶数
	}
	var a []string
	if len(f) > 0 {
		a = append(a, "-vf", strings.Join(f, ","))
	}
	rate := func(defCrf int) []string {
		switch {
		case bitrate > 0:
			return []string{"-b:v", strconv.FormatInt(bitrate, 10)}
		case o.Crf > 0:
			return []string{"-crf", strconv.Itoa(o.Crf)}
		}
		return []string{"-crf", strconv.Itoa(defCrf)}
	}
	if hw != "" && (o.VideoCodec == "h264" || o.VideoCodec == "h265") {
		codec, defCrf := "h264", 23
		if o.VideoCodec == "h265" {
			codec, defCrf = "hevc", 28
		}
		crf := o.Crf
		if crf <= 0 {
			crf = defCrf
		}
		a = append(a, HWRateArgs(hw, codec, crf, bitrate)...)
		if codec == "hevc" && (o.Container == "mp4" || o.Container == "mov") {
			a = append(a, "-tag:v", "hvc1")
		}
		return a
	}
	switch o.VideoCodec {
	case "h264":
		a = append(a, "-c:v", "libx264", "-preset", "medium")
		a = append(a, rate(23)...)
		a = append(a, "-pix_fmt", "yuv420p")
	case "h265":
		a = append(a, "-c:v", "libx265", "-preset", "medium")
		a = append(a, rate(28)...)
		a = append(a, "-pix_fmt", "yuv420p", "-x265-params", "log-level=error")
		if o.Container == "mp4" || o.Container == "mov" {
			a = append(a, "-tag:v", "hvc1") // Apple 设备要求 hvc1
		}
	case "vp9":
		a = append(a, "-c:v", "libvpx-vp9", "-row-mt", "1", "-deadline", "good", "-cpu-used", "4")
		if bitrate > 0 {
			a = append(a, "-b:v", strconv.FormatInt(bitrate, 10))
		} else {
			crf := 32
			if o.Crf > 0 {
				crf = o.Crf
			}
			a = append(a, "-crf", strconv.Itoa(crf), "-b:v", "0") // 恒定质量模式必须 -b:v 0
		}
		a = append(a, "-pix_fmt", "yuv420p")
	case "mpeg4", "mpeg2", "wmv2", "flv1", "theora":
		// 这几种编码不支持 CRF：没设码率时用固定质量 -q:v（契约 6.16.2）。
		enc := videoEncoders[o.VideoCodec]
		q := map[string]string{"mpeg4": "4", "mpeg2": "3", "wmv2": "4", "flv1": "5", "theora": "7"}[o.VideoCodec]
		a = append(a, "-c:v", enc)
		if o.VideoCodec == "mpeg4" {
			a = append(a, "-vtag", "xvid")
		}
		if bitrate > 0 {
			a = append(a, "-b:v", strconv.FormatInt(bitrate, 10))
		} else {
			a = append(a, "-q:v", q)
		}
		a = append(a, "-pix_fmt", "yuv420p")
	}
	return a
}

// planImage 生成单帧图片输出（契约 6.16.5）：firstFrame=true 取第 0 帧（图片输入、视频不足 1 秒），
// 否则 -ss 1；seekUnknown=true（时长未知的视频）时 Final 用 -ss 1、Fallback 用第 0 帧，调用方在没有产出时重试。
func planImage(in, out string, o ConvertOptions, src ConvertSource, spec containerSpec, firstFrame, seekUnknown bool) (ConvertPlan, error) {
	if in == "" || out == "" {
		return ConvertPlan{}, invalid("输入或输出路径为空")
	}
	if !src.HasVideo {
		return ConvertPlan{}, invalid("输入文件没有视频画面，无法转成 %s", o.Container)
	}
	build := func(seek bool) []string {
		a := []string{"-y"}
		if seek {
			a = append(a, "-ss", "1")
		}
		a = append(a, ImagePatternArgs(in)...)
		a = append(a, "-i", "file:"+in, "-map", "0:"+strconv.Itoa(src.VideoIndex), "-frames:v", "1", "-an", "-sn", "-dn")
		f, scaled := videoFilters(ConvertOptions{Width: o.Width, Height: o.Height})
		if o.Container == "ico" {
			if !scaled {
				f = append(f, "scale='min(256,iw)':'min(256,ih)':force_original_aspect_ratio=decrease")
			}
			f = append(f, "format=rgba")
		}
		if len(f) > 0 {
			a = append(a, "-vf", strings.Join(f, ","))
		}
		a = append(a, spec.imageCodec...)
		if spec.muxer == "image2" {
			a = append(a, "-update", "1")
		}
		a = append(a, "-f", spec.muxer, "file:"+out)
		return a
	}
	plan := ConvertPlan{Final: build(!firstFrame)}
	if seekUnknown {
		plan.Fallback = build(false)
	}
	return plan, nil
}

func audioArgs(codec string, bitrate int64, want, audioOnly bool) []string {
	if !want {
		return nil
	}
	br := func(def int64) []string {
		b := bitrate
		if b == 0 {
			b = def
		}
		if b == 0 {
			return nil
		}
		return []string{"-b:a", strconv.FormatInt(b, 10)}
	}
	switch codec {
	case "copy":
		return []string{"-c:a", "copy"}
	case "aac":
		return append([]string{"-c:a", "aac"}, br(defaultAudioBitrate("aac"))...)
	case "mp3":
		if bitrate > 0 {
			return []string{"-c:a", "libmp3lame", "-b:a", strconv.FormatInt(bitrate, 10)}
		}
		return []string{"-c:a", "libmp3lame", "-q:a", "2"} // VBR 约 190 kbps
	case "opus":
		return append([]string{"-c:a", "libopus"}, br(defaultAudioBitrate("opus"))...)
	case "vorbis":
		if bitrate > 0 {
			return []string{"-c:a", "libvorbis", "-b:a", strconv.FormatInt(bitrate, 10)}
		}
		return []string{"-c:a", "libvorbis", "-q:a", "5"}
	case "flac":
		return []string{"-c:a", "flac"}
	case "pcm":
		return []string{"-c:a", "pcm_s16le"}
	case "ac3":
		return append([]string{"-c:a", "ac3"}, br(defaultAudioBitrate("ac3"))...)
	}
	return nil
}

// imagePatternExts 是 ffmpeg 用 image2 解封装的图片扩展名。文件名里带 % 时（如 a%03d.png），
// image2 会把它当成序列模板而找不到文件，所以要加 -pattern_type none。
// 只对这些扩展名加：其他解封装器（mp4、gif、png_pipe……）不认识这个选项，加了反而报错（ffmpeg 7.1 实测）。
var imagePatternExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".bmp": true, ".webp": true,
	".tif": true, ".tiff": true, ".ppm": true, ".pgm": true, ".pbm": true, ".pam": true,
}

// ImagePatternArgs 返回放在 ffmpeg -i 之前的 -pattern_type none（仅当文件名带 % 且是 image2 支持的图片），否则 nil。
func ImagePatternArgs(in string) []string {
	if strings.Contains(filepath.Base(in), "%") && imagePatternExts[strings.ToLower(filepath.Ext(in))] {
		return []string{"-pattern_type", "none"}
	}
	return nil
}
