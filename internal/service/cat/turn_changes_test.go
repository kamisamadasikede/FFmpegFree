package cat

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTurnChangesCompareSavedBaseline(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	root := t.TempDir()
	write := func(p, body string) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(root, p), []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("dirty.txt", "already dirty\nkeep\n")
	write("edit.txt", "one\ntwo\nthree\n")
	write("gone.txt", "gone\n")
	base := snapshotWorkspace(ctx, root)
	write("edit.txt", "one\nchanged\nthree\nnew\n")
	write("new.txt", "created\n")
	write("asset.bin", "\x00\x01")
	_ = os.Remove(filepath.Join(root, "gone.txt"))
	out := compareSnapshot(ctx, base, snapshotWorkspace(ctx, root), &turnState{convID: "c", turnID: "t"}, true)
	if out.FileCount != 4 || out.Added != 3 || out.Deleted != 2 {
		t.Fatalf("%+v", out)
	}
	for _, c := range out.Files {
		if c.Path == "dirty.txt" {
			t.Fatal("preexisting dirty file counted")
		}
		if c.Path == "asset.bin" && !c.Binary {
			t.Fatal("binary counted as text")
		}
	}
}
func TestTurnChangesIncompleteSnapshotDoesNotInventDeletion(t *testing.T) {
	base := workspaceSnapshot{files: map[string]workspaceFile{"a": {hash: "a", text: "old"}}}
	now := workspaceSnapshot{files: map[string]workspaceFile{}, cut: true}
	out := compareSnapshot(context.Background(), base, now, &turnState{}, true)
	if out.FileCount != 0 || !out.Truncated {
		t.Fatal(out)
	}
}
