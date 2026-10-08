package ffmpeg

import "strconv"

// 直播预览（契约 v0.25 / 6.10.3）：不再有 2 fps 的 JPEG 输出。
// 推流的预览是 tee 里的一路（见 TeeSlaves），拉流的预览是把远端流转封装成 FLV 推到本机 TCP。

// PullRemuxPlan 描述一次拉流预览：-c copy 转成 FLV，写到本机 TCP。
type PullRemuxPlan struct {
	URL            string
	InputWhitelist string
	Port           int
	// Unknown 为 true：探测失败，用可选 map 让 ffmpeg 自己挑流。
	Unknown bool
	Video   bool // 要视频
	Audio   bool // 要音频（不支持的音频传 false，只出画面）
}

// BuildPullRemuxArgs 生成拉流预览的参数（不含 -progress）。条件不够时 ok=false。
func BuildPullRemuxArgs(p PullRemuxPlan) (args []string, ok bool) {
	if p.URL == "" || p.InputWhitelist == "" || p.Port <= 0 {
		return nil, false
	}
	if !p.Unknown && !p.Video && !p.Audio {
		return nil, false
	}
	a := []string{"-protocol_whitelist", p.InputWhitelist, "-fflags", "+nobuffer", "-flags", "low_delay",
		"-analyzeduration", "1000000", "-probesize", "1000000", "-i", p.URL}
	switch {
	case p.Unknown:
		a = append(a, "-map", "0:v:0?", "-map", "0:a:0?")
	default:
		if p.Video {
			a = append(a, "-map", "0:v:0")
		} else {
			a = append(a, "-vn")
		}
		if p.Audio {
			a = append(a, "-map", "0:a:0")
		} else {
			a = append(a, "-an")
		}
	}
	a = append(a, "-c", "copy", "-f", "flv", "-flvflags", "no_duration_filesize", "-flush_packets", "1",
		"-protocol_whitelist", "tcp", "tcp://127.0.0.1:"+strconv.Itoa(p.Port)+"?tcp_nodelay=1")
	return a, true
}

// PreviewPlayable 判断探测到的编码能不能在应用里播（契约 6.10.3.6）。
// video / audio 是 ffprobe 的 codec_name，空串表示没有这条流。
// 返回要不要带视频、要不要带音频、以及是不是整体不支持（UNSUPPORTED reason=codec）。
func PreviewPlayable(video, audio string) (sendVideo, sendAudio, unsupported bool) {
	videoOK := video == "h264"
	audioOK := audio == "aac" || audio == "mp3"
	switch {
	case video == "" && audio == "":
		return false, false, true
	case video != "" && !videoOK:
		return false, false, true // HEVC 和其他视频编码一律不支持，不转码
	case video == "" && !audioOK:
		return false, false, true
	}
	return videoOK, audio != "" && audioOK, false
}

// PullInputWhitelist 返回拉流输入侧允许的协议（按地址 scheme；-protocol_whitelist 写在 -i 之前，只管这个输入）。
// http / https 同时放行两者和 tls / crypto，因为 HLS 之类的清单会跳转到子地址。
func PullInputWhitelist(scheme string) string {
	switch scheme {
	case "rtmps":
		return "rtmps,tcp,tls,crypto"
	case "srt":
		return "srt,udp"
	case "http", "https":
		return "http,https,tcp,tls,crypto"
	}
	return "rtmp,tcp"
}
