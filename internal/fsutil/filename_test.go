package fsutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"我的视频", "我的视频"},
		{"日本語のタイトル 한국어", "日本語のタイトル 한국어"},
		{"a/b\\c:d*e?f\"g<h>i|j", "abcdefghij"},
		{"a\x00b\x1fc\x7fd\u0085e", "abcde"},
		{"  hello  ", "hello"},
		{"name...", "name"},
		{"name. . .", "name"},
		{"name \t", "name"},
		{"..", ""},
		{".", ""},
		{"...   ", ""},
		{"", ""},
		{"a\u200bb\u200fc\u202ed\u2066e\u2069f\ufeffg", "abcdefg"},
		{"e\u0301", "é"}, // NFC
		{"CON", "_CON"},
		{"con", "_con"},
		{"Con.txt", "_Con.txt"},
		{"NUL.tar.gz", "_NUL.tar.gz"},
		{"COM1", "_COM1"},
		{"com0", "_com0"},
		{"LPT9.log", "_LPT9.log"},
		{"COM¹", "_COM¹"},
		{"LPT²", "_LPT²"},
		{"COM³.x", "_COM³.x"},
		{"COM１", "_COM１"},
		{"CON .txt", "_CON .txt"},
		{"COM10", "COM10"},
		{"CONSOLE", "CONSOLE"},
		{"COM", "COM"},
		{"my.CON", "my.CON"},
	}
	for _, c := range cases {
		if got := SanitizeFileName(c.in); got != c.want {
			t.Errorf("Sanitize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// 先删非法字符再判断保留名："C:\ON" → "CON" → "_CON"；"CON:" → "CON" → "_CON"
	if got := SanitizeFileName("CON:"); got != "_CON" {
		t.Errorf("CON: → %q", got)
	}
	if got := SanitizeFileName("C/O/N"); got != "_CON" {
		t.Errorf("C/O/N → %q", got)
	}
	if got := SanitizeFileName("C\u200bON"); got != "_CON" {
		t.Errorf("零宽字符夹在保留名中: %q", got)
	}
}

func TestSanitizeInvalidUTF8(t *testing.T) {
	got := SanitizeFileName("a\xffb\xfe")
	if got != "ab" || !utf8.ValidString(got) {
		t.Fatalf("%q", got)
	}
}

func TestSanitizeTruncate(t *testing.T) {
	long := strings.Repeat("a", 150)
	if got := SanitizeFileName(long); utf8.RuneCountInString(got) != 100 {
		t.Fatalf("runes=%d", utf8.RuneCountInString(got))
	}
	// CJK：100 个字 = 300 字节 > 200，按字节再截到 66 个字（198 字节）
	cjk := strings.Repeat("字", 150)
	got := SanitizeFileName(cjk)
	if len(got) > MaxNameBytes || utf8.RuneCountInString(got) != 66 || !utf8.ValidString(got) {
		t.Fatalf("bytes=%d runes=%d", len(got), utf8.RuneCountInString(got))
	}
	// 4 字节字符（emoji）：50 个 = 200 字节
	em := strings.Repeat("😀", 120)
	got = SanitizeFileName(em)
	if len(got) > MaxNameBytes || utf8.RuneCountInString(got) != 50 {
		t.Fatalf("emoji bytes=%d runes=%d", len(got), utf8.RuneCountInString(got))
	}
	// 截断后再处理：截断点落在尾部空白 / 点上
	s := strings.Repeat("a", 97) + "..." + "zzz"
	got = SanitizeFileName(s)
	if got != strings.Repeat("a", 97) {
		t.Fatalf("截断后应再去掉尾部的点: %q", got)
	}
	// 截断后暴露出保留名
	got = SanitizeFileName("CON" + strings.Repeat(" ", 100) + "x")
	if got != "_CON" {
		t.Fatalf("截断暴露保留名: %q", got)
	}
	// 结果永远不超过 100 字符，含保留名前缀
	for _, in := range []string{"CON." + strings.Repeat("x", 200), "NUL" + strings.Repeat(".", 300)} {
		g := SanitizeFileName(in)
		if utf8.RuneCountInString(g) > MaxNameRunes || IsReservedName(g) {
			t.Fatalf("%q → %q", in, g)
		}
	}
}

func TestSanitizeOr(t *testing.T) {
	if SanitizeFileNameOr("///", DefaultName) != "edit" || SanitizeFileNameOr("x", "edit") != "x" {
		t.Fatal("fallback")
	}
}

func TestSanitizeIdempotent(t *testing.T) {
	for _, in := range []string{"CON", "a b", "  x.", "字字字", "e\u0301", "COM¹.txt"} {
		a := SanitizeFileName(in)
		if b := SanitizeFileName(a); a != b {
			t.Errorf("%q: %q != %q", in, a, b)
		}
	}
}

func TestOutputPathTooLong(t *testing.T) {
	dir := `C:\` + strings.Repeat("d", 100)
	name := strings.Repeat("n", 100)
	// 4+100... = len(dir)=103, +1 +100 +4 +4 +5 = 217
	if OutputPathLength(dir, name, ".mp4") != 103+1+100+4+4+5 {
		t.Fatalf("len=%d", OutputPathLength(dir, name, ".mp4"))
	}
	if OutputPathTooLong("windows", dir, name, ".mp4") {
		t.Fatal("217 不应超长")
	}
	long := dir + `\` + strings.Repeat("e", 60)
	if !OutputPathTooLong("windows", long, name, ".mp4") {
		t.Fatal("应超长")
	}
	if OutputPathTooLong("linux", long, name, ".mp4") {
		t.Fatal("非 Windows 不检查")
	}
	// 边界：恰好 259 通过，260 失败
	base := OutputPathLength(`C:\x`, name, ".mp4")
	pad := strings.Repeat("p", 259-base)
	if OutputPathTooLong("windows", `C:\x`+pad, name, ".mp4") {
		t.Fatal("259 应通过")
	}
	if !OutputPathTooLong("windows", `C:\x`+pad+"p", name, ".mp4") {
		t.Fatal("260 应失败")
	}
	// 尾部分隔符不重复计
	if OutputPathLength(`C:\x\`, "a", ".mp4") != OutputPathLength(`C:\x`, "a", ".mp4") {
		t.Fatal("trailing sep")
	}
	// UTF-16：emoji 占 2 个单元
	if OutputPathLength(`C:\😀`, "a", ".mp4") != OutputPathLength(`C:\ab`, "a", ".mp4") {
		t.Fatal("utf16")
	}
}
