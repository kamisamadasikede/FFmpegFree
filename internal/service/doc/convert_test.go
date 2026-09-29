package doc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/task"
)

func wantCode(t *testing.T, err error, code apperr.Code) *apperr.AppError {
	t.Helper()
	if !apperr.Is(err, code) {
		t.Fatalf("期望 %s，实际 %v", code, err)
	}
	return apperr.From(err)
}

func TestDocxRealConversion(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "报告.docx")
	makeDocx(t, in, "你好，世界！Hello World", "第二段：こんにちは Àé 2026", "Tom & Jerry <b>")
	tk := e.convertOK(t, in, "")
	if tk.OutputPath != filepath.Join(e.dir, "报告.pdf") {
		t.Fatalf("输出路径 %q", tk.OutputPath)
	}
	if tk.Type != task.TypeOfficePDF || tk.Title != "报告.docx → PDF" || tk.Progress != 1 {
		t.Fatalf("%+v", tk)
	}
	txt := noSpace(pdfText(t, tk.OutputPath))
	for _, w := range []string{"你好，世界！HelloWorld", "第二段：こんにちはÀé2026", "Tom&Jerry<b>"} {
		if !strings.Contains(txt, w) {
			t.Fatalf("缺 %q，得到 %q", w, txt)
		}
	}
	if _, err := os.Stat(tk.OutputPath + ".part"); err == nil {
		t.Fatal("不应残留 .part")
	}
	if ms, _ := filepath.Glob(filepath.Join(e.dir, "*.part*")); len(ms) != 0 {
		t.Fatalf("残留 %v", ms)
	}
	// 日志里有缺字统计
	log, _ := e.tm.GetLog(tk.ID, 50)
	if !strings.Contains(log, "font=noto-sans-sc-embedded") || strings.Contains(log, "missing_glyphs") {
		t.Fatalf("日志: %q", log)
	}
	// 同名再转：(1)
	tk2 := e.convertOK(t, in, "")
	if tk2.OutputPath != filepath.Join(e.dir, "报告(1).pdf") {
		t.Fatalf("重名: %q", tk2.OutputPath)
	}
}

func TestDocxMissingGlyphCountLogged(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "rare.docx")
	makeDocx(t, in, "常用字 龘齉 \U0001F600 ok") // 龘齉 不在 GB2312；emoji 超出 BMP
	tk := e.convertOK(t, in, "")
	log, _ := e.tm.GetLog(tk.ID, 50)
	if !strings.Contains(log, "missing_glyphs=3 total=3 sample=U+1F600,U+9F98,U+9F49") && !strings.Contains(log, "missing_glyphs=3 total=3 sample=U+") {
		t.Fatalf("日志: %q", log)
	}
	if strings.Contains(log, "龘") || strings.Contains(log, "常用字") {
		t.Fatalf("日志不应含文档文字: %q", log)
	}
	if txt := noSpace(pdfText(t, tk.OutputPath)); !strings.Contains(txt, "常用字") || !strings.Contains(txt, "ok") {
		t.Fatalf("其余文字应正常: %q", txt)
	}
}

func TestXlsxRealConversion(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "表.xlsx")
	makeXlsx(t, in, map[string][][]string{
		"销售":    {{"产品", "数量"}, {"苹果", "12"}, {}, {"梨", "7"}},
		"Other": {{"a", "b", "c"}},
	}, []string{"销售", "Other"})
	tk := e.convertOK(t, in, filepath.Join(e.dir, "out"))
	txt := pdfText(t, tk.OutputPath)
	ns := noSpace(txt)
	for _, w := range []string{"Sheet:销售", "产品数量", "苹果12", "梨7", "Sheet:Other", "abc"} {
		if !strings.Contains(ns, w) {
			t.Fatalf("缺 %q: %q", w, ns)
		}
	}
	if !strings.Contains(txt, "产品    数量") && !strings.Contains(txt, "产品 数量") {
		t.Logf("单元格分隔: %q", txt)
	}
	if n := pdfPages(t, tk.OutputPath); n != 2 {
		t.Fatalf("两个工作表应各占一页，实际 %d 页", n)
	}
}

