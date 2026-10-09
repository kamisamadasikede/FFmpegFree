package doc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewTextTruncateUTF8(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	// 写超过 2 MiB 的合法 UTF-8（中文）
	var b strings.Builder
	for b.Len() < previewTextMax+100 {
		b.WriteString("你好世界")
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	out, err := s.previewText(DocPreview{PreviewID: "x", Name: "a.txt", Ext: "txt", SizeBytes: int64(b.Len()), State: "ready"}, p, "text")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Truncated || len(out.Text) > previewTextMax {
		t.Fatalf("truncated=%v len=%d", out.Truncated, len(out.Text))
	}
	if out.EditBlock != "too_large" || out.Editable {
		t.Fatalf("editBlock=%q editable=%v", out.EditBlock, out.Editable)
	}
	if out.Revision == "" || out.Encoding == "" {
		t.Fatalf("missing revision/encoding")
	}
}

func TestPreviewCSVLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.csv")
	var b strings.Builder
	for i := 0; i < 1200; i++ {
		b.WriteString("a,b,c\n")
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	out, err := s.previewCSV(DocPreview{PreviewID: "x", Name: "a.csv", Ext: "csv", State: "ready"}, p)
	if err != nil {
		t.Fatal(err)
	}
	if out.Kind != "csv" || len(out.Rows) != previewCSVRows {
		t.Fatalf("kind=%s rows=%d", out.Kind, len(out.Rows))
	}
	if out.TotalRows < previewCSVRows {
		t.Fatalf("totalRows=%d", out.TotalRows)
	}
	if out.Editable || out.EditBlock != "too_large" {
		t.Fatalf("editable=%v block=%q", out.Editable, out.EditBlock)
	}
}

func TestPreviewRawTooLarge(t *testing.T) {
	s := &Service{}
	out, err := s.previewNoEngine(DocPreview{PreviewID: "x", Ext: "docx", SizeBytes: previewRawMax + 1, State: "ready"}, "/nope.docx", "docx", previewRawMax+1)
	if err != nil {
		t.Fatal(err)
	}
	if out.Kind != "unavailable" || out.Reason != "too_large_for_simple" {
		t.Fatalf("%+v", out)
	}
}

func TestCacheKeyStable(t *testing.T) {
	a := cacheKey("/a", 1, 2, "office")
	b := cacheKey("/a", 1, 2, "office")
	c := cacheKey("/a", 1, 2, "wps")
	if a != b || a == c || len(a) != 64 {
		t.Fatalf("a=%s b=%s c=%s", a, b, c)
	}
}
