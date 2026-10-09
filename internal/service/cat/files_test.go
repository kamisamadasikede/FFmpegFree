package cat_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/cat"
)

func TestListCatFilesProjectTree(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sub := filepath.Join(root, "Src")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "A.txt"), []byte("xy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "inside.txt"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".git", "node_modules", "target", ".DS_Store", "Thumbs.db", ".env"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{DataRoot: t.TempDir()})
	p, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: conv.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Root != root || got.Truncated {
		t.Fatalf("%+v", got)
	}
	dirs, files := splitDir(got.Entries)
	if len(files) != 2 || files[0].Name != "A.txt" || files[0].Size != 2 || files[1].Name != "b.txt" || files[1].Size != 5 {
		t.Fatalf("files %+v", got.Entries)
	}
	if len(dirs) != 2 || dirs[0].Name != ".env" || dirs[1].Name != "Src" || !dirs[1].IsDir || dirs[1].Size != 0 {
		t.Fatalf("dirs %+v", dirs)
	}
	for _, n := range namesOf(got.Entries) {
		switch n {
		case ".git", "node_modules", "target", ".DS_Store", "Thumbs.db", "inside.txt":
			t.Fatalf("unexpected %s in %v", n, got.Entries)
		}
	}
	nested, err := svc.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: conv.ID, RelPath: `Src`})
	if err != nil {
		t.Fatal(err)
	}
	if len(nested.Entries) != 1 || nested.Entries[0].RelPath != "Src/inside.txt" || nested.Entries[0].Name != "inside.txt" {
		t.Fatalf("%+v", nested.Entries)
	}
	if _, err := svc.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: conv.ID, RelPath: `Src/inside.txt`}); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("file is not a folder: %v", err)
	}
	if _, err := svc.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: conv.ID, RelPath: `no-such`}); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("missing: %v", err)
	}
	for _, rel := range []string{`..`, `Src/../../etc`, `C:\Windows`, `/etc`, `\\server\share`} {
		_, err := svc.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: conv.ID, RelPath: rel})
		if !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Detail != "reason=path" {
			t.Fatalf("rel %q: %v", rel, err)
		}
	}
	if _, err := svc.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: "missing"}); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
}

func TestListCatFilesTruncatesAndSymlink(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 501; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("f%04d.txt", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	p, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: conv.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.Entries) != 500 || !got.Entries[0].IsDir || got.Entries[0].Name != "adir" {
		t.Fatalf("truncated=%v n=%d first=%+v", got.Truncated, len(got.Entries), got.Entries[0])
	}

	outside := t.TempDir()
	small := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(small, "jump")); err != nil {
		t.Logf("symlink unsupported, skip link assertions: %v", err)
		return
	}
	if err := os.WriteFile(filepath.Join(small, "keep.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	p2, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: small})
	if err != nil {
		t.Fatal(err)
	}
	c2, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p2.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := svc.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: c2.ID})
	if err != nil {
		t.Fatal(err)
	}
	var sawLink bool
	for _, e := range listed.Entries {
		if e.Name == "jump" {
			sawLink = true
			if e.IsDir || e.Size != 0 {
				t.Fatalf("symlink must be a non-dir size 0: %+v", e)
			}
		}
	}
	if !sawLink {
		t.Fatalf("missing symlink entry: %+v", listed.Entries)
	}
	_, err = svc.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: c2.ID, RelPath: "jump"})
	if !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Detail != "reason=path" {
		t.Fatalf("follow symlink: %v", err)
	}
}

func TestPlainConvDirCreateListDeleteAndLegacy(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	var opened []string
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{
		DataRoot: data,
		OpenFolder: func(dir string) error {
			opened = append(opened, dir)
			return nil
		},
	})
	conv, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, Title: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	neu := paths.CatConvDir(data, conv.ID)
	if neu == "" {
		t.Fatal("empty conv dir")
	}
	if fi, err := os.Stat(neu); err != nil || !fi.IsDir() {
		t.Fatalf("create should mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(neu, "note.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: conv.ID})
	if err != nil || len(got.Entries) != 1 || got.Entries[0].Name != "note.txt" || got.Root != neu {
		t.Fatalf("%+v %v", got, err)
	}
	if err := svc.RevealCatConversationFolder(ctx, cat.RevealConversationFolderRequest{ConvID: conv.ID}); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != neu {
		t.Fatalf("opened %v", opened)
	}

	// 旧 cwd 已存在时列表和打开都用旧目录，不搬。
	old := paths.LegacyCatCwd(data, conv.ID)
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "old.txt"), []byte("o"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = svc.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: conv.ID})
	if err != nil || got.Root != old || len(got.Entries) != 1 || got.Entries[0].Name != "old.txt" {
		t.Fatalf("legacy %+v %v", got, err)
	}

	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, "keep.txt"), []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: proj})
	if err != nil {
		t.Fatal(err)
	}
	in, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.CatConvDir(data, in.ID)); !os.IsNotExist(err) {
		t.Fatalf("project conv must not get an app conv dir: %v", err)
	}
	opened = nil
	if err := svc.RevealCatConversationFolder(ctx, cat.RevealConversationFolderRequest{ConvID: in.ID}); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != proj {
		t.Fatalf("project reveal %v", opened)
	}
	if err := svc.DeleteCatConversation(ctx, in.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(proj, "keep.txt")); err != nil {
		t.Fatal("project file removed")
	}

	if err := svc.DeleteCatConversation(ctx, conv.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(neu); !os.IsNotExist(err) {
		t.Fatalf("new conv dir still there: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("legacy conv dir still there: %v", err)
	}
	if _, err := os.Stat(proj); err != nil {
		t.Fatal(err)
	}
}

func TestListCatFilesProjectMissing(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{DataRoot: t.TempDir()})
	p, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: conv.ID})
	if !apperr.Is(err, apperr.CatProjectMissing) {
		t.Fatal(err)
	}
	err = svc.RevealCatConversationFolder(ctx, cat.RevealConversationFolderRequest{ConvID: conv.ID})
	if !apperr.Is(err, apperr.CatProjectMissing) {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("reveal must not recreate the project folder")
	}
}

func namesOf(es []cat.FileEntry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Name
	}
	return out
}

func splitDir(es []cat.FileEntry) (dirs, files []cat.FileEntry) {
	for _, e := range es {
		if e.IsDir {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	return dirs, files
}