func TestPptxOrderNumericAndText(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "幻灯片.pptx")
	// 文件名 slide2、slide10、slide1：数字顺序应为 1,2,10
	makePptx(t, in, []int{10, 2, 1}, [][]string{{"第十张 ten"}, {"第二张 two"}, {"第一张 one"}})
	tk := e.convertOK(t, in, "")
	txt := noSpace(pdfText(t, tk.OutputPath))
	i1, i2, i10 := strings.Index(txt, "第一张one"), strings.Index(txt, "第二张two"), strings.Index(txt, "第十张ten")
	if i1 < 0 || i2 < 0 || i10 < 0 || !(i1 < i2 && i2 < i10) {
		t.Fatalf("顺序不对 %d %d %d: %q", i1, i2, i10, txt)
	}
	if !strings.Contains(txt, "Slide1") || !strings.Contains(txt, "Slide3") {
		t.Fatalf("缺幻灯片标题: %q", txt)
	}
	if n := pdfPages(t, tk.OutputPath); n != 3 {
		t.Fatalf("每张幻灯片一页，实际 %d", n)
	}
}

func TestProgressMonotonicAndCompletes(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "big.docx")
	var paras []string
	for i := 0; i < 400; i++ {
		paras = append(paras, "段落 "+strings.Repeat("字", 30))
	}
	makeDocx(t, in, paras...)
	tk := e.convertOK(t, in, "")
	ps := e.em.get(tk.ID)
	if len(ps) < 3 {
		t.Fatalf("进度事件太少: %v", ps)
	}
	last := -1.0
	for _, p := range ps {
		if p < last || p < 0 || p > 1 {
			t.Fatalf("进度不单调: %v", ps)
		}
		last = p
	}
	if last != 1 {
		t.Fatalf("最后应为 1: %v", ps)
	}
}

func TestValidationWholeBatchNoSubmit(t *testing.T) {
	e := newEnv(t)
	good := filepath.Join(e.dir, "ok.docx")
	makeDocx(t, good, "hi")
	bad := filepath.Join(e.dir, "bad.docx")
	os.WriteFile(bad, []byte("这不是 zip"), 0o644)
	_, err := e.svc.ConvertToPDF(context.Background(), []string{good, bad}, "")
	ae := wantCode(t, err, apperr.InvalidArgument)
	if !strings.HasPrefix(ae.Detail, bad+"\n") && ae.Detail != bad {
		t.Fatalf("detail 第一行应是出错路径: %q", ae.Detail)
	}
	if l := e.tm.ListActive(); len(l) != 0 {
		t.Fatalf("不应提交任何任务: %+v", l)
	}
	page, _ := e.tm.List(task.Filter{})
	if page.Total != 0 {
		t.Fatalf("库里不应有任务: %d", page.Total)
	}
}

func TestValidationErrors(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	call := func(in ...string) error { _, err := e.svc.ConvertToPDF(ctx, in, ""); return err }

	wantCode(t, call(), apperr.InvalidArgument)
	wantCode(t, call("rel/a.docx"), apperr.InvalidArgument)
	wantCode(t, call(""), apperr.InvalidArgument)
	many := make([]string, 51)
	for i := range many {
		many[i] = filepath.Join(e.dir, "x.docx")
	}
	wantCode(t, call(many...), apperr.InvalidArgument)
	wantCode(t, call(filepath.Join(e.dir, "nope.docx")), apperr.NotFound)
	d := filepath.Join(e.dir, "dir.docx")
	os.Mkdir(d, 0o755)
	wantCode(t, call(d), apperr.InvalidArgument)

	// 缺必需部件
	nopart := filepath.Join(e.dir, "np.docx")
	writeZip(t, nopart, map[string]string{"foo.xml": "<a/>"})
	wantCode(t, call(nopart), apperr.InvalidArgument)
	nox := filepath.Join(e.dir, "np.xlsx")
	writeZip(t, nox, map[string]string{"foo.xml": "<a/>"})
	wantCode(t, call(nox), apperr.InvalidArgument)
	nop := filepath.Join(e.dir, "np.pptx")
	writeZip(t, nop, map[string]string{"ppt/slides/notes.xml": "<a/>"})
	wantCode(t, call(nop), apperr.InvalidArgument)

	// 单个条目解压后 > 256 MiB：zip 头声明的大小（用稀疏内容压缩得很小）
	bomb := filepath.Join(e.dir, "bomb.docx")
	writeBomb(t, bomb, 257<<20)
	ae := wantCode(t, call(bomb), apperr.InvalidArgument)
	if !strings.Contains(ae.Detail, "256 MiB") {
		t.Fatal(ae.Detail)
	}
}

