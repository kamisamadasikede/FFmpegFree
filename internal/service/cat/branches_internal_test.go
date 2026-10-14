package cat

import (
	"context"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

func TestSplitRefs(t *testing.T) {
	l, r := splitRefs("refs/heads/main\nrefs/heads/a/b\nrefs/remotes/origin/HEAD\nrefs/remotes/origin/main\nrefs/remotes/origin/x\nrefs/remotes/up/a/b\nrefs/remotes/weird\n")
	if len(l) != 2 || l[0] != "a/b" || l[1] != "main" {
		t.Fatalf("locals %v", l)
	}
	if len(r) != 1 || r[0] != "origin/x" {
		t.Fatalf("remotes %v", r)
	}
	if remoteBranchLocalName("origin/feat/x") != "feat/x" {
		t.Fatal("local name")
	}
}

func TestSwitchBlockedWhileTurnRunning(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	reg := catagent.NewRegistry()
	reg.Register(catagent.NewClaudeAdapter(catagent.CLIAgentConfig{DataRoot: dir}))
	s := New(Config{Store: st, Registry: reg, DataRoot: dir})
	c, err := s.CreateCatConversation(ctx, CreateConversationRequest{AgentKind: catagent.KindClaudeCLI})
	if err != nil {
		t.Fatal(err)
	}
	s.setTurn(&turnState{convID: c.ID, turnID: "t1", cancel: func() {}, done: make(chan struct{})})
	err = s.SwitchCatBranch(ctx, SwitchCatBranchRequest{ConvID: c.ID, Branch: "main"})
	if !apperr.Is(err, apperr.TaskConflict) || apperr.From(err).Detail != "reason=turn_running" || apperr.From(err).Message != MsgBranchTurnRunning {
		t.Fatalf("running: %v", err)
	}
}
