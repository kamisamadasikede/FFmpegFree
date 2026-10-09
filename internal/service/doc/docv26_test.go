package doc

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doccomp"
	"FFmpegFree/internal/service/convert"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// ---------- v0.26 文档多格式转换的测试环境 ----------

type fakeComp struct {
	st  doccomp.Status
	exe string
}

func (f *fakeComp) Status() doccomp.Status                             { return f.st }
func (f *fakeComp) Wait(context.Context, time.Duration) doccomp.Status { return f.st }
func (f *fakeComp) ExePath() string {
	if f.st.State == doccomp.StateReady {
		return f.exe
	}
	return ""
}
func (f *fakeComp) NotReadyError() *apperr.AppError {
	return apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。")
}

var (
	realCompOnce sync.Once
	realComp     *doccomp.Manager
)

// realComponent 返回箱子上真实检测到的文档组件（整个包共用一次检测）；没有就跳过。
func realComponent(t *testing.T) *doccomp.Manager {
	t.Helper()
	if testing.Short() {
		t.Skip("-short")
	}
	realCompOnce.Do(func() {
		dir, _ := os.MkdirTemp("", "doccomp-test-")
		m := doccomp.New(doccomp.Config{Dir: dir})
		m.Start()
		m.Wait(context.Background(), 3*time.Minute)
		realComp = m
	})
	if realComp.Status().State != doccomp.StateReady {
		t.Skip("没有可用的文档组件")
	}
	return realComp
}

type docEnv struct {
	*env
	conv *convert.Service
	out  string
}

func newDocEnv(t *testing.T, comp Component) *docEnv {
	t.Helper()
	var conv *convert.Service
	e := newEnv(t, func(c *Config) {
		st := c.Recent.(*store.Store)
		var err error
		conv, err = convert.New(context.Background(), convert.Config{Presets: st, Tasks: c.Tasks.(*task.Manager), Sources: st,
			DataDir: c.DataDir, DefaultOutputDir: c.DefaultOutputDir, Logf: t.Logf,
			UploadsDir: func(context.Context) string { return filepath.Join(c.DataDir, "..", "uploads") }})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conv.Close(time.Second) })
		c.Component, c.Sources, c.TempRoot = comp, conv, filepath.Join(c.DataDir, "tmp", "doc")
		c.UploadsDir = func(context.Context) string { return filepath.Join(c.DataDir, "..", "uploads") }
	})
	de := &docEnv{env: e, conv: conv, out: filepath.Join(e.dir, "out")}
	e.defOD = de.out
	return de
}

func (e *docEnv) add(t *testing.T, paths ...string) []AddDocSourceResult {
	t.Helper()
	res, err := e.svc.AddDocSources(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	// 等副本复制完（6.15）
	deadline := time.Now().Add(20 * time.Second)
	for i := range res {
		for res[i].Source != nil && res[i].Source.CopyState == store.CopyCopying && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			pg, _ := e.svc.ListDocSources(context.Background(), convert.ConvertSourceFilter{Limit: 200})
			for _, it := range pg.Items {
				if it.Source.SourceID == res[i].Source.SourceID {
					s := it.Source
					res[i].Source = &s
				}
			}
		}
	}
	return res
}

func (e *docEnv) submit(t *testing.T, target string, ids ...string) []task.Task {
	t.Helper()
	r, err := e.svc.SubmitDocConvert(context.Background(), DocSubmitRequest{SourceIDs: ids, Target: target})
	if err != nil {
		t.Fatalf("提交 → %s: %v", target, err)
	}
	return r.Tasks
}

func (e *docEnv) waitLong(t *testing.T, id string) task.Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	tk, err := e.tm.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

// ---------- 夹具 ----------

var gbkHello = []byte{0xC4, 0xE3, 0xBA, 0xC3, 0xA3, 0xAC, 0xCE, 0xC4, 0xB5, 0xB5, '\n', 0xB5, 0xDA, 0xB6, 0xFE, 0xD0, 0xD0, '\n'} // 你好，文档\n第二行\n

