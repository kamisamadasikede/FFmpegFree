package doc

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doccomp"
	"FFmpegFree/internal/doceng"
	"FFmpegFree/internal/task"
)

// ---------- v0.28 PDF 作为输入（6.12.64 的单测） ----------

type pdfSpec struct {
	pages      int
	lines      []string // 每页写这些行（ASCII，Helvetica）
	user, own  string   // 非空时加密（RC4）
	protect    bool
	noTextPage bool // 只画一个方框，没有文字层（扫描件）
}

func makePDF(t *testing.T, path string, sp pdfSpec) string {
	t.Helper()
	p := fpdf.New("P", "mm", "A4", "")
	if sp.protect {
		p.SetProtection(fpdf.CnProtectPrint, sp.user, sp.own)
	}
	p.SetFont("Helvetica", "", 12)
	n := sp.pages
	if n == 0 {
		n = 1
	}
	for i := 0; i < n; i++ {
		p.AddPage()
		if sp.noTextPage {
			p.Rect(20, 20, 100, 60, "F")
			continue
		}
		for _, l := range sp.lines {
			p.CellFormat(0, 8, l, "", 1, "L", false, 0, "")
		}
	}
	var b bytes.Buffer
	if err := p.Output(&b); err != nil {
		t.Fatal(err)
	}
	return writeFile(t, path, b.Bytes())
}

func TestPDFQuality(t *testing.T) {
	cases := []struct {
		s, want string
	}{
		{"", qualityEmpty},
		{" \n\t\r\u3000\u00a0", qualityEmpty},
		{"正常的文字 hello", ""},
		// 10 个非空白字符里 3 个私用区 = 30% → garbled
		{"abcdefg\ue000\ue001\U000F0000", qualityGarbled},
		// 100 个里 29 个 → 通过
		{strings.Repeat("a", 71) + strings.Repeat("\ufffd", 29), ""},
		{strings.Repeat("a", 70) + strings.Repeat("\ufffd", 30), qualityGarbled},
		// 换行、制表符本来就是空白，不计入；其他控制字符计入
		{"ab\n\n\n\t\t\tcd", ""},
		{"ab\x01\x02", qualityGarbled},
		{"abcdefg\U00100000\x07\ufffd", qualityGarbled},
	}
	for _, c := range cases {
		if got := pdfQuality(c.s); got != c.want {
			t.Errorf("%q → %q，应为 %q", c.s, got, c.want)
		}
	}
}

func TestPDFMarkdownEscapeAndTxt(t *testing.T) {
	pages := []pdfPage{
		{paras: [][]string{{"# not heading"}, {"1. not list"}, {"a*b_c [x](y) <tag> `code` \\"}, {"- item", "+ plus"}}},
		{paras: [][]string{{"第二页", "接着"}, {"> quote | pipe"}}},
	}
	md := string(renderPDFMarkdown(pages))
	for _, want := range []string{"\\# not heading", "1\\. not list", "a\\*b\\_c \\[x\\](y) \\<tag> \\`code\\` \\\\", "\\- item + plus", "第二页接着", "\\> quote | pipe"} {
		if !strings.Contains(md, want) {
			t.Errorf("md 缺 %q：\n%s", want, md)
		}
	}
	if strings.Contains(md, "\r") || bytes.HasPrefix([]byte(md), []byte{0xEF, 0xBB, 0xBF}) || !strings.Contains(md, "\n\n") {
		t.Errorf("md 换行 / BOM 不对：%q", md)
	}
	txt := renderPDFTxt(pages, "\r\n")
	if bytes.HasPrefix(txt, []byte{0xEF, 0xBB, 0xBF}) || !bytes.Contains(txt, []byte("第二页\r\n接着")) || !bytes.Contains(txt, []byte("\r\n\r\n")) {
		t.Errorf("txt：%q", txt)
	}
	if strings.Contains(string(renderPDFTxt(pages, "\n")), "\r") {
		t.Error("非 Windows 换行是 \\n")
	}
	h := string(renderPDFSimpleHTML(pages, "a<b"))
	if !strings.Contains(h, "<title>a&lt;b</title>") || !strings.Contains(h, "<hr>") || !strings.Contains(h, "<p>&lt;tag&gt;") && !strings.Contains(h, "&lt;tag&gt;") {
		t.Errorf("html：%s", h)
	}
	if want := "\n"; txtNewline() != want && txtNewline() != "\r\n" {
		t.Error(txtNewline())
	}
}

