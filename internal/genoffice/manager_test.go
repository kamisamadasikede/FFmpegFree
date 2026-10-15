package genoffice

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixturePage(title string) map[string]any {
	return map[string]any{"background": "#EEF4FF", "elements": []any{
		map[string]any{"type": "shape", "shape": "rect", "x": 0, "y": 0, "w": 1280, "h": 18, "fill": "#2874D0"},
		map[string]any{"type": "text", "x": 72, "y": 96, "w": 1100, "h": 120, "paragraphs": []any{map[string]any{"runs": []any{map[string]any{"text": title, "sizePt": 36, "bold": true, "font": "Microsoft YaHei", "color": "#203B60"}}}}},
		map[string]any{"type": "text", "x": 72, "y": 280, "w": 1040, "h": 100, "paragraphs": []any{map[string]any{"runs": []any{map[string]any{"text": "原生文本 · 实时生成与编辑", "sizePt": 22, "font": "Microsoft YaHei"}}}}},
	}}
}
func TestManagerNativePreviewAndAuthorization(t *testing.T) {
	if _, err := nodePath(t.TempDir(), false); err != nil {
		t.Skip(err)
	}
	t.Setenv("FFC_OFFICE_PAINT_MS", "0")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root := t.TempDir()
	file := filepath.Join(root, "live.pptx")
	m := New(Config{Dir: t.TempDir()})
	t.Cleanup(m.Close)
	guard := func() error { return nil }
	state, err := m.Open(ctx, file, root, true, guard)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := m.Start(ctx, file, root, guard)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Mode != "genoffice" || preview.URL == "" || preview.Token == "" {
		t.Fatal("missing canvas capability")
	}
	post := func(token, origin string) int {
		body, _ := json.Marshal(map[string]any{"id": preview.DocumentID, "action": "state"})
		req, _ := http.NewRequest("POST", strings.TrimSuffix(preview.URL, "/editor.html")+"/api", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		r, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Body.Close()
		return r.StatusCode
	}
	base := strings.TrimSuffix(preview.URL, "/editor.html")
	if post("wrong", base) != 401 || post(preview.Token, "https://example.com") != 403 || post(preview.Token, base) != 200 {
		t.Fatal("preview origin/token protection failed")
	}
	_, err = m.Call(ctx, preview.DocumentID, map[string]any{"action": "page", "expectedRevision": state["revision"], "requestId": "first-page", "page": fixturePage("ffc · 第一页")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Call(ctx, preview.DocumentID, map[string]any{"action": "save"}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(file); err != nil {
		t.Fatal(err)
	}
	m.Release(preview.ID)
	if _, err = m.Call(ctx, preview.DocumentID, map[string]any{"action": "read"}); err != nil {
		t.Fatalf("closing view killed live document: %v", err)
	}
}
func TestFilePreviewBytesAndPages(t *testing.T) {
	root := t.TempDir()
	m := New(Config{Dir: t.TempDir()})
	t.Cleanup(m.Close)
	guard := func() error { return nil }
	file := filepath.Join(root, "note.docx")
	if err := os.WriteFile(file, []byte("hello-docx"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := m.Start(context.Background(), file, root, guard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(preview.URL, "/docs.html") || preview.Token == "" || preview.DocumentID == "" {
		t.Fatalf("docs page was not opened: url=%s", preview.URL)
	}
	base := strings.TrimSuffix(preview.URL, "/docs.html")
	get := func(path, token, origin string) (int, []byte) {
		req, _ := http.NewRequest("GET", base+path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res.StatusCode, body
	}
	if code, body := get("/meta?id="+preview.DocumentID, "wrong", base); code != 401 || strings.Contains(string(body), preview.Token) {
		t.Fatalf("meta accepted a bad token: %d %s", code, body)
	}
	code, body := get("/meta?id="+preview.DocumentID, preview.Token, base)
	if code != 200 || !strings.Contains(string(body), "note.docx") || strings.Contains(string(body), preview.Token) {
		t.Fatalf("meta: %d %s", code, body)
	}
	code, body = get("/bytes?id="+preview.DocumentID, preview.Token, base)
	if code != 200 || string(body) != "hello-docx" {
		t.Fatalf("bytes: %d %q", code, body)
	}
	req, _ := http.NewRequest("POST", base+"/bytes?id="+preview.DocumentID, strings.NewReader("saved-docx"))
	req.Header.Set("Authorization", "Bearer "+preview.Token)
	req.Header.Set("Origin", base)
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != "saved-docx" {
		t.Fatalf("save did not land: %q %v", got, err)
	}
	if code, _ := get("/bytes?id="+preview.DocumentID, preview.Token, "https://example.com"); code != 403 {
		t.Fatalf("foreign origin read: %d", code)
	}
	sheet := filepath.Join(root, "book.xlsx")
	if err = os.WriteFile(sheet, []byte("sheet"), 0o600); err != nil {
		t.Fatal(err)
	}
	openedSheet, err := m.Start(context.Background(), sheet, root, guard)
	if err != nil || !strings.HasSuffix(openedSheet.URL, "/sheets.html") {
		t.Fatalf("xlsx page was not opened: %v url=%s", err, openedSheet.URL)
	}
	for _, item := range []struct{ name, page string }{{"a.pdf", "/pdf.html"}, {"a.html", "/html.html"}, {"a.md", "/markdown.html"}} {
		target := filepath.Join(root, item.name)
		if err = os.WriteFile(target, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		opened, e := m.Start(context.Background(), target, root, guard)
		if e != nil || !strings.HasSuffix(opened.URL, item.page) {
			t.Fatalf("%s: %v url=%s", item.name, e, opened.URL)
		}
	}
}
func docxWithText(text string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + html.EscapeString(text) + `</w:t></w:r></w:p></w:body></w:document>`,
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			panic(err)
		}
		if _, err = w.Write([]byte(body)); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
func pdfWithText(text string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	add := func(body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets)-1, body)
	}
	add("<< /Type /Catalog /Pages 2 0 R >>")
	add("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	add("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 400 200] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")
	stream := "BT /F1 24 Tf 40 100 Td (" + text + ") Tj ET"
	add(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	start := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, off := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), start)
	return b.Bytes()
}
func xlsxWithText(text string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	escaped := html.EscapeString(text)
	files := map[string]string{
		"[Content_Types].xml":        `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`,
		"_rels/.rels":                `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`,
		"xl/styles.xml":              `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts><fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs></styleSheet>`,
		"xl/worksheets/sheet1.xml":   `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>` + escaped + `</t></is></c></row></sheetData></worksheet>`,
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			panic(err)
		}
		if _, err = w.Write([]byte(body)); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
func TestSheetsReadsCell(t *testing.T) {
	root := t.TempDir()
	m := New(Config{Dir: t.TempDir()})
	t.Cleanup(m.Close)
	file := filepath.Join(root, "book.xlsx")
	if err := os.WriteFile(file, xlsxWithText("官方表格已打开"), 0o600); err != nil {
		t.Fatal(err)
	}
	opened, err := m.Start(context.Background(), file, root, func() error { return nil })
	if err != nil || !strings.HasSuffix(opened.URL, "/sheets.html") {
		t.Fatalf("xlsx page was not opened: %v url=%s", err, opened.URL)
	}
	base := strings.TrimSuffix(opened.URL, "/sheets.html")
	post := func(body string) (int, []byte) {
		req, e := http.NewRequest("POST", base+"/sheets?id="+opened.DocumentID, strings.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+opened.Token)
		req.Header.Set("Origin", base)
		req.Header.Set("Content-Type", "application/json")
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		return res.StatusCode, raw
	}
	code, raw := post(`{"command":"open"}`)
	if code != 200 || strings.Contains(string(raw), opened.Token) {
		t.Fatalf("workbook open: %d %s", code, raw)
	}
	var workbook struct {
		SessionID string `json:"sessionId"`
		Sheets    []struct {
			ID string `json:"id"`
		} `json:"sheets"`
	}
	if json.Unmarshal(raw, &workbook) != nil || workbook.SessionID == "" || len(workbook.Sheets) == 0 || workbook.Sheets[0].ID == "" {
		t.Fatalf("workbook did not open: %s", raw)
	}
	var saw string
	deadline := time.Now().Add(20 * time.Second)
	for {
		payload, e := json.Marshal(map[string]any{"command": "read_range", "sheetId": workbook.Sheets[0].ID, "range": map[string]int{"startRow": 0, "endRow": 0, "startColumn": 0, "endColumn": 0}})
		if e != nil {
			t.Fatal(e)
		}
		code, raw = post(string(payload))
		saw = string(raw)
		if code == 200 && strings.Contains(saw, "官方表格已打开") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("workbook cell was not read: %d %s", code, saw)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if strings.Contains(saw, opened.Token) {
		t.Fatal("cell response included the preview secret")
	}
}
func TestOfficialPagesOpenFiles(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip(err)
	}
	root := t.TempDir()
	m := New(Config{Dir: t.TempDir()})
	t.Cleanup(m.Close)
	guard := func() error { return nil }
	type sample struct {
		name, file, page, expect string
		body                     []byte
	}
	samples := []sample{
		{"docs", "note.docx", "/docs.html", "官方文档已打开", docxWithText("官方文档已打开")},
		{"pdf", "note.pdf", "/pdf.html", "OfficialPdf", pdfWithText("OfficialPdf")},
		{"html", "page.html", "/html.html", "官方网页已打开", []byte("<!doctype html><h1>官方网页已打开</h1>")},
		{"markdown", "note.md", "/markdown.html", "官方笔记已打开", []byte("官方笔记已打开\n")},
		{"sheets", "book.xlsx", "/sheets.html", "官方表格已打开", xlsxWithText("官方表格已打开")},
	}
	if only := os.Getenv("FFC_OFFICE_QA_ONLY"); only != "" {
		filtered := make([]sample, 0, 1)
		for _, item := range samples {
			if item.name == only {
				filtered = append(filtered, item)
			}
		}
		samples = filtered
	}
	pages := make([]map[string]string, 0, len(samples))
	for _, item := range samples {
		target := filepath.Join(root, item.file)
		if err := os.WriteFile(target, item.body, 0o600); err != nil {
			t.Fatal(err)
		}
		opened, err := m.Start(context.Background(), target, root, guard)
		if err != nil || !strings.HasSuffix(opened.URL, item.page) || opened.Token == "" {
			t.Fatalf("%s page was not opened: %v url=%s", item.name, err, opened.URL)
		}
		pages = append(pages, map[string]string{"name": item.name, "url": opened.URL, "id": opened.DocumentID, "token": opened.Token, "expect": item.expect})
	}
	raw, err := json.Marshal(pages)
	if err != nil {
		t.Fatal(err)
	}
	css, err := os.ReadFile("runtime/docs.css")
	if err != nil {
		t.Fatal(err)
	}
	at := bytes.Index(css, []byte("/fonts/"))
	end := -1
	if at >= 0 {
		end = bytes.IndexByte(css[at:], ')')
	}
	if at < 0 || end <= len("/fonts/") {
		t.Fatal("document stylesheet has no shared font")
	}
	base := pages[0]["url"]
	base = base[:strings.LastIndex(base, "/")]
	fontResp, err := http.Get(base + string(css[at:at+end]))
	if err != nil {
		t.Fatal(err)
	}
	fontBody, _ := io.ReadAll(io.LimitReader(fontResp.Body, 32))
	fontResp.Body.Close()
	if fontResp.StatusCode != 200 || len(fontBody) == 0 {
		t.Fatalf("shared font was not served: %d", fontResp.StatusCode)
	}
	script, err := filepath.Abs("../../office/tests/open-pages.cjs")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", script)
	cmd.Env = append(os.Environ(), "FFC_OFFICE_QA_PAGES="+string(raw))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("official page did not open the file: %v\n%s", err, output)
	}
}
func TestEmbeddedCanvasBrowser(t *testing.T) {
	if os.Getenv("FFC_GENOFFICE_BROWSER_QA") != "1" {
		t.Skip("opt-in browser integration")
	}
	t.Setenv("FFC_OFFICE_PAINT_MS", "0")
	root := t.TempDir()
	m := New(Config{Dir: t.TempDir()})
	defer m.Close()
	ctx := context.Background()
	file := filepath.Join(root, "live.pptx")
	guard := func() error { return nil }
	if _, err := m.Open(ctx, file, root, true, guard); err != nil {
		t.Fatal(err)
	}
	preview, err := m.Start(ctx, file, root, guard)
	if err != nil {
		t.Fatal(err)
	}
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		step := r.URL.Query().Get("step")
		var deck struct {
			Revision int `json:"revision"`
			Slides   []struct {
				Elements []struct {
					ID   string `json:"id"`
					Text string `json:"text"`
				} `json:"elements"`
			} `json:"slides"`
		}
		raw, e := m.Call(ctx, preview.DocumentID, map[string]any{"action": "read"})
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		_ = json.Unmarshal(raw, &deck)
		args := map[string]any{"action": "page", "expectedRevision": deck.Revision, "requestId": "browser-" + step, "page": fixturePage("ffc · 第" + step + "页")}
		if step == "edit" || step == "conflict" {
			title := "ffc · 标题实时更新"
			if step == "conflict" {
				title = "ffc · Agent 已写入新版本"
			}
			args = map[string]any{"action": "apply", "expectedRevision": deck.Revision, "requestId": "browser-" + step, "ops": []any{map[string]any{"op": "setText", "target": map[string]any{"slide": 0, "el": deck.Slides[0].Elements[1].ID}, "paragraphs": []any{map[string]any{"srcPara": 0, "runs": []any{map[string]any{"text": title, "srcPara": 0, "srcRun": 0}}}}}}}
		}
		result, e := m.Call(ctx, preview.DocumentID, args)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		_, _ = w.Write(result)
	}))
	defer control.Close()
	previewJSON, _ := json.Marshal(preview)
	script, err := filepath.Abs("../../office/tests/browser.cjs")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", script)
	cmd.Env = append(os.Environ(), "FFC_OFFICE_QA_PREVIEW="+string(previewJSON), "FFC_OFFICE_QA_CONTROL="+control.URL)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("canvas browser QA: %v\n%s", err, output)
	}
	t.Log(strings.TrimSpace(string(output)))
}
func TestRPCRejectsCapabilityOverride(t *testing.T) {
	if _, err := nodePath(t.TempDir(), false); err != nil {
		t.Skip(err)
	}
	m := New(Config{Dir: t.TempDir()})
	defer m.Close()
	root := t.TempDir()
	ctx := context.Background()
	allowed := true
	guard := func() error {
		if !allowed {
			return fmt.Errorf("authorization revoked")
		}
		return nil
	}
	s, err := m.Open(ctx, filepath.Join(root, "deck.pptx"), root, true, guard)
	if err != nil {
		t.Fatal(err)
	}
	id := s["documentId"].(string)
	allowed = false
	if _, err = m.Call(ctx, id, map[string]any{"action": "read", "root": t.TempDir()}); err == nil {
		t.Fatal("revoked capability accepted")
	}
}
