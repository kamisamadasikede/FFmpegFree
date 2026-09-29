package doc

import (
	"crypto/sha256"
	"encoding/hex"
	"golang.org/x/text/encoding/japanese"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustFace(t *testing.T, data []byte) *face {
	t.Helper()
	sf, err := parseSfnt(data)
	if err != nil {
		t.Fatal(err)
	}
	return &face{name: "t", data: data, f: sf}
}

func TestEmbeddedFontIsStaticTrueType(t *testing.T) {
	sf, err := parseSfnt(embeddedTTF)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"fvar", "CFF ", "CFF2", "gvar"} {
		if _, ok := sf.tables[bad]; ok {
			t.Fatalf("内嵌字体不应含 %s 表", bad)
		}
	}
	if _, ok := sf.tables["glyf"]; !ok {
		t.Fatal("缺 glyf")
	}
	if string(embeddedTTF[:4]) != "\x00\x01\x00\x00" {
		t.Fatalf("sfnt 版本 %q（.otf 是 OTTO，.ttc 是 ttcf）", embeddedTTF[:4])
	}
}

// 契约 6.12.1「字体合规」：除 nameID 0（版权声明，含 Reserved Font Name 'Source'）外，name 表任何记录都不得含 Source；
// nameID 1/4/6/16/17 与 5/7/10 尤其如此；nameID 0、13、14 原样保留。
func TestEmbeddedFontNameHasNoSource(t *testing.T) {
	sf, err := parseSfnt(embeddedTTF)
	if err != nil {
		t.Fatal(err)
	}
	got := map[uint16]string{}
	for _, r := range sf.names() {
		if r.PlatformID != 3 || r.LanguageID != 1033 {
			t.Errorf("只应保留 Windows 英文记录: %+v", r)
		}
		got[r.NameID] = r.Value
		if r.NameID != 0 && strings.Contains(strings.ToLower(r.Value), "source") {
			t.Errorf("nameID %d 含 Source: %q", r.NameID, r.Value)
		}
	}
	for _, id := range []uint16{1, 4, 6, 16, 17, 5, 7, 10} {
		if v, ok := got[id]; ok && strings.Contains(strings.ToLower(v), "source") {
			t.Errorf("nameID %d 含 Source: %q", id, v)
		}
	}
	for _, id := range []uint16{7, 8, 9, 10, 11, 12, 16, 17} {
		if v, ok := got[id]; ok {
			t.Errorf("nameID %d 应已删除: %q", id, v)
		}
	}
	if v := got[0]; !strings.Contains(v, "Adobe") || !strings.Contains(v, "Reserved Font Name 'Source'") {
		t.Errorf("nameID 0 应原样保留版权声明: %q", v)
	}
	if got[13] == "" || !strings.Contains(got[13], "SIL Open Font License") {
		t.Errorf("nameID 13 应保留: %q", got[13])
	}
	if got[14] == "" {
		t.Error("nameID 14 应保留")
	}
	want := map[uint16]string{1: "FFmpegFree CJK Subset", 4: "FFmpegFree CJK Subset Regular", 6: "FFmpegFreeCJKSubset-Regular",
		5: "Version 1.0; subset of Noto Sans SC 2.004 wght=400", 3: "FFmpegFreeCJKSubset-Regular;subset"}
	for id, v := range want {
		if got[id] != v {
			t.Errorf("nameID %d = %q，期望 %q", id, got[id], v)
		}
	}
}

// 字体文件的 SHA-256 与 README 记录一致（换字体必须同时更新 README，避免来源记录过期）。
func TestEmbeddedFontSHA256MatchesReadme(t *testing.T) {
	sum := sha256.Sum256(embeddedTTF)
	hexs := hex.EncodeToString(sum[:])
	readme, err := os.ReadFile("fonts/README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), hexs) {
		t.Fatalf("fonts/README.md 没有记录当前字体的 SHA-256 %s", hexs)
	}
	ofl, err := os.ReadFile("fonts/OFL.txt")
	if err != nil {
		t.Fatal(err)
	}
	osum := sha256.Sum256(ofl)
	if !strings.Contains(string(readme), hex.EncodeToString(osum[:])) {
		t.Fatal("fonts/README.md 没有记录 OFL.txt 的 SHA-256")
	}
}