func TestInspectPDF(t *testing.T) {
	d := t.TempDir()
	ctx := context.Background()
	ok := makePDF(t, filepath.Join(d, "ok.pdf"), pdfSpec{pages: 3, lines: []string{"hello"}})
	if n, err := inspectPDF(ctx, ok); err != nil || n != 3 {
		t.Fatalf("%d %v", n, err)
	}
	many := makePDF(t, filepath.Join(d, "many.pdf"), pdfSpec{pages: 501, lines: []string{"x"}})
	if _, err := inspectPDF(ctx, many); !apperr.Is(err, apperr.InvalidArgument) || !strings.HasPrefix(apperr.From(err).Detail, "reason=too_many_pages") ||
		apperr.From(err).Message != "PDF 页数太多，最多支持 500 页。" {
		t.Fatalf("501 页：%v", err)
	}
	if _, err := inspectPDF(ctx, makePDF(t, filepath.Join(d, "500.pdf"), pdfSpec{pages: 500, lines: []string{"x"}})); err != nil {
		t.Fatalf("500 页应通过：%v", err)
	}
	owner := makePDF(t, filepath.Join(d, "owner.pdf"), pdfSpec{lines: []string{"owner only"}, protect: true, own: "secret"})
	if _, err := inspectPDF(ctx, owner); err != nil {
		t.Fatalf("只有所有者密码应能转：%v", err)
	}
	user := makePDF(t, filepath.Join(d, "user.pdf"), pdfSpec{lines: []string{"x"}, protect: true, user: "123", own: "abc"})
	if _, err := inspectPDF(ctx, user); !apperr.Is(err, apperr.DocEncrypted) || apperr.From(err).Message != "这个文件有密码保护，不能转换。请先去掉密码再添加。" {
		t.Fatalf("要用户密码：%v", err)
	}
	// 库不认的加密处理程序 → 也按加密
	b, _ := os.ReadFile(owner)
	weird := writeFile(t, filepath.Join(d, "weird.pdf"), bytes.Replace(b, []byte("/Filter /Standard"), []byte("/Filter /FooBarX"), 1))
	if !bytes.Contains(b, []byte("/Filter /Standard")) {
		t.Fatalf("夹具变了：%s", b[len(b)-600:])
	}
	if _, err := inspectPDF(ctx, weird); !apperr.Is(err, apperr.DocEncrypted) {
		t.Fatalf("不认的加密：%v", err)
	}
	// 读不出页数：不拒绝
	broken := writeFile(t, filepath.Join(d, "broken.pdf"), []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ngarbage without xref\n"))
	if n, err := inspectPDF(ctx, broken); err != nil || n != -1 {
		t.Fatalf("读不出页数：%d %v", n, err)
	}
	// 不是 PDF / 空文件 → DOC_CORRUPT
	if _, err := inspectPDF(ctx, writeFile(t, filepath.Join(d, "fake.pdf"), []byte("hello world"))); !apperr.Is(err, apperr.DocCorrupt) {
		t.Fatal(err)
	}
	if _, err := inspectPDF(ctx, writeFile(t, filepath.Join(d, "empty.pdf"), nil)); !apperr.Is(err, apperr.DocCorrupt) {
		t.Fatal(err)
	}
	// %PDF- 不在开头但在 1024 字节内：通过头检查
	pre := writeFile(t, filepath.Join(d, "pre.pdf"), append(bytes.Repeat([]byte{' '}, 100), b...))
	if _, err := inspectPDF(ctx, pre); apperr.Is(err, apperr.DocCorrupt) {
		t.Fatal(err)
	}
	// 201 MiB → too_large
	big := filepath.Join(d, "big.pdf")
	f, _ := os.Create(big)
	f.Write([]byte("%PDF-1.4\n"))
	f.Truncate(MaxPDFInputBytes + 1)
	f.Close()
	if _, err := inspectPDF(ctx, big); !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Detail != "reason=too_large" || apperr.From(err).Message != "PDF 太大了，最多支持 200 MB。" {
		t.Fatalf("201 MiB：%v", err)
	}
}

func TestAddPDFSources(t *testing.T) {
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	d := filepath.Join(e.dir, "src")
	os.MkdirAll(d, 0o755)
	ok := makePDF(t, filepath.Join(d, "ok.pdf"), pdfSpec{lines: []string{"hello"}})
	upper := makePDF(t, filepath.Join(d, "UP.PDF"), pdfSpec{lines: []string{"hello"}})
	owner := makePDF(t, filepath.Join(d, "owner.pdf"), pdfSpec{lines: []string{"x"}, protect: true, own: "o"})
	user := makePDF(t, filepath.Join(d, "user.pdf"), pdfSpec{lines: []string{"x"}, protect: true, user: "u", own: "o"})
	many := makePDF(t, filepath.Join(d, "many.pdf"), pdfSpec{pages: 501, lines: []string{"x"}})
	broken := writeFile(t, filepath.Join(d, "broken.pdf"), []byte("%PDF-1.4\nno xref here\n"))
	fake := writeFile(t, filepath.Join(d, "fake.pdf"), []byte("not a pdf"))
	big := filepath.Join(d, "big.pdf")
	f, _ := os.Create(big)
	f.Write([]byte("%PDF-1.4\n"))
	f.Truncate(201 << 20)
	f.Close()
	mid := filepath.Join(d, "mid.pdf") // 150 MiB：超过其他文档的 100 MiB，但 PDF 允许
	f, _ = os.Create(mid)
	f.Write([]byte("%PDF-1.4\n"))
	f.Truncate(150 << 20)
	f.Close()

	res := e.add(t, ok, upper, owner, broken, mid, user, many, fake, big)
	for i := 0; i < 5; i++ {
		r := res[i]
		if r.Error != nil || r.Source == nil || r.Source.Family != FamilyPDF || r.Source.Ext != "pdf" || r.Source.SheetCount != 0 {
			t.Errorf("%s 应建行：%+v %+v", r.Path, r.Error, r.Source)
		}
	}
	want := []struct {
		code   apperr.Code
		detail string
		msg    string
	}{
		{apperr.DocEncrypted, "", "这个文件有密码保护，不能转换。请先去掉密码再添加。"},
		{apperr.InvalidArgument, "reason=too_many_pages", "PDF 页数太多，最多支持 500 页。"},
		{apperr.DocCorrupt, "", ""},
		{apperr.InvalidArgument, "reason=too_large", "PDF 太大了，最多支持 200 MB。"},
	}
	for i, w := range want {
		r := res[5+i]
		if r.Source != nil || r.Error == nil || r.Error.Code != w.code || !strings.HasPrefix(r.Error.Detail, w.detail) || w.msg != "" && r.Error.Message != w.msg {
			t.Errorf("%s：%+v，应为 %v", r.Path, r.Error, w)
		}
	}
}

func TestPDFFormatMatrix(t *testing.T) {
	find := func(m DocFormatMatrix, ext string) *DocSourceFormats {
		for i := range m.Sources {
			if m.Sources[i].Ext == ext {
				return &m.Sources[i]
			}
		}
		return nil
	}
	combos := []struct {
		name         string
		word, comp   bool
		layoutAvail  bool
		layoutEngine []string
		htmlSimple   bool
		txtEngines   []string
	}{
		{"都没有", false, false, false, nil, true, []string{"go"}},
		{"只有 Word", true, false, true, []string{"office"}, true, []string{"go"}},
		{"只有组件", false, true, true, []string{"component"}, false, []string{"go", "component"}},
		{"都有", true, true, true, []string{"office", "component"}, false, []string{"go", "component"}},
	}
	for _, c := range combos {
		var engines []doceng.Detected
		if c.word {
			engines = append(engines, doceng.Detected{ID: doceng.IDOffice, Installed: true, Available: true, WordMajor: 16, Families: []string{"text", "pdf"}})
		}
		// WPS 永远不出现在 pdf 的 engines 里
		engines = append(engines, doceng.Detected{ID: doceng.IDWPS, Installed: true, Available: true, Families: []string{"text", "sheet", "slide", "pdf"}})
		if c.comp {
			engines = append(engines, doceng.Detected{ID: doceng.IDComponent, Installed: true, Available: true, Families: []string{"text", "sheet", "slide", "pdf"}})
		}
		m := buildMatrix(true, func(src, tg string) []string {
			return doceng.EnginesForTarget(doceng.PrefOrder("wps"), engines, nil, src, tg)
		})
		sf := find(m, "pdf")
		if sf == nil || sf.Family != FamilyPDF {
			t.Fatalf("%s：没有 pdf 源", c.name)
		}
		var exts []string
		for _, tg := range sf.Targets {
			exts = append(exts, tg.Ext)
			for _, id := range tg.Engines {
				if id == "wps" {
					t.Errorf("%s：wps 出现在 %s", c.name, tg.Ext)
				}
			}
			switch tg.Ext {
			case "doc", "docx", "odt", "rtf":
				if tg.Available != c.layoutAvail || !tg.NeedsComponent || tg.Simple || tg.HintKey != "pdf_layout" || strings.Join(tg.Engines, ",") != strings.Join(c.layoutEngine, ",") {
					t.Errorf("%s %s：%+v", c.name, tg.Ext, tg)
				}
				if !c.layoutAvail && tg.DisabledReason != "需要文档组件" {
					t.Errorf("%s：%+v", c.name, tg)
				}
			case "txt", "md":
				if !tg.Available || tg.NeedsComponent || !tg.Simple || strings.Join(tg.Engines, ",") != strings.Join(c.txtEngines, ",") {
					t.Errorf("%s %s：%+v", c.name, tg.Ext, tg)
				}
				if tg.Ext == "md" && tg.HintKey != "md_lossy" || tg.Ext == "txt" && (tg.HintKey != "simple_mode" || tg.Hint != "只提取文字，不保留排版和图片。") {
					t.Errorf("%s %s hint：%+v", c.name, tg.Ext, tg)
				}
			case "html":
				if !tg.Available || tg.NeedsComponent || tg.Simple != c.htmlSimple {
					t.Errorf("%s html：%+v", c.name, tg)
				}
				if c.htmlSimple && (tg.HintKey != "simple_mode" || strings.Join(tg.Engines, ",") != "go") || !c.htmlSimple && (tg.HintKey != "" || strings.Join(tg.Engines, ",") != "component") {
					t.Errorf("%s html：%+v", c.name, tg)
				}
			}
		}
		if strings.Join(exts, ",") != "doc,docx,odt,rtf,txt,html,md" {
			t.Errorf("%s 目标：%v", c.name, exts)
		}
	}
	m := buildMatrix(false, nil)
	if m.Inputs[len(m.Inputs)-1] != "pdf" {
		t.Errorf("inputs：%v", m.Inputs)
	}
	// 其他源不会多出 pdf 家族的目标
	if sf := find(m, "docx"); sf == nil || len(sf.Targets) != 7 {
		t.Errorf("docx：%+v", sf)
	}
}

// 没有组件：txt / md / 简易 html 走纯 Go（engine=go），txt 不带 BOM。
func TestPDFToTextNoComponent(t *testing.T) {
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	d := filepath.Join(e.dir, "src")
	os.MkdirAll(d, 0o755)
	p := makePDF(t, filepath.Join(d, "doc.pdf"), pdfSpec{pages: 2, lines: []string{"Hello PDF world", "# second line"}})
	r := e.add(t, p)
	if r[0].Error != nil {
		t.Fatal(r[0].Error)
	}
	id := r[0].Source.SourceID
	// doc / docx 没有 Word 也没有组件 → 提交被拒（需要文档组件）
	if _, err := e.svc.SubmitDocConvert(context.Background(), DocSubmitRequest{SourceIDs: []string{id}, Target: "docx"}); !apperr.Is(err, apperr.DocComponentNotReady) {
		t.Fatalf("docx：%v", err)
	}
	for _, tg := range []string{"txt", "md", "html"} {
		ts := e.submit(t, tg, id)
		var pm docParams
		_ = jsonUnmarshal(ts[0].Params, &pm)
		if pm.Engine != "go" || ts[0].Type != task.TypeDocConvert {
			t.Errorf("%s params：%s %s", tg, ts[0].Params, ts[0].Type)
		}
		done := e.wait(t, ts[0].ID)
		if done.Status != task.StatusSucceeded || done.Result == nil || done.Result.Engine != "go" {
			t.Fatalf("%s：%+v %+v", tg, done.Error, done.Result)
		}
		b, _ := os.ReadFile(done.OutputPath)
		if bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) || !bytes.Contains(b, []byte("Hello PDF world")) {
			t.Errorf("%s 输出：%q", tg, b)
		}
		hasFallback := false
		for _, w := range done.Result.Warnings {
			hasFallback = hasFallback || w == "simple_fallback"
		}
		if hasFallback != (tg == "html") {
			t.Errorf("%s warnings：%v", tg, done.Result.Warnings)
		}
		switch tg {
		case "txt":
			if !bytes.Contains(b, []byte("Hello PDF world"+txtNewline())) || !bytes.Contains(b, []byte("# second line"+txtNewline())) {
				t.Errorf("txt：%q", b)
			}
		case "md":
			if !bytes.Contains(b, []byte("\\# second line")) {
				t.Errorf("md：%q", b)
			}
		case "html":
			if !bytes.Contains(b, []byte("<hr>")) || !bytes.Contains(b, []byte("<title>doc</title>")) {
				t.Errorf("html：%s", b)
			}
		}
	}
}

