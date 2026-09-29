package ffmpeg

import (
	"regexp"
	"strconv"
)

// MinMajor 是支持的最低 ffmpeg 主版本（契约第 9.1 节）。
const MinMajor = 6

// Version 是从 `-version` 输出解析出的版本信息。
type Version struct {
	// Raw 是 "ffmpeg version" 后面的原始版本串，如 "6.1.1-3ubuntu5"、"n7.0"、"N-12345-gabc"。
	Raw string
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