func TestOFLShipsAlongside(t *testing.T) {
	b, err := os.ReadFile("fonts/OFL.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "SIL OPEN FONT LICENSE Version 1.1") || !strings.Contains(string(b), "Reserved Font Name 'Source'") {
		t.Fatal("OFL.txt 应是原样的 OFL 1.1 且保留版权行")
	}
	if _, err := os.Stat("fonts/README.md"); err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddedCoverage(t *testing.T) {
	f := mustFace(t, embeddedTTF)
	for _, r := range "你好世界的一是不了Hello 0123 àéÿ，。！？、（）“”—…あいうカタカナ　｜Ａ→∑■°±×÷" {
		if !f.has(r) {
			t.Errorf("内嵌字体应覆盖 %q U+%04X", r, r)
		}
	}
	// GB2312 全部 6763 个汉字
	n := 0
	for hi := 0xB0; hi <= 0xF7; hi++ {
		for lo := 0xA1; lo <= 0xFE; lo++ {
			s, err := gb2312(byte(hi), byte(lo))
			if err != nil {
				continue
			}
			n++
			if !f.has(s) {
				t.Fatalf("GB2312 汉字 %q 缺失", s)
			}
		}
	}
	if n != 6763 {
		t.Fatalf("GB2312 汉字数 %d", n)
	}
	// JIS X 0208 第一水准（EUC-JP 0xB0A1–0xCFFE）全部汉字：日文文档常用字（語、読、黒、龍 …）
	jn := 0
	for hi := 0xB0; hi <= 0xCF; hi++ {
		for lo := 0xA1; lo <= 0xFE; lo++ {
			out, err := japanese.EUCJP.NewDecoder().Bytes([]byte{byte(hi), byte(lo)})
			rs := []rune(string(out))
			if err != nil || len(rs) != 1 || rs[0] == 0xFFFD {
				continue
			}
			jn++
			if !f.has(rs[0]) {
				t.Fatalf("JIS 第一水准汉字 %q 缺失", rs[0])
			}
		}
	}
	if jn != 2965 {
		t.Fatalf("JIS 第一水准汉字数 %d", jn)
	}
	for _, r := range "龘齉\U0001F600ᄀ" { // GB2312 之外的生僻字、emoji、谚文：没有
		if f.has(r) {
			t.Errorf("不应覆盖 %q", r)
		}
	}
	if !f.cjk() {
		t.Fatal("cjk 应为 true")
	}
}

func TestParseSfntRejects(t *testing.T) {
	for name, data := range map[string][]byte{
		"空":   nil,
		"ttc": append([]byte("ttcf"), make([]byte, 64)...),
		"otf": append([]byte("OTTO"), make([]byte, 64)...),
		"乱":   []byte("hello world, this is not a font at all"),
	} {
		if _, err := parseSfnt(data); err == nil {
			t.Errorf("%s 应被拒绝", name)
		}
	}
	// 可变字体（有 fvar）被拒绝：用真实的源文件如果存在，否则跳过
	if b, err := os.ReadFile("/tmp/pdft/NotoSansSC-var.ttf"); err == nil {
		if _, err := parseSfnt(b); err == nil {
			t.Error("可变字体应被拒绝")
		}
	}
}

func TestCmapFormat4Latin(t *testing.T) {
	// DejaVu Sans（系统字体，cmap 有 format 4 / 12）：拉丁有，CJK 无
	p := "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
	b, err := os.ReadFile(p)
	if err != nil {
		t.Skip("没有 DejaVuSans")
	}
	f := mustFace(t, b)
	if !f.has('A') || !f.has('é') || !f.has('ế') || !f.has('α') {
		t.Fatal("DejaVu 应覆盖拉丁 / 西里尔 / 希腊")
	}
	if f.has('你') || f.has('あ') {
		t.Fatal("DejaVu 不含 CJK")
	}
	if f.cjk() {
		t.Fatal("DejaVu cjk 应为 false")
	}
}

func rs(s string) map[rune]int {
	m := map[rune]int{}
	for _, r := range s {
		m[r]++
	}
	return m
}

func TestChooseFontRules(t *testing.T) {
	dejavu := "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
	if _, err := os.Stat(dejavu); err != nil {
		t.Skip("没有 DejaVuSans")
	}
	sys := []SystemFont{{"dejavu", dejavu}}

	// 1. 内嵌覆盖 → 内嵌
	fs := newFontSet(Config{SystemFonts: sys})
	if c, ok := fs.choose(rs("你好 Hello")); !ok || c.name() != EmbeddedFontName {
		t.Fatalf("%v %v", c.name(), ok)
	}
	// 2. 内嵌不覆盖（越南语 ế U+1EBF 不在子集），系统字体覆盖 → 整份文档用系统字体
	if fs.embedded.has('ế') {
		t.Skip("内嵌子集意外含 ế")
	}
	if c, ok := fs.choose(rs("Hello Việt")); !ok || c.name() != "dejavu" {
		t.Fatalf("应改用系统字体: %v %v", c.name(), ok)
	}
	// 3. 内嵌和系统都不能全覆盖 → 仍用内嵌（缺字方框，不算失败）
	if c, ok := fs.choose(rs("你好 ế")); !ok || c.name() != EmbeddedFontName {
		t.Fatalf("应回到内嵌: %v %v", c.name(), ok)
	}
	// 4. 内嵌加载失败 + 系统可用 → 系统字体
	fs = newFontSet(Config{DisableEmbedded: true, SystemFonts: sys})
	if c, ok := fs.choose(rs("Hello Việt")); !ok || c.name() != "dejavu" {
		t.Fatalf("%v %v", c.name(), ok)
	}
	// 5. 内嵌失败 + 没有系统字体：纯 Latin-1 → Helvetica；含 CJK → 不可用
	fs = newFontSet(Config{DisableEmbedded: true, SystemFonts: []SystemFont{}})
	if c, ok := fs.choose(rs("Hello Àé ÿ")); !ok || c.name() != "helvetica" {
		t.Fatalf("%v %v", c.name(), ok)
	}
	if _, ok := fs.choose(rs("你好")); ok {
		t.Fatal("没有字体且含 CJK 应不可用")
	}
	if _, ok := fs.choose(rs("Ā")); ok { // U+0100 已超过 Latin-1
		t.Fatal("U+0100 超出 Latin-1 应不可用")
	}
	// 6. 系统字体路径不存在 / 不是 ttf → 跳过
	junk := filepath.Join(t.TempDir(), "x.ttf")
	os.WriteFile(junk, []byte("not a font"), 0o644)
	fs = newFontSet(Config{DisableEmbedded: true, SystemFonts: []SystemFont{{"a", "/no/such.ttf"}, {"b", junk}, {"dejavu", dejavu}}})
	if c, ok := fs.choose(rs("Hello")); !ok || c.name() != "dejavu" {
		t.Fatalf("%v %v", c.name(), ok)
	}
}

func TestMissingRunesStats(t *testing.T) {
	f := mustFace(t, embeddedTTF)
	k, total, sample := missingRunes(f, map[rune]int{'a': 5, '龘': 2, '齉': 1, ' ': 9, '\n': 1, '好': 3})
	if k != 2 || total != 3 || len(sample) != 2 {
		t.Fatalf("%d %d %v", k, total, string(sample))
	}
	k, total, _ = missingRunes(nil, map[rune]int{'a': 1, 'é': 1, '好': 4, 'Ā': 1})
	if k != 2 || total != 5 {
		t.Fatalf("Helvetica 统计 %d %d", k, total)
	}
}

func TestDocxWithoutAnyUnicodeFontIsUnsupported(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.DisableEmbedded = true; c.SystemFonts = []SystemFont{} })
	cjk := filepath.Join(e.dir, "cjk.docx")
	makeDocx(t, cjk, "你好")
	_, err := e.svc.ConvertToPDF(t.Context(), []string{cjk}, "")
	ae := wantCode(t, err, "UNSUPPORTED")
	if !strings.Contains(ae.Detail, "没有可用的字体") && !strings.Contains(ae.Message, "Unicode 字体") {
		t.Fatalf("%+v", ae)
	}
	if l := e.tm.ListActive(); len(l) != 0 {
		t.Fatal("不应提交任务")
	}
	// 纯 Latin-1 文档仍可用内置字体转换
	lat := filepath.Join(e.dir, "lat.docx")
	makeDocx(t, lat, "Hello World Àé")
	tk := e.convertOK(t, lat, "")
	if txt := pdfText(t, tk.OutputPath); !strings.Contains(noSpace(txt), "HelloWorldÀé") {
		t.Fatalf("%q", txt)
	}
	log, _ := e.tm.GetLog(tk.ID, 20)
	if !strings.Contains(log, "font=helvetica") {
		t.Fatal(log)
	}
	c := e.svc.GetDocCapabilities()
	if c.Font.Available || c.Font.Name != "" || c.Font.Cjk {
		t.Fatalf("%+v", c.Font)
	}
}