// 提取失败 + 没组件 → DOC_PDF_NO_TEXT（不可重试）；库 panic 不崩溃 → DOC_CORRUPT（engine=go）。
func TestPDFNoTextAndPanic(t *testing.T) {
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	d := filepath.Join(e.dir, "src")
	os.MkdirAll(d, 0o755)
	scan := makePDF(t, filepath.Join(d, "scan.pdf"), pdfSpec{noTextPage: true})
	r := e.add(t, scan)
	if r[0].Error != nil {
		t.Fatal(r[0].Error)
	}
	ts := e.submit(t, "txt", r[0].Source.SourceID)
	done := e.wait(t, ts[0].ID)
	if done.Status != task.StatusFailed || done.Error == nil || done.Error.Code != apperr.DocPDFNoText ||
		done.Error.Detail != "reason=no_text\nquality=empty" || done.Error.Message != "这个 PDF 里没有能提取的文字，可能是扫描件。" {
		t.Fatalf("%+v", done.Error)
	}
	if _, err := e.tm.Retry(ts[0].ID); !apperr.Is(err, apperr.Unsupported) || !strings.Contains(apperr.From(err).Detail, "not_retryable") {
		t.Fatalf("DOC_PDF_NO_TEXT 不可重试：%v", err)
	}

	ok := makePDF(t, filepath.Join(d, "ok.pdf"), pdfSpec{lines: []string{"fine"}})
	r = e.add(t, ok)
	pdfLibHook = func(int) { panic("boom") }
	defer func() { pdfLibHook = nil }()
	ts = e.submit(t, "md", r[0].Source.SourceID)
	done = e.wait(t, ts[0].ID)
	if done.Status != task.StatusFailed || done.Error == nil || done.Error.Code != apperr.DocCorrupt || !strings.HasPrefix(done.Error.Detail, "engine=go\nparse=panic") {
		t.Fatalf("%+v", done.Error)
	}
}

