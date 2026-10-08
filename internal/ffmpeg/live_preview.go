package ffmpeg

import (
	"strconv"
	"strings"
)

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
	// HLS 为 true（契约 v0.25.1）：直播从最新的一个分片开始（-live_start_index -1，默认是倒数第 3 个，
	// 一开始就一次性到 3 个分片、白白多出几秒延迟）。匀速输出由本机预览服务按时间戳排期（见 service/live 的 flvPacer），
	// 不用 -re：-re 只限制“不能比时间戳快”，分片到得晚时会一口气追上，HLS 刚开始（MediaMTX 按需生成分片）时照样一阵一阵。
	HLS bool
}

// BuildPullRemuxArgs 生成拉流预览的参数（不含 -progress）。条件不够时 ok=false。
func BuildPullRemuxArgs(p PullRemuxPlan) (args []string, ok bool) {
	if p.URL == "" || p.InputWhitelist == "" || p.Port <= 0 {
		return nil, false
	}
	if !p.Unknown && !p.Video && !p.Audio {
		return nil, false
	}
	window := PullProbeWindow(p.URL)
	if !p.Unknown && !p.Video {
		// 纯音频（契约 v0.25.3）：探测已确认没有视频。FLV（MediaMTX 的 RTMP）头里的标志仍说有视频，ffmpeg 会把整个窗口等完
		// 才开始输出（RTMP 5 秒窗口实测：探测 5.3 秒 + 转封装又 5.4 秒，约 10.7 秒才 playing）。音频参数在序列头里，0.5 秒足够
		// （实测转封装起步约 0.8 秒）。
		window = "500000"
	}
	a := []string{"-protocol_whitelist", p.InputWhitelist, "-fflags", "+nobuffer", "-flags", "low_delay",
		"-analyzeduration", window, "-probesize", window,
		// 单次读写最多等 8 秒（同探测）：远端连上后不再给数据时 ffmpeg 报错退出，而不是一直卡着。
		"-rw_timeout", "8000000"}
	if p.HLS {
		a = append(a, "-live_start_index", "-1")
	}
	a = append(a, "-i", p.URL)
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

// PullProbeWindow 返回拉流探测和转封装用的 -analyzeduration / -probesize（契约 v0.25.1）。
// RTMP / RTMPS 用 5 秒 / 5 MB，要盖住一个完整 GOP：MediaMTX 的 RTMP 在第一个关键帧前不给 SPS/PPS，GOP 2 秒时 1 秒常常
// 找不到编码参数（实测 12 次失败 4 次，5 秒 0 次；探测多花约 0.7 秒）。其他协议仍是 1 秒 / 1 MB：SRT（MPEG-TS）没有文件头，
// 会一直读到窗口用完，5 秒时探测常常超过 12 秒的上限（实测 6~13 秒），反而丢掉编码检查。
func PullProbeWindow(rawURL string) string {
	u := strings.ToLower(rawURL)
	if strings.HasPrefix(u, "rtmp://") || strings.HasPrefix(u, "rtmps://") {
		return "5000000"
	}
	return "1000000"
}

// LooksLikeHLS 判断拉流地址是不是 HLS：地址的路径以 .m3u8 结尾（不分大小写，忽略查询串），或者探测到的封装名含 hls。
func LooksLikeHLS(rawURL, formatName string) bool {
	for _, f := range strings.Split(formatName, ",") {
		if strings.TrimSpace(f) == "hls" || strings.TrimSpace(f) == "applehttp" {
			return true
		}
	}
	if rawURL == "" {
		return false
	}
	path := rawURL
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	return strings.HasSuffix(strings.ToLower(path), ".m3u8")
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
