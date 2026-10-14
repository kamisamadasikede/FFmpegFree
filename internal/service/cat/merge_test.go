package cat_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/cat"
)

func mergeCommit(t *testing.T, dir, file, body, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", file)
	gitRun(t, dir, "commit", "-m", msg)
}

func revParse(t *testing.T, dir, ref string) string {
	t.Helper()
	return strings.TrimSpace(gitOut(t, dir, "rev-parse", ref))
}

func wantReason(t *testing.T, err error, code apperr.Code, msg, reason string) {
	t.Helper()
	e := apperr.From(err)
	if err == nil || !apperr.Is(err, code) || e.Message != msg || strings.SplitN(e.Detail, "\n", 2)[0] != reason {
		t.Fatalf("want %s %q %q, got %v (detail %q)", code, msg, reason, err, e.Detail)
	}
}

func TestGitCatMergeFastForwardAndDiverged(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	initRepo(t, root)
	gitRun(t, root, "switch", "-c", "feat")
	mergeCommit(t, root, "b.txt", "b\n", "b")
	feat := revParse(t, root, "feat")
	gitRun(t, root, "switch", "main")
	svc, id := convAt(t, root)

	res, err := svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "feat"})
	if err != nil || res.Status != cat.MergeStatusMerged || res.Head != feat {
		t.Fatalf("ff: %+v %v", res, err)
	}
	// 已包含 → up_to_date，不跑 merge
	res, err = svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "feat"})
	if err != nil || res.Status != cat.MergeStatusUpToDate || res.Head != feat {
		t.Fatalf("contained: %+v %v", res, err)
	}
	// 已包含时脏工作区也不拦（第 6 步先于第 7 步）
	_ = os.WriteFile(filepath.Join(root, "junk.txt"), []byte("x"), 0o644)
	if res, err = svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "feat"}); err != nil || res.Status != cat.MergeStatusUpToDate {
		t.Fatalf("contained dirty: %+v %v", res, err)
	}
	_ = os.Remove(filepath.Join(root, "junk.txt"))

	// 分叉无冲突 → 两父合并提交
	gitRun(t, root, "switch", "feat")
	mergeCommit(t, root, "c.txt", "c\n", "c")
	gitRun(t, root, "switch", "main")
	mergeCommit(t, root, "d.txt", "d\n", "d")
	res, err = svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "feat"})
	if err != nil || res.Status != cat.MergeStatusMerged || res.Head != revParse(t, root, "HEAD") {
		t.Fatalf("diverged: %+v %v", res, err)
	}
	if parents := strings.Fields(gitOut(t, root, "rev-list", "--parents", "-n1", "HEAD")); len(parents) != 3 {
		t.Fatalf("want merge commit, got %v", parents)
	}
}

func TestGitCatMergeConflict(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	initRepo(t, root)
	before := revParse(t, root, "HEAD")
	gitRun(t, root, "switch", "-c", "feat")
	mergeCommit(t, root, "a.txt", "theirs\n", "t1")
	mergeCommit(t, root, "z.txt", "theirs\n", "t2")
	mergeCommit(t, root, "bin.dat", "\x00\x01theirs", "t3")
	gitRun(t, root, "switch", "main")
	mergeCommit(t, root, "a.txt", "ours\n", "o1")
	mergeCommit(t, root, "z.txt", "ours\n", "o2")
	mergeCommit(t, root, "bin.dat", "\x00\x02ours", "o3")
	before = revParse(t, root, "HEAD")
	svc, id := convAt(t, root)

	res, err := svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "feat"})
	if err != nil || res.Status != cat.MergeStatusConflict || strings.Join(res.Files, ",") != "a.txt,bin.dat,z.txt" {
		t.Fatalf("conflict: %+v %v", res, err)
	}
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || st.MergeState != "merging" || len(st.Conflicts) != 3 {
		t.Fatalf("status: %+v %v", st, err)
	}
	// 再合并 → merge_in_progress
	_, err = svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "feat"})
	wantReason(t, err, apperr.TaskConflict, cat.MsgMergeInProgress, "reason=merge_in_progress")
	if err := svc.GitCatConflictAbort(ctx, cat.GitCatConflictAbortRequest{ConvID: id}); err != nil {
		t.Fatal(err)
	}
	if revParse(t, root, "HEAD") != before {
		t.Fatal("abort must restore HEAD")
	}
}

