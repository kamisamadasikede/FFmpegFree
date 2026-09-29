// Package fsutil 放跨服务共用的文件系统小工具：输出文件名净化、Windows 输出路径长度检查。
// 剪辑导出、直播存档文件名、Doc 输出名都用同一套规则（契约 6.11.3「输出文件名」），所有平台一致，
// 避免在 Mac 上导出、拷到 Windows 出问题。
package fsutil

import (
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	// MaxNameRunes 是净化后文件名（不含扩展名）的最大字符数。
	MaxNameRunes = 100
	// MaxNameBytes 是净化后文件名的最大 UTF-8 字节数。
	MaxNameBytes = 200
	// DefaultName 是净化后为空时使用的名字。
	DefaultName = "edit"
	// WindowsMaxPath 是 Windows 上整条输出路径（含扩展名）允许的最大字符数（UTF-16 单元）。
	WindowsMaxPath = 259
)

// illegalChars 是路径分隔符和 Windows 非法字符。
const illegalChars = `\/:*?"<>|`

// isFormatChar 判断 Unicode 格式类字符：零宽 / 方向控制 / BOM。
// 这类字符在文件名里不可见，会造成"看起来重名"或方向欺骗（RLO 把扩展名显示成别的）。
func isFormatChar(r rune) bool {
	switch {
	case r >= 0x200B && r <= 0x200F:
	case r >= 0x202A && r <= 0x202E:
	case r >= 0x2066 && r <= 0x2069:
	case r == 0xFEFF:
	default:
		return false
	}
	return true
}

// SanitizeFileName 按契约 6.11.3「输出文件名」净化文件名（不含扩展名）：
//
//  1. 丢弃非法 UTF-8，先做 Unicode NFC，再删除：控制字符（U+0000~001F、007F~009F）、Unicode 格式类字符
//     （U+200B–200F、202A–202E、2066–2069、FEFF）、路径分隔符和 Windows 非法字符 \ / : * ? " < > |；
//  2. 去掉首尾空白和尾部的点与空格（Windows 会静默吞掉它们）；
//  3. Windows 保留设备名（CON PRN AUX NUL COM0~9 LPT0~9，含 ¹²³ 变体；取第一个 "." 之前的部分、不分大小写）在整个名字前加 "_"；
//  4. 先按字符数截到 100，再保证 UTF-8 字节 ≤200（从末尾逐个字符删，不切开字符），然后再做一遍 2、3（不再重复 1）。
//
// 放开中日韩。全部去掉后返回 ""，由调用方决定兜底（见 SanitizeFileNameOr）。
func SanitizeFileName(s string) string { return sanitize(s, "") }

// SanitizeArchiveName 是直播存档用的变体（契约 6.10）：在 SanitizeFileName 的基础上，把 | ' [ ] 替换为 _
// （这些字符在 ffmpeg 输出名 / 各种 shell、播放列表里容易出问题）。
func SanitizeArchiveName(s string) string { return sanitize(s, "|'[]") }

// SanitizeFileNameOr 净化后为空时返回 fallback（fallback 本身不再净化，调用方给常量）。
func SanitizeFileNameOr(s, fallback string) string {
	if n := SanitizeFileName(s); n != "" {
		return n
	}
	return fallback
}

func sanitize(s, underscore string) string {
	s = strings.ToValidUTF8(s, "")
	s = norm.NFC.String(s)
	s = strings.Map(func(r rune) rune {
		switch {
		case underscore != "" && strings.ContainsRune(underscore, r):
			return '_'
		case unicode.IsControl(r), isFormatChar(r), strings.ContainsRune(illegalChars, r):
			return -1
		}
		return r
	}, s)
	s = finish(s)
	s = truncate(s, MaxNameRunes, MaxNameBytes)
	return finish(s)
}

// finish 是第 2、3 步：去首尾空白和尾部的点 / 空格，避开保留名。
func finish(s string) string {
	s = strings.TrimFunc(s, unicode.IsSpace)
	s = strings.TrimRightFunc(s, func(r rune) bool { return r == '.' || unicode.IsSpace(r) })
	if IsReservedName(s) {
		s = "_" + s
	}
	return s
}

func truncate(s string, maxRunes, maxBytes int) string {
	n, b := 0, 0
	for i, r := range s {
		w := utf8.RuneLen(r)
		if n+1 > maxRunes || b+w > maxBytes {
			return s[:i]
		}
		n++
		b += w
	}
	return s
}

// IsReservedName 判断 name 是否命中 Windows 保留设备名：只看第一个 "." 之前的部分（与扩展名无关，CON.txt 同样保留），
// 忽略其尾部空格，不分大小写。
func IsReservedName(name string) bool {
	part, _, _ := strings.Cut(name, ".")
	part = strings.ToUpper(strings.TrimRight(part, " "))
	switch part {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(part) < 4 {
		return false
	}
	if !strings.HasPrefix(part, "COM") && !strings.HasPrefix(part, "LPT") {
		return false
	}
	rest := part[3:]
	r, size := utf8.DecodeRuneInString(rest)
	if size != len(rest) {
		return false
	}
	switch {
	case r >= '0' && r <= '9', r == '¹', r == '²', r == '³', r >= '０' && r <= '９':
		return true
	}
	return false
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += len(utf16.Encode([]rune{r}))
	}
	return n
}

// OutputPathLength 返回按最坏情况估算的输出路径长度（UTF-16 单元，Windows 的 MAX_PATH 就是按它数的）：
// <dir>\<name>(99)<ext>.part ——为 ".part" 临时文件和最坏 "(99)" 重名后缀预留。
// ext 含点，如 ".mp4"。
func OutputPathLength(dir, name, ext string) int {
	dir = strings.TrimRight(dir, `\/`)
	return utf16Len(dir) + 1 + utf16Len(name) + len("(99)") + utf16Len(ext) + len(".part")
}

// OutputPathTooLong 在 goos 是 windows 时判断整条输出路径是否超过 259 字符；其他平台恒为 false。
// 传 runtime.GOOS；参数化只是为了能在别的平台上测试。
func OutputPathTooLong(goos, dir, name, ext string) bool {
	return goos == "windows" && OutputPathLength(dir, name, ext) > WindowsMaxPath
}
