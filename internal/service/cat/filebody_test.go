package cat_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/cat"
)

func TestReadWriteCatFileText(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sub := filepath.Join(root, "Src")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, conv := catProjConv(t, root)

	got, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: `Src\main.go`})
	if err != nil {
		t.Fatal(err)
	}
	if got.RelPath != "Src/main.go" || got.Kind != "text" || got.Content != "package main\n" || got.Language != "go" || !got.Editable || got.DataBase64 != "" {
		t.Fatalf("%+v", got)
	}
	md, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: "README.md"})
	if err != nil || md.Language != "markdown" || !md.Editable || md.Mime != "text/plain" {
		t.Fatalf("%+v %v", md, err)
	}
	saved, err := svc.WriteCatFile(ctx, cat.WriteFileRequest{ConvID: conv, RelPath: "note.txt", Content: "改过\n"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.RelPath != "note.txt" || saved.Size != int64(len("改过\n")) {
		t.Fatalf("%+v", saved)
	}
	again, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: "note.txt"})
	if err != nil || again.Content != "改过\n" {
		t.Fatalf("%+v %v", again, err)
	}
	if _, err := svc.WriteCatFile(ctx, cat.WriteFileRequest{ConvID: conv, RelPath: "note.txt", Content: "a\x00b"}); !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Message != "只能保存文本。" {
		t.Fatalf("nul: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil || string(body) != "改过\n" {
		t.Fatalf("nul must not write: %q %v", body, err)
	}
	big := strings.Repeat("a", 256*1024+1)
	if _, err := svc.WriteCatFile(ctx, cat.WriteFileRequest{ConvID: conv, RelPath: "note.txt", Content: big}); !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Message != "这个文件太大，不能在这里修改。" {
		t.Fatalf("big write: %v", err)
	}
	if _, err := svc.WriteCatFile(ctx, cat.WriteFileRequest{ConvID: conv, RelPath: "missing.txt", Content: "x"}); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "missing.txt")); !os.IsNotExist(err) {
		t.Fatal("write must not create")
	}
	if _, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: "Src"}); !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Message != "这是一个文件夹。" {
		t.Fatalf("dir: %v", err)
	}
	for _, rel := range []string{"", "..", `Src/../../etc`, `C:\Windows`, "/etc", `\\server\share`} {
		_, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: rel})
		if rel == "" {
			if !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Message != "这是一个文件夹。" {
				t.Fatalf("empty: %v", err)
			}
			continue
		}
		if !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Detail != "reason=path" {
			t.Fatalf("rel %q: %v", rel, err)
		}
	}
}

func TestReadCatFileKindsAndLimits(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a}
	if err := os.WriteFile(filepath.Join(root, "a.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "noise.txt"), bytes.Repeat([]byte{0x01}, 80), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "old.doc"), []byte("not really"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "big.txt"), bytes.Repeat([]byte{'a'}, 256*1024+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "huge.pdf"), bytes.Repeat([]byte{'%'}, 8*1024*1024+1), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, conv := catProjConv(t, root)

	img, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	if img.Kind != "image" || img.Editable || img.Content != "" || img.Mime != "image/png" || img.DataBase64 != base64.StdEncoding.EncodeToString(png) {
		t.Fatalf("%+v", img)
	}
	if _, err := svc.WriteCatFile(ctx, cat.WriteFileRequest{ConvID: conv, RelPath: "a.png", Content: "nope"}); !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Message != "这个文件不能在这里修改。" {
		t.Fatalf("png write: %v", err)
	}
	if rest, err := os.ReadFile(filepath.Join(root, "a.png")); err != nil || !bytes.Equal(rest, png) {
		t.Fatalf("png changed: %q %v", rest, err)
	}
	bin, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: "bin.dat"})
	if err != nil || bin.Kind != "binary" || bin.Content != "" || bin.DataBase64 != "" || bin.Editable {
		t.Fatalf("%+v %v", bin, err)
	}
	noise, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: "noise.txt"})
	if err != nil || noise.Kind != "binary" || noise.Editable {
		t.Fatalf("%+v %v", noise, err)
	}
	doc, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: "old.doc"})
	if err != nil || doc.Kind != "doc" || doc.Content != "" || doc.DataBase64 != "" || doc.Editable {
		t.Fatalf("%+v %v", doc, err)
	}
	big, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: "big.txt"})
	if err != nil || big.Kind != "tooLarge" || big.Content != "" || big.Editable || big.Size != int64(256*1024+1) {
		t.Fatalf("%+v %v", big, err)
	}
	if _, err := svc.WriteCatFile(ctx, cat.WriteFileRequest{ConvID: conv, RelPath: "big.txt", Content: "x"}); !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Message != "这个文件太大，不能在这里修改。" {
		t.Fatalf("big existing: %v", err)
	}
	pdf, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: "huge.pdf"})
	if err != nil || pdf.Kind != "tooLarge" || pdf.DataBase64 != "" || pdf.Size != int64(8*1024*1024+1) {
		t.Fatalf("%+v %v", pdf, err)
	}
}

func TestReadWriteCatFileSymlink(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "jump.txt")); err != nil {
		t.Logf("symlink unsupported, skip link assertions: %v", err)
		return
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "sub", "out")); err != nil {
		t.Fatal(err)
	}
	svc, conv := catProjConv(t, root)
	_, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: "jump.txt"})
	if !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Detail != "reason=path" {
		t.Fatalf("read link: %v", err)
	}
	_, err = svc.WriteCatFile(ctx, cat.WriteFileRequest{ConvID: conv, RelPath: "jump.txt", Content: "pwn"})
	if !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Detail != "reason=path" {
		t.Fatalf("write link: %v", err)
	}
	secret, err := os.ReadFile(filepath.Join(outside, "secret.txt"))
	if err != nil || string(secret) != "nope" {
		t.Fatalf("target changed: %q %v", secret, err)
	}
	_, err = svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: "sub/out/secret.txt"})
	if !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Detail != "reason=path" {
		t.Fatalf("read through dir link: %v", err)
	}
}

func TestReadWriteCatFileProjectMissing(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	svc, conv := catProjConv(t, dir)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	_, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv, RelPath: "a.txt"})
	if !apperr.Is(err, apperr.CatProjectMissing) {
		t.Fatal(err)
	}
	_, err = svc.WriteCatFile(ctx, cat.WriteFileRequest{ConvID: conv, RelPath: "a.txt", Content: "x"})
	if !apperr.Is(err, apperr.CatProjectMissing) {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("read and write must not recreate the project folder")
	}
}

func TestReadWritePlainConvFile(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{DataRoot: data})
	conv, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, Title: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	dir := paths.CatConvDir(data, conv.ID)
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: conv.ID, RelPath: "note.txt"})
	if err != nil || got.Content != "hi" || !got.Editable {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := svc.WriteCatFile(ctx, cat.WriteFileRequest{ConvID: conv.ID, RelPath: "note.txt", Content: "yo"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "note.txt"))
	if err != nil || string(body) != "yo" {
		t.Fatalf("%q %v", body, err)
	}
	if _, err := svc.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: "missing", RelPath: "note.txt"}); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
}

func catProjConv(t *testing.T, root string) (*cat.Service, string) {
	t.Helper()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{DataRoot: t.TempDir()})
	p, err := svc.CreateCatProject(context.Background(), cat.CreateProjectRequest{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.CreateCatConversation(context.Background(), cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	return svc, conv.ID
}
