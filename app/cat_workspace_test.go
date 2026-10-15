package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCatExportReplacesSnapshotAndCleansTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conversation.md")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeCatExport(path, "# New\n\n你好\n"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "# New\n\n你好\n" {
		t.Fatalf("%q %v", b, err)
	}
	blocked := filepath.Join(dir, "folder")
	_ = os.Mkdir(blocked, 0700)
	if err := writeCatExport(blocked, "cannot replace directory"); err == nil {
		t.Fatal("directory replaced")
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("orphaned temporary file: %+v", files)
	}
}
