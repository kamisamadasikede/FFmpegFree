package ffmpeg

import (
	"fmt"
	"strconv"
	"strings"
)

// ScreenRegion 是要采集的显示器区域。
type ScreenRegion struct {
	X, Y          int
	Width, Height int // 0 = 采集整个桌面 / 让采集端自己决定
	DeviceIndex   int // macOS avfoundation 的设备序号
	Desktop       bool
	// WindowTitle 非空 = 只采集这个窗口（仅 Windows gdigrab `-i title=<标题>`，忽略 X/Y/Width/Height）。标题作为单个 argv 元素传给 ffmpeg，
	// 不经过 shell；gdigrab 把 `title=` 之后的全部内容当窗口标题，所以引号、空格、`=` 等字符不需要转义。
	WindowTitle string
}

// ScreenPushPlan 描述一次屏幕推流（不含存档）。
type ScreenPushPlan struct {
	GOOS       string // windows | darwin | linux
	Display    string // linux：DISPLAY，如 ":0" 或 ":99.0"
	Region     ScreenRegion
	HideCursor bool
	Silent     bool // audio=silent：补静音音轨
	Scheme     string
	URL        string
	FPS        float64 // 采集帧率
	Enc        LiveEncode
	// ArchiveTee 不为空时同时存档：是 TeePath 的结果（`file:` + 转义后的路径），命令改用 tee 复合输出。
	ArchiveTee string
	// PreviewPort > 0 时主输出用 tee，多一个预览分支（和存档分支并列，onfail=ignore）。
	PreviewPort int
}

// ScreenInputArgs 生成屏幕采集输入参数（各平台）。
func ScreenInputArgs(p ScreenPushPlan) []string {
	fps := fnum(p.FPS)
	cursor := "1"
	if p.HideCursor {
		cursor = "0"
	}
	switch p.GOOS {
	case "windows":
		a := []string{"-f", "gdigrab", "-framerate", fps}
		if p.HideCursor {
			a = append(a, "-draw_mouse", "0")
		}
		if p.Region.WindowTitle != "" {
			return append(a, "-i", "title="+p.Region.WindowTitle)
		}
		if !p.Region.Desktop && p.Region.Width > 0 && p.Region.Height > 0 {
			a = append(a, "-offset_x", strconv.Itoa(p.Region.X), "-offset_y", strconv.Itoa(p.Region.Y),
				"-video_size", fmt.Sprintf("%dx%d", p.Region.Width, p.Region.Height))
		}
		return append(a, "-i", "desktop")
	case "darwin":
		return []string{"-f", "avfoundation", "-framerate", fps, "-capture_cursor", cursor,
			"-i", fmt.Sprintf("%d:none", p.Region.DeviceIndex)}
	default: // linux
		a := []string{"-f", "x11grab", "-framerate", fps, "-draw_mouse", cursor}
		if !p.Region.Desktop && p.Region.Width > 0 && p.Region.Height > 0 {
			a = append(a, "-video_size", fmt.Sprintf("%dx%d", p.Region.Width, p.Region.Height))
		}
		return append(a, "-i", fmt.Sprintf("%s+%d,%d", p.Display, p.Region.X, p.Region.Y))
	}
}

// BuildScreenPushArgs 生成屏幕推流的参数。无存档时输出侧与文件推流一致；有存档（ArchiveTee 非空）时用 tee 一次编码写两路
// （契约 6.10）：`-flags +global_header -f tee "[网络一路]<url>|[f=mp4:...]<存档>"`，两路都写 onfail=abort，白名单写进每个 slave。
func BuildScreenPushArgs(p ScreenPushPlan) []string {
	a := ScreenInputArgs(p)
	audioMap := ""
	if p.Silent {
		a = append(a, "-protocol_whitelist", inputWhitelist, "-f", "lavfi", "-i", anullsrc)
		audioMap = "1:a"
	}
	a = append(a, "-map", "0:v:0")
	if p.Silent {
		a = append(a, "-map", audioMap) // 屏幕采集是无限流，不需要 -shortest
	}
	a = append(a, liveEncodeArgs(p.Enc, p.Silent)...)
	return AppendPushOutput(a, p.Scheme, p.URL, p.ArchiveTee, p.PreviewPort)
}

// TeeDescription 返回 tee 的输出描述。网络一路写 onfail=abort（默认 continue 会在连接失败时仍然退出码 0），
// 存档一路是分片 mp4（强杀后仍可播放），也写 onfail=abort；protocol_whitelist 对 tee 的 slave 必须写在 slave 选项里。
func TeeDescription(scheme, url, archiveTee string) string {
	net := "[f=" + OutputFormat(scheme) + ":onfail=abort:protocol_whitelist=" + ProtocolWhitelist(scheme) + "]" + TeeEscape(url)
	arc := "[f=mp4:onfail=abort:movflags=+frag_keyframe+empty_moov:flush_packets=1:protocol_whitelist=file]" + archiveTee
	return net + "|" + arc
}

// AppendPushOutput 追加推流输出。没有存档也没有预览时是普通封装；否则用 tee（契约 v0.25：预览分支始终和主输出同一份编码）。
func AppendPushOutput(a []string, scheme, url, archiveTee string, previewPort int) []string {
	pv := ""
	if previewPort > 0 {
		pv = "tcp://127.0.0.1:" + strconv.Itoa(previewPort) + "?tcp_nodelay=1"
	}
	if pv == "" && archiveTee != "" {
		// 只有存档：沿用 6.10 的 tee 描述（测试锁定了它的格式）。
		return append(a, "-flags", "+global_header", "-f", "tee", TeeDescription(scheme, url, archiveTee))
	}
	if pv == "" {
		a = append(a, "-protocol_whitelist", ProtocolWhitelist(scheme), "-f", OutputFormat(scheme))
		if scheme != "srt" {
			a = append(a, "-flvflags", "no_duration_filesize")
		}
		return append(a, url)
	}
	return append(a, "-flags", "+global_header", "-f", "tee", TeeSlaves(scheme, url, archiveTee, pv))
}

// TeeSlaves 拼 tee 描述：网络一路（onfail=abort）| 可选存档 | 可选预览（onfail=ignore，写慢就丢，不拖垮推流）。
// previewTCP 是未转义的 tcp:// 地址。
func TeeSlaves(scheme, url, archiveTee, previewTCP string) string {
	net := "[f=" + OutputFormat(scheme) + ":onfail=abort:protocol_whitelist=" + ProtocolWhitelist(scheme)
	if scheme != "srt" {
		net += ":flvflags=no_duration_filesize"
	}
	net += "]" + TeeEscape(url)
	parts := []string{net}
	if archiveTee != "" {
		parts = append(parts, "[f=mp4:onfail=abort:movflags=+frag_keyframe+empty_moov:flush_packets=1:protocol_whitelist=file]"+archiveTee)
	}
	if previewTCP != "" {
		parts = append(parts, "[f=flv:onfail=ignore:use_fifo=1:fifo_options=queue_size=120\\:drop_pkts_on_overflow=1:flvflags=no_duration_filesize:flush_packets=1:protocol_whitelist=tcp]"+TeeEscape(previewTCP))
	}
	return strings.Join(parts, "|")
}
