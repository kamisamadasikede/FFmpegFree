// Package outname 是输出文件名的净化规则（契约 6.11.3「输出文件名」，EditService 与 DocService 共用同一套规则）。
//
// 规则（所有平台一致，避免在 Mac 上导出的文件拷到 Windows 出问题）：
//  1. 删除所有控制字符（U+0000~U+001F、U+007F~U+009F）以及路径分隔符和 Windows 非法字符 \ / : * ? " < > |；
//  2. 去掉首尾空白和尾部的点与空格（Windows 会静默吞掉它们）；
//  3. Windows 保留设备名（CON PRN AUX NUL COM0~COM9 LPT0~LPT9，含 COM¹ COM² COM³ LPT¹ LPT² LPT³，
//     不区分大小写，与扩展名无关）在名字前加下划线；
//  4. 按字符（rune）截断到 MaxRunes（100），并且 UTF-8 字节数不超过 MaxBytes（200），截断后再做 2、3；
//  5. 处理后为空则用 fallback。
package outname

import (
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	// MaxRunes 是净化后名字的最大字符数。
	MaxRunes = 100
	// MaxBytes 是净化后名字的最大 UTF-8 字节数。
	MaxBytes = 200
	// WindowsMaxPath 是 Windows 传统路径长度上限（MAX_PATH 260 含结尾 NUL）。
	WindowsMaxPath = 259
)

var reserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
}

func isReserved(name string) bool {
	// Windows 只看第一个点之前的部分（CON.txt、CON.tar.gz 都是保留名），并忽略其后的空格。
	base := name
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	base = strings.ToUpper(strings.TrimRight(base, " "))
	if reserved[base] {
		return true
	}
	rs := []rune(base)
	if len(rs) == 4 && (string(rs[:3]) == "COM" || string(rs[:3]) == "LPT") {
		d := rs[3]
		if d >= '0' && d <= '9' || d == '¹' || d == '²' || d == '³' {
			return true
		}
	}
	return false
}

func isIllegal(r rune) bool {
	if r <= 0x1F || (r >= 0x7F && r <= 0x9F) {
		return true
	}
	switch r {
	case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
		return true
	}
	return false
}

// trimEnds 去首尾空白，再去尾部的点与空格。
func trimEnds(s string) string {
	s = strings.TrimFunc(s, unicode.IsSpace)
	return strings.TrimRight(s, ". ")
}

// finish 做第 2、3 步：去首尾、避开保留名。
func finish(s string) string {
	s = trimEnds(s)
	if s != "" && isReserved(s) {
		s = "_" + s
	}
	return s
}

// truncate 按 MaxRunes 与 MaxBytes 截断（不会切开一个字符）。
func truncate(s string, maxRunes, maxBytes int) string {
	n, b := 0, 0
	for i, r := range s {
		sz := utf8.RuneLen(r)
		if n+1 > maxRunes || b+sz > maxBytes {
			return s[:i]
		}
		n++
		b += sz
	}
	return s
}

// Sanitize 净化文件名主干（不含扩展名）。结果保证：非空、不含非法字符、不以点或空格结尾、
// 不是保留名、≤100 字符且 UTF-8 ≤200 字节。fallback 为空时用 "file"。
func Sanitize(name, fallback string) string {
	return SanitizeMax(name, fallback, MaxRunes, MaxBytes)
}

// SanitizeMax 同 Sanitize，但字符 / 字节上限可调（整条路径限制需要更短的名字时用）。maxRunes、maxBytes 至少按 1 处理。
func SanitizeMax(name, fallback string, maxRunes, maxBytes int) string {
	if maxRunes < 1 {
		maxRunes = 1
	}
	if maxBytes < 1 {
		maxBytes = 1
	}
	if fallback == "" {
		fallback = "file"
	}
	clean := func(in string) string {
		var b strings.Builder
		for _, r := range in {
			if !isIllegal(r) {
				b.WriteRune(r)
			}
		}
		s := trimEnds(b.String())
		s = truncate(s, maxRunes, maxBytes)
		s = finish(s)
		// 加下划线可能让长度超限：再截一次，截断后可能重新变成保留名的情况不会发生（保留名前缀已含 _）。
		if utf8.RuneCountInString(s) > maxRunes || len(s) > maxBytes {
			s = finish(truncate(s, maxRunes, maxBytes))
		}
		return s
	}
	if s := clean(name); s != "" {
		return s
	}
	if s := clean(fallback); s != "" {
		return s
	}
	return "file"
}

// Utf16Len 返回 s 的 UTF-16 码元数（Windows 路径长度按它计）。
func Utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// FitStem 在净化的基础上，保证 dir + 分隔符 + stem + suffixReserve 个码元不超过 maxPath（UTF-16 码元）。
// suffixReserve 应包含扩展名、".part" 和重名后缀 "(n)" 需要的余量。maxPath<=0 表示不限制（非 Windows）。
// dir 太长导致 stem 一个字符都放不下时 ok=false。
func FitStem(dir, name, fallback string, suffixReserve, maxPath int) (stem string, ok bool) {
	stem = Sanitize(name, fallback)
	if maxPath <= 0 {
		return stem, true
	}
	budget := maxPath - Utf16Len(dir) - 1 - suffixReserve
	if budget < 1 {
		return "", false
	}
	for Utf16Len(stem) > budget {
		rs := []rune(stem)
		// 每次少一个字符，再重新净化（可能露出尾部的点 / 空格）。
		stem = SanitizeMax(string(rs[:len(rs)-1]), fallback, MaxRunes, MaxBytes)
		if len(rs) == 1 {
			break
		}
	}
	if Utf16Len(stem) > budget {
		return "", false
	}
	return stem, true
}
