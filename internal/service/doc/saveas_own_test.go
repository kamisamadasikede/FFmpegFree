package doc

import (
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
	"FFmpegFree/internal/task"
)

// 另存为的目标就是这一行自己的原文件：不拒绝，当作覆盖保存，写完刷新这一行的副本（6.12.42，架构师定）。
func TestSaveDocTextAsOwnSource(t *testing.T) {
	ctx := context.Background()
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	dir := filepath.Join(e.dir, "src")
	os.MkdirAll(dir, 0o755)
	a := filepath.Join(dir, "a.txt")
	os.WriteFile(a, []byte("甲"), 0o644)
	res := e.add(t, a)
	sid := res[0].Source.SourceID
	text := "改过了"

	got, err := e.svc.SaveDocTextAs(ctx, DocSaveAsRequest{SourceID: sid, TargetPath: a, Text: &text})
	if err != nil {
		t.Fatalf("另存为到自己的原文件应当成功：%v", err)
	}
	if b, _ := os.ReadFile(a); string(b) != text {
		t.Fatalf("原文件没写入：%q", b)
	}
	if got.Revision != sha256Hex([]byte(text)) {
		t.Fatalf("revision 不对：%+v", got)
	}
	// 副本也刷新了
	if full, err := e.st.GetConvertSource(ctx, sid); err == nil && full.CopyState == store.CopyReady && full.StoredPath != "" {
		if b, _ := os.ReadFile(full.StoredPath); string(b) != text {
			t.Fatalf("副本没刷新：%q", b)
		}
	}
	// 之后用新 revision 覆盖保存照常可用
	prev, err := e.svc.GetDocPreview(ctx, DocPreviewRequest{SourceID: sid})
	if err == nil && prev.Revision != "" && prev.Revision != got.Revision {
		t.Fatalf("预览 revision 没跟上：%s vs %s", prev.Revision, got.Revision)
	}

	// 自己这一行正在转换 → converting，不写
	if err := e.st.InsertTask(ctx, store.Task{ID: "TOWN", Type: store.TypeDocConvert, Status: store.StatusRunning, SourceID: sid,
		InputPaths: []string{a}, OutputPath: filepath.Join(dir, "a.pdf"), CreatedAt: 9}); err != nil {
		t.Fatal(err)
	}
	again := "又改了"
	_, err = e.svc.SaveDocTextAs(ctx, DocSaveAsRequest{SourceID: sid, TargetPath: a, Text: &again})
	wantSaveReason(t, err, apperr.TaskConflict, "converting")
	if b, _ := os.ReadFile(a); string(b) != text {
		t.Fatalf("正在转换时不应写：%q", b)
	}
}

// 结果行另存为到自己的输出：当作覆盖保存，刷新 result.sizeBytes；另存为到别的行的原文件仍是 in_use。
func TestSaveDocTextAsOwnTaskOutput(t *testing.T) {
	ctx := context.Background()
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	dir := filepath.Join(e.dir, "src")
	os.MkdirAll(dir, 0o755)
	md := filepath.Join(dir, "a.md")
	os.WriteFile(md, []byte("# 标题\n"), 0o644)
	other := filepath.Join(dir, "b.html")
	os.WriteFile(other, []byte("<p>乙</p>"), 0o644)
	res := e.add(t, md, other)
	done := e.wait(t, e.submit(t, "html", res[0].Source.SourceID)[0].ID)
	if done.Status != task.StatusSucceeded {
		t.Fatalf("%+v", done)
	}
	text := "<p>改过的输出</p>"
	if _, err := e.svc.SaveDocTextAs(ctx, DocSaveAsRequest{TaskID: done.ID, TargetPath: done.OutputPath, Text: &text}); err != nil {
		t.Fatalf("另存为到自己的输出应当成功：%v", err)
	}
	if b, _ := os.ReadFile(done.OutputPath); string(b) != text {
		t.Fatalf("输出没写入：%q", b)
	}
	if cur, err := e.tm.Get(done.ID); err != nil || cur.Result == nil || cur.Result.SizeBytes != int64(len(text)) {
		t.Fatalf("result.sizeBytes 没刷新：%+v %v", cur.Result, err)
	}
	// 结果行另存为到别的源文件行的原文件 → in_use
	_, err := e.svc.SaveDocTextAs(ctx, DocSaveAsRequest{TaskID: done.ID, TargetPath: other, Text: &text})
	wantSaveReason(t, err, apperr.IOError, "in_use")
	if b, _ := os.ReadFile(other); string(b) != "<p>乙</p>" {
		t.Fatalf("别的行的原文件被改了：%q", b)
	}
}

