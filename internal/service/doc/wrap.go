package doc

import (
	"strings"
	"unicode"
)

// fpdf 的 MultiCell 把汉字当作可断点，断行后从 sep+1 继续，会把断点处那个字丢掉（实测：
// 长中文段落每次自动换行都少一个字）。这里自己按字宽折行，再用 CellFormat 逐行输出。

// isWrapCJK 报告 r 是否可以在它前后直接断行（汉字、假名、谚文、CJK/全角标点）。
func isWrapCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r) || (r >= 0x3000 && r <= 0x303F) || (r >= 0xFF00 && r <= 0xFFEF)
}

// 行首禁则 / 行尾禁则（简化版）：这些字符不出现在行首 / 行尾。
const (
	noLineStart = "，。、．，；：！？）］｝〉》」』】〕”’…—～ゝゞーぁぃぅぇぉっゃゅょゎァィゥェォッャュョヮ,.;:!?)]}%"
	noLineEnd   = "（［｛〈《「『【〔“‘([{"
)

// wrapText 把 s 折成不超过 maxW（mm，由 width 度量）的若干行。'\n' 强制换行；空格处可断，行尾空格丢弃；
// CJK 字符之间可断；超长的单词按字符硬断。每个字符恰好出现一次（不丢字）——除了折行处丢掉的空格。
func wrapText(s string, maxW float64, width func(string) float64) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		out = append(out, wrapLine(strings.TrimRight(para, "\r"), maxW, width)...)
	}
	return out
}

func wrapLine(s string, maxW float64, width func(string) float64) []string {
	if s == "" {
		return []string{""}
	}
	// 切成不可再分的 token：一个 CJK 字符、一段非空白非 CJK 字符、一段空格。
	var toks []string
	rs := []rune(s)
	for i := 0; i < len(rs); {
		r := rs[i]
		j := i + 1
		switch {
		case r == ' ' || r == '\t':
			for j < len(rs) && (rs[j] == ' ' || rs[j] == '\t') {
				j++
			}
		case isWrapCJK(r):
		default:
			for j < len(rs) && rs[j] != ' ' && rs[j] != '\t' && !isWrapCJK(rs[j]) {
				j++
			}
		}
		toks = append(toks, string(rs[i:j]))
		i = j
	}
	// 禁则：行首禁则字符并入前一个 token；行尾禁则字符并入后一个 token。
	var glued []string
	pending := ""
	for _, t := range toks {
		first := []rune(t)[0]
		if len(glued) > 0 && pending == "" && strings.ContainsRune(noLineStart, first) && !isSpaceTok(t) {
			glued[len(glued)-1] += t
			continue
		}
		t = pending + t
		pending = ""
		last := []rune(t)
		if strings.ContainsRune(noLineEnd, last[len(last)-1]) && len(last) == 1 {
			pending = t
			continue
		}
		glued = append(glued, t)
	}
	if pending != "" {
		glued = append(glued, pending)
	}

	var lines []string
	cur := ""
	flush := func() {
		lines = append(lines, strings.TrimRight(cur, " \t"))
		cur = ""
	}
	for _, t := range glued {
		if isSpaceTok(t) {
			if cur != "" { // 行首的空格保留缩进不必要，丢弃
				cur += t
			}
			continue
		}
		if width(cur+t) <= maxW {
			cur += t
			continue
		}
		if cur != "" && strings.TrimSpace(cur) != "" {
			flush()
		}
		cur = ""
		if width(t) <= maxW {
			cur = t
			continue
		}
		// 单个 token 超宽：按字符硬断，每行至少放一个字符，保证前进。
		for _, r := range t {
			if cur != "" && width(cur+string(r)) > maxW {
				flush()
			}
			cur += string(r)
		}
	}
	if cur != "" || len(lines) == 0 {
		flush()
	}
	return lines
}

func isSpaceTok(t string) bool { return t[0] == ' ' || t[0] == '\t' }
