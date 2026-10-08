package ffmpeg

import (
	"fmt"
	"strconv"
)

// LiveEncode 是直播推流共用的编码参数（已经过服务层校验并填好默认值）。
type LiveEncode struct {
	Width, Height int     // 0 = 不缩放；只给一个按比例；输出保证偶数
	Fps           float64 // 文件推流：>0 时加 fps 滤镜；屏幕推流不用（采集端 -framerate 已定）
	GOPFps        float64 // 用来算 -g（2×fps）的帧率，必须 > 0
	VideoKbps     int
	AudioKbps     int
	// HW 非空时视频用该硬件 H.264 编码器（如 h264_nvenc），空 = libx264（契约 9.7）。
	HW string
}

// ProtocolWhitelist 返回输出侧允许的协议（契约 6.10）。
func ProtocolWhitelist(scheme string) string {
	switch scheme {
	case "rtmps":
		return "rtmps,tcp,tls,crypto"
	case "srt":
		return "srt,udp"
	}
	return "rtmp,tcp"
}

// OutputFormat 返回输出封装：rtmp(s) 用 flv，srt 用 mpegts。
func OutputFormat(scheme string) string {
	if scheme == "srt" {
		return "mpegts"
	}
	return "flv"
}

// liveVideoFilter 返回 -vf 滤镜链：可选 fps、缩放（补偶数）。
func liveVideoFilter(e LiveEncode) string {
	var f []string
	if e.Fps > 0 {
		f = append(f, "fps="+fnum(e.Fps))
	}
	w, h := e.Width, e.Height
	switch {
	case w > 0 && h > 0:
		w, h = evenFloor(w), evenFloor(h)
		f = append(f, fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", w, h),
			fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2", w, h))
	case w > 0:
		f = append(f, fmt.Sprintf("scale=%d:-2", evenFloor(w)))
	case h > 0:
		f = append(f, fmt.Sprintf("scale=-2:%d", evenFloor(h)))
	default:
		f = append(f, "scale=trunc(iw/2)*2:trunc(ih/2)*2")
	}
	return joinComma(f)
}

func joinComma(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += ","
		}
		out += x
	}
	return out
}

// liveEncodeArgs 是始终重编码的编码参数（不用 -c copy）。withAudio 为 false 时不带音频编码（-an）。
func liveEncodeArgs(e LiveEncode, withAudio bool) []string {
	g := int(e.GOPFps*2 + 0.5)
	if g < 1 {
		g = 60
	}
	k := strconv.Itoa(e.VideoKbps) + "k"
	a := []string{"-vf", liveVideoFilter(e)}
	if e.HW != "" {
		a = append(a, HWLiveArgs(e.HW, e.VideoKbps, g)...)
	} else {
		a = append(a,
			"-c:v", "libx264", "-preset", "veryfast", "-tune", "zerolatency", "-pix_fmt", "yuv420p",
			"-b:v", k, "-maxrate", k, "-bufsize", strconv.Itoa(e.VideoKbps*2)+"k",
			"-g", strconv.Itoa(g))
	}
	if withAudio {
		a = append(a, "-c:a", "aac", "-b:a", strconv.Itoa(e.AudioKbps)+"k", "-ar", "44100", "-ac", "2")
	} else {
		a = append(a, "-an")
	}
	return a
}

// 输入侧放行 file：-protocol_whitelist 按位置生效，写在 -i 之前只管那个输入（契约 6.10）。
const inputWhitelist = "file"

const anullsrc = "anullsrc=r=44100:cl=stereo"

// FilePushPlan 描述一次文件推流。
type FilePushPlan struct {
	Input    string // 绝对路径
	Loop     bool
	HasAudio bool // 源文件有音轨；没有则补 anullsrc 并加 -shortest
	Scheme   string
	URL      string // 校验并重新组装后的地址
	Enc      LiveEncode
	// PreviewPort > 0 时主输出改用 tee，多一个 onfail=ignore 的预览分支（契约 v0.25，不多编码一次）。
	PreviewPort int
}

// BuildFilePushArgs 生成文件推流的 ffmpeg 参数（不含 -progress 等，由 Run 添加）。
// 顺序：-protocol_whitelist file [-re] [-stream_loop -1] -i file:<path> [-protocol_whitelist file -f lavfi -i anullsrc]
// -map ... 编码参数 [-shortest] 然后是输出（普通封装，或 tee，见 AppendPushOutput）。
func BuildFilePushArgs(p FilePushPlan) []string {
	a := []string{"-protocol_whitelist", inputWhitelist, "-re"}
	if p.Loop {
		a = append(a, "-stream_loop", "-1")
	}
	a = append(a, "-i", "file:"+p.Input)
	audioMap := "0:a:0"
	if !p.HasAudio {
		a = append(a, "-protocol_whitelist", inputWhitelist, "-f", "lavfi", "-i", anullsrc)
		audioMap = "1:a"
	}
	a = append(a, "-map", "0:v:0", "-map", audioMap)
	a = append(a, liveEncodeArgs(p.Enc, true)...)
	if !p.HasAudio {
		a = append(a, "-shortest") // anullsrc 是无限流，不加会永远不结束（7.1.5 实测）
	}
	return AppendPushOutput(a, p.Scheme, p.URL, "", p.PreviewPort)
}
