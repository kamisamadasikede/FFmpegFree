package cat

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

func TestGitCatMergeBlockedWhileTurnRunning(t *testing.T) {
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
	_, err = s.GitCatMerge(ctx, GitCatMergeRequest{ConvID: c.ID, Branch: "main"})
	if !apperr.Is(err, apperr.TaskConflict) || apperr.From(err).Detail != "reason=turn_running" || apperr.From(err).Message != MsgMergeTurnRunning {
		t.Fatalf("running: %v", err)
	}
}

func TestGitMergeEnvNonInteractive(t *testing.T) {
	env := strings.Join(append(gitMergeCmdEnv(), gitMergeEnv...), "\n")
	for _, want := range []string{"GIT_TERMINAL_PROMPT=0", "GIT_MERGE_AUTOEDIT=no", "GIT_EDITOR=true", "LC_ALL=C"} {
		if !strings.Contains(env, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if gitMergeTimeout.Seconds() != 120 {
		t.Fatal("timeout")
	}
}

func TestErrMergeFailedClipsDetail(t *testing.T) {
	e := apperr.From(errMergeFailed(strings.Repeat("x", 9000)))
	if !strings.HasPrefix(e.Detail, "reason=git_failed\n") || len(e.Detail) != len("reason=git_failed\n")+4096 || e.Message != MsgMergeFailed {
		t.Fatalf("%q", e.Detail[:40])
	}
}
