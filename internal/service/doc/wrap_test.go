package doc

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// 每个汉字宽 10，拉丁字母宽 5，空格宽 3（测试用度量）。
func testWidth(s string) float64 {
	var w float64
	for _, r := range s {
		switch {
		case isWrapCJK(r):
			w += 10
		case r == ' ':
			w += 3
		default:
			w += 5
		}
	}
	return w
}

func TestWrapTextNeverDropsCharacters(t *testing.T) {
	for _, src := range []string{
		strings.Repeat("这是一段没有空格的很长的中文文字，用来检查自动换行。", 5),
		strings.Repeat("日本語の長い文章です。", 8),
		"Hello world this is a fairly long English sentence with several words",
		"averyveryveryverylongwordwithoutanyspacesatall and short",
		"混合 mixed 文字 text，含标点。（括号）「引号」",
		"a\nb\n\nc",
	} {
		for _, maxW := range []float64{25, 47, 100, 333} {
			lines := wrapText(src, maxW, testWidth)
			strip := func(s string) string {
				return strings.Map(func(r rune) rune {
					if r == ' ' || r == '\n' {
						return -1
					}
					return r
				}, s)
			}
			if got, want := strip(strings.Join(lines, "")), strip(src); got != want {
				t.Fatalf("maxW=%v 丢字或多字:\n got %q\nwant %q", maxW, got, want)
			}
			for _, l := range lines {
				if !utf8.ValidString(l) {
					t.Fatalf("非法 UTF-8: %q", l)
				}
				// 单字符行允许超宽（宽度小于一个字符时）；其余不得超宽
				if testWidth(l) > maxW && utf8.RuneCountInString(l) > 1 && !strings.ContainsAny(l, "，。（）「」、") {
					t.Fatalf("行超宽 maxW=%v: %q", maxW, l)
				}
			}
		}
	}
}

func TestWrapTextKinsoku(t *testing.T) {
	// 宽 30 只放 3 个汉字：句号不能落在行首
	lines := wrapText("你好世界。再见", 30, testWidth)
	for _, l := range lines {
		if strings.HasPrefix(l, "。") {
			t.Fatalf("行首出现句号: %q", lines)
		}
	}
}

func TestWrapTextEmptyAndNewlines(t *testing.T) {
	if l := wrapText("", 100, testWidth); len(l) != 1 || l[0] != "" {
		t.Fatalf("%q", l)
	}
	if l := wrapText("a\nb", 100, testWidth); len(l) != 2 {
		t.Fatalf("%q", l)
	}
}
