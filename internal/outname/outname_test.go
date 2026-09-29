package outname

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"报告 2026 年度", "报告 2026 年度"},
		{"a/b\\c:d*e?f\"g<h>i|j", "abcdefghij"},
		{"tab\there\x00\x1f\u0085x", "tabherex"},
		{"  name. . ", "name"},
		{"...", "x"},
		{"", "x"},
		{"CON", "_CON"},
		{"con", "_con"},
		{"CON.txt", "_CON.txt"},
		{"nul .x", "_nul .x"},
		{"COM1", "_COM1"},
		{"COM¹", "_COM¹"},
		{"lpt³", "_lpt³"},
		{"COM10", "COM10"},
		{"CONSOLE", "CONSOLE"},
		{"COMa", "COMa"},
		{"カタカナ.日本語", "カタカナ.日本語"},
	}
	for _, c := range cases {
		if got := Sanitize(c.in, "x"); got != c.want {
			t.Errorf("Sanitize(%q)=%q 期望 %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeLimits(t *testing.T) {
	// 200 个 ASCII → 100 字符
	got := Sanitize(strings.Repeat("a", 200), "x")
	if utf8.RuneCountInString(got) != 100 {
		t.Fatalf("字符数 %d", utf8.RuneCountInString(got))
	}
	// 100 个汉字 = 300 字节 → 受 200 字节限制，66 个字
	got = Sanitize(strings.Repeat("字", 100), "x")
	if len(got) > MaxBytes || utf8.RuneCountInString(got) != 66 {
		t.Fatalf("bytes=%d runes=%d", len(got), utf8.RuneCountInString(got))
	}
	// 截断后露出尾部空格 / 点要再去掉
	in := strings.Repeat("a", 99) + " " + "bbb"
	if got := Sanitize(in, "x"); got != strings.Repeat("a", 99) {
		t.Fatalf("%q", got)
	}
	in = strings.Repeat("a", 96) + "CON" + "xyz"
	if got := Sanitize(in, "x"); strings.HasSuffix(got, " ") || utf8.RuneCountInString(got) > 100 {
		t.Fatalf("%q", got)
	}
	// 保留名 + 恰好 100 字符：加下划线后仍不超限
	got = Sanitize("CON"+strings.Repeat("a", 0), "x")
	if got != "_CON" {
		t.Fatal(got)
	}
	// 截断后成为保留名："CON" + 空格 + 后续
	if got := Sanitize("CON"+strings.Repeat(" ", 1)+strings.Repeat("z", 200), "x"); isReserved(got) {
		t.Fatalf("不应是保留名: %q", got)
	}
}

func TestSanitizeFallback(t *testing.T) {
	if got := Sanitize("///", "document"); got != "document" {
		t.Fatal(got)
	}
	if got := Sanitize("", ""); got != "file" {
		t.Fatal(got)
	}
}

func TestFitStem(t *testing.T) {
	dir := `C:\` + strings.Repeat("d", 200)
	stem, ok := FitStem(dir, strings.Repeat("n", 100), "f", 20, WindowsMaxPath)
	if !ok || Utf16Len(dir)+1+Utf16Len(stem)+20 > WindowsMaxPath {
		t.Fatalf("%d %v", len(stem), ok)
	}
	if len(stem) != WindowsMaxPath-Utf16Len(dir)-1-20 {
		t.Fatalf("应尽量多留: %d", len(stem))
	}
	// 目录太长，一个字符都放不下
	if _, ok := FitStem(`C:\`+strings.Repeat("d", 250), "a", "f", 20, WindowsMaxPath); ok {
		t.Fatal("应放不下")
	}
	// 不限制
	if s, ok := FitStem(dir, "abc", "f", 20, 0); !ok || s != "abc" {
		t.Fatal(s, ok)
	}
	// 截断露出尾部点：a....b → 预算 5 时得到 "a"
	s, ok := FitStem("C:", "a....b", "f", WindowsMaxPath-2-1-5, WindowsMaxPath)
	if !ok || strings.HasSuffix(s, ".") {
		t.Fatalf("%q %v", s, ok)
	}
}
