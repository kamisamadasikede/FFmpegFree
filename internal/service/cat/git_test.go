package cat_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("没有 git")
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func convAt(t *testing.T, dir string) (*cat.Service, string) {
	t.Helper()
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{DataRoot: t.TempDir()})
	p, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	return svc, conv.ID
}

func changeBy(st cat.GitStatusResult, path string) (cat.GitChange, bool) {
	for _, c := range st.Changes {
		if c.Path == path {
			return c, true
		}
	}
	return cat.GitChange{}, false
}

func TestGitStatusStageCommitAndPR(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.email", "t@example.com")
	gitRun(t, root, "config", "user.name", "t")
	gitRun(t, root, "config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "old.ts"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "old.ts")
	gitRun(t, root, "commit", "-m", "old")
	gitRun(t, root, "mv", "old.ts", "newname.ts")

	svc, id := convAt(t, root)
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Repo || st.Branch != "main" {
		t.Fatalf("status: %+v", st)
	}
	a, ok := changeBy(st, "a.txt")
	if !ok || a.Letter != "M" || a.Staged || !a.Unstaged || a.Added != 1 || a.Name != "a.txt" {
		t.Fatalf("a.txt: %+v", a)
	}
	n, ok := changeBy(st, "new.txt")
	if !ok || n.Letter != "U" || n.Added != 1 || n.Staged {
		t.Fatalf("new.txt: %+v", n)
	}
	rn, ok := changeBy(st, "newname.ts")
	if !ok || rn.Letter != "R" || !rn.Staged {
		t.Fatalf("rename: %+v", rn)
	}

	if err := svc.GitCatStage(ctx, cat.GitPathsRequest{ConvID: id, Paths: []string{`..\outside`}}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("escape: %v", err)
	}
	if err := svc.GitCatStage(ctx, cat.GitPathsRequest{ConvID: id, Paths: []string{`.git/config`}}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("git dir: %v", err)
	}
	if err := svc.GitCatUnstage(ctx, cat.GitPathsRequest{ConvID: id}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("unstage none: %v", err)
	}
	if err := svc.GitCatStage(ctx, cat.GitPathsRequest{ConvID: id, Paths: []string{"a.txt"}}); err != nil {
		t.Fatal(err)
	}
	st, err = svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	a, _ = changeBy(st, "a.txt")
	if !a.Staged || a.Unstaged {
		t.Fatalf("staged a.txt: %+v", a)
	}
	if err := svc.GitCatCommit(ctx, cat.GitPathsRequest{ConvID: id, Message: "  "}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("empty message: %v", err)
	}
	if err := svc.GitCatCommit(ctx, cat.GitPathsRequest{ConvID: id, Message: "next"}); err != nil {
		t.Fatal(err)
	}
	st, err = svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := changeBy(st, "a.txt"); ok {
		t.Fatalf("a.txt still changed: %+v", st.Changes)
	}
	if _, ok := changeBy(st, "new.txt"); !ok {
		t.Fatalf("new.txt missing: %+v", st.Changes)
	}

	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")
	gitRun(t, root, "remote", "add", "origin", bare)
	gitRun(t, root, "push", "-u", "origin", "HEAD")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "ahead")
	st, err = svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	if st.Ahead != 1 || st.Behind != 0 || st.Upstream != "main" {
		t.Fatalf("ahead: %+v", st)
	}
	if _, err := svc.GitCatPullRequest(ctx, cat.GitStatusRequest{ConvID: id}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("local remote should not open a PR page: %v", err)
	}
	gitRun(t, root, "remote", "set-url", "origin", "https://github.com/octocat/Hello-World.git")
	pr, err := svc.GitCatPullRequest(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	if pr.URL != "https://github.com/octocat/Hello-World/compare/main?expand=1" {
		t.Fatal(pr.URL)
	}
}

func TestGitStatusNotRepo(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, id := convAt(t, root)
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	if st.Repo || len(st.Changes) != 0 {
		t.Fatalf("not a repo: %+v", st)
	}
	if err := svc.GitCatStage(ctx, cat.GitPathsRequest{ConvID: id}); !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Message != "这里还不是 git 仓库。" {
		t.Fatalf("stage: %v", err)
	}
	if _, err := svc.GitCatDiff(ctx, cat.GitDiffRequest{ConvID: id, RelPath: "a.txt"}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("diff not repo: %v", err)
	}
}

func TestGitStageSkipsWindowsNul(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows device name")
	}
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.email", "t@example.com")
	gitRun(t, root, "config", "user.name", "t")
	gitRun(t, root, "config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	literal := `\\?\` + abs + `\nul`
	if err := os.WriteFile(literal, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(literal) })

	svc, id := convAt(t, root)
	err = svc.GitCatStage(ctx, cat.GitPathsRequest{ConvID: id})
	if err == nil || apperr.From(err).Message != "nul 在 Windows 上是设备名，没法暂存。" {
		t.Fatalf("stage all: %v", err)
	}
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := changeBy(st, "a.txt")
	if !ok || !a.Staged || a.Unstaged {
		t.Fatalf("a.txt should be staged: %+v", a)
	}
	n, ok := changeBy(st, "nul")
	if !ok || n.Staged || !n.Unstaged {
		t.Fatalf("nul should stay unstaged: %+v", n)
	}
	if err := svc.GitCatStage(ctx, cat.GitPathsRequest{ConvID: id, Paths: []string{"nul"}}); err == nil || apperr.From(err).Message != "nul 在 Windows 上是设备名，没法暂存。" {
		t.Fatalf("stage nul: %v", err)
	}
}

