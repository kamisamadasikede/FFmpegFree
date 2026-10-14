//go:build !windows

package cat_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/cat"
)

// 预检之后 git merge 才报 would be overwritten：应归到 TASK_CONFLICT reason=git_dirty，仓库不进入合并态。
func TestGitCatMergeOverwrittenMapsToDirty(t *testing.T) {
	requireGit(t)
	real, _ := exec.LookPath("git")
	root := t.TempDir()
	initRepo(t, root)
	gitRun(t, root, "switch", "-q", "-c", "dev")
	hcommit(t, root, "b.txt", "b\n", "dev work")
	gitRun(t, root, "switch", "-q", "main")
	svc, id := convAt(t, root)
	for name, msg := range map[string]string{
		"local":     "error: Your local changes to the following files would be overwritten by merge:\n\ta.txt\nPlease commit your changes or stash them before you merge.\nAborting",
		"untracked": "error: The following untracked working tree files would be overwritten by merge:\n\tb.txt\nPlease move or remove them before you merge.\nAborting",
	} {
		t.Run(name, func(t *testing.T) {
			bin := t.TempDir()
			errf := filepath.Join(bin, "stderr.txt")
			os.WriteFile(errf, []byte(msg+"\n"), 0o644)
			script := "#!/bin/sh\nif [ \"$1\" = merge ]; then\n  if [ \"$LC_ALL\" != C ]; then echo 'no C locale' >&2; exit 3; fi\n  cat '" + errf + "' >&2; exit 1\nfi\nexec '" + real + "' \"$@\"\n"
			os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			_, err := svc.GitCatMerge(context.Background(), cat.GitCatMergeRequest{ConvID: id, Branch: "dev"})
			e := apperr.From(err)
			if !apperr.Is(err, apperr.TaskConflict) || e.Detail != "reason=git_dirty" || e.Message != cat.MsgMergeDirty {
				t.Fatalf("want git_dirty, got %v", err)
			}
			if _, statErr := os.Stat(filepath.Join(root, ".git", "MERGE_HEAD")); statErr == nil {
				t.Fatal("left MERGE_HEAD")
			}
		})
	}
}
