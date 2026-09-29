package live

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// InvalidURLText 是 RedactURL 解析失败时的返回值，绝不回显原文。
const InvalidURLText = "<invalid-url>"

// RedactURL 返回脱敏后的地址，可直接显示（契约 6.10）：
// 用户信息 → ***@；rtmp / rtmps 保留 host、端口和第一段路径（应用名），其后的路径（流名，可含 /）→ ***；
// 所有查询参数保留键、值 → ***（没有 = 的裸参数整个换成 ***）；fragment 去掉；解析失败返回 <invalid-url>。
// 其他 scheme（tcp / tls / udp / srt，出现在 ffmpeg 改写后的日志里）：host、端口保留，路径整体 → ***。
func RedactURL(raw string) string {
	raw = strings.TrimSpace(raw)
	i := strings.Index(raw, "://")
	if i <= 0 {
		return InvalidURLText
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return InvalidURLText
	}
	scheme := strings.ToLower(u.Scheme)
	var b strings.Builder
	b.WriteString(scheme)
	b.WriteString("://")
	if u.User != nil {
		b.WriteString("***@")
	}
	b.WriteString(u.Host)
	segs := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	if len(segs) > 0 && segs[0] != "" {
		switch scheme {
		case "rtmp", "rtmps":
			b.WriteString("/" + segs[0])
			if len(segs) > 1 {
				b.WriteString("/***")
			}
		default:
			b.WriteString("/***")
		}
	}
	if u.RawQuery != "" {
		var parts []string
		for _, kv := range strings.Split(u.RawQuery, "&") {
			if kv == "" {
				continue
			}
			if k, _, ok := strings.Cut(kv, "="); ok {
				parts = append(parts, k+"=***")
			} else {
				parts = append(parts, "***")
			}
		}
		if len(parts) > 0 {
			b.WriteString("?" + strings.Join(parts, "&"))
		}
	}
	return b.String()
}

var residualURL = regexp.MustCompile(`(rtmps?|srt|tcp|tls|udp)://[^\s'"<>]+`)

const minSecretLen = 3

// NewRedactor 返回处理 ffmpeg 输出的一行的脱敏函数（契约 6.10）：
//  1. 原始 URL 及其 URL 编码 / 解码形式整体换成 RedactURL 的结果；再把从中提取的每个秘密片段
//     （用户名、密码、流名及其各段、每个查询值，长度 ≥ 3，原样和解码后的形式）按字面换成 ***；
//  2. 行里残留的任何 rtmp(s) / srt / tcp / tls / udp 地址（ffmpeg 改写后的形式）交给 RedactURL。
func NewRedactor(rawURL string) func(string) string {
	rawURL = strings.TrimSpace(rawURL)
	redacted := RedactURL(rawURL)

	// 整体形式：原文、解码、编码（去重，长的在前）。
	fulls := uniq([]string{rawURL, unescape(rawURL), url.QueryEscape(rawURL), url.PathEscape(rawURL)})
	sort.SliceStable(fulls, func(i, j int) bool { return len(fulls[i]) > len(fulls[j]) })

	var secrets []string
	add := func(s string) {
		if len(s) >= minSecretLen {
			secrets = append(secrets, s, unescape(s), url.QueryEscape(s), url.PathEscape(s))
		}
	}
	if u, err := url.Parse(rawURL); err == nil {
		if u.User != nil {
			add(u.User.Username())
			if p, ok := u.User.Password(); ok {
				add(p)
			}
		}
		scheme := strings.ToLower(u.Scheme)
		segs := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
		if scheme == "rtmp" || scheme == "rtmps" {
			if len(segs) > 1 {
				stream := strings.Join(segs[1:], "/")
				add(stream)
				for _, sg := range segs[1:] {
					add(sg)
				}
			}
		}
		for _, kv := range strings.Split(u.RawQuery, "&") {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				add(k)
				continue
			}
			add(v)
			// streamid=publish:live/key:user:pass 这类复合值：再按常见分隔符拆开。
			for _, part := range strings.FieldsFunc(unescape(v), func(r rune) bool { return strings.ContainsRune(":/,;", r) }) {
				add(part)
			}
		}
	}
	secrets = uniq(secrets)
	sort.SliceStable(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })

	return func(line string) string {
		// 占位符：整体替换后的脱敏地址里可能含有应用名等与秘密片段相同的字面量，先保护起来。
		var holds []string
		for _, f := range fulls {
			if f != "" && strings.Contains(line, f) {
				line = strings.ReplaceAll(line, f, "\x00"+itoa(len(holds))+"\x00")
				holds = append(holds, redacted)
			}
		}
		for _, s := range secrets {
			if strings.Contains(line, s) {
				line = strings.ReplaceAll(line, s, "***")
			}
		}
		line = residualURL.ReplaceAllStringFunc(line, func(m string) string {
			trail := ""
			for len(m) > 0 && strings.ContainsRune(".:,;)", rune(m[len(m)-1])) {
				trail = string(m[len(m)-1]) + trail
				m = m[:len(m)-1]
			}
			return RedactURL(m) + trail
		})
		for i, h := range holds {
			line = strings.ReplaceAll(line, "\x00"+itoa(i)+"\x00", h)
		}
		return line
	}
}

func unescape(s string) string {
	if d, err := url.QueryUnescape(s); err == nil {
		return d
	}
	return s
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
