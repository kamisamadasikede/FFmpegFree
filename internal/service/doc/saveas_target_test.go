package doc

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doccomp"
	"FFmpegFree/internal/store"
)

func wantSaveReason(t *testing.T, err error, code apperr.Code, reason string) {
	t.Helper()
	if err == nil || !apperr.Is(err, code) || !strings.HasPrefix(apperr.From(err).Detail, "reason="+reason) {
		t.Fatalf("want %s reason=%s, got %v", code, reason, err)
	}
}

// 另存为目标被别的记录用着：正在转换 → TASK_CONFLICT converting；源文件 / 输出 / 副本 → IO_ERROR in_use；都不写。
func TestSaveDocTextAsTargetUsedByRecord(t *testing.T) {
	ctx := context.Background()
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	dir := filepath.Join(e.dir, "src")
	os.MkdirAll(dir, 0o755)
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	os.WriteFile(a, []byte("甲"), 0o644)
	os.WriteFile(b, []byte("乙"), 0o644)
	res := e.add(t, a, b)
	sid := res[0].Source.SourceID
	text := "改过了"

	// 1) 目标是另一行的原文件 → in_use
	_, err := e.svc.SaveDocTextAs(ctx, DocSaveAsRequest{SourceID: sid, TargetPath: b, Text: &text})
	wantSaveReason(t, err, apperr.IOError, "in_use")
	if got, _ := os.ReadFile(b); string(got) != "乙" {
		t.Fatalf("目标被改了：%q", got)
	}

	// 2) 目标是一条已完成记录的输出 → in_use
	outDone := filepath.Join(dir, "done.txt")
	os.WriteFile(outDone, []byte("旧"), 0o644)
	e.st.InsertTask(ctx, store.Task{ID: "TDONE", Type: store.TypeDocConvert, Status: store.StatusSucceeded, InputPaths: []string{b}, OutputPath: outDone, CreatedAt: 1})
	_, err = e.svc.SaveDocTextAs(ctx, DocSaveAsRequest{SourceID: sid, TargetPath: outDone, Text: &text})
	wantSaveReason(t, err, apperr.IOError, "in_use")

	// 3) 目标是正在转换的记录的输出 → converting
	outRun := filepath.Join(dir, "running.txt")
	e.st.InsertTask(ctx, store.Task{ID: "TRUN", Type: store.TypeDocConvert, Status: store.StatusRunning, InputPaths: []string{b}, OutputPath: outRun, CreatedAt: 2})
	_, err = e.svc.SaveDocTextAs(ctx, DocSaveAsRequest{SourceID: sid, TargetPath: outRun, Text: &text})
	wantSaveReason(t, err, apperr.TaskConflict, "converting")
	if _, err := os.Stat(outRun); !os.IsNotExist(err) {
		t.Fatal("正在转换的目标不应被写")
	}

	// 4) 目标是正在转换的记录的输入 → converting
	inRun := filepath.Join(dir, "input.txt")
	os.WriteFile(inRun, []byte("入"), 0o644)
	e.st.InsertTask(ctx, store.Task{ID: "TIN", Type: store.TypeOfficePDF, Status: store.StatusQueued, InputPaths: []string{inRun}, OutputPath: filepath.Join(dir, "x.pdf"), CreatedAt: 3})
	_, err = e.svc.SaveDocTextAs(ctx, DocSaveAsRequest{SourceID: sid, TargetPath: inRun, Text: &text})
	wantSaveReason(t, err, apperr.TaskConflict, "converting")

	// 5) 没人用的位置照常保存
	free := filepath.Join(dir, "free.txt")
	if _, err := e.svc.SaveDocTextAs(ctx, DocSaveAsRequest{SourceID: sid, TargetPath: free, Text: &text}); err != nil {
		t.Fatal(err)
	}
}