func TestGitCatMergePrechecks(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	initRepo(t, root)
	gitRun(t, root, "switch", "-c", "feat")
	mergeCommit(t, root, "a.txt", "feat\n", "f")
	gitRun(t, root, "switch", "main")
	svc, id := convAt(t, root)

	for _, b := range []string{"nope", "-f", "bad..name", ""} {
		_, err := svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: b})
		wantReason(t, err, apperr.NotFound, cat.MsgMergeBranchNotFound, "reason=branch_not_found")
	}
	_, err := svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "main"})
	wantReason(t, err, apperr.InvalidArgument, cat.MsgMergeSameBranch, "reason=same_branch")
	if apperr.From(err).Detail != "reason=same_branch" {
		t.Fatal("same_branch must not carry raw detail")
	}

	// 已跟踪改动 / 未跟踪文件 → git_dirty，仓库不变
	head := revParse(t, root, "HEAD")
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("local\n"), 0o644)
	_, err = svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "feat"})
	wantReason(t, err, apperr.TaskConflict, cat.MsgMergeDirty, "reason=git_dirty")
	gitRun(t, root, "checkout", "--", "a.txt")
	_ = os.WriteFile(filepath.Join(root, "new.txt"), []byte("u\n"), 0o644)
	_, err = svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "feat"})
	wantReason(t, err, apperr.TaskConflict, cat.MsgMergeDirty, "reason=git_dirty")
	_ = os.Remove(filepath.Join(root, "new.txt"))
	if revParse(t, root, "HEAD") != head {
		t.Fatal("dirty must not move HEAD")
	}

	// rebase 进行中 → merge_in_progress（先于分支校验）
	gd := strings.TrimSpace(gitOut(t, root, "rev-parse", "--git-path", "rebase-merge"))
	if !filepath.IsAbs(gd) {
		gd = filepath.Join(root, gd)
	}
	_ = os.MkdirAll(gd, 0o755)
	_, err = svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "nope"})
	wantReason(t, err, apperr.TaskConflict, cat.MsgMergeInProgress, "reason=merge_in_progress")
	_ = os.RemoveAll(gd)

	// git_failed：无共同历史，原始 stderr 在 detail 第二行起
	gitRun(t, root, "switch", "--orphan", "lonely")
	mergeCommit(t, root, "l.txt", "l\n", "l")
	gitRun(t, root, "switch", "main")
	_, err = svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "lonely"})
	wantReason(t, err, apperr.InvalidArgument, cat.MsgMergeFailed, "reason=git_failed")
	if d := apperr.From(err).Detail; !strings.Contains(d, "unrelated histories") {
		t.Fatalf("detail: %q", d)
	}
}

func TestGitCatMergeNotRepoAndRemote(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	plain := t.TempDir()
	svc, id := convAt(t, plain)
	_, err := svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "main"})
	wantReason(t, err, apperr.InvalidArgument, cat.MsgMergeNotRepo, "reason=not_git_repo")

	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare", "-b", "main")
	seed := t.TempDir()
	initRepo(t, seed)
	gitRun(t, seed, "remote", "add", "origin", bare)
	gitRun(t, seed, "push", "-u", "origin", "main")
	gitRun(t, seed, "switch", "-c", "feat/x")
	mergeCommit(t, seed, "r.txt", "r\n", "r")
	gitRun(t, seed, "push", "-u", "origin", "feat/x")
	root := t.TempDir()
	gitRun(t, root, "clone", "-b", "main", bare, ".")
	gitRun(t, root, "config", "user.email", "t@example.com")
	gitRun(t, root, "config", "user.name", "t")
	svc, id = convAt(t, root)
	res, err := svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "origin/feat/x", Remote: true})
	if err != nil || res.Status != cat.MergeStatusMerged || res.Head != revParse(t, root, "origin/feat/x") {
		t.Fatalf("remote: %+v %v", res, err)
	}
	_, err = svc.GitCatMerge(ctx, cat.GitCatMergeRequest{ConvID: id, Branch: "origin/ghost", Remote: true})
	wantReason(t, err, apperr.NotFound, cat.MsgMergeBranchNotFound, "reason=branch_not_found")
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}
