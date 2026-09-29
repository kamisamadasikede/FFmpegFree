// Package live 是直播推流的纯函数部分（契约 v0.10 第 4 节 LiveService、6.10）：
// 推流地址校验与标准化、脱敏（RedactURL / NewRedactor）、存档文件名净化。
// 本包不依赖 ffmpeg、任务管理器和 Wails，方便表驱动测试。
package live

import (
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

// LIVE_URL_INVALID 的 detail 第一行固定为 "reason=<值>"（架构师定，稳定枚举：只追加、不改名、不删除；
// detail 不带地址、口令或任何片段）。
const (
	ReasonSchemeUnsupported = "scheme_unsupported"
	ReasonMalformed         = "malformed"
	ReasonMissingHost       = "missing_host"
	ReasonParamNotAllowed   = "param_not_allowed"
)

// MaxURLBytes 是推流地址的最大长度。
const MaxURLBytes = 2048

// URLError 是地址校验失败：Reason 是稳定枚举，Message 是给用户看的原因（不含地址原文）。
type URLError struct {
	Reason  string
	Message string
}

func (e *URLError) Error() string { return e.Message + " (reason=" + e.Reason + ")" }

func bad(reason, msg string) *URLError { return &URLError{Reason: reason, Message: msg} }

// PushURL 是通过校验的推流地址。
type PushURL struct {
	Scheme string // rtmp | rtmps | srt（小写）
	Host   string // 小写，IDN 已转 punycode，IPv6 不带方括号
	Port   int    // 没写时为默认端口（rtmp 1935、rtmps 443）
	// FFmpeg 是传给 ffmpeg 的地址：scheme 小写、IDN 主机名换成 punycode，其余保持用户写的原样。
	FFmpeg string
	// Key 是"同一个标准化地址"的比较键：scheme / host 小写、去掉默认端口、去掉用户信息和末尾的 /、保留路径与查询。
	// 只在内存里比较，不展示、不落库。
	Key string
	// Redacted 是脱敏后的地址，可直接显示。
	Redacted string
}

var srtAllowedParams = map[string]bool{
	"passphrase": true, "pbkeylen": true, "streamid": true, "latency": true,
	"connect_timeout": true, "maxbw": true, "pkt_size": true, "mode": true,
}

// ParsePushURL 校验并标准化推流地址（规则见契约 LiveService「推流地址校验规则」）。
// 失败返回 *URLError。
func ParsePushURL(raw string) (PushURL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return PushURL{}, bad(ReasonMalformed, "推流地址不能为空")
	}
	if len(raw) > MaxURLBytes {
		return PushURL{}, bad(ReasonMalformed, "推流地址太长（最多 2048 字节）")
	}
	for _, r := range raw {
		if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune(`|\"'`, r) {
			return PushURL{}, bad(ReasonMalformed, "推流地址含有空白、控制字符或 | \\ \" ' 等不允许的字符")
		}
	}
	i := strings.Index(raw, "://")
	if i <= 0 {
		return PushURL{}, bad(ReasonMalformed, "推流地址格式不正确，应形如 rtmp://主机/应用/流名")
	}
	scheme := strings.ToLower(raw[:i])
	if !validSchemeChars(scheme) {
		return PushURL{}, bad(ReasonMalformed, "推流地址格式不正确，应形如 rtmp://主机/应用/流名")
	}
	switch scheme {
	case "rtmp", "rtmps", "srt":
	default:
		return PushURL{}, bad(ReasonSchemeUnsupported, "暂不支持这种推流地址，请使用 rtmp、rtmps 或 srt")
	}

	rest := raw[i+3:]
	authEnd := strings.IndexAny(rest, "/?#")
	authority, tail := rest, ""
	if authEnd >= 0 {
		authority, tail = rest[:authEnd], rest[authEnd:]
	}
	userinfo := ""
	hostport := authority
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		userinfo, hostport = authority[:at+1], authority[at+1:]
	}
	host, portStr, err := splitHostPort(hostport)
	if err != nil {
		return PushURL{}, bad(ReasonMalformed, "推流地址的主机或端口格式不正确")
	}
	if host == "" {
		return PushURL{}, bad(ReasonMissingHost, "推流地址缺少主机名")
	}
	port := 0
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p < 1 || p > 65535 {
			return PushURL{}, bad(ReasonMalformed, "推流地址的端口必须在 1~65535 之间")
		}
		port = p
	}
	asciiHost := host
	if !isASCII(host) {
		h, err := idna.Lookup.ToASCII(host)
		if err != nil || h == "" {
			return PushURL{}, bad(ReasonMalformed, "推流地址的主机名无法转换为 punycode")
		}
		asciiHost = h
	}
	asciiHost = strings.ToLower(asciiHost)

	// 用标准库再验一遍整体结构（userinfo 转义、路径转义等）。
	u, err := url.Parse(scheme + "://" + userinfo + hostPortString(asciiHost, portStr) + tail)
	if err != nil {
		return PushURL{}, bad(ReasonMalformed, "推流地址格式不正确")
	}

	path, query := u.EscapedPath(), u.RawQuery
	switch scheme {
	case "rtmp", "rtmps":
		if strings.Trim(path, "/") == "" {
			return PushURL{}, bad(ReasonMalformed, "rtmp 地址至少要包含应用名，如 rtmp://主机/live/流名")
		}
		if port == 0 {
			if scheme == "rtmp" {
				port = 1935
			} else {
				port = 443
			}
		}
	case "srt":
		if portStr == "" {
			return PushURL{}, bad(ReasonMalformed, "srt 地址必须写端口，如 srt://主机:9000")
		}
		nq, err := normalizeSRTQuery(query)
		if err != nil {
			return PushURL{}, err
		}
		query = nq
		tail = path
		if query != "" {
			tail += "?" + query
		}
	}

	key := scheme + "://" + hostPortString(asciiHost, strconv.Itoa(port))
	key += strings.TrimRight(path, "/")
	if query != "" {
		key += "?" + query
	}
	p := PushURL{
		Scheme: scheme, Host: strings.Trim(asciiHost, "[]"), Port: port,
		FFmpeg: scheme + "://" + userinfo + hostPortString(asciiHost, portStr) + tail,
		Key:    key,
	}
	p.Redacted = RedactURL(p.FFmpeg)
	return p, nil
}

