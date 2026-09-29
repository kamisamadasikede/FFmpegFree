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
}

type containerSpec struct {
	audioOnly    bool
	video        []string // 允许的 VideoCodec（不含 ""）
	audio        []string // 允许的 AudioCodec（不含 "" 和 none；音频容器必须有音频）
	defaultAudio string
	faststart    bool
}

var containers = map[string]containerSpec{
	"mp4":  {video: []string{"copy", "h264", "h265", "vp9"}, audio: []string{"copy", "aac", "mp3", "opus", "ac3"}, defaultAudio: "aac", faststart: true},
	"mov":  {video: []string{"copy", "h264", "h265"}, audio: []string{"copy", "aac", "mp3", "ac3", "pcm"}, defaultAudio: "aac", faststart: true},
	"mkv":  {video: []string{"copy", "h264", "h265", "vp9"}, audio: []string{"copy", "aac", "mp3", "opus", "vorbis", "flac", "ac3", "pcm"}, defaultAudio: "aac"},
	"webm": {video: []string{"copy", "vp9"}, audio: []string{"copy", "opus", "vorbis"}, defaultAudio: "opus"},
	"avi":  {video: []string{"copy", "h264"}, audio: []string{"copy", "mp3", "aac", "ac3", "pcm"}, defaultAudio: "mp3"},
	"flv":  {video: []string{"copy", "h264"}, audio: []string{"copy", "aac", "mp3"}, defaultAudio: "aac"},
	"gif":  {},
	"mp3":  {audioOnly: true, audio: []string{"mp3", "copy"}, defaultAudio: "mp3"},
	"aac":  {audioOnly: true, audio: []string{"aac", "copy"}, defaultAudio: "aac"},
	"m4a":  {audioOnly: true, audio: []string{"aac", "copy"}, defaultAudio: "aac", faststart: true},
	"wav":  {audioOnly: true, audio: []string{"pcm"}, defaultAudio: "pcm"},
	"flac": {audioOnly: true, audio: []string{"flac"}, defaultAudio: "flac"},
	"ogg":  {audioOnly: true, audio: []string{"vorbis", "opus", "copy"}, defaultAudio: "vorbis"},
	"opus": {audioOnly: true, audio: []string{"opus", "copy"}, defaultAudio: "opus"},
}

// ContainerNames 返回支持的目标容器（用于错误提示和测试）。
func ContainerNames() []string {
	return []string{"mp4", "mkv", "mov", "webm", "avi", "flv", "gif", "mp3", "aac", "m4a", "wav", "flac", "ogg", "opus"}
}

// IsAudioContainer 判断容器是否是纯音频输出。
func IsAudioContainer(c string) bool { return containers[c].audioOnly }

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
	if !contains([]string{"", "copy", "h264", "h265", "vp9"}, o.VideoCodec) {
		return invalid("不支持的视频编码 %q，可选：copy h264 h265 vp9", o.VideoCodec)
	}
	if !contains([]string{"", "none", "copy", "aac", "mp3", "opus", "vorbis", "flac", "pcm", "ac3"}, o.AudioCodec) {
		return invalid("不支持的音频编码 %q，可选：copy aac mp3 opus vorbis flac pcm ac3 none", o.AudioCodec)
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
	case "aac", "ac3":
		return 192_000
	case "opus":
		return 128_000
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
	if !ok || spec.audioOnly || o.Container == "gif" || o.TargetSizeMB > 0 {
		return ""
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
	if o.Container == "gif" {
		return "gif"
	}
	switch o.VideoCodec {
	case "copy":
		return "copy"
	case "h264":
		return "libx264"
	case "h265":
		return "libx265"
	case "vp9":
		return "libvpx-vp9"
	}
	return ""
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
	aargs := audioArgs(audio, audioBitrate, wantAudio, spec.audioOnly)

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
	if !scaled && (o.VideoCodec == "h264" || o.VideoCodec == "h265") {
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
	}
	return a
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
