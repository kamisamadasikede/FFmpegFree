package doc

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// 回归：xlsx 首列为空、后面的单元格以 , . % 开头。行首空白 + 禁则标点曾让内容整段消失。
func TestXlsxEmptyFirstColumnAndPunctuationLeadingCells(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "lead.xlsx")
	makeXlsx(t, in, map[string][][]string{"S": {
		{"", ".5 tail"},
		{"", ",comma first", "%pct"},
		{"", "", ".dot", ",x", "。句号开头"},
		{"", "，全角逗号开头"},
		{"a", ".b", ",c", "%d"},
	}}, []string{"S"})
	tk := e.convertOK(t, in, t.TempDir())
	txt := pdfText(t, tk.OutputPath)
	for _, want := range []string{".5 tail", ",comma first", "%pct", ".dot", ",x", "。句号开头", "，全角逗号开头", ".b", ",c", "%d"} {
		if !strings.Contains(txt, want) {
			t.Errorf("PDF 里缺少 %q\n%s", want, txt)
		}
	}
}

func TestPptxAndDocxPunctuationLeadingParagraphs(t *testing.T) {
	e := newEnv(t)
	paras := []string{"  ,abc", "\t.5 tail", "%x", "，你好", "。"}
	makeDocx(t, filepath.Join(e.dir, "p.docx"), paras...)
	makePptx(t, filepath.Join(e.dir, "p.pptx"), []int{1}, [][]string{paras})
	for _, n := range []string{"p.docx", "p.pptx"} {
		tk := e.convertOK(t, filepath.Join(e.dir, n), t.TempDir())
		got := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\f' || r == '\t' {
				return -1
			}
			return r
		}, pdfText(t, tk.OutputPath))
		for _, w := range []string{",abc", ".5tail", "%x", "，你好", "。"} {
			if !strings.Contains(got, w) {
				t.Errorf("%s 缺少 %q: %q", n, w, got)
			}
		}
	}
}

// 100 万个连续标点：转换要在几秒内完成（折行线性），取消要及时返回。
func TestConvertManyPunctuationFastAndCancelable(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "punct.docx")
	makeDocx(t, in, strings.Repeat("，", 1_000_000))
	start := time.Now()
	tk := e.convertOK(t, in, t.TempDir())
	if d := time.Since(start); d > 30*time.Second {
		t.Fatalf("100 万个连续标点转换用了 %v", d)
	}
	if got := strings.Count(pdfText(t, tk.OutputPath), "，"); got != 1_000_000 {
		t.Fatalf("PDF 里逗号数 %d", got)
	}
	// 取消
	ts, err := e.svc.ConvertToPDF(context.Background(), []string{in}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	c0 := time.Now()
	e.tm.Cancel(ts[0].ID)
	tk2 := e.wait(t, ts[0].ID)
	if d := time.Since(c0); d > 3*time.Second {
		t.Fatalf("取消后 %v 才结束", d)
	}
	if tk2.Status != task.StatusCanceled && tk2.Status != task.StatusSucceeded {
		t.Fatalf("状态 %v", tk2.Status)
	}
}