func TestDocxSaveAsTargetUsedByRecord(t *testing.T) {
	ctx := context.Background()
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	dir := filepath.Join(e.dir, "src")
	os.MkdirAll(dir, 0o755)
	data := minimalDocx(t)
	a, b := filepath.Join(dir, "a.docx"), filepath.Join(dir, "b.docx")
	os.WriteFile(a, data, 0o644)
	os.WriteFile(b, data, 0o644)
	res := e.add(t, a, b)
	sid := res[0].Source.SourceID
	n := int64(len(data))

	// Begin：目标是另一行的原文件 → in_use
	_, err := e.svc.BeginDocBinarySave(ctx, DocBinarySaveBegin{SourceID: sid, Mode: "save_as", TargetPath: b, TotalBytes: n})
	wantSaveReason(t, err, apperr.IOError, "in_use")

	// Begin：目标是正在转换的输出 → converting
	outRun := filepath.Join(dir, "running.docx")
	e.st.InsertTask(ctx, store.Task{ID: "TRUN", Type: store.TypeDocConvert, Status: store.StatusRunning, InputPaths: []string{b}, OutputPath: outRun, CreatedAt: 2})
	_, err = e.svc.BeginDocBinarySave(ctx, DocBinarySaveBegin{SourceID: sid, Mode: "save_as", TargetPath: outRun, TotalBytes: n})
	wantSaveReason(t, err, apperr.TaskConflict, "converting")

	// Commit 前目标被一条正在转换的记录用上 → converting，不写
	late := filepath.Join(dir, "late.docx")
	sess, err := e.svc.BeginDocBinarySave(ctx, DocBinarySaveBegin{SourceID: sid, Mode: "save_as", TargetPath: late, TotalBytes: n})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AppendDocBinaryChunk(ctx, DocBinaryChunk{SaveID: sess.SaveID, Seq: 0, Data: base64.StdEncoding.EncodeToString(data)}); err != nil {
		t.Fatal(err)
	}
	e.st.InsertTask(ctx, store.Task{ID: "TLATE", Type: store.TypeDocConvert, Status: store.StatusQueued, InputPaths: []string{b}, OutputPath: late, CreatedAt: 3})
	sum := sha256.Sum256(data)
	_, err = e.svc.CommitDocBinarySave(ctx, DocBinarySaveCommit{SaveID: sess.SaveID, SHA256: hex.EncodeToString(sum[:])})
	wantSaveReason(t, err, apperr.TaskConflict, "converting")
	if _, err := os.Stat(late); !os.IsNotExist(err) {
		t.Fatal("被拒绝的另存为不应写入目标")
	}
}

// 另存为成功后不能把新内容写进原行的副本（副本应与原文件一致）。
func TestDocxSaveAsKeepsSourceCopy(t *testing.T) {
	ctx := context.Background()
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	dir := filepath.Join(e.dir, "src")
	os.MkdirAll(dir, 0o755)
	data := minimalDocx(t)
	a := filepath.Join(dir, "a.docx")
	os.WriteFile(a, data, 0o644)
	res := e.add(t, a)
	src := res[0].Source
	if src.CopyState != store.CopyReady {
		t.Skipf("副本未就绪：%s", src.CopyState)
	}
	full, err := e.st.GetConvertSource(ctx, src.SourceID)
	if err != nil || full.StoredPath == "" {
		t.Skip("没有副本路径")
	}
	before, _ := os.ReadFile(full.StoredPath)

	edited := minimalDocxWithText(t, "另存的新内容")
	n := int64(len(edited))
	sess, err := e.svc.BeginDocBinarySave(ctx, DocBinarySaveBegin{SourceID: src.SourceID, Mode: "save_as", TargetPath: filepath.Join(dir, "new.docx"), TotalBytes: n})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AppendDocBinaryChunk(ctx, DocBinaryChunk{SaveID: sess.SaveID, Seq: 0, Data: base64.StdEncoding.EncodeToString(edited)}); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(edited)
	if _, err := e.svc.CommitDocBinarySave(ctx, DocBinarySaveCommit{SaveID: sess.SaveID, SHA256: hex.EncodeToString(sum[:])}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(full.StoredPath)
	if string(after) != string(before) {
		t.Fatal("另存为不应改动原行的副本")
	}
}

func minimalDocxWithText(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`},
		{"_rels/.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`},
		{"word/document.xml", `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`},
	} {
		w, err := zw.Create(f.name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(f.body))
	}
	zw.Close()
	return buf.Bytes()
}
