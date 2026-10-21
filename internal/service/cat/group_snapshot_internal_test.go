package cat

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/store"
)

func snapshotGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, stderr, err := runGit(context.Background(), 5*time.Second, root, args...)
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, stderr)
	}
	return strings.TrimSpace(string(out))
}

func snapshotTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	initSnapshotTestRepo(t, root)
	return root
}

func initSnapshotTestRepo(t *testing.T, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	snapshotGit(t, root, "init", "--quiet")
	snapshotGit(t, root, "config", "user.name", "Snapshot Test")
	snapshotGit(t, root, "config", "user.email", "snapshot@example.invalid")
	snapshotGit(t, root, "config", "commit.gpgsign", "false")
	snapshotGit(t, root, "config", "core.autocrlf", "false")
	writeSnapshotFile(t, root, "tracked.txt", "base\n")
	snapshotGit(t, root, "add", "tracked.txt")
	snapshotGit(t, root, "commit", "--quiet", "-m", "baseline")
}

func writeSnapshotFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func assertSnapshotFile(t *testing.T, root, rel, want string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil || string(b) != want {
		t.Fatalf("%s = %q (%v), want %q", rel, b, err, want)
	}
}

func TestGroupSnapshotPreservesUntrackedAndIndex(t *testing.T) {
	root := snapshotTestRepo(t)
	ctx := context.Background()
	original := "user's file\r\n\x00binary\xff"
	writeSnapshotFile(t, root, "existing.bin", original)
	writeSnapshotFile(t, root, "tracked.txt", "staged\n")
	snapshotGit(t, root, "add", "tracked.txt")
	staged := snapshotGit(t, root, "ls-files", "--stage")
	writeSnapshotFile(t, root, "tracked.txt", "unstaged\n")
	before := createGitSnapshot(ctx, root)
	if before == "" {
		t.Fatal("snapshot failed")
	}
	if got := snapshotGit(t, root, "ls-files", "--stage"); got != staged {
		t.Fatal("snapshot changed the index")
	}
	writeSnapshotFile(t, root, "existing.bin", "agent overwrite")
	writeSnapshotFile(t, root, "tracked.txt", "agent overwrite")
	for _, rel := range []string{"tracked.txt", "existing.bin"} {
		if err := restoreFromSnapshot(ctx, root, before, rel); err != nil {
			t.Fatal(err)
		}
	}
	assertSnapshotFile(t, root, "existing.bin", original)
	assertSnapshotFile(t, root, "tracked.txt", "unstaged\n")
	if got := snapshotGit(t, root, "ls-files", "--stage"); got != staged {
		t.Fatal("revert changed the index")
	}
}

func TestGroupSnapshotRestoresDeletedAndRemovesNewFile(t *testing.T) {
	root := snapshotTestRepo(t)
	writeSnapshotFile(t, root, "existing.txt", "untracked original")
	before := createGitSnapshot(context.Background(), root)
	for _, rel := range []string{"tracked.txt", "existing.txt"} {
		if err := os.Remove(filepath.Join(root, rel)); err != nil {
			t.Fatal(err)
		}
		if err := restoreFromSnapshot(context.Background(), root, before, rel); err != nil {
			t.Fatal(err)
		}
	}
	assertSnapshotFile(t, root, "tracked.txt", "base\n")
	assertSnapshotFile(t, root, "existing.txt", "untracked original")
	writeSnapshotFile(t, root, "new.txt", "agent created")
	if err := restoreFromSnapshot(context.Background(), root, before, "new.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new file not removed: %v", err)
	}
}

func TestGroupSnapshotFailsClosed(t *testing.T) {
	root := snapshotTestRepo(t)
	writeSnapshotFile(t, root, ".gitignore", "private/\n")
	writeSnapshotFile(t, root, "private/original.txt", "private original")
	before := createGitSnapshot(context.Background(), root)
	writeSnapshotFile(t, root, "private/original.txt", "agent overwrite")
	if err := restoreFromSnapshot(context.Background(), root, before, "private/original.txt"); err == nil {
		t.Fatal("ignored original was not backed up; must refuse")
	}
	assertSnapshotFile(t, root, "private/original.txt", "agent overwrite")
	writeSnapshotFile(t, root, "untracked.txt", "do not delete")
	head := snapshotGit(t, root, "rev-parse", "HEAD")
	badV2, _ := json.Marshal(revertSnapshot{Version: 2, Tracked: strings.Repeat("0", 40)})
	for _, raw := range []string{"", "{", "not-an-object", string(badV2), head} {
		if err := restoreFromSnapshot(context.Background(), root, raw, "untracked.txt"); err == nil {
			t.Fatalf("unsafe snapshot %q was accepted", raw)
		}
		assertSnapshotFile(t, root, "untracked.txt", "do not delete")
	}
	// Legacy tracked snapshots remain usable, but only for known existing blobs.
	writeSnapshotFile(t, root, "tracked.txt", "agent overwrite")
	if err := restoreFromSnapshot(context.Background(), root, head, "tracked.txt"); err != nil {
		t.Fatal(err)
	}
	assertSnapshotFile(t, root, "tracked.txt", "base\n")
}