// zip 条目数上限：MaxZipEntries+1 个条目 → INVALID_ARGUMENT（与 256 MiB 条目的 zip 炸弹同一个错误码）；
// 恰好 MaxZipEntries 个不触发。三种格式都测。条目数超过 65535 时 Go 会写 zip64 记录，一并覆盖。
func TestZipEntryCountLimit(t *testing.T) {
	if MaxZipEntries != 100_000 {
		t.Fatalf("MaxZipEntries=%d", MaxZipEntries)
	}
	e := newEnv(t)
	write := func(path, part, body string, total int) {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(f)
		w, _ := zw.Create(part)
		w.Write([]byte(body))
		for i := 1; i < total; i++ {
			if _, err := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("x/%d", i), Method: zip.Store}); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	docxBody := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>ok</w:t></w:r></w:p></w:body></w:document>`
	sld := `<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><a:p><a:r><a:t>ok</a:t></a:r></a:p></p:sld>`
	cases := []struct{ name, part, body string }{
		{"big.docx", "word/document.xml", docxBody},
		{"big.pptx", "ppt/slides/slide1.xml", sld},
		{"big.xlsx", "xl/workbook.xml", "<workbook/>"},
	}
	for _, c := range cases {
		p := filepath.Join(e.dir, c.name)
		write(p, c.part, c.body, MaxZipEntries+1)
		if n, ok := zipEntryCount(p); !ok || n != MaxZipEntries+1 {
			t.Fatalf("%s 条目数读取 %d %v", c.name, n, ok)
		}
		_, err := e.svc.ConvertToPDF(context.Background(), []string{p}, "")
		ae := wantCode(t, err, apperr.InvalidArgument)
		if !strings.Contains(ae.Detail, "100000") {
			t.Fatalf("%s detail=%q", c.name, ae.Detail)
		}
	}
	// 恰好上限：校验通过（docx 能转换）
	p := filepath.Join(e.dir, "edge.docx")
	write(p, "word/document.xml", docxBody, MaxZipEntries)
	if n, ok := zipEntryCount(p); !ok || n != MaxZipEntries {
		t.Fatalf("edge 条目数 %d %v", n, ok)
	}
	e.convertOK(t, p, t.TempDir())
}

// 启动清理 .part：五个条件逐条验证。
func TestCleanupInterruptedParts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	dir := t.TempDir()
	other := t.TempDir() // 没有任何任务记录登记的目录
	mk := func(p string, old bool) {
		t.Helper()
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if old {
			past := time.Now().Add(-time.Hour)
			os.Chtimes(p, past, past)
		}
	}
	out := filepath.Join(dir, "报告.pdf")
	insert := func(id string, typ store.TaskType, st store.TaskStatus, outPath string) {
		t.Helper()
		if err := e.st.InsertTask(ctx, store.Task{ID: id, Type: typ, Status: st, Title: id, OutputPath: outPath, CreatedAt: time.Now().UnixMilli(), Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
	insert("A", store.TypeOfficePDF, store.StatusInterrupted, out)
	// 应删：<name>.part.pdf、<name>(3).part.pdf
	mk(filepath.Join(dir, "报告.part.pdf"), true)
	mk(filepath.Join(dir, "报告(3).part.pdf"), true)
	// 不删：修改时间晚于启动（新任务正在写）
	mk(filepath.Join(dir, "报告(4).part.pdf"), false)
	os.Chtimes(filepath.Join(dir, "报告(4).part.pdf"), time.Now().Add(time.Minute), time.Now().Add(time.Minute))
	// 不删：最终文件、超出 1..99 的编号、别的名字、别的扩展名
	mk(out, true)
	mk(filepath.Join(dir, "报告(100).part.pdf"), true)
	mk(filepath.Join(dir, "别的.part.pdf"), true)
	mk(filepath.Join(dir, "报告.part.mp4"), true)
	mk(filepath.Join(dir, "报告.part"), true)
	// 不删：符号链接（指向别处的文件）
	victim := filepath.Join(other, "victim.txt")
	mk(victim, true)
	if err := os.Symlink(victim, filepath.Join(dir, "报告(5).part.pdf")); err != nil {
		t.Skip("不能建符号链接:", err)
	}
	// 不删：同名目录
	os.MkdirAll(filepath.Join(dir, "报告(6).part.pdf"), 0o755)
	os.WriteFile(filepath.Join(dir, "报告(6).part.pdf", "inner"), []byte("x"), 0o644)
	// 未登记目录里同样命名的文件不碰
	mk(filepath.Join(other, "报告.part.pdf"), true)
	// 其他类型 / 状态的任务不影响
	out2 := filepath.Join(other, "别的任务.pdf")
	insert("B", store.TypeConvert, store.StatusInterrupted, out2)
	insert("C", store.TypeOfficePDF, store.StatusSucceeded, out2)
	insert("D", store.TypeOfficePDF, store.StatusInterrupted, "relative/path.pdf")
	insert("E", store.TypeOfficePDF, store.StatusInterrupted, filepath.Join(other, "x.docx"))
	mk(filepath.Join(other, "别的任务.part.pdf"), true)
	mk(filepath.Join(other, "x.part.docx"), true)

	svc := New(Config{Lister: e.st})
	if n := svc.CleanupInterruptedParts(ctx); n != 2 {
		t.Fatalf("应删除 2 个，实际 %d", n)
	}
	gone := []string{filepath.Join(dir, "报告.part.pdf"), filepath.Join(dir, "报告(3).part.pdf")}
	for _, p := range gone {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s 应已删除", p)
		}
	}
	kept := []string{filepath.Join(dir, "报告(4).part.pdf"), out, filepath.Join(dir, "报告(100).part.pdf"), filepath.Join(dir, "别的.part.pdf"),
		filepath.Join(dir, "报告.part.mp4"), filepath.Join(dir, "报告.part"), victim, filepath.Join(dir, "报告(5).part.pdf"),
		filepath.Join(dir, "报告(6).part.pdf", "inner"), filepath.Join(other, "报告.part.pdf"),
		filepath.Join(other, "别的任务.part.pdf"), filepath.Join(other, "x.part.docx")}
	for _, p := range kept {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("%s 不应被删: %v", p, err)
		}
	}
	// 没有 Lister 时什么都不做
	if n := New(Config{}).CleanupInterruptedParts(ctx); n != 0 {
		t.Fatal(n)
	}
}