func TestPDFNotRetryableErr(t *testing.T) {
	for _, c := range []struct {
		e    *apperr.AppError
		want bool
	}{
		{errPDFTooLarge(), true},
		{errPDFTooManyPages(501), true},
		{errPDFNoText("garbled"), true},
		{apperr.New(apperr.InvalidArgument, "x"), false},
		{apperr.New(apperr.DocTimeout, "x"), false},
	} {
		if notRetryableErr(c.e) != c.want {
			t.Errorf("%+v", c.e)
		}
	}
}

func TestPDFExtractTimeout(t *testing.T) {
	d := t.TempDir()
	p := makePDF(t, filepath.Join(d, "a.pdf"), pdfSpec{pages: 3, lines: []string{"x"}})
	block := make(chan struct{})
	pdfLibHook = func(int) { <-block }
	defer func() { pdfLibHook = nil; close(block) }()
	_, err := extractPDFTextTimeout(context.Background(), p, 50e6)
	var pe *errPDFParse
	if err == nil || !errorsAs(err, &pe) || pe.why != "timeout" {
		t.Fatal(err)
	}
}

// 真实组件（箱子上的系统 soffice）：pdf → docx / html / odt 走组件，pdf_layout；
// 纯 Go 解析失败（模拟 panic）+ 有组件 → 自动改用组件，engine=component；扫描件 → 组件导出也为空 → DOC_PDF_NO_TEXT（engine=component）。
func TestRealPDFConvert(t *testing.T) {
	comp := realComponent(t)
	e := newDocEnv(t, comp)
	src := filepath.Join(e.dir, "src")
	os.MkdirAll(src, 0o755)
	// 用组件从 txt 生成一个带中文的 PDF
	pdfPath := filepath.Join(src, "报告.pdf")
	{
		dir := t.TempDir()
		in := writeFile(t, filepath.Join(dir, "in.txt"), []byte("中文第一段 PDFTEXT\n\nSecond paragraph here\n"))
		out, err := doccomp.Convert(context.Background(), doccomp.Job{Exe: comp.ExePath(), Input: in, ConvertTo: "pdf:writer_pdf_Export",
			InFilter: "Text (encoded):UTF8", WorkDir: filepath.Join(dir, "w"), OutExt: "pdf", Timeout: 2 * 60e9})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(out)
		writeFile(t, pdfPath, b)
	}
	r := e.add(t, pdfPath)
	if r[0].Error != nil {
		t.Fatal(r[0].Error)
	}
	id := r[0].Source.SourceID
	for _, tg := range []string{"docx", "html", "txt"} {
		ts := e.submit(t, tg, id)
		done := e.waitLong(t, ts[0].ID)
		if done.Status != task.StatusSucceeded {
			t.Fatalf("%s：%+v", tg, done.Error)
		}
		wantEng := "component"
		if tg == "txt" {
			wantEng = "go"
		}
		if done.Result == nil || done.Result.Engine != wantEng || len(done.Result.Warnings) != 0 {
			t.Errorf("%s result：%+v", tg, done.Result)
		}
		text := readOutText(t, done.OutputPath, tg)
		if !strings.Contains(text, "PDFTEXT") || !strings.Contains(noSpace(text), "中文第一段") {
			t.Errorf("%s 内容：%q", tg, text)
		}
	}
	// 纯 Go 失败 → 组件（txt / md）
	pdfLibHook = func(int) { panic("boom") }
	for _, tg := range []string{"txt", "md"} {
		ts := e.submit(t, tg, id)
		done := e.waitLong(t, ts[0].ID)
		if done.Status != task.StatusSucceeded || done.Result.Engine != "component" {
			t.Fatalf("%s 回退：%+v %+v", tg, done.Error, done.Result)
		}
		b, _ := os.ReadFile(done.OutputPath)
		if bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) || !strings.Contains(string(b), "PDFTEXT") {
			t.Errorf("%s：%q", tg, b)
		}
		log, _ := os.ReadFile(filepath.Join(e.data, "logs", ts[0].ID+".log"))
		if !strings.Contains(string(log), "改用文档组件") {
			t.Errorf("日志：%s", log)
		}
	}
	pdfLibHook = nil
	// 扫描件：纯 Go 为空 → 组件 → 仍为空 → DOC_PDF_NO_TEXT（engine=component）
	scan := makePDF(t, filepath.Join(src, "scan.pdf"), pdfSpec{noTextPage: true})
	r = e.add(t, scan)
	ts := e.submit(t, "txt", r[0].Source.SourceID)
	done := e.waitLong(t, ts[0].ID)
	if done.Status != task.StatusFailed || done.Error.Code != apperr.DocPDFNoText || done.Error.Detail != "reason=no_text\nquality=empty\nengine=component" {
		t.Fatalf("扫描件：%+v", done.Error)
	}
}

