package doc

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/localassets"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"

	"github.com/xuri/excelize/v2"
)

type rec struct {
	mu   sync.Mutex
	prog map[string][]float64
}

func (r *rec) Emit(name string, p any) {
	if pe, ok := p.(task.ProgressEvent); ok {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.prog[pe.ID] = append(r.prog[pe.ID], pe.Progress)
	}
}

func (r *rec) get(id string) []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]float64(nil), r.prog[id]...)
}

type env struct {
	svc   *Service
	tm    *task.Manager
	st    *store.Store
	reg   *localassets.Registry
	dir   string
	data  string
	em    *rec
	defOD string
}

type envOpt func(*Config)

func newEnv(t *testing.T, opts ...envOpt) *env {
	t.Helper()
	dir := t.TempDir()
	data := filepath.Join(dir, "appdata")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), filepath.Join(data, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &env{st: st, dir: dir, data: data, em: &rec{prog: map[string][]float64{}}, reg: localassets.New(localassets.Config{})}
	e.tm = task.NewManager(task.Config{Store: st, Emitter: e.em, LogDir: filepath.Join(data, "logs"), BatchConcurrency: 2,
		ProgressInterval: -1, Logf: func(string, ...any) {}})
	t.Cleanup(func() { e.tm.Shutdown(3 * time.Second) })
	cfg := Config{Recent: st, Lister: st, Tasks: e.tm, Local: e.reg, DataDir: data,
		DefaultOutputDir: func(context.Context) string { return e.defOD }}
	for _, o := range opts {
		o(&cfg)
	}
	e.svc = New(cfg)
	return e
}

func (e *env) wait(t *testing.T, id string) task.Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tk, err := e.tm.Wait(ctx, id)
	if err != nil {
		t.Fatalf("等待任务: %v", err)
	}
	return tk
}

func (e *env) convertOK(t *testing.T, in, outDir string) task.Task {
	t.Helper()
	ts, err := e.svc.ConvertToPDF(context.Background(), []string{in}, outDir)
	if err != nil || len(ts) != 1 {
		t.Fatalf("ConvertToPDF: %v %v", ts, err)
	}
	tk := e.wait(t, ts[0].ID)
	if tk.Status != task.StatusSucceeded {
		t.Fatalf("任务未成功: %+v err=%+v", tk.Status, tk.Error)
	}
	return tk
}

// ---------- 现场构造 OOXML ----------

func esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

// makeDocx 构造最小 docx（zip + xml）：每个字符串一个段落，段落内拆成两个 run 以验证拼接。
func makeDocx(t *testing.T, path string, paras ...string) {
	t.Helper()
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, p := range paras {
		r := []rune(p)
		h := len(r) / 2
		fmt.Fprintf(&b, `<w:p><w:r><w:t xml:space="preserve">%s</w:t></w:r><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, esc(string(r[:h])), esc(string(r[h:])))
	}
	b.WriteString(`</w:body></w:document>`)
	writeZip(t, path, map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`,
		"word/document.xml":   b.String(),
	})
}

// makePptx 构造最小 pptx：slides[i] 是第 i+1 张（文件名 slide<n>.xml，n 由 nums 指定，用于测试数字排序）。
func makePptx(t *testing.T, path string, nums []int, slides [][]string) {
	t.Helper()
	files := map[string]string{"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`}
	for i, paras := range slides {
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody>`)
		for _, p := range paras {
			fmt.Fprintf(&b, `<a:p><a:r><a:t>%s</a:t></a:r></a:p>`, esc(p))
		}
		b.WriteString(`</p:txBody></p:sp></p:spTree></p:cSld></p:sld>`)
		files[fmt.Sprintf("ppt/slides/slide%d.xml", nums[i])] = b.String()
	}
	writeZip(t, path, files)
}

// makeXlsx 用 excelize 生成 xlsx（手写共享字符串 / 样式部件太繁琐；这一份不是“手写 zip”，如实说明）。
func makeXlsx(t *testing.T, path string, sheets map[string][][]string, order []string) {
	t.Helper()
	x := excelize.NewFile()
	defer x.Close()
	for i, name := range order {
		if i == 0 {
			x.SetSheetName("Sheet1", name)
		} else if _, err := x.NewSheet(name); err != nil {
			t.Fatal(err)
		}
		for r, row := range sheets[name] {
			for c, v := range row {
				cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
				x.SetCellValue(name, cell, v)
			}
		}
	}
	if err := x.SaveAs(path); err != nil {
		t.Fatal(err)
	}
}

// ---------- pdftotext / pdftoppm ----------

func pdfText(t *testing.T, path string) string {
	t.Helper()
	p, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("没有 pdftotext（poppler-utils），跳过")
	}
	out, err := exec.Command(p, "-enc", "UTF-8", path, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	return string(out)
}

func pdfPages(t *testing.T, path string) int {
	t.Helper()
	p, err := exec.LookPath("pdfinfo")
	if err != nil {
		t.Skip("没有 pdfinfo")
	}
	out, err := exec.Command(p, path).Output()
	if err != nil {
		t.Fatalf("pdfinfo: %v", err)
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "Pages:") {
			var n int
			fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(l, "Pages:")), "%d", &n)
			return n
		}
	}
	t.Fatal("pdfinfo 没有 Pages")
	return 0
}

func noSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\f' || r == '\t' {
			return -1
		}
		return r
	}, s)
}