const png1x1 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func writeODF(t *testing.T, path, mime, content string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	// ODF 要求 mimetype 不压缩、不带数据描述符：用 CreateRaw 写好 CRC 和大小
	w, _ := zw.CreateRaw(&zip.FileHeader{Name: "mimetype", Method: zip.Store, CRC32: crc32.ChecksumIEEE([]byte(mime)),
		CompressedSize64: uint64(len(mime)), UncompressedSize64: uint64(len(mime))})
	w.Write([]byte(mime))
	w, _ = zw.Create("content.xml")
	w.Write([]byte(content))
	w, _ = zw.Create("META-INF/manifest.xml")
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">` +
		`<manifest:file-entry manifest:full-path="/" manifest:media-type="` + mime + `"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>`))
	zw.Close()
	f.Close()
}

const odfNS = `xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" ` +
	`xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" xmlns:presentation="urn:oasis:names:tc:opendocument:xmlns:presentation:1.0" ` +
	`xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" office:version="1.2"`

func makeODT(t *testing.T, path string, paras ...string) {
	var b strings.Builder
	for _, p := range paras {
		b.WriteString("<text:p>" + esc(p) + "</text:p>")
	}
	writeODF(t, path, "application/vnd.oasis.opendocument.text",
		`<?xml version="1.0" encoding="UTF-8"?><office:document-content `+odfNS+`><office:body><office:text>`+b.String()+`</office:text></office:body></office:document-content>`)
}

func makeODP(t *testing.T, path string, slides ...string) {
	var b strings.Builder
	for i, s := range slides {
		b.WriteString(`<draw:page draw:name="p` + itoa(i+1) + `"><draw:frame svg:x="2cm" svg:y="2cm" svg:width="20cm" svg:height="3cm"><draw:text-box><text:p>` + esc(s) + `</text:p></draw:text-box></draw:frame></draw:page>`)
	}
	writeODF(t, path, "application/vnd.oasis.opendocument.presentation",
		`<?xml version="1.0" encoding="UTF-8"?><office:document-content `+odfNS+`><office:body><office:presentation>`+b.String()+`</office:presentation></office:body></office:document-content>`)
}

