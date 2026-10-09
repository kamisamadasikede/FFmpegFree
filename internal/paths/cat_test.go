package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCatConvDirPrefersLegacyRealDir(t *testing.T) {
	root := t.TempDir()
	id := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	neu := CatConvDir(root, id)
	old := LegacyCatCwd(root, id)
	if neu == "" || old == "" {
		t.Fatal("empty path")
	}
	if filepath.Base(neu) != id || filepath.Base(old) != "cwd-"+id {
		t.Fatalf("neu %s old %s", neu, old)
	}
	if got := ResolveCatConvDir(root, id); got != neu {
		t.Fatalf("no legacy: %s", got)
	}
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ResolveCatConvDir(root, id); got != old {
		t.Fatalf("legacy wins: %s", got)
	}
	if !IsCatConvDir(root, id, neu) || !IsCatConvDir(root, id, old) {
		t.Fatal("own dirs")
	}
	if IsCatConvDir(root, id, root) || IsCatConvDir(root, id, filepath.Dir(neu)) {
		t.Fatal("parent is not the conv dir")
	}
	if CatConvDir("", id) != "" || CatConvDir(root, "") != "" || CatConvDir(root, "   ") != "" {
		t.Fatal("empty inputs")
	}
}

func TestCatConvDirSanitizesSegment(t *testing.T) {
	root := t.TempDir()
	got := CatConvDir(root, "ab/cd")
	if filepath.Base(got) != "ab_cd" {
		t.Fatal(got)
	}
}