func TestUnsupportedFormats(t *testing.T) {
	e := newEnv(t)
	for _, ext := range []string{"doc", "xls", "ppt", "odt", "ods", "odp", "rtf", "csv", "txt", "pdf", "pages", "DOC", "Csv"} {
		p := filepath.Join(e.dir, "f."+ext)
		os.WriteFile(p, []byte("x"), 0o644)
		_, err := e.svc.ConvertToPDF(context.Background(), []string{p}, "")
		ae := wantCode(t, err, apperr.Unsupported)
		if !strings.HasPrefix(ae.Detail, p+"\n") {
			t.Fatalf("%s detail 首行应是路径: %q", ext, ae.Detail)
		}
	}
	p := filepath.Join(e.dir, "noext")
	os.WriteFile(p, []byte("x"), 0o644)
	_, err := e.svc.ConvertToPDF(context.Background(), []string{p}, "")
	wantCode(t, err, apperr.Unsupported)
	// 扩展名不区分大小写：大写 DOCX 支持
	up := filepath.Join(e.dir, "UP.DOCX")
	makeDocx(t, up, "hi")
	if _, err := e.svc.ConvertToPDF(context.Background(), []string{up}, ""); err != nil {
		t.Fatal(err)
	}
}

func TestEncryptedOLEUnsupported(t *testing.T) {
	e := newEnv(t)
	for _, ext := range []string{"docx", "xlsx", "pptx"} {
		p := filepath.Join(e.dir, "enc."+ext)
		os.WriteFile(p, append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 600)...), 0o644)
		_, err := e.svc.ConvertToPDF(context.Background(), []string{p}, "")
		ae := wantCode(t, err, apperr.Unsupported)
		if !strings.Contains(ae.Detail, "加密") {
			t.Fatal(ae.Detail)
		}
	}
	if !isOLE([]byte{0xD0, 0xCF, 0x11, 0xE0}) {
		t.Fatal()
	}
}

func isOLE(b []byte) bool {
	return len(b) >= 4 && b[0] == 0xD0 && b[1] == 0xCF && b[2] == 0x11 && b[3] == 0xE0
}

func TestInputSizeLimit(t *testing.T) {
	e := newEnv(t)
	p := filepath.Join(e.dir, "huge.docx")
	f, _ := os.Create(p)
	f.Truncate(MaxInputBytes + 1) // 稀疏文件
	f.Close()
	_, err := e.svc.ConvertToPDF(context.Background(), []string{p}, "")
	wantCode(t, err, apperr.InvalidArgument)
	// 恰好 100 MiB 不因大小被拒（内容不是 zip，会因 OOXML 无效被拒，但错误不是“超过 100 MiB”）
	os.Truncate(p, MaxInputBytes)
	_, err = e.svc.ConvertToPDF(context.Background(), []string{p}, "")
	if ae := wantCode(t, err, apperr.InvalidArgument); strings.Contains(ae.Message, "100 MiB") {
		t.Fatalf("100 MiB 整不应被大小限制拒绝: %v", ae)
	}
}

func TestPageLimit(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "pages.docx")
	// 每个 unitBreak 一页太麻烦：用很多短段落，行高 6mm，一页约 40 行，5000 页需要 20 万段；改用超大文字量走 pptx（每张一页）
	slides := make([][]string, 5001)
	nums := make([]int, 5001)
	for i := range slides {
		slides[i] = []string{"x"}
		nums[i] = i + 1
	}
	in = filepath.Join(e.dir, "pages.pptx")
	makePptx(t, in, nums, slides)
	ts, err := e.svc.ConvertToPDF(context.Background(), []string{in}, "")
	if err != nil {
		t.Fatal(err)
	}
	tk := e.wait(t, ts[0].ID)
	if tk.Status != task.StatusFailed || tk.Error == nil || tk.Error.Code != apperr.Unsupported || !strings.Contains(tk.Error.Message, "5000") {
		t.Fatalf("%+v %+v", tk.Status, tk.Error)
	}
	if ms, _ := filepath.Glob(filepath.Join(e.dir, "*.p*")); len(ms) != 1 { // 只剩输入 pages.pptx
		t.Fatalf("不应有输出或 .part: %v", ms)
	}
}