// realDocx 用组件把一段文字转成真正的 docx（makeDocx 是给纯 Go 提取用的最小包，缺 _rels，组件打不开）。
func realDocx(t *testing.T, comp *doccomp.Manager, dst, text string) {
	t.Helper()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.txt")
	os.WriteFile(in, []byte(text), 0o644)
	out, err := doccomp.Convert(context.Background(), doccomp.Job{Exe: comp.ExePath(), Input: in, ConvertTo: "docx:MS Word 2007 XML",
		InFilter: "Text (encoded):UTF8", WorkDir: filepath.Join(dir, "w"), OutExt: "docx", Timeout: 2 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	writeFile(t, dst, b)
}

func writeFile(t *testing.T, p string, b []byte) string {
	t.Helper()
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readS(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ---------- 真组件端到端 ----------

func TestRealDocConvertMatrix(t *testing.T) {
	comp := realComponent(t)
	e := newDocEnv(t, comp)
	src := filepath.Join(e.dir, "src")
	os.MkdirAll(filepath.Join(src, "img"), 0o755)
	png, _ := base64.StdEncoding.DecodeString(png1x1)
	writeFile(t, filepath.Join(src, "img", "dot.png"), png)
	files := map[string]string{
		"txt":  writeFile(t, filepath.Join(src, "说明.txt"), gbkHello),
		"md":   writeFile(t, filepath.Join(src, "readme.md"), []byte("# 标题一\n\n正文 **加粗**\n\n| 列A | 列B |\n|---|---|\n| 1 | 2 |\n\n![本地图](img/dot.png)\n\n![远程图](https://example.invalid/x.png)\n\n![丢失](img/none.png)\n\n<script>alert(1)</script>\n")),
		"html": writeFile(t, filepath.Join(src, "page.htm"), []byte("<html><head><style>p{color:red}</style><script>var x=1</script></head><body><h1>网页标题</h1><p>段落</p><table><tr><th>甲</th><th>乙</th></tr><tr><td>1</td><td>2</td></tr></table></body></html>")),
		"csv":  writeFile(t, filepath.Join(src, "数据.csv"), []byte{0xC3, 0xFB, 0xB3, 0xC6, ',', 0xCA, 0xFD, 0xC1, 0xBF, '\n', 0xC6, 0xBB, 0xB9, 0xFB, ',', '3', '\n'}), // 名称,数量\n苹果,3
	}
	files["docx"] = filepath.Join(src, "报告.docx")
	realDocx(t, comp, files["docx"], "报告第一段\n报告第二段\n")
	files["odt"] = filepath.Join(src, "笔记.odt")
	makeODT(t, files["odt"], "笔记第一段", "笔记第二段")
	files["xlsx"] = filepath.Join(src, "三表.xlsx")
	makeXlsx(t, files["xlsx"], map[string][][]string{"第一": {{"表一", "1"}}, "第二": {{"表二", "2"}}, "第三": {{"表三", "3"}}}, []string{"第一", "第二", "第三"})
	files["odp"] = filepath.Join(src, "演示.odp")
	makeODP(t, files["odp"], "第一页内容", "第二页内容")

	order := []string{"txt", "md", "html", "csv", "docx", "odt", "xlsx", "odp"}
	var paths []string
	for _, k := range order {
		paths = append(paths, files[k])
	}
	t0 := time.Now()
	res := e.add(t, paths...)
	t.Logf("添加 %d 个文件用时 %s", len(paths), time.Since(t0).Round(time.Millisecond))
	ids := map[string]string{}
	for i, r := range res {
		if r.Error != nil || r.Source == nil {
			t.Fatalf("%s: %+v", paths[i], r.Error)
		}
		ids[order[i]] = r.Source.SourceID
		if r.Source.Ext != map[string]string{"html": "html"}[order[i]] && order[i] == "html" {
			t.Fatalf("htm 应规范化为 html: %+v", r.Source)
		}
		if r.Source.Media != nil {
			t.Fatal("文档行没有 media")
		}
	}
	if res[3].Source.SheetCount != 1 || res[6].Source.SheetCount != 3 || res[0].Source.SheetCount != 0 || res[0].Source.Family != FamilyText || res[7].Source.Family != FamilySlide {
		t.Fatalf("sheetCount / family: %+v %+v", res[3].Source, res[6].Source)
	}

	type sub struct{ src, target string }
	jobs := []sub{
		{"txt", "pdf"}, {"txt", "docx"}, {"txt", "odt"}, {"txt", "doc"}, {"txt", "rtf"}, {"txt", "html"}, {"txt", "md"},
		{"md", "html"}, {"md", "docx"}, {"md", "pdf"}, {"md", "txt"},
		{"html", "md"}, {"html", "pdf"}, {"html", "docx"},
		{"csv", "xlsx"}, {"csv", "ods"}, {"csv", "xls"}, {"csv", "pdf"},
		{"docx", "pdf"}, {"docx", "txt"}, {"docx", "md"}, {"docx", "doc"},
		{"odt", "pdf"}, {"odt", "docx"},
		{"xlsx", "csv"}, {"xlsx", "ods"}, {"xlsx", "pdf"},
		{"odp", "pptx"}, {"odp", "ppt"}, {"odp", "pdf"},
	}
	t1 := time.Now()
	var tasks []task.Task
	sawQueue := false
	for _, j := range jobs {
		ts := e.submit(t, j.target, ids[j.src])
		if len(ts) != 1 {
			t.Fatalf("%v", ts)
		}
		tk := ts[0]
		wantType := task.TypeDocConvert
		if tk.Type != wantType || tk.SourceID != ids[j.src] || !strings.Contains(tk.Title, " → "+strings.ToUpper(j.target)) {
			t.Fatalf("%+v", tk)
		}
		if tk.QueuePosition != nil {
			sawQueue = true
		}
		if goPair(j.src, j.target) && tk.QueuePosition != nil {
			t.Fatalf("md ↔ html 不排队: %+v", tk)
		}
		tasks = append(tasks, tk)
	}
	if !sawQueue {
		t.Error("一次提交很多个组件任务时应有排队位置")
	}
	outs := map[sub]string{}
	for i, tk := range tasks {
		done := e.waitLong(t, tk.ID)
		if done.Status != task.StatusSucceeded {
			t.Errorf("%v: %s %+v", jobs[i], done.Status, done.Error)
			continue
		}
		if done.QueuePosition != nil {
			t.Errorf("完成的任务不该有排队位置")
		}
		fi, err := os.Stat(done.OutputPath)
		if err != nil || fi.Size() == 0 || !strings.EqualFold(filepath.Ext(done.OutputPath), "."+jobs[i].target) {
			t.Errorf("%v: 输出 %s %v", jobs[i], done.OutputPath, err)
		}
		outs[jobs[i]] = done.OutputPath
		wantEngine := "component"
		if goPair(jobs[i].src, jobs[i].target) {
			wantEngine = "go"
		}
		if done.Result == nil || done.Result.Engine != wantEngine || done.Result.SizeBytes == 0 {
			t.Errorf("%v: result %+v", jobs[i], done.Result)
		}
		if jobs[i].src == "xlsx" && jobs[i].target == "csv" {
			if done.Result == nil || len(done.Result.Warnings) != 1 || done.Result.Warnings[0] != WarningCSVFirstSheetOnly {
				t.Errorf("多表转 CSV 应有警告: %+v", done.Result)
			}
		}
	}
	t.Logf("%d 个转换（并发 2）总用时 %s", len(jobs), time.Since(t1).Round(time.Millisecond))
	if t.Failed() {
		return
	}
	// 内容抽查
	if s := pdfText(t, outs[sub{"txt", "pdf"}]); !strings.Contains(noSpace(s), "你好，文档") {
		t.Errorf("GBK txt → pdf: %q", s)
	}
	if s := readS(t, outs[sub{"txt", "md"}]); !strings.Contains(s, "你好，文档") || !utf8.ValidString(s) {
		t.Errorf("txt → md: %q", s)
	}
	h := readS(t, outs[sub{"md", "html"}])
	if !strings.HasPrefix(h, "<!DOCTYPE html>") || !strings.Contains(h, "<title>readme</title>") || !strings.Contains(h, "<table>") ||
		strings.Contains(h, "<script>") || !strings.Contains(h, `src="https://example.invalid/x.png"`) || !strings.Contains(h, `src="img/dot.png"`) {
		t.Errorf("md → html: %s", h)
	}
	if s := pdfText(t, outs[sub{"md", "pdf"}]); !strings.Contains(s, "标题一") || !strings.Contains(s, "远程图") || !strings.Contains(s, "丢失") {
		t.Errorf("md → pdf 应保留标题、网络图片和丢失图片换成 alt: %q", s)
	}
	if s := readS(t, outs[sub{"md", "txt"}]); !strings.Contains(s, "标题一") || strings.Contains(s, "alert") {
		t.Errorf("md → txt: %q", s)
	}
	if s := readS(t, outs[sub{"html", "md"}]); !strings.Contains(s, "# 网页标题") || !strings.Contains(s, "| 甲 | 乙 |") || strings.Contains(s, "var x") || strings.Contains(s, "color") {
		t.Errorf("html → md: %q", s)
	}
	if s := readS(t, outs[sub{"docx", "md"}]); !strings.Contains(s, "报告第一段") || strings.Contains(s, "\r") {
		t.Errorf("docx → md: %q", s)
	}
	if s := readS(t, outs[sub{"docx", "txt"}]); !strings.Contains(s, "报告第二段") {
		t.Errorf("docx → txt: %q", s)
	}
	c := readS(t, outs[sub{"xlsx", "csv"}])
	if !strings.Contains(c, "表一") || strings.Contains(c, "表二") || strings.HasPrefix(c, "\xEF\xBB\xBF") {
		t.Errorf("xlsx → csv 只要第一个工作表、UTF-8 无 BOM: %q", c)
	}
	if s := pdfText(t, outs[sub{"csv", "pdf"}]); !strings.Contains(s, "苹果") {
		t.Errorf("GBK csv → pdf: %q", s)
	}
	if s := pdfText(t, outs[sub{"odp", "pdf"}]); !strings.Contains(s, "第二页内容") {
		t.Errorf("odp → pdf: %q", s)
	}
	// 二次转换：上一步的输出作为新源（真实的 doc / xls / ppt / pptx / ods）
	t2 := time.Now()
	more := e.add(t, outs[sub{"txt", "doc"}], outs[sub{"csv", "xls"}], outs[sub{"odp", "ppt"}], outs[sub{"odp", "pptx"}], outs[sub{"xlsx", "ods"}], outs[sub{"txt", "rtf"}])
	for _, r := range more {
		if r.Error != nil {
			t.Fatalf("%s: %+v", r.Path, r.Error)
		}
	}
	if more[1].Source.SheetCount != 1 || more[4].Source.SheetCount != 3 {
		t.Errorf("xls / ods 的 sheetCount: %d %d", more[1].Source.SheetCount, more[4].Source.SheetCount)
	}
	second := []struct {
		i      int
		target string
	}{{0, "docx"}, {0, "pdf"}, {1, "xlsx"}, {1, "csv"}, {2, "pptx"}, {3, "odp"}, {3, "pdf"}, {4, "csv"}, {5, "odt"}, {5, "md"}}
	for _, s := range second {
		ts := e.submit(t, s.target, more[s.i].Source.SourceID)
		done := e.waitLong(t, ts[0].ID)
		if done.Status != task.StatusSucceeded {
			t.Errorf("%s → %s: %+v", more[s.i].Path, s.target, done.Error)
		}
		if s.i == 4 && s.target == "csv" && (done.Result == nil || len(done.Result.Warnings) == 0) {
			t.Errorf("ods(3 表) → csv 应有警告")
		}
	}
	t.Logf("二次转换 %d 个用时 %s", len(second), time.Since(t2).Round(time.Millisecond))
}

// 简易转换（组件未就绪）：docx / odt / txt → PDF 走 office_pdf（带 sourceId，不排队），doc → PDF 被拒；md ↔ html 照常。
func TestSimpleModeWithoutComponent(t *testing.T) {
	comp := &fakeComp{st: doccomp.Status{State: doccomp.StateMissing, CanDownload: true}}
	e := newDocEnv(t, comp)
	src := filepath.Join(e.dir, "src")
	os.MkdirAll(src, 0o755)
	txt := writeFile(t, filepath.Join(src, "a.txt"), gbkHello)
	odt := filepath.Join(src, "b.odt")
	makeODT(t, odt, "开放文档第一段", "第二段")
	docx := filepath.Join(src, "c.docx")
	makeDocx(t, docx, "Word 第一段")
	md := writeFile(t, filepath.Join(src, "d.md"), []byte("# hi\n"))
	doc := filepath.Join(src, "e.doc")
	writeCFB(t, doc, cfbStream{"WordDocument", make([]byte, 64)})
	res := e.add(t, txt, odt, docx, md, doc)
	for _, r := range res {
		if r.Error != nil {
			t.Fatalf("%s: %+v", r.Path, r.Error)
		}
	}
	m := e.svc.GetFormatMatrix(context.Background())
	if m.ComponentReady {
		t.Fatal("未就绪")
	}
	for i, want := range []string{"开放文档第一段", "Word 第一段"} {
		ts := e.submit(t, "pdf", res[i+1].Source.SourceID)
		if ts[0].Type != task.TypeOfficePDF || ts[0].SourceID == "" || ts[0].QueuePosition != nil {
			t.Fatalf("%+v", ts[0])
		}
		done := e.wait(t, ts[0].ID)
		if done.Status != task.StatusSucceeded || !strings.Contains(pdfText(t, done.OutputPath), want) {
			t.Fatalf("%+v", done)
		}
	}
	// 同名第二次：转换页的重名格式 "a (1).pdf"
	a1 := e.wait(t, e.submit(t, "pdf", res[0].Source.SourceID)[0].ID)
	a2 := e.wait(t, e.submit(t, "pdf", res[0].Source.SourceID)[0].ID)
	if a1.Result == nil || a1.Result.Engine != "simple" {
		t.Fatalf("简易转换 result.engine=simple: %+v", a1.Result)
	}
	if filepath.Base(a1.OutputPath) != "a.pdf" || filepath.Base(a2.OutputPath) != "a (1).pdf" || !strings.Contains(noSpace(pdfText(t, a2.OutputPath)), "你好，文档") {
		t.Fatalf("%s %s", a1.OutputPath, a2.OutputPath)
	}
	// 文档页的记录出现在这一行下面
	pg, _ := e.svc.ListDocSources(context.Background(), convert.ConvertSourceFilter{Limit: 50, RecordLimit: 10})
	if pg.Total != 5 {
		t.Fatalf("%+v", pg)
	}
	// 转换页看不到文档行
	if mp, err := e.conv.ListSources(context.Background(), convert.ConvertSourceFilter{Limit: 50}); err != nil || mp.Total != 0 {
		t.Fatalf("%+v %v", mp, err)
	}
	// 需要组件的被拒
	_, err := e.svc.SubmitDocConvert(context.Background(), DocSubmitRequest{SourceIDs: []string{res[4].Source.SourceID}, Target: "pdf"})
	if !apperr.Is(err, apperr.DocComponentNotReady) {
		t.Fatal(err)
	}
	_, err = e.svc.SubmitDocConvert(context.Background(), DocSubmitRequest{SourceIDs: []string{res[0].Source.SourceID}, Target: "xlsx"})
	if !apperr.Is(err, apperr.DocFormatUnsupported) || apperr.From(err).Message != "不支持转成这个格式。" {
		t.Fatal(err)
	}
	// md → html 不需要组件
	if d := e.wait(t, e.submit(t, "html", res[3].Source.SourceID)[0].ID); d.Status != task.StatusSucceeded {
		t.Fatalf("%+v", d)
	}
	// 简易转换的记录重转仍是简易转换
	rt, err := e.conv.Reconvert(context.Background(), convert.ReconvertRequest{TaskID: a1.ID})
	if err != nil {
		t.Fatal(err)
	}
	if d := e.wait(t, rt.ID); d.Status != task.StatusSucceeded || d.OutputPath != a1.OutputPath {
		t.Fatalf("%+v", d)
	}
	if _, err := e.conv.Reconvert(context.Background(), convert.ReconvertRequest{TaskID: a1.ID, PresetID: "x"}); !apperr.Is(err, apperr.InvalidArgument) || !strings.HasPrefix(apperr.From(err).Detail, "reason=params_locked") {
		t.Fatal(err)
	}
}

func TestAddDocSourcesRejects(t *testing.T) {
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	d := filepath.Join(e.dir, "src")
	os.MkdirAll(d, 0o755)
	pdf := writeFile(t, filepath.Join(d, "a.pdf"), []byte("not a pdf")) // v0.28：PDF 可以添加；不是 PDF → DOC_CORRUPT
	exe := writeFile(t, filepath.Join(d, "a.exe"), []byte("MZ"))
	noext := writeFile(t, filepath.Join(d, "noext"), []byte("x"))
	enc := filepath.Join(d, "enc.docx")
	writeCFB(t, enc, cfbStream{"EncryptionInfo", []byte{1}}, cfbStream{"EncryptedPackage", []byte{1}})
	bad := writeFile(t, filepath.Join(d, "bad.xlsx"), []byte("not a zip"))
	big := filepath.Join(d, "big.txt")
	f, _ := os.Create(big)
	f.Truncate(MaxInputBytes + 1)
	f.Close()
	res := e.add(t, pdf, exe, noext, enc, bad, big, filepath.Join(d, "missing.docx"), "relative.docx")
	want := []apperr.Code{apperr.DocCorrupt, apperr.DocFormatUnsupported, apperr.DocFormatUnsupported, apperr.DocEncrypted, apperr.DocCorrupt, apperr.InvalidArgument, apperr.NotFound, apperr.InvalidArgument}
	for i, r := range res {
		if r.Source != nil || r.Error == nil || r.Error.Code != want[i] {
			t.Errorf("%s: %+v，应为 %s", r.Path, r.Error, want[i])
		}
	}
	if res[5].Error.Detail != "reason=too_large" || res[1].Error.Message != "不支持这种文件。" {
		t.Errorf("%+v %+v", res[5].Error, res[0].Error)
	}
	if pg, _ := e.svc.ListDocSources(context.Background(), convert.ConvertSourceFilter{Limit: 10}); pg.Total != 0 {
		t.Fatal("被拒的不建行")
	}
}

// 不可重试：加密 / 损坏的失败记录 Retry 返回 UNSUPPORTED（reason=not_retryable）。
func TestNotRetryable(t *testing.T) {
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	d := filepath.Join(e.dir, "src")
	os.MkdirAll(d, 0o755)
	p := filepath.Join(d, "x.docx")
	makeDocx(t, p, "hi")
	r := e.add(t, p)
	// 提交后把副本换成加密文件：运行前的检查命中，任务失败 DOC_ENCRYPTED
	in := r[0].Source.StoredPath
	if in == "" {
		in = p
	}
	ts := e.submit(t, "pdf", r[0].Source.SourceID)
	done := e.wait(t, ts[0].ID)
	if done.Status != task.StatusSucceeded {
		t.Fatalf("%+v", done)
	}
	writeCFB(t, in, cfbStream{"EncryptionInfo", []byte{1}}, cfbStream{"EncryptedPackage", []byte{1}})
	_, err := e.svc.SubmitDocConvert(context.Background(), DocSubmitRequest{SourceIDs: []string{r[0].Source.SourceID}, Target: "pdf"})
	if !apperr.Is(err, apperr.DocEncrypted) {
		t.Fatalf("提交时也要查加密: %v", err)
	}
	markFailedWith(t, e.st, done.ID, apperr.New(apperr.DocEncrypted, "x"))
	if _, err := e.tm.Retry(done.ID); !apperr.Is(err, apperr.Unsupported) || !strings.HasPrefix(apperr.From(err).Detail, "reason=not_retryable") || apperr.From(err).Message != "这个文件不能重试，请移出后重新添加。" {
		t.Fatal(err)
	}
}

func markFailedWith(t *testing.T, st *store.Store, id string, ae *apperr.AppError) {
	t.Helper()
	tk, err := st.GetTask(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	tk.Status, tk.Error = task.StatusFailed, ae
	if err := st.UpdateTask(context.Background(), tk); err != nil {
		t.Fatal(err)
	}
}

func TestMarkdownImagesForComponent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.png"), []byte("x"), 0o644)
	md := []byte("![本地](a.png) ![没有](b.png) ![网络](https://x.invalid/c.png) ![](//cdn.invalid/d.png) <b>raw</b>")
	h, err := markdownToHTML(md, "t", imagesForComponent, dir)
	if err != nil {
		t.Fatal(err)
	}
	s := string(h)
	if !strings.Contains(s, `src="file:///`) || strings.Contains(s, "b.png") || strings.Contains(s, "x.invalid") || strings.Contains(s, "cdn.invalid") ||
		!strings.Contains(s, "没有") || !strings.Contains(s, "网络") || strings.Contains(s, "<b>raw") {
		t.Fatal(s)
	}
	h2, _ := markdownToHTML(md, "t", imagesForHTML, dir)
	if !bytes.Contains(h2, []byte(`src="https://x.invalid/c.png"`)) {
		t.Fatal(string(h2))
	}
}
