package doc

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/task"
)

// detail 首行统一为 reason=<枚举>（契约 2.2 / 6.12.6）：code 和 message 保持原样（前端精确匹配 message），
// 只有 detail 首行是 reason，其后可以有自由文本行。这里逐个场景断言 code、message、detail 首行。

func detailHead(ae *apperr.AppError) string {
	first, _, _ := strings.Cut(ae.Detail, "\n")
	return first
}

// wantReason 断言错误的 code、message 与 detail 首行；reason 为空表示“不带 reason 行”。
func wantReason(t *testing.T, name string, err error, code apperr.Code, message, reason string) *apperr.AppError {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: 期望 %s，没有错误", name, code)
	}
	ae := apperr.From(err)
	if ae.Code != code {
		t.Errorf("%s: code=%s，期望 %s（%+v）", name, ae.Code, code, ae)
	}
	if ae.Message != message {
		t.Errorf("%s: message=%q，期望 %q", name, ae.Message, message)
	}
	head := detailHead(ae)
	switch {
	case reason == "" && strings.HasPrefix(head, "reason="):
		t.Errorf("%s: 不应带 reason 行，detail=%q", name, ae.Detail)
	case reason != "" && head != "reason="+reason:
		t.Errorf("%s: detail 首行=%q，期望 reason=%s（detail=%q）", name, head, reason, ae.Detail)
	}
	return ae
}

const (
	msgFormat     = "暂不支持这种格式"
	msgOOXML      = "不是有效的 OOXML 文件"
	msgPages      = "超过 5000 页"
	msgNoFont     = "没有可用的 Unicode 字体"
	msgTooBig     = "文件超过 100 MiB"
	msgTooBigPDF  = "文件超过 512 MiB"
	msgNotPDF     = "不是 PDF 文件"
	msgPDFExt     = "只支持 .pdf 文件"
	oleHeaderFile = "\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"
)

