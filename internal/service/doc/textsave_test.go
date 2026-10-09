package doc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doccomp"
)

func TestSaveDocTextRoundTripAndConflict(t *testing.T) {
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	dir := filepath.Join(e.dir, "src")
	os.MkdirAll(dir, 0o755)
	gb, _ := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("标题\r\n内容"))
	p := filepath.Join(dir, "a.txt")
	os.WriteFile(p, gb, 0o644)
	res := e.add(t, p)
	if res[0].Error != nil || res[0].Source == nil {
		t.Fatalf("%+v", res[0])
	}
	sid := res[0].Source.SourceID
	prev, err := e.svc.GetDocPreview(context.Background(), DocPreviewRequest{SourceID: sid})
	if err != nil {
		t.Fatal(err)
	}
	if !prev.Editable || prev.Encoding != encGBK || prev.LineEnding != lineCRLF {
		t.Fatalf("%+v", prev)
	}
	text := "标题\n改过了"
	got, err := e.svc.SaveDocText(context.Background(), DocSaveRequest{SourceID: sid, Revision: prev.Revision, Text: &text})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	back, _ := simplifiedchinese.GBK.NewDecoder().Bytes(raw)
	if !strings.Contains(string(back), "改过了") || !strings.Contains(string(raw), "\r\n") {
		t.Fatalf("bytes=%q decoded=%q", raw, back)
	}
	// revision 冲突
	_, err = e.svc.SaveDocText(context.Background(), DocSaveRequest{SourceID: sid, Revision: prev.Revision, Text: &text})
	if err == nil || !apperr.Is(err, apperr.TaskConflict) || !strings.Contains(apperr.From(err).Detail, "file_changed") {
		t.Fatalf("want file_changed: %v", err)
	}
	// emoji 拒绝
	bad := "有😀"
	_, err = e.svc.SaveDocText(context.Background(), DocSaveRequest{SourceID: sid, Revision: got.Revision, Text: &bad})
	if err == nil || !strings.Contains(apperr.From(err).Detail, "reason=encoding") {
		t.Fatalf("want encoding: %v", err)
	}
	raw2, _ := os.ReadFile(p)
	if string(raw2) != string(raw) {
		t.Fatal("file changed after refuse")
	}
	// 另存为 utf8
	target := filepath.Join(dir, "b.txt")
	out, err := e.svc.SaveDocTextAs(context.Background(), DocSaveAsRequest{SourceID: sid, TargetPath: target, Text: &bad, Encoding: "utf8"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out.Path)
	if !strings.Contains(string(b), "😀") {
		t.Fatalf("%q", b)
	}
}
