package ffmpeg

import (
	"strings"

	"FFmpegFree/internal/apperr"
)

// LiveClassifyInput 是直播错误分类的输入。
type LiveClassifyInput struct {
	// Tail 是 stderr 尾部（已经过脱敏，见 RunOptions.Redact）。
	Tail string
	// Scheme 是推流协议：rtmp | rtmps | srt（用于 LIVE_CONNECT_FAILED 的 detail 首行 scheme=<值>）。
	Scheme string
	// Started 表示推流已经开始（收到过第一条可判定为"已经在输出"的 progress，见 task 层）。
	// 此前的失败是 LIVE_CONNECT_FAILED / LIVE_PUSH_REJECTED，之后是 LIVE_PUSH_INTERRUPTED。
	Started bool
	// Screen 表示屏幕推流：额外识别采集失败（权限、打不开显示）。
	Screen bool
}

// ClassifyLiveError 把直播任务的 ffmpeg 非零退出归类为契约错误码（契约 6.10）。永远不返回 nil：认不出来的是 INTERNAL。
// 和 ClassifyConvertError 一样只看 classifiableLines()（剔除 Input # / Output # / Stream mapping / Metadata 段落），
// 系统错误文本按行尾匹配。
//
//	屏幕采集（avfoundation 权限）           → SCREEN_PERMISSION_DENIED
//	屏幕采集（x11grab 打不开显示）          → UNSUPPORTED_PLATFORM
//	未开始：RTMP 服务器明确拒绝            → LIVE_PUSH_REJECTED（detail = 脱敏 stderr 尾部）
//	未开始：DNS / 拒绝连接 / 超时 / 不可达 / SRT 握手失败 → LIVE_CONNECT_FAILED（detail 首行 scheme=<协议>，其后是脱敏 stderr 尾部）
//	已开始：Broken pipe / Connection reset / 写出 Input/output error / Error writing trailer → LIVE_PUSH_INTERRUPTED
//	其他                                  → INTERNAL
//
// SRT 已知局限：服务器没开与被拒绝的 stderr 完全一样，统一 LIVE_CONNECT_FAILED。
func ClassifyLiveError(in LiveClassifyInput) *apperr.AppError {
	lines := classifiableLines(in.Tail)
	joined := strings.Join(lines, "\n")
	contains := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(joined, s) {
				return true
			}
		}
		return false
	}
	endsWith := func(subs ...string) bool {
		for _, l := range lines {
			l = strings.TrimRight(l, " \t\r.。!")
			for _, s := range subs {
				if strings.HasSuffix(l, s) {
					return true
				}
			}
		}
		return false
	}
	anyLine := func(pred func(string) bool) bool {
		for _, l := range lines {
			if pred(l) {
				return true
			}
		}
		return false
	}

	if in.Screen {
		switch {
		case anyLine(func(l string) bool {
			return strings.Contains(l, "avfoundation") &&
				(strings.Contains(l, "not authorized") || strings.Contains(l, "permission") || strings.Contains(l, "denied") ||
					strings.Contains(l, "failed to create av capture input device") || strings.Contains(l, "screen recording"))
		}), contains("screen recording permission", "not authorized to capture"):
			return apperr.New(apperr.ScreenPermissionDenied, "没有屏幕录制权限，请在系统设置的“隐私与安全性 → 屏幕录制”里允许 FFmpegFree").WithDetail(in.Tail)
		case anyLine(func(l string) bool {
			return strings.Contains(l, "x11grab") && (strings.Contains(l, "cannot open display") || strings.Contains(l, "can't open display") || strings.Contains(l, "could not open display"))
		}), contains("cannot open display", "unable to open display"):
			return apperr.New(apperr.UnsupportedPlatform, "无法打开屏幕采集（X11 显示不可用）").WithDetail(in.Tail)
		}
	}

	if !in.Started {
		// 服务器明确拒绝（RTMP：`[rtmp @ …] Server error: authentication failed` / 流名冲突等）。
		if anyLine(func(l string) bool { return strings.Contains(l, "server error:") }) {
			return apperr.New(apperr.LivePushRejected, "推流服务器拒绝了推流（鉴权失败、流名冲突或握手被拒）").WithDetail(in.Tail)
		}
		connect := endsWith("connection refused", "connection timed out", "network is unreachable", "no route to host", "host is unreachable",
			"connection reset by peer", "name or service not known", "no address associated with hostname", "temporary failure in name resolution") ||
			contains("failed to resolve hostname", "cannot open connection") ||
			anyLine(func(l string) bool { // [srt @ …] Connection to srt://… failed: Input/output error
				return (strings.Contains(l, "connection to ") && strings.Contains(l, " failed")) || strings.Contains(l, "error opening output") && endsWith("input/output error")
			})
		if connect {
			return apperr.New(apperr.LiveConnectFailed, "连接推流服务器失败").WithDetail("scheme=" + in.Scheme + "\n" + in.Tail)
		}
		return apperr.New(apperr.Internal, "推流启动失败").WithDetail(in.Tail)
	}

	if endsWith("broken pipe", "connection reset by peer", "connection timed out", "input/output error", "connection reset", "end of file") ||
		contains("error writing trailer", "error muxing a packet", "error submitting a packet to the muxer") {
		return apperr.New(apperr.LivePushInterrupted, "推流中断，与推流服务器的连接已断开").WithDetail(in.Tail)
	}
	return apperr.New(apperr.Internal, "推流异常退出").WithDetail(in.Tail)
}
