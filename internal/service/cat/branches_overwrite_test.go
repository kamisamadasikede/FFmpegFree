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

// 预检通过之后 git 才报 would be overwritten（例如预检和切换之间文件被改）：
// 用包一层的 git 让 switch 返回真实 stderr，确认归到 git_dirty 而不是通用失败。
func TestSwitchCatBranchOverwrittenMapsToDirty(t *testing.T) {
	requireGit(t)
	real, _ := exec.LookPath("git")
	root := t.TempDir()
	initRepo(t, root)
	gitRun(t, root, "branch", "dev")
	svc, id := convAt(t, root)

	for name, stderr := range map[string]string{
		"checkout":  "error: Your local changes to the following files would be overwritten by checkout:\n\ta.txt\nPlease commit your changes or stash them before you switch branches.\nAborting",
		"merge":     "error: Your local changes to the following files would be overwritten by merge:\n\ta.txt\nPlease commit your changes or stash them before you merge.\nAborting",
		"untracked": "error: The following untracked working tree files would be overwritten by checkout:\n\tu.txt\nPlease move or remove them before you switch branches.\nAborting",
	} {
		t.Run(name, func(t *testing.T) {
			bin := t.TempDir()
			errf := filepath.Join(bin, "stderr.txt")
			if err := os.WriteFile(errf, []byte(stderr+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			script := "#!/bin/sh\nif [ \"$1\" = switch ]; then\n  if [ \"$LC_ALL\" != C ]; then echo 'no C locale' >&2; exit 3; fi\n  cat '" + errf + "' >&2; exit 1\nfi\nexec '" + real + "' \"$@\"\n"
			if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			err := svc.SwitchCatBranch(context.Background(), cat.SwitchCatBranchRequest{ConvID: id, Branch: "dev"})
			e := apperr.From(err)
			if !apperr.Is(err, apperr.TaskConflict) || e.Detail != "reason=git_dirty" || e.Message != cat.MsgBranchDirty {
				t.Fatalf("want git_dirty, got %v", err)
			}
		})
	}
	if cat.MsgBranchDirty != "有未提交的改动，请先提交或暂存，再切换分支。" || cat.MsgBranchNotFound != "找不到这个分支，可能已被删除。" {
		t.Fatal("copy drift")
	}
}
