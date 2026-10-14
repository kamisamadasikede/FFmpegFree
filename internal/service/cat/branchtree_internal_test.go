package cat

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/proc"
	"FFmpegFree/internal/store"
)

func TestBranchTreeGitEnv(t *testing.T) {
	cmd := branchTreeCmd(context.Background(), "git", ".", "status")
	env := strings.Join(cmd.Env, "\n")
	for _, w := range []string{"GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "LC_ALL=C", "GCM_INTERACTIVE=never"} {
		if !strings.Contains(env, w) {
			t.Fatalf("missing %s", w)
		}
	}
	// proc.Configure 的结果与同平台 runGit 一致（Windows 上为隐藏窗口）
	ref := exec.Command("git")
	proc.Configure(ref)
	if (ref.SysProcAttr == nil) != (cmd.SysProcAttr == nil) {
		t.Fatal("proc.Configure not applied")
	}
}

func TestBranchRepoKeyWindows(t *testing.T) {
	if k := branchRepoKeyFor("windows", "C:/Users/X/repo/"); k != `c:\Users\X\repo` {
		t.Fatal(k)
	}
	if k := branchRepoKeyFor("linux", "/a/b/"); k != "/a/b" {
		t.Fatal(k)
	}
	if s := clipBytes(strings.Repeat("中", 2000), 4096); len(s) > 4096 || !strings.HasPrefix(strings.Repeat("中", 2000), s) {
		t.Fatal("clip")
	}
}

func newBTInternal(t *testing.T) (*Service, string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	repo := t.TempDir()
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@e"}, {"config", "user.name", "t"}, {"commit", "-q", "--allow-empty", "-m", "i"}, {"branch", "dev"}} {
		c := exec.Command("git", a...)
		c.Dir = repo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatal(string(out))
		}
	}
	reg := catagent.NewRegistry()
	reg.Register(catagent.NewClaudeAdapter(catagent.CLIAgentConfig{DataRoot: dir}))
	s := New(Config{Store: st, Registry: reg, DataRoot: dir})
	p, err := s.CreateCatProject(ctx, CreateProjectRequest{Path: repo})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateCatConversation(ctx, CreateConversationRequest{AgentKind: catagent.KindClaudeCLI, ProjectID: p.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	return s, c.ID, repo
}

func TestBranchTreeTurnRunning(t *testing.T) {
	ctx := context.Background()
	s, id, _ := newBTInternal(t)
	s.setTurn(&turnState{convID: id, turnID: "t1", cancel: func() {}, done: make(chan struct{})})
	check := func(err error) {
		t.Helper()
		if !apperr.Is(err, apperr.TaskConflict) || apperr.From(err).Detail != "reason=turn_running" || apperr.From(err).Message != MsgTreeTurnRunning {
			t.Fatalf("running: %v", err)
		}
	}
	_, err := s.GitCatCreateBranch(ctx, GitCatCreateBranchRequest{ConvID: id, From: "main", Name: "n", Checkout: true})
	check(err)
	_, err = s.GitCatRenameBranch(ctx, GitCatRenameBranchRequest{ConvID: id, From: "main", To: "m2"})
	check(err)
	_, err = s.GitCatDeleteBranch(ctx, GitCatDeleteBranchRequest{ConvID: id, Branch: "dev"})
	check(err)
	// 不签出的新建、重命名非当前分支照常
	if _, err := s.GitCatCreateBranch(ctx, GitCatCreateBranchRequest{ConvID: id, From: "main", Name: "n"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GitCatRenameBranch(ctx, GitCatRenameBranchRequest{ConvID: id, From: "dev", To: "dev2"}); err != nil {
		t.Fatal(err)
	}
}

// 并发只跑一个 fetch：用一个慢的假 remote（ext 协议不可用时退回 sleep 脚本当 upload-pack）。
func TestBranchTreeFetchSingleFlight(t *testing.T) {
	ctx := context.Background()
	s, id, repo := newBTInternal(t)
	script := filepath.Join(t.TempDir(), "slow.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 1\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := exec.Command("git", "remote", "add", "origin", "file:///nonexistent")
	c.Dir = repo
	_ = c.Run()
	c = exec.Command("git", "config", "remote.origin.uploadpack", script)
	c.Dir = repo
	_ = c.Run()
	var wg sync.WaitGroup
	results := make([]bool, 2)
	start := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, _ = s.FetchCatRemotes(ctx, FetchCatRemotesRequest{ConvID: id, Force: true})
	}()
	close(start)
	// 等第一个进入 running
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.btree.mu.Lock()
		running := false
		for _, st := range s.btree.fetch {
			running = running || st.running
		}
		s.btree.mu.Unlock()
		if running || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	tr, err := s.FetchCatRemotes(ctx, FetchCatRemotesRequest{ConvID: id, Force: true})
	results[1] = tr.Fetching
	wg.Wait()
	if err != nil || !results[1] {
		t.Fatalf("second call should return fetching=true: %+v %v", tr, err)
	}
	tr, _ = s.ListCatBranchTree(ctx, CatBranchTreeRequest{ConvID: id})
	if tr.Fetching || tr.FetchError == "" || tr.FetchedAt != 0 {
		t.Fatalf("after %+v", tr)
	}
}