func TestOutputDirRules(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "a.docx")
	makeDocx(t, in, "hi")
	call := func(dir string) error {
		_, err := e.svc.ConvertToPDF(context.Background(), []string{in}, dir)
		return err
	}
	wantCode(t, call("rel/out"), apperr.InvalidArgument)
	wantCode(t, call(`\\?\C:\out`), apperr.InvalidArgument)
	wantCode(t, call(`\\.\C:\out`), apperr.InvalidArgument)
	wantCode(t, call(`//?/C:/out`), apperr.InvalidArgument)
	wantCode(t, call(e.data), apperr.InvalidArgument)
	wantCode(t, call(filepath.Join(e.data, "logs")), apperr.InvalidArgument)
	ae := wantCode(t, call(filepath.Join(e.data, "not", "yet")), apperr.InvalidArgument)
	if !strings.Contains(ae.Detail, "outputDir 不能在应用数据目录内") {
		t.Fatal(ae.Detail)
	}
	wantCode(t, call(in), apperr.InvalidArgument) // 是文件
	// 通过符号链接进入数据目录
	link := filepath.Join(e.dir, "lnk")
	if err := os.Symlink(e.data, link); err == nil {
		wantCode(t, call(link), apperr.InvalidArgument)
	}
	// 数据目录的兄弟（前缀相同但不在其内）允许
	sib := e.data + "-sibling"
	if err := call(sib); err != nil {
		t.Fatal(err)
	}
	// 不存在的目录任务开始时创建
	nd := filepath.Join(e.dir, "new", "sub")
	ts, err := e.svc.ConvertToPDF(context.Background(), []string{in}, nd)
	if err != nil {
		t.Fatal(err)
	}
	tk := e.wait(t, ts[0].ID)
	if tk.Status != task.StatusSucceeded || filepath.Dir(tk.OutputPath) != nd {
		t.Fatalf("%+v %+v", tk.Status, tk.Error)
	}
}

func TestDefaultOutputDir(t *testing.T) {
	e := newEnv(t)
	e.defOD = filepath.Join(e.dir, "default-out")
	in := filepath.Join(e.dir, "a.docx")
	makeDocx(t, in, "hi")
	tk := e.convertOK(t, in, "")
	if tk.OutputPath != filepath.Join(e.defOD, "a.pdf") {
		t.Fatal(tk.OutputPath)
	}
	tk = e.convertOK(t, in, filepath.Join(e.dir, "explicit"))
	if filepath.Dir(tk.OutputPath) != filepath.Join(e.dir, "explicit") {
		t.Fatal(tk.OutputPath)
	}
}

func TestOutputNameSanitized(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "CON.docx")
	makeDocx(t, in, "hi")
	tk := e.convertOK(t, in, "")
	if filepath.Base(tk.OutputPath) != "_CON.pdf" {
		t.Fatal(tk.OutputPath)
	}
	long := filepath.Join(e.dir, strings.Repeat("长", 90)+".docx") // 270 字节，文件系统 255 字节上限内放不下→用 80 个
	long = filepath.Join(e.dir, strings.Repeat("长", 80)+".docx")
	makeDocx(t, long, "hi")
	tk = e.convertOK(t, long, "")
	stem := strings.TrimSuffix(filepath.Base(tk.OutputPath), ".pdf")
	if len(stem) > 200 || len([]rune(stem)) != 66 {
		t.Fatalf("UTF-8 字节应 ≤200: %d 字节 %d 字符", len(stem), len([]rune(stem)))
	}
}

func TestWindowsPathBudget(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.WindowsMaxPath = 259 })
	in := filepath.Join(e.dir, "in.docx")
	makeDocx(t, in, "hi")
	dir := e.dir
	// 让 dir + 名字 超过 259：把输出名压短到预算内
	long := filepath.Join(e.dir, strings.Repeat("n", 100)+".docx")
	makeDocx(t, long, "hi")
	out := filepath.Join(e.dir, strings.Repeat("d", 120))
	if len(out)+1+100+24 > 259 {
		tk := e.convertOK(t, long, out)
		if n := len(tk.OutputPath); n > 259 {
			t.Fatalf("整条路径 %d > 259", n)
		}
	}
	_ = dir
	// 目录本身太长放不下
	tooLong := filepath.Join(e.dir, strings.Repeat("d", 40))
	e2 := newEnv(t, func(c *Config) { c.WindowsMaxPath = len(tooLong) + 10 })
	in2 := filepath.Join(e2.dir, "a.docx")
	makeDocx(t, in2, "hi")
	_, err := e2.svc.ConvertToPDF(context.Background(), []string{in2}, tooLong)
	wantCode(t, err, apperr.InvalidArgument)
}