func TestSystemFontUsedForWholeDoc(t *testing.T) {
	dejavu := "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
	if _, err := os.Stat(dejavu); err != nil {
		t.Skip("没有 DejaVuSans")
	}
	e := newEnv(t, func(c *Config) { c.SystemFonts = []SystemFont{{"dejavu", dejavu}} })
	in := filepath.Join(e.dir, "cyr.docx")
	makeDocx(t, in, "Việt Nam Hello ế")
	tk := e.convertOK(t, in, "")
	log, _ := e.tm.GetLog(tk.ID, 20)
	if !strings.Contains(log, "font=dejavu") || strings.Contains(log, "missing_glyphs") {
		t.Fatal(log)
	}
	if txt := noSpace(pdfText(t, tk.OutputPath)); !strings.Contains(txt, "ViệtNamHelloế") {
		t.Fatalf("%q", txt)
	}
}

func TestCapabilities(t *testing.T) {
	e := newEnv(t)
	c := e.svc.GetDocCapabilities()
	if !c.Experimental {
		t.Fatal("experimental 首版应为 true")
	}
	if !c.Font.Available || c.Font.Name != "noto-sans-sc-embedded" || !c.Font.Cjk {
		t.Fatalf("%+v", c.Font)
	}
	sup := map[string]DocFormat{}
	for _, f := range c.Formats {
		sup[f.Ext] = f
	}
	for _, x := range []string{"docx", "xlsx", "pptx"} {
		if f := sup[x]; !f.Supported || f.Fidelity != "text-only" {
			t.Fatalf("%s: %+v", x, f)
		}
	}
	for _, x := range []string{"doc", "xls", "ppt", "odt", "ods", "odp", "rtf", "csv", "txt"} {
		if f := sup[x]; f.Ext == "" || f.Supported || f.Reason == "" || f.Fidelity != "" {
			t.Fatalf("%s: %+v", x, f)
		}
	}
	if !strings.Contains(sup["csv"].Reason, "不支持") || !strings.Contains(sup["txt"].Reason, "不支持") {
		t.Fatal("csv/txt 应标 unsupported")
	}
	l := c.Limits
	if l.MaxInputsPerSubmit != 50 || l.MaxInputBytes != 100<<20 || l.MaxPages != 5000 || l.MaxPDFBytes != 512<<20 || l.ChunkBytes != 1<<20 || l.WholeLoadBytes != 64<<20 {
		t.Fatalf("%+v", l)
	}
}