func TestGroupSnapshotNestedProjectAndLiteralPaths(t *testing.T) {
	root := snapshotTestRepo(t)
	writeSnapshotFile(t, root, "project/file[1].txt", "original")
	writeSnapshotFile(t, root, "project/file1.txt", "sibling")
	snapshotGit(t, root, "add", ".")
	snapshotGit(t, root, "commit", "--quiet", "-m", "nested")
	project := filepath.Join(root, "project")
	writeSnapshotFile(t, project, "file[1].txt", "user unstaged")
	before := createGitSnapshot(context.Background(), project)
	writeSnapshotFile(t, project, "file[1].txt", "agent overwrite")
	if err := restoreFromSnapshot(context.Background(), project, before, "file[1].txt"); err != nil {
		t.Fatal(err)
	}
	assertSnapshotFile(t, project, "file[1].txt", "user unstaged")
	assertSnapshotFile(t, project, "file1.txt", "sibling")
}

func TestGroupSnapshotRejectsUnsafePaths(t *testing.T) {
	root := snapshotTestRepo(t)
	before := createGitSnapshot(context.Background(), root)
	for _, rel := range []string{"", ".", "../outside", ".git/config", "dir/../../outside", "dir/./file", "dir//file", ".git./config", "tracked.txt.", "C:/outside", "tracked.txt:stream"} {
		if err := restoreFromSnapshot(context.Background(), root, before, rel); err == nil {
			t.Fatalf("accepted unsafe path %q", rel)
		}
	}
	t.Run("symlink parent", func(t *testing.T) {
		outside := t.TempDir()
		writeSnapshotFile(t, outside, "file.txt", "outside")
		if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if err := restoreFromSnapshot(context.Background(), root, before, "link/file.txt"); err == nil {
			t.Fatal("accepted symlink parent")
		}
		assertSnapshotFile(t, outside, "file.txt", "outside")
	})
}

func TestGroupRevertRequiresReliableEndState(t *testing.T) {
	root := snapshotTestRepo(t)
	before := createGitSnapshot(context.Background(), root)
	writeSnapshotFile(t, root, "tracked.txt", "agent overwrite")
	s := &Service{}
	meta := store.CatGroupTurnMeta{SnapshotBefore: before, ClaimedFiles: []string{"tracked.txt"}}
	req := RevertOverreachRequest{Files: []string{"tracked.txt"}}
	if result, err := s.revertGroupFiles(context.Background(), root, meta, req); err == nil || len(result.Reverted) != 0 {
		t.Fatal("missing end-state was accepted")
	}
	meta.EndHashes = map[string]string{"tracked.txt": revertFileState(filepath.Join(root, "tracked.txt"))}
	writeSnapshotFile(t, root, "tracked.txt", "later user edit")
	if result, err := s.revertGroupFiles(context.Background(), root, meta, req); err == nil || len(result.Reverted) != 0 {
		t.Fatal("later edit was overwritten")
	}
	assertSnapshotFile(t, root, "tracked.txt", "later user edit")
	if err := os.Remove(filepath.Join(root, "tracked.txt")); err != nil {
		t.Fatal(err)
	}
	meta.EndHashes["tracked.txt"] = "missing"
	if result, err := s.revertGroupFiles(context.Background(), root, meta, req); err != nil || len(result.Reverted) != 1 {
		t.Fatalf("deleted file restore: %+v %v", result, err)
	}
	assertSnapshotFile(t, root, "tracked.txt", "base\n")
}

func TestGroupSnapshotHashFallbackForNestedProject(t *testing.T) {
	root := snapshotTestRepo(t)
	writeSnapshotFile(t, root, "project/file.txt", "original")
	snapshotGit(t, root, "add", ".")
	snapshotGit(t, root, "commit", "--quiet", "-m", "nested")
	project := filepath.Join(root, "project")
	before := createGitSnapshot(context.Background(), project)
	writeSnapshotFile(t, project, "file.txt", "agent changed")
	after := createGitSnapshot(context.Background(), project)
	s := &Service{}
	meta := store.CatGroupTurnMeta{SnapshotBefore: before, SnapshotAfter: after, ClaimedFiles: []string{"file.txt"}}
	res, err := s.revertGroupFiles(context.Background(), project, meta, RevertOverreachRequest{Files: []string{"file.txt"}})
	if err != nil || len(res.Reverted) != 1 {
		t.Fatalf("nested snapshot fallback failed: %+v %v", res, err)
	}
	assertSnapshotFile(t, project, "file.txt", "original")
	if got := blobHashFromSnapshot(context.Background(), project, after, "missing.txt"); got != "" {
		t.Fatalf("missing path returned %q", got)
	}
}

func TestGroupSnapshotCaseVariantsCannotDeleteOriginal(t *testing.T) {
	root := snapshotTestRepo(t)
	writeSnapshotFile(t, root, "original.txt", "user original")
	writeSnapshotFile(t, root, ".gitignore", "private/\n")
	writeSnapshotFile(t, root, "private/original.txt", "ignored original")
	before := createGitSnapshot(context.Background(), root)
	// On case-sensitive filesystems these are new paths, not aliases.
	if _, err := os.Stat(filepath.Join(root, "TRACKED.TXT")); err != nil {
		t.Skip("case-sensitive worktree")
	}
	writeSnapshotFile(t, root, "original.txt", "agent overwrite")
	if err := restoreFromSnapshot(context.Background(), root, before, "ORIGINAL.TXT"); err != nil {
		t.Fatal(err)
	}
	assertSnapshotFile(t, root, "original.txt", "user original")
	_ = restoreFromSnapshot(context.Background(), root, before, "TRACKED.TXT")
	assertSnapshotFile(t, root, "tracked.txt", "base\n")
	if err := restoreFromSnapshot(context.Background(), root, before, "PRIVATE/ORIGINAL.TXT"); err == nil {
		t.Fatal("case-variant ignored original was accepted")
	}
	assertSnapshotFile(t, root, "private/original.txt", "ignored original")
}