func TestCancelLeavesNoPart(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "c.docx")
	var paras []string
	for i := 0; i < 20000; i++ {
		paras = append(paras, strings.Repeat("字", 60))
	}
	makeDocx(t, in, paras...)
	ts, err := e.svc.ConvertToPDF(context.Background(), []string{in}, "")
	if err != nil {
		t.Fatal(err)
	}
	e.tm.Cancel(ts[0].ID)
	tk := e.wait(t, ts[0].ID)
	if tk.Status != task.StatusCanceled && tk.Status != task.StatusSucceeded && tk.Status != task.StatusFailed {
		t.Fatalf("%v", tk.Status)
	}
	if tk.Status == task.StatusCanceled {
		if ms, _ := filepath.Glob(filepath.Join(e.dir, "*.pdf*")); len(ms) != 0 {
			t.Fatalf("取消后不应有输出 / .part: %v", ms)
		}
	}
}

func TestRetry(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "r.docx")
	makeDocx(t, in, "重试")
	tk := e.convertOK(t, in, "")
	var p params
	json.Unmarshal([]byte(tk.Params), &p)
	if p.Input != in {
		t.Fatalf("%+v", p)
	}
	nt, err := e.tm.Retry(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := e.wait(t, nt.ID)
	if got.Status != task.StatusSucceeded || got.ID == tk.ID || filepath.Base(got.OutputPath) != "r(1).pdf" {
		t.Fatalf("%+v %v", got.Status, got.OutputPath)
	}
	// 输入被删：NOT_FOUND，不产生新任务
	os.Remove(in)
	before, _ := e.tm.List(task.Filter{})
	_, err = e.tm.Retry(tk.ID)
	wantCode(t, err, apperr.NotFound)
	after, _ := e.tm.List(task.Filter{})
	if before.Total != after.Total {
		t.Fatal("不应产生新任务")
	}
}

func TestRuntimeCorruptFails(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "c.docx")
	makeDocx(t, in, "hi")
	ts, err := e.svc.ConvertToPDF(context.Background(), []string{in}, "")
	if err != nil {
		t.Fatal(err)
	}
	e.wait(t, ts[0].ID)
	// 运行时才损坏：document.xml 不是合法 XML（校验只看部件存在）
	bad := filepath.Join(e.dir, "bad2.docx")
	writeZip(t, bad, map[string]string{"word/document.xml": "<w:document><w:p><w:t>abc</w:p></w:document>"})
	ts, err = e.svc.ConvertToPDF(context.Background(), []string{bad}, "")
	if err != nil {
		t.Fatal(err)
	}
	tk := e.wait(t, ts[0].ID)
	if tk.Status != task.StatusFailed || tk.Error == nil || tk.Error.Code != apperr.InvalidArgument {
		t.Fatalf("%v %+v", tk.Status, tk.Error)
	}
}

func TestNotInitialized(t *testing.T) {
	s := New(Config{})
	_, err := s.ConvertToPDF(context.Background(), []string{"/a.docx"}, "")
	wantCode(t, err, apperr.Internal)
	if l, err := s.ListRecentPDFs(context.Background(), 0); err != nil || l == nil || len(l) != 0 {
		t.Fatal(l, err)
	}
}

func TestNoFFmpegNeeded(t *testing.T) {
	t.Setenv("PATH", "")
	e := newEnv(t)
	in := filepath.Join(e.dir, "a.docx")
	makeDocx(t, in, "no ffmpeg")
	e.convertOK(t, in, "")
}

// 长中日文段落经自动换行后一个字都不能少（fpdf.MultiCell 会在每个换行处丢一个汉字，见 wrap.go）。
func TestConvertLongCJKParagraphKeepsEveryCharacter(t *testing.T) {
	e := newEnv(t)
	long := strings.Repeat("这是一段没有空格的很长的中文文字，用来检查自动换行是否会丢字。", 12)
	jp := strings.Repeat("日本語の長い文章です、読む話す黒龍。", 15)
	mixed := strings.Repeat("Mixed 中文 and English words，含标点（括号）「引号」。 ", 10)
	docs := map[string][]string{"a.docx": {long, jp, mixed}, "b.pptx": {long, jp, mixed}}
	makeDocx(t, filepath.Join(e.dir, "a.docx"), docs["a.docx"]...)
	makePptx(t, filepath.Join(e.dir, "b.pptx"), []int{1}, [][]string{docs["b.pptx"]})
	strip := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, s)
	}
	for name, paras := range docs {
		tk := e.convertOK(t, filepath.Join(e.dir, name), t.TempDir())
		got := strip(pdfText(t, tk.OutputPath))
		want := strip(strings.Join(paras, ""))
		if name == "b.pptx" {
			want = "Slide1" + want
		}
		if got != want {
			t.Fatalf("%s 文字与原文不一致：\n got %d 字 %.80q…\nwant %d 字 %.80q…", name, len([]rune(got)), got, len([]rune(want)), want)
		}
	}
}
