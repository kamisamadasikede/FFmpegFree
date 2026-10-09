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
)

func minimalDocx(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`},
		{"_rels/.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`},
		{"word/document.xml", `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p/></w:body></w:document>`},
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

func TestDocxBinarySaveAndValidation(t *testing.T) {
	e := newDocEnv(t, &fakeComp{st: doccomp.Status{State: doccomp.StateMissing}})
	dir := filepath.Join(e.dir, "src")
	os.MkdirAll(dir, 0o755)
	data := minimalDocx(t)
	p := filepath.Join(dir, "a.docx")
	os.WriteFile(p, data, 0o644)
	res := e.add(t, p)
	if res[0].Source == nil {
		t.Fatal(res[0].Error)
	}
	sid := res[0].Source.SourceID
	prev, err := e.svc.GetDocPreview(context.Background(), DocPreviewRequest{SourceID: sid})
	if err != nil || !prev.Editable {
		t.Fatalf("%+v %v", prev, err)
	}
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	sess, err := e.svc.BeginDocBinarySave(context.Background(), DocBinarySaveBegin{
		SourceID: sid, Mode: "overwrite", Revision: prev.Revision, TotalBytes: int64(len(data)),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.AppendDocBinaryChunk(context.Background(), DocBinaryChunk{
		SaveID: sess.SaveID, Seq: 0, Data: base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		t.Fatal(err)
	}
	// chunk_order
	_, err = e.svc.AppendDocBinaryChunk(context.Background(), DocBinaryChunk{
		SaveID: sess.SaveID, Seq: 0, Data: base64.StdEncoding.EncodeToString([]byte("x")),
	})
	if err == nil || !strings.Contains(apperr.From(err).Detail, "chunk_order") {
		t.Fatalf("want chunk_order: %v", err)
	}
	out, err := e.svc.CommitDocBinarySave(context.Background(), DocBinarySaveCommit{SaveID: sess.SaveID, SHA256: hexSum})
	if err != nil {
		t.Fatal(err)
	}
	if out.BackupPath == "" || !strings.Contains(filepath.Base(out.BackupPath), ".bak-") {
		t.Fatalf("backup=%q", out.BackupPath)
	}
	if _, err := os.Stat(out.BackupPath); err != nil {
		t.Fatal(err)
	}
	// checksum fail ends session
	data2 := minimalDocx(t)
	os.WriteFile(p, data2, 0o644)
	prev2, _ := e.svc.GetDocPreview(context.Background(), DocPreviewRequest{SourceID: sid})
	sess2, err := e.svc.BeginDocBinarySave(context.Background(), DocBinarySaveBegin{
		SourceID: sid, Mode: "overwrite", Revision: prev2.Revision, TotalBytes: int64(len(data2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	e.svc.AppendDocBinaryChunk(context.Background(), DocBinaryChunk{
		SaveID: sess2.SaveID, Seq: 0, Data: base64.StdEncoding.EncodeToString(data2),
	})
	_, err = e.svc.CommitDocBinarySave(context.Background(), DocBinarySaveCommit{SaveID: sess2.SaveID, SHA256: "deadbeef"})
	if err == nil || !strings.Contains(apperr.From(err).Detail, "checksum") {
		t.Fatalf("want checksum: %v", err)
	}
	_, err = e.svc.AppendDocBinaryChunk(context.Background(), DocBinaryChunk{SaveID: sess2.SaveID, Seq: 0, Data: "YQ=="})
	if err == nil || !strings.Contains(apperr.From(err).Detail, "save_session") {
		t.Fatalf("session should end: %v", err)
	}
}

func TestBackupRetention(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "报告.docx")
	os.WriteFile(src, minimalDocx(t), 0o644)
	sum, _, _ := fileSHA256(src)
	var paths []string
	for i := 0; i < 5; i++ {
		bp, err := backupDocxOverwrite(src, sum)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, bp)
		// 改文件内容让下一份备份能建（revision 仍用同一文件）
		timeSleep()
	}
	pruneDocxBackups(src)
	ents, _ := os.ReadDir(dir)
	n := 0
	for _, e := range ents {
		if strings.Contains(e.Name(), ".bak-") {
			n++
		}
	}
	if n > maxDocxBackups+1 { // +1 原文件不算；只计 bak
		t.Fatalf("backups=%d", n)
	}
	_ = paths
}

func timeSleep() {
	// 备份文件名精确到秒；同秒会加 -2。这里不需要真睡，backupDocxOverwrite 处理同秒。
}
