package live

import (
	"net/url"
	"strings"
	"unicode"
)

// PullURL 是通过校验的拉流地址（只用于后端出预览画面；播放仍由前端播放器直接拉远端地址）。
type PullURL struct {
	Scheme string // rtmp | rtmps | srt | http | https
	// FFmpeg 是传给 ffmpeg 的地址（rtmp / rtmps / srt 与推流地址同一套规范化；http(s) 原样）。
	FFmpeg string
	// Key 是"同一个标准化地址"的比较键，只在内存里比较。
	Key string
	// Redacted 是脱敏后的地址，可直接显示。
	Redacted string
}

// ParsePullURL 校验拉流地址：rtmp / rtmps / srt 复用推流地址规则；http / https 只做基本校验（主机必填、无空白 / 控制字符 / | \ " '，≤ 2048 字节）。
// ws / wss 没有对应的 ffmpeg 协议，返回 scheme_unsupported。失败返回 *URLError，detail 规则同 ParsePushURL。
func ParsePullURL(raw string) (PullURL, error) {
	raw = strings.TrimSpace(raw)
	i := strings.Index(raw, "://")
	if i > 0 {
		switch strings.ToLower(raw[:i]) {
		case "rtmp", "rtmps", "srt":
			p, err := ParsePushURL(raw)
			if err != nil {
				return PullURL{}, err
			}
			return PullURL{Scheme: p.Scheme, FFmpeg: p.FFmpeg, Key: p.Key, Redacted: p.Redacted}, nil
		case "http", "https":
			return parseHTTPPull(raw, strings.ToLower(raw[:i]))
		}
	}
	if raw == "" || i <= 0 {
		return PullURL{}, bad(ReasonMalformed, "拉流地址格式不正确，应形如 rtmp://主机/应用/流名 或 http://主机/路径.flv")
	}
	return PullURL{}, bad(ReasonSchemeUnsupported, "暂不支持这种拉流地址，请使用 rtmp、rtmps、srt、http 或 https")
}

func parseHTTPPull(raw, scheme string) (PullURL, error) {
	if len(raw) > MaxURLBytes {
		return PullURL{}, bad(ReasonMalformed, "拉流地址太长（最多 2048 字节）")
	}
	for _, r := range raw {
		if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune(`|\"'`, r) {
			return PullURL{}, bad(ReasonMalformed, "拉流地址含有空白、控制字符或 | \\ \" ' 等不允许的字符")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return PullURL{}, bad(ReasonMalformed, "拉流地址格式不正确")
	}
	if u.Hostname() == "" {
		return PullURL{}, bad(ReasonMissingHost, "拉流地址缺少主机名")
	}
	key := scheme + "://" + strings.ToLower(u.Host) + u.EscapedPath()
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}
	return PullURL{Scheme: scheme, FFmpeg: raw, Key: key, Redacted: RedactURL(raw)}, nil
}