// ConvertToPDF 提交时同步校验的错误：detail 是 reason 行 + 出错文件路径行（+ 原因说明）。
func TestReasonSubmitErrors(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	mk := func(name string, data []byte) string {
		p := filepath.Join(e.dir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// 缺必需部件的 docx / xlsx / pptx
	noPart := func(name string, files map[string]string) string {
		p := filepath.Join(e.dir, name)
		writeZip(t, p, files)
		return p
	}
	bomb := filepath.Join(e.dir, "bomb.docx")
	writeBomb(t, bomb, 257<<20)
	big := filepath.Join(e.dir, "big.docx")
	makeDocx(t, big, "hi")
	if err := os.Truncate(big, MaxInputBytes+1); err != nil {
		t.Fatal(err)
	}
	forgedDir := filepath.Join(e.dir, "forged-dir.docx") // 声明的中央目录 98 MiB，远超 9 600 000 字节上限
	forgedZip(t, forgedDir, 98<<20, func(n, _ int) []byte { return eocdBytes(1, uint32(n), 0) })
	forgedZip64 := filepath.Join(e.dir, "forged-zip64.docx") // 32 位字段是占位符但没有 zip64 记录
	forgedZip(t, forgedZip64, 1<<20, func(n, en int) []byte { return eocdBytes(uint16(en%65536), uint32(n), 0xFFFFFFFF) })
	manyEntries := filepath.Join(e.dir, "many.docx") // 条目数 100001（真实 zip，EOCD 里的条目数如实）
	func() {
		f, _ := os.Create(manyEntries)
		defer f.Close()
		zw := zip.NewWriter(f)
		w, _ := zw.Create("word/document.xml")
		w.Write([]byte("<x/>"))
		for i := 1; i <= MaxZipEntries; i++ {
			if _, err := zw.CreateHeader(&zip.FileHeader{Name: "x/" + itoa(i), Method: zip.Store}); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	cases := []struct {
		name    string
		path    string
		code    apperr.Code
		message string
		reason  string
	}{
		{"旧版 doc", mk("a.doc", []byte("x")), apperr.Unsupported, msgFormat, "format"},
		{"旧版 xls", mk("a.xls", []byte("x")), apperr.Unsupported, msgFormat, "format"},
		{"odt", mk("a.odt", []byte("x")), apperr.Unsupported, msgFormat, "format"},
		{"rtf", mk("a.rtf", []byte("x")), apperr.Unsupported, msgFormat, "format"},
		{"csv", mk("a.csv", []byte("x")), apperr.Unsupported, msgFormat, "format"},
		{"txt", mk("a.txt", []byte("x")), apperr.Unsupported, msgFormat, "format"},
		{"pdf 当输入", mk("a.pdf", []byte("x")), apperr.Unsupported, msgFormat, "format"},
		{"未知扩展名", mk("a.pages", []byte("x")), apperr.Unsupported, msgFormat, "format"},
		{"没有扩展名", mk("noext", []byte("x")), apperr.Unsupported, msgFormat, "format"},
		{"加密 docx（OLE）", mk("enc.docx", []byte(oleHeaderFile+strings.Repeat("\x00", 600))), apperr.Unsupported, msgFormat, "encrypted"},
		{"加密 xlsx（OLE）", mk("enc.xlsx", []byte(oleHeaderFile+strings.Repeat("\x00", 600))), apperr.Unsupported, msgFormat, "encrypted"},
		{"加密 pptx（OLE）", mk("enc.pptx", []byte(oleHeaderFile+strings.Repeat("\x00", 600))), apperr.Unsupported, msgFormat, "encrypted"},
		{"损坏 docx（不是 zip）", mk("bad.docx", []byte("这不是 zip")), apperr.InvalidArgument, msgOOXML, "invalid_ooxml"},
		{"空文件 docx", mk("empty.docx", nil), apperr.InvalidArgument, msgOOXML, "invalid_ooxml"},
		{"docx 缺 word/document.xml", noPart("nodoc.docx", map[string]string{"foo.xml": "<a/>"}), apperr.InvalidArgument, msgOOXML, "invalid_ooxml"},
		{"xlsx 缺 xl/workbook.xml", noPart("nowb.xlsx", map[string]string{"foo.xml": "<a/>"}), apperr.InvalidArgument, msgOOXML, "invalid_ooxml"},
		{"pptx 没有幻灯片", noPart("nosld.pptx", map[string]string{"ppt/slides/notes.xml": "<a/>"}), apperr.InvalidArgument, msgOOXML, "invalid_ooxml"},
		{"伪造 EOCD：占位符没有 zip64 记录", forgedZip64, apperr.InvalidArgument, msgOOXML, "invalid_ooxml"},
		{"伪造 EOCD：中央目录超限", forgedDir, apperr.InvalidArgument, msgOOXML, "too_large"},
		{"zip 条目数 100001", manyEntries, apperr.InvalidArgument, msgOOXML, "too_large"},
		{"条目解压后超过 256 MiB", bomb, apperr.InvalidArgument, msgOOXML, "too_large"},
		{"文件超过 100 MiB", big, apperr.InvalidArgument, msgTooBig, "too_large"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := e.svc.ConvertToPDF(ctx, []string{c.path}, "")
			ae := wantReason(t, c.name, err, c.code, c.message, c.reason)
			// 整体校验失败：reason 行之后紧跟出错文件路径行
			if lines := strings.Split(ae.Detail, "\n"); len(lines) < 2 || lines[1] != c.path {
				t.Errorf("第二行应是出错文件路径 %q，detail=%q", c.path, ae.Detail)
			}
			if l := e.tm.ListActive(); len(l) != 0 {
				t.Errorf("整体失败不应提交任务: %+v", l)
			}
		})
	}
}

// 不属于六个 reason 的错误：保持原来的 code 和 detail，不带 reason 行。
func TestReasonAbsentForOtherErrors(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	good := filepath.Join(e.dir, "ok.docx")
	makeDocx(t, good, "hi")
	dirAsFile := filepath.Join(e.dir, "folder.docx")
	if err := os.Mkdir(dirAsFile, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.ConvertToPDF(ctx, []string{"relative.docx"}, "")
	wantReason(t, "相对路径", err, apperr.InvalidArgument, "文件路径必须是绝对路径", "")
	_, err = e.svc.ConvertToPDF(ctx, []string{filepath.Join(e.dir, "missing.docx")}, "")
	wantReason(t, "文件不存在", err, apperr.NotFound, "文件不存在", "")
	_, err = e.svc.ConvertToPDF(ctx, []string{dirAsFile}, "")
	wantReason(t, "目录当文件", err, apperr.InvalidArgument, "这是文件夹，不是文件", "")
	_, err = e.svc.ConvertToPDF(ctx, nil, "")
	wantReason(t, "没有输入", err, apperr.InvalidArgument, "没有要转换的文件", "")
	_, err = e.svc.ConvertToPDF(ctx, []string{good}, "relative-dir")
	wantReason(t, "输出目录非绝对", err, apperr.InvalidArgument, "输出目录必须是绝对路径", "")
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = e.svc.ConvertToPDF(cctx, []string{good}, "")
	wantReason(t, "取消", err, apperr.Canceled, "操作已取消", "")
}

// 缺 Unicode 字体：提交时同步扫描文本，整体失败。
func TestReasonNoFont(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.DisableEmbedded = true; c.SystemFonts = []SystemFont{} })
	cjk := filepath.Join(e.dir, "cjk.docx")
	makeDocx(t, cjk, "你好")
	_, err := e.svc.ConvertToPDF(context.Background(), []string{cjk}, "")
	ae := wantReason(t, "缺字体", err, apperr.Unsupported, msgNoFont, "no_font")
	if lines := strings.Split(ae.Detail, "\n"); len(lines) < 3 || lines[1] != cjk {
		t.Errorf("reason 行后应是路径行和说明行: %q", ae.Detail)
	}
}

// 任务运行期错误：任务 failed，Task.Error 的 code / message / detail 首行。任务错误没有路径行（路径只在提交时整体校验失败才加）。
func TestReasonTaskErrors(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	run := func(name, in string) *apperr.AppError {
		ts, err := e.svc.ConvertToPDF(ctx, []string{in}, "")
		if err != nil {
			t.Fatalf("%s: 提交失败: %v", name, err)
		}
		tk := e.wait(t, ts[0].ID)
		if tk.Status != task.StatusFailed || tk.Error == nil {
			t.Fatalf("%s: 任务应失败: %+v", name, tk)
		}
		return tk.Error
	}

	// 超过 5000 页（5001 张幻灯片，每张一页）
	slides := make([][]string, MaxPages+1)
	nums := make([]int, MaxPages+1)
	for i := range slides {
		slides[i] = []string{"x"}
		nums[i] = i + 1
	}
	pages := filepath.Join(e.dir, "pages.pptx")
	makePptx(t, pages, nums, slides)
	ae := run("超过 5000 页", pages)
	wantReason(t, "超过 5000 页", ae, apperr.Unsupported, msgPages, "too_many_pages")
	if !strings.Contains(ae.Detail, "已排到第") {
		t.Errorf("reason 行后应保留原说明: %q", ae.Detail)
	}

	// 提交时校验通过（有必需部件），运行时才发现 XML 损坏
	broken := filepath.Join(e.dir, "broken.docx")
	writeZip(t, broken, map[string]string{"word/document.xml": "<w:document><w:body><w:p><w:t>a</w:p></w:body>"})
	ae = run("运行期 XML 损坏", broken)
	wantReason(t, "运行期 XML 损坏", ae, apperr.InvalidArgument, msgOOXML, "invalid_ooxml")
}

// PDF 预览 / 打开：只有“不是 PDF / 扩展名不对 → format”和“超过 512 MiB → too_large”带 reason。
func TestReasonPDFErrors(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	open := func(p string) error { _, err := e.svc.OpenPDF(ctx, p); return err }

	txt := filepath.Join(e.dir, "a.txt")
	os.WriteFile(txt, []byte("hi"), 0o644)
	ae := wantReason(t, "扩展名不是 pdf", open(txt), apperr.InvalidArgument, msgPDFExt, "format")
	if !strings.Contains(ae.Detail, txt) {
		t.Errorf("说明行应含路径: %q", ae.Detail)
	}
	notpdf := filepath.Join(e.dir, "fake.pdf")
	os.WriteFile(notpdf, []byte("这不是 PDF"), 0o644)
	wantReason(t, "内容不是 PDF", open(notpdf), apperr.InvalidArgument, msgNotPDF, "format")
	empty := filepath.Join(e.dir, "empty.pdf")
	os.WriteFile(empty, nil, 0o644)
	wantReason(t, "空 PDF", open(empty), apperr.InvalidArgument, msgNotPDF, "format")

	big := filepath.Join(e.dir, "big.pdf")
	writePDF(t, big, 100)
	os.Truncate(big, MaxPDFBytes+1)
	wantReason(t, "PDF 超过 512 MiB", open(big), apperr.InvalidArgument, msgTooBigPDF, "too_large")

	// 打开后文件被撑大到超限：ReadPDFChunk 同样是 too_large
	grow := filepath.Join(e.dir, "grow.pdf")
	writePDF(t, grow, 4096)
	src, err := e.svc.OpenPDF(ctx, grow)
	if err != nil {
		t.Fatal(err)
	}
	os.Truncate(grow, MaxPDFBytes+1)
	_, err = e.svc.ReadPDFChunk(src.ID, 0, 1024)
	wantReason(t, "ReadPDFChunk 文件超限", err, apperr.InvalidArgument, msgTooBigPDF, "too_large")

	// 不带 reason 的 PDF 错误
	wantReason(t, "相对路径", open("rel.pdf"), apperr.InvalidArgument, "PDF 路径必须是绝对路径", "")
	wantReason(t, "文件不存在", open(filepath.Join(e.dir, "none.pdf")), apperr.NotFound, "文件不存在", "")
	_, err = e.svc.ReadPDFChunk("no-such-handle", 0, 1024)
	wantReason(t, "句柄失效", err, apperr.NotFound, "PDF 句柄已失效，请重新打开", "")
	_, err = e.svc.ReadPDFChunk(src.ID, 0, 0)
	wantReason(t, "length 越界", err, apperr.InvalidArgument, "length 必须在 1 到 1 MiB 之间", "")
}

// 加密 PDF 能正常打开（密码由前端 pdf.js 处理），后端不返回错误，所以没有 reason=encrypted 的 PDF 错误；
// reason=encrypted 只用于加密的 Office 文档（OLE 容器）。
func TestEncryptedPDFOpensWithoutError(t *testing.T) {
	e := newEnv(t)
	p := filepath.Join(e.dir, "enc.pdf")
	body := "%PDF-1.6\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R/Encrypt<</Filter/Standard/V 4/R 4>>>>\n%%EOF\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.OpenPDF(context.Background(), p); err != nil {
		t.Fatalf("加密 PDF 应能打开: %v", err)
	}
}

// withPath 在已有 reason 行的 detail 里把路径插到第二行，reason 保持首行。
func TestWithPathKeepsReasonFirst(t *testing.T) {
	err := withPath(reasonErr(apperr.Unsupported, "m", "format", ".doc：旧版"), "/a/b.doc")
	if got, want := apperr.From(err).Detail, "reason=format\n/a/b.doc\n.doc：旧版"; got != want {
		t.Fatalf("%q != %q", got, want)
	}
	err = withPath(reasonErr(apperr.Unsupported, "m", "format", ""), "/a/b.doc")
	if got, want := apperr.From(err).Detail, "reason=format\n/a/b.doc"; got != want {
		t.Fatalf("%q != %q", got, want)
	}
	err = withPath(apperr.New(apperr.NotFound, "m"), "/a/b.doc")
	if got, want := apperr.From(err).Detail, "/a/b.doc"; got != want {
		t.Fatalf("%q != %q", got, want)
	}
}