func TestGitCatDiff(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.email", "t@example.com")
	gitRun(t, root, "config", "user.name", "t")
	gitRun(t, root, "config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, id := convAt(t, root)

	d, err := svc.GitCatDiff(ctx, cat.GitDiffRequest{ConvID: id, RelPath: "a.txt"})
	if err != nil || d.Binary || d.Text != "+hello" || d.Path != "a.txt" {
		t.Fatalf("no head: %v %+v", err, d)
	}

	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "init")
	d, err = svc.GitCatDiff(ctx, cat.GitDiffRequest{ConvID: id, RelPath: "a.txt"})
	if err != nil || d.Binary || d.Text != "" {
		t.Fatalf("clean: %v %+v", err, d)
	}

	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gone.txt"), []byte("old\nline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte("a\x00b"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "gone.txt")
	gitRun(t, root, "commit", "-m", "gone")
	if err := os.Remove(filepath.Join(root, "gone.txt")); err != nil {
		t.Fatal(err)
	}

	d, err = svc.GitCatDiff(ctx, cat.GitDiffRequest{ConvID: id, RelPath: "a.txt"})
	if err != nil || d.Binary || !strings.Contains(d.Text, "+two") || strings.Contains(d.Text, "diff --git") {
		t.Fatalf("modify: %v %+v", err, d)
	}
	d, err = svc.GitCatDiff(ctx, cat.GitDiffRequest{ConvID: id, RelPath: "new.txt"})
	if err != nil || d.Binary || d.Text != "+hi" {
		t.Fatalf("untracked: %v %+v", err, d)
	}
	d, err = svc.GitCatDiff(ctx, cat.GitDiffRequest{ConvID: id, RelPath: "gone.txt"})
	if err != nil || d.Binary || !strings.Contains(d.Text, "-old") || !strings.Contains(d.Text, "-line") {
		t.Fatalf("deleted: %v %+v", err, d)
	}
	d, err = svc.GitCatDiff(ctx, cat.GitDiffRequest{ConvID: id, RelPath: "bin.dat"})
	if err != nil || !d.Binary || d.Text != "" {
		t.Fatalf("binary: %v %+v", err, d)
	}
	if _, err := svc.GitCatDiff(ctx, cat.GitDiffRequest{ConvID: id, RelPath: `..\outside`}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("escape: %v", err)
	}
	if _, err := svc.GitCatDiff(ctx, cat.GitDiffRequest{ConvID: id, RelPath: `.git/config`}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("git dir: %v", err)
	}
}

func TestGitRemotePublishPullAndBranch(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.email", "t@example.com")
	gitRun(t, root, "config", "user.name", "t")
	gitRun(t, root, "config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "init")
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")
	gitRun(t, root, "remote", "add", "origin", bare)

	svc, id := convAt(t, root)
	if err := svc.GitCatRemote(ctx, cat.GitRemoteRequest{ConvID: id, Op: "nope"}); err == nil || apperr.From(err).Message != "不认识这个操作。" {
		t.Fatalf("bad op: %v", err)
	}
	if err := svc.GitCatRemote(ctx, cat.GitRemoteRequest{ConvID: id, Op: "push"}); err == nil || apperr.From(err).Message != "还没有上游分支。" {
		t.Fatalf("push without upstream: %v", err)
	}
	if err := svc.GitCatRemote(ctx, cat.GitRemoteRequest{ConvID: id, Op: "publish"}); err != nil {
		t.Fatal(err)
	}
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || st.Upstream == "" {
		t.Fatalf("published: %v %+v", err, st)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.GitCatCommit(ctx, cat.GitPathsRequest{ConvID: id, Message: "next"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.GitCatRemote(ctx, cat.GitRemoteRequest{ConvID: id, Op: "push"}); err != nil {
		t.Fatal(err)
	}

	other := t.TempDir()
	gitRun(t, other, "-c", "core.autocrlf=false", "clone", "-b", "main", bare, ".")
	gitRun(t, other, "config", "user.email", "t@example.com")
	gitRun(t, other, "config", "user.name", "t")
	svc2, id2 := convAt(t, other)
	if err := svc2.GitCatRemote(ctx, cat.GitRemoteRequest{ConvID: id2, Op: "fetch"}); err != nil {
		t.Fatal(err)
	}
	if err := svc2.GitCatRemote(ctx, cat.GitRemoteRequest{ConvID: id2, Op: "ff"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(other, "a.txt"))
	if err != nil || string(b) != "one\ntwo\n" {
		t.Fatalf("pulled: %v %q", err, b)
	}
	if err := svc2.GitCatBranch(ctx, cat.GitBranchRequest{ConvID: id2, Name: "a..b"}); err == nil || apperr.From(err).Message != "分支名称不能用。" {
		t.Fatalf("bad branch: %v", err)
	}
	if err := svc2.GitCatBranch(ctx, cat.GitBranchRequest{ConvID: id2, Name: "feature/login"}); err != nil {
		t.Fatal(err)
	}
	st, err = svc2.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id2})
	if err != nil || st.Branch != "feature/login" {
		t.Fatalf("branch: %v %+v", err, st)
	}
}
