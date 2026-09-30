package ffmpeg

import (
	"regexp"
	"strconv"
	"strings"
)

// MinMajor 是支持的最低 ffmpeg 主版本（契约第 9.1 节）。
const MinMajor = 6

// Version 是从 `-version` 输出解析出的版本信息。
type Version struct {
	// Raw 是 "ffmpeg version" 后面的原始版本串，如 "6.1.1-3ubuntu5"、"n7.0"、"N-12345-gabc"。
	Raw string
	// Display 是给人看、放进 FFmpegStatus.version 的规范化版本号（NormalizeVersion(Raw)）：
	// 只留开头的数字版本（"9.0.2-https://www.martin-riedl.de" → "9.0.2"，"n7.1" → "7.1"，"7.1-static" → "7.1"），
	// 识别不了数字版本（git 主干 / 日期版）时保留原样的前 32 个字符，但不含网址。
	Display string
	// Major 是主版本号；Known 为 false（无法解析）时无意义。
	Major int
	Known bool
}

var (
	// 每个程序输出的第一行形如 "ffmpeg version 6.1.1-... Copyright (c) ..."，ffprobe 同理。
	versionLineRe = regexp.MustCompile(`(?m)^\s*\w+ version (\S+)`)
	// 发行版本：可选前缀 n/N，然后是 主版本[.次版本...]，后面必须是分隔符或结尾。
	// 不匹配 "N-12345-gabc"（git 主干构建）、"2024-05-20-git-abc"（gyan.dev 的 git 构建，
	// 开头是年份），这两类被视为"无法解析"。
	releaseRe = regexp.MustCompile(`^[nN]?(\d+)(?:\.\d+)*(?:$|[-_+~.a-zA-Z])`)
)

// ParseVersion 解析 `ffmpeg -version` / `ffprobe -version` 的输出。
//
// 无法识别 "version" 行时返回 ok=false。能识别版本行但拿不到主版本号（git 构建）时
// 返回 ok=true 且 Known=false。主版本号大于等于 100 的一律当作日期（年份）处理，视为无法解析。
func ParseVersion(output string) (v Version, ok bool) {
	m := versionLineRe.FindStringSubmatch(output)
	if m == nil {
		return Version{}, false
	}
	v.Raw = m[1]
	v.Display = NormalizeVersion(v.Raw)
	if r := releaseRe.FindStringSubmatch(v.Raw); r != nil {
		if major, err := strconv.Atoi(r[1]); err == nil && major < 100 {
			v.Major, v.Known = major, true
		}
	}
	return v, true
}

// Acceptable 判断版本是否满足最低要求。
//
// 取舍：无法解析主版本号的 git 构建（如 N-12345-gabc、日期版）视为"可接受"，
// 因为这类构建通常比最新发行版还新，直接判低版本会误伤；但仍要求编码器检查通过。
func (v Version) Acceptable() bool {
	return !v.Known || v.Major >= MinMajor
}

// displayVersionMax 是识别不了数字版本时，规范化结果保留的最大字符数（按字符，不是字节）。
const displayVersionMax = 32

// versionCutMarkers 是版本串里网址 / 域名的起点：从这里起（含前面的连接符）都丢掉。
var versionCutMarkers = []string{"https://", "http://", "://", "-www.", "_www.", "www."}

// NormalizeVersion 把 "ffmpeg version" 后面的原始版本串整理成给人看的版本号（契约 4 节 FFmpegStatus.version）：
//
//	9.0.2-https://www.martin-riedl.de → 9.0.2      （martin-riedl 的 macOS 构建）
//	n7.1 / N7.1                       → 7.1
//	7.1-static / 7.1.5-static         → 7.1 / 7.1.5
//	6.1.1-3ubuntu5                    → 6.1.1      （发行版后缀不要）
//	N-12345-gabcdef                   → N-12345-gabcdef（git 主干构建，没有数字版本，原样保留）
//	2024-05-20-git-abc-www.gyan.dev   → 2024-05-20-git-abc（日期版，去掉网址，最多 32 个字符）
//
// 判断方法与 ParseVersion 一致：开头是 [nN]? 数字版本（主版本小于 100）就只取数字版本。
func NormalizeVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if r := releaseRe.FindStringSubmatch(raw); r != nil {
		if major, err := strconv.Atoi(r[1]); err == nil && major < 100 {
			return numericVersionRe.FindString(strings.TrimLeft(raw, "nN"))
		}
	}
	cut := len(raw)
	for _, m := range versionCutMarkers {
		if i := strings.Index(strings.ToLower(raw), m); i >= 0 && i < cut {
			cut = i
		}
	}
	s := strings.TrimRight(raw[:cut], "-_+~./: ")
	if r := []rune(s); len(r) > displayVersionMax {
		s = strings.TrimRight(string(r[:displayVersionMax]), "-_+~./: ")
	}
	return s
}

// numericVersionRe 取开头的 数字[.数字...]。
var numericVersionRe = regexp.MustCompile(`^\d+(?:\.\d+)*`)