func docxSaveAs(t *testing.T, e *docEnv, sid, target string, data []byte) (DocBinarySaveResult, error) {
	t.Helper()
	ctx := context.Background()
	sess, err := e.svc.BeginDocBinarySave(ctx, DocBinarySaveBegin{SourceID: sid, Mode: "save_as", TargetPath: target, TotalBytes: int64(len(data))})
	if err != nil {
		return DocBinarySaveResult{}, err
	}
	if _, err := e.svc.AppendDocBinaryChunk(ctx, DocBinaryChunk{SaveID: sess.SaveID, Seq: 0, Data: base64.StdEncoding.EncodeToString(data)}); err != nil {
		return DocBinarySaveResult{}, err
	}
	sum := sha256.Sum256(data)
	return e.svc.CommitDocBinarySave(ctx, DocBinarySaveCommit{SaveID: sess.SaveID, SHA256: hex.EncodeToString(sum[:])})
}

// docx 另存为到自己的原文件：按覆盖保存，留备份、刷新副本；正在转换 → converting；别的行 → in_use。
func TestDocxSaveAsOwnSource(t *testing.T) {
	ctx := context.Background()
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	dir := filepath.Join(e.dir, "src")
	os.MkdirAll(dir, 0o755)
	orig := minimalDocx(t)
	a, b := filepath.Join(dir, "a.docx"), filepath.Join(dir, "b.docx")
	os.WriteFile(a, orig, 0o644)
	os.WriteFile(b, orig, 0o644)
	res := e.add(t, a, b)
	sid := res[0].Source.SourceID

	edited := minimalDocxWithText(t, "写回自己")
	got, err := docxSaveAs(t, e, sid, a, edited)
	if err != nil {
		t.Fatalf("另存为到自己的原文件应当成功：%v", err)
	}
	if cur, _ := os.ReadFile(a); string(cur) != string(edited) {
		t.Fatal("原文件没写入")
	}
	if got.BackupPath == "" || !strings.Contains(filepath.Base(got.BackupPath), ".bak-") {
		t.Fatalf("应当按覆盖规则留备份：%+v", got)
	}
	if bk, _ := os.ReadFile(got.BackupPath); string(bk) != string(orig) {
		t.Fatal("备份内容不是原来的文件")
	}
	if full, err := e.st.GetConvertSource(ctx, sid); err == nil && full.CopyState == store.CopyReady && full.StoredPath != "" {
		if cp, _ := os.ReadFile(full.StoredPath); string(cp) != string(edited) {
			t.Fatal("副本没刷新")
		}
	}

	// 别的行的原文件 → in_use（Begin 就拒）
	_, err = docxSaveAs(t, e, sid, b, edited)
	wantSaveReason(t, err, apperr.IOError, "in_use")

	// 自己这一行正在转换 → converting
	if err := e.st.InsertTask(ctx, store.Task{ID: "TOWN", Type: store.TypeDocConvert, Status: store.StatusQueued, SourceID: sid,
		InputPaths: []string{a}, OutputPath: filepath.Join(dir, "a.pdf"), CreatedAt: 9}); err != nil {
		t.Fatal(err)
	}
	_, err = docxSaveAs(t, e, sid, a, minimalDocxWithText(t, "不该写"))
	wantSaveReason(t, err, apperr.TaskConflict, "converting")
	if cur, _ := os.ReadFile(a); string(cur) != string(edited) {
		t.Fatal("正在转换时不应写")
	}
}

// Begin 之后自己这一行开始转换：Commit 再查一次 → converting，不写。
func TestDocxSaveAsOwnSourceConvertingAtCommit(t *testing.T) {
	ctx := context.Background()
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	dir := filepath.Join(e.dir, "src")
	os.MkdirAll(dir, 0o755)
	orig := minimalDocx(t)
	a := filepath.Join(dir, "a.docx")
	os.WriteFile(a, orig, 0o644)
	sid := e.add(t, a)[0].Source.SourceID
	edited := minimalDocxWithText(t, "晚了")
	sess, err := e.svc.BeginDocBinarySave(ctx, DocBinarySaveBegin{SourceID: sid, Mode: "save_as", TargetPath: a, TotalBytes: int64(len(edited))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AppendDocBinaryChunk(ctx, DocBinaryChunk{SaveID: sess.SaveID, Seq: 0, Data: base64.StdEncoding.EncodeToString(edited)}); err != nil {
		t.Fatal(err)
	}
	if err := e.st.InsertTask(ctx, store.Task{ID: "TLATE", Type: store.TypeDocConvert, Status: store.StatusRunning, SourceID: sid,
		InputPaths: []string{a}, OutputPath: filepath.Join(dir, "a.pdf"), CreatedAt: 9}); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(edited)
	_, err = e.svc.CommitDocBinarySave(ctx, DocBinarySaveCommit{SaveID: sess.SaveID, SHA256: hex.EncodeToString(sum[:])})
	wantSaveReason(t, err, apperr.TaskConflict, "converting")
	if cur, _ := os.ReadFile(a); string(cur) != string(orig) {
		t.Fatal("被拒绝时不应写")
	}
}