func readOutText(t *testing.T, p, ext string) string {
	t.Helper()
	if ext != "docx" {
		b, _ := os.ReadFile(p)
		return string(b)
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			return string(b)
		}
	}
	t.Fatal("docx 里没有 document.xml")
	return ""
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

func errorsAs(err error, target any) bool { return errors.As(err, target) }

// owner_only：库解不了的加密（AES-256 R5/R6）里，空用户密码能打开的单独报权限保护；要密码的照旧。
func TestPDFOwnerOnlyEncryption(t *testing.T) {
	ctx := context.Background()
	fx := func(n string) string { return filepath.Join("testdata", "pdfenc", n+".pdf") }
	cls := map[string]pdfEncState{
		"aes256_owner": pdfEncOwnerOnly, "aes256r5_owner": pdfEncOwnerOnly, "aes256_owner_objstm": pdfEncOwnerOnly,
		"aes128_owner": pdfEncOwnerOnly, "rc4_128_owner": pdfEncOwnerOnly, "rc4_40_owner": pdfEncOwnerOnly,
		"aes256_user": pdfEncUser, "aes256r5_user": pdfEncUser, "aes128_user": pdfEncUser, "rc4_128_user": pdfEncUser, "rc4_40_user": pdfEncUser,
	}
	for n, want := range cls {
		f, err := os.Open(fx(n))
		if err != nil {
			t.Fatal(err)
		}
		fi, _ := f.Stat()
		if got := classifyPDFEncryption(f, fi.Size()); got != want {
			t.Errorf("%s：%v，应为 %v", n, got, want)
		}
		f.Close()
	}
	// fpdf 生成的 RC4（R2）也能分清
	d := t.TempDir()
	for _, c := range []struct {
		sp   pdfSpec
		want pdfEncState
	}{
		{pdfSpec{lines: []string{"x"}, protect: true, own: "o"}, pdfEncOwnerOnly},
		{pdfSpec{lines: []string{"x"}, protect: true, user: "u", own: "o"}, pdfEncUser},
		{pdfSpec{lines: []string{"x"}}, pdfEncNone},
	} {
		p := makePDF(t, filepath.Join(d, "f.pdf"), c.sp)
		f, _ := os.Open(p)
		fi, _ := f.Stat()
		if got := classifyPDFEncryption(f, fi.Size()); got != c.want {
			t.Errorf("fpdf %+v：%v", c.sp, got)
		}
		f.Close()
	}

	const ownerMsg = "这个 PDF 设置了权限保护，暂时不能转换。"
	const userMsg = "这个文件有密码保护，不能转换。请先去掉密码再添加。"
	for _, n := range []string{"aes256_owner", "aes256r5_owner", "aes256_owner_objstm"} {
		_, err := inspectPDF(ctx, fx(n))
		e := apperr.From(err)
		if !apperr.Is(err, apperr.DocEncrypted) || e.Message != ownerMsg || e.Detail != "reason=owner_only" || !notRetryableErr(e) {
			t.Errorf("%s：%v", n, err)
		}
		if _, err := extractPDFText(ctx, fx(n)); apperr.From(err).Detail != "reason=owner_only" {
			t.Errorf("%s 提取：%v", n, err)
		}
		if _, err := inspectDoc(ctx, fx(n), "pdf"); apperr.From(err).Message != ownerMsg {
			t.Errorf("%s inspectDoc：%v", n, err)
		}
	}
	for _, n := range []string{"aes256_user", "aes256r5_user", "aes128_user", "rc4_128_user", "rc4_40_user"} {
		_, err := inspectPDF(ctx, fx(n))
		if e := apperr.From(err); !apperr.Is(err, apperr.DocEncrypted) || e.Message != userMsg || e.Detail == "reason=owner_only" {
			t.Errorf("%s：%v", n, err)
		}
	}
	// 库能处理的只有所有者密码（RC4 / AES-128）照常转
	for _, n := range []string{"aes128_owner", "rc4_128_owner", "rc4_40_owner"} {
		if _, err := inspectPDF(ctx, fx(n)); err != nil {
			t.Errorf("%s 应能转：%v", n, err)
		}
		if n == "rc4_40_owner" {
			continue // qpdf 的 40 位 RC4 库能打开但解不开流（原有行为，走组件 / 解析失败），这里只看不拒收
		}
		pages, err := extractPDFText(ctx, fx(n))
		if err != nil || !strings.Contains(pagesPlain(pages), "owner only hello") {
			t.Errorf("%s 提取：%v %q", n, err, pagesPlain(pages))
		}
	}
}
