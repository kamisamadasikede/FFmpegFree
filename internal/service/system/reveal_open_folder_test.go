package system

import (
	"os"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/apperr"
)

func TestOpenFolderUsesDirBranchAndNeverCreates(t *testing.T) {
	var got []string
	m := &Manager{launch: func(name string, args ...string) error { got = append([]string{name}, args...); return nil }}
	dir := t.TempDir()
	if err := m.OpenFolder(dir); err != nil || len(got) == 0 {
		t.Fatalf("%v %v", got, err)
	}
	gone := filepath.Join(dir, "gone")
	if err := m.OpenFolder(gone); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Fatal("不应新建文件夹")
	}
	f := filepath.Join(dir, "f.txt")
	_ = os.WriteFile(f, nil, 0o644)
	if err := m.OpenFolder(f); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
}
