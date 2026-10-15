package doc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewSavedVersionIsImmutable(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.pptx")
	dest := filepath.Join(dir, "preview.pptx")
	_ = os.WriteFile(source, []byte("saved-version"), 0600)
	fi, _ := os.Stat(source)
	job := previewJob{path: source, size: fi.Size(), mtime: fi.ModTime().UnixNano()}
	if e := copyPreviewSource(job, dest); e != nil {
		t.Fatal(e)
	}
	_ = os.WriteFile(source, []byte("new-version-written"), 0600)
	b, _ := os.ReadFile(dest)
	if string(b) != "saved-version" {
		t.Fatal("snapshot changed")
	}
	if e := copyPreviewSource(job, filepath.Join(dir, "stale.pptx")); e == nil {
		t.Fatal("stale revision accepted")
	}
}
