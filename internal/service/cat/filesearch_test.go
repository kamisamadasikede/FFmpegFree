package cat_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/cat"
)

func TestSearchCatFilesNameAndContent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "readme.md"), []byte("# Hello Cat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\nfunc Hello() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "x.js"), []byte("func Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte("Hello\x00nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, id := convAt(t, root)

	empty, err := svc.SearchCatFiles(ctx, cat.SearchFilesRequest{ConvID: id, Query: "  ", Kind: "name"})
	if err != nil || len(empty.Hits) != 0 {
		t.Fatalf("empty: %+v %v", empty, err)
	}
	names, err := svc.SearchCatFiles(ctx, cat.SearchFilesRequest{ConvID: id, Query: "Main", Kind: "name"})
	if err != nil {
		t.Fatal(err)
	}
	if len(names.Hits) != 1 || names.Hits[0].RelPath != "src/main.go" || names.Hits[0].IsDir {
		t.Fatalf("name: %+v", names.Hits)
	}
	dirs, err := svc.SearchCatFiles(ctx, cat.SearchFilesRequest{ConvID: id, Query: "src", Kind: "name"})
	if err != nil {
		t.Fatal(err)
	}
	sawDir := false
	for _, h := range dirs.Hits {
		if h.RelPath == "src" && h.IsDir {
			sawDir = true
		}
		if h.RelPath == "node_modules" {
			t.Fatal("node_modules leaked")
		}
	}
	if !sawDir {
		t.Fatalf("dir: %+v", dirs.Hits)
	}
	body, err := svc.SearchCatFiles(ctx, cat.SearchFilesRequest{ConvID: id, Query: "Hello", Kind: "content"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, h := range body.Hits {
		got[h.RelPath] = h.Line
		if h.RelPath == "node_modules/x.js" || h.RelPath == "bin.dat" {
			t.Fatalf("should skip: %+v", h)
		}
	}
	if got["readme.md"] != 1 || got["src/main.go"] != 2 {
		t.Fatalf("content: %+v", body.Hits)
	}
	if _, err := svc.SearchCatFiles(ctx, cat.SearchFilesRequest{ConvID: id, Query: "x", Kind: "nope"}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("kind: %v", err)
	}
}