// normalizeSRTQuery 检查 srt 查询参数白名单并返回重新组装的查询串：
// 键先 URL 解码再转小写（ffmpeg 7.1.5 对参数名区分大小写、也不做百分号解码，写成 PASSPHRASE / pass%70hrase 会被悄悄忽略），
// 值原样保留；不在白名单、同名重复、mode 不是 caller 为 param_not_allowed；值非法为 malformed。
func normalizeSRTQuery(query string) (string, *URLError) {
	if query == "" {
		return "", nil
	}
	seen := map[string]bool{}
	var parts []string
	for _, kv := range strings.Split(query, "&") {
		if kv == "" {
			continue
		}
		k, v, hasEq := strings.Cut(kv, "=")
		dk, err := url.QueryUnescape(k)
		if err != nil {
			return "", bad(ReasonMalformed, "推流地址的查询参数格式不正确")
		}
		dk = strings.ToLower(dk)
		if !srtAllowedParams[dk] {
			return "", bad(ReasonParamNotAllowed, "srt 地址含有不支持的参数（只允许 passphrase、pbkeylen、streamid、latency、connect_timeout、maxbw、pkt_size、mode=caller）")
		}
		if seen[dk] {
			return "", bad(ReasonParamNotAllowed, "srt 地址里有重复的参数")
		}
		seen[dk] = true
		dv, err := url.QueryUnescape(v)
		if err != nil {
			return "", bad(ReasonMalformed, "推流地址的查询参数格式不正确")
		}
		if e := checkSRTValue(dk, dv); e != nil {
			return "", e
		}
		if hasEq {
			parts = append(parts, dk+"="+v)
		} else {
			parts = append(parts, dk)
		}
	}
	return strings.Join(parts, "&"), nil
}

func checkSRTValue(key, val string) *URLError {
	switch key {
	case "mode":
		if strings.ToLower(val) != "caller" {
			return bad(ReasonParamNotAllowed, "srt 只支持 caller 模式（mode=caller），不支持 listener / rendezvous")
		}
	case "passphrase":
		if n := len(val); n < 10 || n > 79 {
			return bad(ReasonMalformed, "srt 口令（passphrase）长度必须是 10~79 个字符")
		}
	case "pbkeylen":
		if val != "0" && val != "16" && val != "24" && val != "32" {
			return bad(ReasonMalformed, "srt 的 pbkeylen 只能是 0、16、24、32")
		}
	case "streamid":
		if utf8.RuneCountInString(val) > 512 {
			return bad(ReasonMalformed, "srt 的 streamid 最长 512 个字符")
		}
	case "latency", "connect_timeout", "maxbw", "pkt_size":
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil || n < 0 {
			return bad(ReasonMalformed, "srt 的 "+key+" 必须是非负整数")
		}
		if key == "pkt_size" && (n < 1 || n > 1456) {
			return bad(ReasonMalformed, "srt 的 pkt_size 范围是 1~1456")
		}
	}
	return nil
}

func validSchemeChars(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.') {
			return false
		}
	}
	return s != ""
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// splitHostPort 拆 host:port；IPv6 必须用方括号。返回的 host 不含方括号。
func splitHostPort(hp string) (host, port string, err error) {
	if strings.HasPrefix(hp, "[") {
		end := strings.Index(hp, "]")
		if end < 0 {
			return "", "", errBadHost
		}
		host = hp[1:end]
		if ip := net.ParseIP(host); ip == nil || ip.To4() != nil {
			return "", "", errBadHost
		}
		rest := hp[end+1:]
		switch {
		case rest == "":
		case strings.HasPrefix(rest, ":"):
			port = rest[1:]
			if port == "" {
				return "", "", errBadHost
			}
		default:
			return "", "", errBadHost
		}
		return host, port, nil
	}
	if strings.Count(hp, ":") > 1 {
		return "", "", errBadHost // 未加方括号的 IPv6
	}
	if c := strings.LastIndex(hp, ":"); c >= 0 {
		host, port = hp[:c], hp[c+1:]
		if port == "" {
			return "", "", errBadHost
		}
	} else {
		host = hp
	}
	if strings.ContainsAny(host, "[]@%") {
		return "", "", errBadHost
	}
	return host, port, nil
}

var errBadHost = &URLError{Reason: ReasonMalformed, Message: "主机格式不正确"}

// hostPortString 把 host（IPv6 不带方括号也行）和端口拼回 authority；port 为空则不写。
func hostPortString(host, port string) string {
	h := host
	if strings.Contains(h, ":") && !strings.HasPrefix(h, "[") {
		h = "[" + h + "]"
	}
	if port == "" || port == "0" {
		return h
	}
	return h + ":" + port
}
