package doc

import (
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"
	"unicode"
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
			lines, _ := wrapText(context.Background(), src, maxW, testWidth)
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
	lines, _ := wrapText(context.Background(), "你好世界。再见", 30, testWidth)
	for _, l := range lines {
		if strings.HasPrefix(l, "。") {
			t.Fatalf("行首出现句号: %q", lines)
		}
	}
}

func TestWrapTextEmptyAndNewlines(t *testing.T) {
	if l, _ := wrapText(context.Background(), "", 100, testWidth); len(l) != 1 || l[0] != "" {
		t.Fatalf("%q", l)
	}
	if l, _ := wrapText(context.Background(), "a\nb", 100, testWidth); len(l) != 2 {
		t.Fatalf("%q", l)
	}
}

func stripWS(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// lineWidthOK 是行宽约束：整行 ≤ maxW；或去掉行尾至多 maxHang 个挂起的行首禁则标点后 ≤ maxW；或本来就只有一个字符（比行宽还宽）。
func lineWidthOK(l string, maxW float64) bool {
	if testWidth(l) <= maxW {
		return true
	}
	rs := []rune(l)
	if len(rs) <= 1 {
		return true
	}
	core := rs
	for k := 0; k < maxHang && len(core) > 1 && strings.ContainsRune(noLineStart, core[len(core)-1]); k++ {
		core = core[:len(core)-1]
	}
	return testWidth(string(core)) <= maxW || len(core) <= 1
}

// 回归：行首空白 + 禁则标点曾让整段消失（"  ,abc" → [""]）。
func TestWrapTextLeadingSpaceThenKinsokuKeepsContent(t *testing.T) {
	for _, src := range []string{"  ,abc", " .5 tail", "\t%x", "   ，你好", "  。", "  )", ",", "  ,", " \t ,,, x"} {
		for _, maxW := range []float64{5, 12, 40, 200} {
			lines, err := wrapText(context.Background(), src, maxW, testWidth)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := stripWS(strings.Join(lines, "")), stripWS(src); got != want {
				t.Fatalf("src=%q maxW=%v 丢字: got %q lines=%q", src, maxW, got, lines)
			}
		}
	}
}

// 属性测试：固定种子、多轮随机语料（空白、标点、CJK、拉丁、数字、禁则字符）：
// 折行后所有行拼接、去空白后与原文去空白一致；行宽约束不被破坏；不产生非法 UTF-8。
func TestWrapTextPropertyRandomCorpus(t *testing.T) {
	alphabet := []rune(" \t  ,.%;:!?)]}(「」（）。，、…—ー" + "abcXYZ019" + "中文日本語あいうカタカナ한" + "\n")
	rng := rand.New(rand.NewSource(20260929))
	for round := 0; round < 3000; round++ {
		n := rng.Intn(60)
		rs := make([]rune, n)
		for i := range rs {
			rs[i] = alphabet[rng.Intn(len(alphabet))]
		}
		src := string(rs)
		maxW := float64(5 + rng.Intn(120))
		lines, err := wrapText(context.Background(), src, maxW, testWidth)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := stripWS(strings.Join(lines, "")), stripWS(src); got != want {
			t.Fatalf("round %d maxW=%v src=%q\n got %q", round, maxW, src, got)
		}
		for _, l := range lines {
			if !utf8.ValidString(l) {
				t.Fatalf("round %d 非法 UTF-8 %q", round, l)
			}
			if !lineWidthOK(l, maxW) {
				t.Fatalf("round %d 行超宽 maxW=%v line=%q (w=%v) src=%q", round, maxW, l, testWidth(l), src)
			}
			if l != strings.TrimRight(l, " \t") {
				t.Fatalf("round %d 行尾有空白 %q", round, l)
			}
		}
	}
}

// 连续标点曾是二次方（20 万个 ， 要 8 秒）：100 万个必须在几秒内完成，且不丢字。
func TestWrapTextManyPunctuationIsLinear(t *testing.T) {
	src := strings.Repeat("，", 1_000_000)
	start := time.Now()
	lines, err := wrapText(context.Background(), src, 100, testWidth)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("100 万个连续标点折行用了 %v", d)
	}
	if strings.Count(strings.Join(lines, ""), "，") != 1_000_000 {
		t.Fatal("丢字")
	}
	for _, src := range []string{strings.Repeat(",", 500_000), strings.Repeat("a,", 300_000), strings.Repeat(" ", 500_000) + ",x", strings.Repeat("（", 500_000)} {
		start := time.Now()
		if _, err := wrapText(context.Background(), src, 100, testWidth); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Fatalf("折行用了 %v", d)
		}
	}
}

func TestWrapTextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := wrapText(ctx, strings.Repeat("，", 5_000_000), 100, testWidth); err == nil {
		t.Fatal("已取消的 ctx 应返回错误")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("取消后 %v 才返回", d)
	}
}
