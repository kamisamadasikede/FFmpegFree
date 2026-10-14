package cat_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func initRepo(t *testing.T, dir string) {
	t.Helper()
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "t@example.com")
	gitRun(t, dir, "config", "user.name", "t")
	gitRun(t, dir, "config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "a.txt")
	gitRun(t, dir, "commit", "-m", "init")
}

func TestListCatBranchesPlainConversationNotRepo(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{DataRoot: data})
	conv, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ListCatBranches(ctx, cat.CatBranchesRequest{ConvID: conv.ID})
	if err != nil {
		t.Fatal(err)
	}
	if res.Root == "" || !strings.HasPrefix(res.Root, data) || res.Repo || res.Branch != "" || res.Locals == nil || res.Remotes == nil {
		t.Fatalf("plain: %+v", res)
	}
	if fi, err := os.Stat(res.Root); err != nil || !fi.IsDir() {
		t.Fatalf("root not a dir: %v", err)
	}
	if err := svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: conv.ID, Branch: "main"}); err == nil {
		t.Fatal("switch outside repo should fail")
	}
}

func TestListAndSwitchCatBranches(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare", "-b", "main")
	seed := t.TempDir()
	initRepo(t, seed)
	gitRun(t, seed, "remote", "add", "origin", bare)
	gitRun(t, seed, "push", "-u", "origin", "main")
	gitRun(t, seed, "switch", "-c", "feat/remote-only")
	gitRun(t, seed, "push", "-u", "origin", "feat/remote-only")

	root := t.TempDir()
	gitRun(t, root, "-c", "core.autocrlf=false", "clone", "-b", "main", bare, ".")
	gitRun(t, root, "config", "user.email", "t@example.com")
	gitRun(t, root, "config", "user.name", "t")
	gitRun(t, root, "branch", "dev")
	gitRun(t, root, "branch", "solo")

	svc, id := convAt(t, root)
	res, err := svc.ListCatBranches(ctx, cat.CatBranchesRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Git || !res.Repo || res.Branch != "main" || res.Upstream != "origin/main" || res.Detached {
		t.Fatalf("head: %+v", res)
	}
	if strings.Join(res.Locals, ",") != "dev,main,solo" {
		t.Fatalf("locals: %v", res.Locals)
	}
	if strings.Join(res.Remotes, ",") != "origin/feat/remote-only" {
		t.Fatalf("remotes (origin/main, origin/HEAD must be hidden): %v", res.Remotes)
	}

	// 本地分支，无上游
	if err := svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: id, Branch: "dev"}); err != nil {
		t.Fatal(err)
	}
	res, _ = svc.ListCatBranches(ctx, cat.CatBranchesRequest{ConvID: id})
	if res.Branch != "dev" || res.Upstream != "" {
		t.Fatalf("after dev: %+v", res)
	}
	// 不存在
	err = svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: id, Branch: "nope"})
	if !apperr.Is(err, apperr.NotFound) || apperr.From(err).Message != cat.MsgBranchNotFound {
		t.Fatalf("missing: %v", err)
	}
	if err := svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: id, Branch: "-f"}); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("option-like: %v", err)
	}
	// 脏工作区 → 拦下，不切
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: id, Branch: "main"})
	e := apperr.From(err)
	if !apperr.Is(err, apperr.TaskConflict) || e.Detail != "reason=git_dirty" || e.Message != cat.MsgBranchDirty {
		t.Fatalf("dirty: %v", err)
	}
	res, _ = svc.ListCatBranches(ctx, cat.CatBranchesRequest{ConvID: id})
	if res.Branch != "dev" {
		t.Fatalf("dirty switch must not move: %+v", res)
	}
	// 切到当前分支是空操作，脏也不拦
	if err := svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: id, Branch: "dev"}); err != nil {
		t.Fatalf("same branch: %v", err)
	}
	gitRun(t, root, "checkout", "--", "a.txt")

	// 只在远程 → 新建跟踪分支
	if err := svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: id, Branch: "origin/feat/remote-only", Remote: true}); err != nil {
		t.Fatal(err)
	}
	res, _ = svc.ListCatBranches(ctx, cat.CatBranchesRequest{ConvID: id})
	if res.Branch != "feat/remote-only" || res.Upstream != "origin/feat/remote-only" || len(res.Remotes) != 0 {
		t.Fatalf("tracking: %+v", res)
	}
	// 远程分支本地已有同名 → 直接切本地那条
	if err := svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: id, Branch: "origin/main", Remote: true}); err != nil {
		t.Fatal(err)
	}
	res, _ = svc.ListCatBranches(ctx, cat.CatBranchesRequest{ConvID: id})
	if res.Branch != "main" || res.Upstream != "origin/main" {
		t.Fatalf("remote with local: %+v", res)
	}
	if err := svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: id, Branch: "origin/ghost", Remote: true}); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("ghost remote: %v", err)
	}

	// 分离 HEAD
	gitRun(t, root, "checkout", "--detach")
	res, _ = svc.ListCatBranches(ctx, cat.CatBranchesRequest{ConvID: id})
	if !res.Detached || res.Branch != "" || res.Upstream != "" {
		t.Fatalf("detached: %+v", res)
	}
}

func TestListCatBranchesNoGitOnPath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc, id := convAt(t, root)
	t.Setenv("PATH", t.TempDir())
	res, err := svc.ListCatBranches(ctx, cat.CatBranchesRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	if res.Git || res.Repo || res.Root == "" {
		t.Fatalf("no git: %+v", res)
	}
}
