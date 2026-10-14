package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

func TestClaudeDetect_Missing(t *testing.T) {
	a := NewClaudeAdapter(CLIAgentConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
	})
	a.detect()
	st := a.Status()
	if st.State != StateMissing || st.Error == nil || st.Error.Message != MsgNotReady {
		t.Fatalf("%+v", st)
	}
	_, err := a.RunTurn(TurnOptions{Ctx: context.Background(), Messages: []WireMessage{{Role: "user", Content: "x"}}})
	if !apperr.Is(err, apperr.CatNotReady) {
		t.Fatalf("%v", err)
	}
	models, err := a.ListModels()
	if err != nil || models == nil || len(models) != 0 {
		t.Fatalf("%v %v", models, err)
	}
}

func TestCodexDetect_ReadyOnPATH(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "codex")
	if runtime.GOOS == "windows" {
		fake += ".exe"
	}
	_ = os.WriteFile(fake, []byte("x"), 0o755)
	a := NewCodexAdapter(CLIAgentConfig{
		DataTemp: t.TempDir(),
		LookPath: func(file string) (string, error) {
			if file == "codex" || file == "codex.exe" {
				return fake, nil
			}
			return "", exec.ErrNotFound
		},
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			// version / debug models：空输出 → 空模型列表
			return exec.CommandContext(ctx, "true")
		},
	})
	a.detect()
	if a.Status().State != StateReady {
		t.Fatalf("%+v", a.Status())
	}
	models, err := a.ListModels()
	if err != nil || models == nil {
		t.Fatalf("%v %v", models, err)
	}
	thinks, err := a.ListThinkLevels()
	if err != nil || thinks == nil || len(thinks) != 0 {
		t.Fatalf("thinks must be empty on list failure: %v %v", thinks, err)
	}
}

func TestClaudeRunTurn_StreamingDeltas(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	fake := filepath.Join(t.TempDir(), "claude-fake")
	script := `#!/bin/sh
printf '%s\n' '{"type":"stream_event","session_id":"sess-1","event":{"delta":{"type":"text_delta","text":"你"}}}'
printf '%s\n' '{"type":"stream_event","session_id":"sess-1","event":{"delta":{"type":"text_delta","text":"好"}}}'
printf '%s\n' '{"type":"result","session_id":"sess-1","result":"你好"}'
exit 0
`
	_ = os.WriteFile(fake, []byte(script), 0o755)
	var deltas []string
	a := NewClaudeAdapter(CLIAgentConfig{
		DataRoot: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake)
		},
	})
	a.detect()
	resp, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "c-claude-1",
		Messages:    []WireMessage{{Role: "user", Content: "hi"}},
		OnTextDelta: func(d string) { deltas = append(deltas, d) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "你好" {
		t.Fatalf("content %q", resp.Message.Content)
	}
	if strings.Join(deltas, "") != "你好" {
		t.Fatalf("deltas %v", deltas)
	}
	sid := a.loadSessionID("c-claude-1")
	if sid != "sess-1" {
		t.Fatalf("session %q", sid)
	}
}

func TestCodexRunTurn_AgentMessage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	fake := filepath.Join(t.TempDir(), "codex-fake")
	script := `#!/bin/sh
printf '%s\n' '{"type":"thread.started","thread_id":"thr-9"}'
printf '%s\n' '{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":"世界"}}'
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}'
exit 0
`
	_ = os.WriteFile(fake, []byte(script), 0o755)
	var deltas []string
	a := NewCodexAdapter(CLIAgentConfig{
		DataRoot: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake)
		},
	})
	a.detect()
	resp, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "c-codex-1",
		Messages:    []WireMessage{{Role: "user", Content: "hi"}},
		OnTextDelta: func(d string) { deltas = append(deltas, d) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "世界" || strings.Join(deltas, "") != "世界" {
		t.Fatalf("content=%q deltas=%v", resp.Message.Content, deltas)
	}
	if a.loadSessionID("c-codex-1") != "thr-9" {
		t.Fatalf("session %q", a.loadSessionID("c-codex-1"))
	}
}

func TestClaudeRunTurn_Cancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	fake := filepath.Join(t.TempDir(), "claude-slow")
	script := "#!/bin/sh\nsleep 30\n"
	_ = os.WriteFile(fake, []byte(script), 0o755)
	a := NewClaudeAdapter(CLIAgentConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake)
		},
	})
	a.detect()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := a.RunTurn(TurnOptions{
		Ctx: ctx, ConversationID: "c-cancel",
		Messages: []WireMessage{{Role: "user", Content: "x"}},
		Timeout:  5 * time.Second,
	})
	if !apperr.Is(err, apperr.Canceled) {
		t.Fatalf("want canceled, got %v", err)
	}
}

func TestClaudeArgs_PermissionAndRules(t *testing.T) {
	a := NewClaudeAdapter(CLIAgentConfig{DataTemp: t.TempDir()})
	args := a.buildClaudeArgs(TurnOptions{
		PermissionMode: "plan",
		Rules:          "be careful",
		ModelID:        "sonnet",
	}, "hello", "sess", true)
	joined := strings.Join(args, "\n")
	for _, want := range []string{"-p", "hello", "stream-json", "--verbose", "--include-partial-messages",
		"--permission-mode", "plan", "--append-system-prompt", "be careful", "--model", "sonnet", "--resume", "sess"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
	args2 := a.buildClaudeArgs(TurnOptions{PermissionMode: "approve"}, "x", "", false)
	if !strings.Contains(strings.Join(args2, " "), "bypassPermissions") {
		t.Fatalf("approve → bypassPermissions: %v", args2)
	}
}

func TestCodexArgs_PermissionAndRulesPrefix(t *testing.T) {
	a := NewCodexAdapter(CLIAgentConfig{DataTemp: t.TempDir()})
	args := a.buildCodexArgs(TurnOptions{
		PermissionMode: "auto",
		Rules:          "use tests",
		ModelID:        "gpt-test",
	}, "fix it", "/proj", "thr-1", true)
	joined := strings.Join(args, "\n")
	for _, want := range []string{"exec", "resume", "--json", "--skip-git-repo-check",
		`sandbox_mode="workspace-write"`, `approval_policy="never"`, "-m", "gpt-test",
		"thr-1", "use tests\n\nfix it"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
	// A6：不捏造 --rules flag
	if strings.Contains(joined, "--rules") || strings.Contains(joined, "--append-system-prompt") {
		t.Fatalf("codex must not invent rules flag:\n%s", joined)
	}
}

func TestParseCodexModelsJSON(t *testing.T) {
	got := parseCodexModelsJSON([]byte(`[{"id":"gpt-a"},{"slug":"gpt-b"},{"id":"gpt-a"}]`))
	if len(got) != 2 || got[0].ID != "gpt-a" || got[1].DisplayName != "gpt-b" {
		t.Fatalf("%+v", got)
	}
	empty := parseCodexModelsJSON([]byte(`not-json`))
	if empty == nil || len(empty) != 0 {
		t.Fatalf("%v", empty)
	}
}

func TestCLIInspectUnsupported(t *testing.T) {
	a := NewClaudeAdapter(CLIAgentConfig{DataTemp: t.TempDir()})
	res := a.Inspect(context.Background(), "")
	if res.Reason != InspectUnsupported || res.Skills == nil {
		t.Fatalf("%+v", res)
	}
}

func TestClaudeArgs_CapturedByFakeExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "claude-args")
	out := filepath.Join(dir, "args.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$0\" \"$@\" > '" + out + "'\nprintf '%s\\n' '{\"type\":\"stream_event\",\"event\":{\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}}'\nprintf '%s\\n' '{\"type\":\"result\",\"result\":\"ok\"}'\n"
	_ = os.WriteFile(fake, []byte(script), 0o755)
	a := NewClaudeAdapter(CLIAgentConfig{
		DataRoot: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	a.detect()
	_, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "args-1",
		PermissionMode: "default", Rules: "rule-one",
		Messages: []WireMessage{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(out)
	s := string(raw)
	for _, want := range []string{"-p", "ping", "stream-json", "--permission-mode", "manual", "--append-system-prompt", "rule-one"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
}

func TestClaudeParseResultUsageAndText(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	fake := filepath.Join(t.TempDir(), "claude-usage")
	script := `#!/bin/sh
printf '%s\n' '{"type":"stream_event","event":{"delta":{"type":"text_delta","text":"Hi"}}}'
printf '%s\n' '{"type":"result","session_id":"s1","result":"Hi","duration_ms":500,"usage":{"input_tokens":10,"output_tokens":5}}'
exit 0
`
	_ = os.WriteFile(fake, []byte(script), 0o755)
	a := NewClaudeAdapter(CLIAgentConfig{
		DataRoot: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake)
		},
	})
	a.detect()
	resp, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "u1",
		Messages: []WireMessage{{Role: "user", Content: "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "Hi" {
		t.Fatalf("content %q", resp.Message.Content)
	}
	if resp.TokenTotal != 15 || resp.DurationMs != 500 {
		t.Fatalf("usage %+v", resp)
	}
}

func TestCodexParseTurnCompletedUsage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	fake := filepath.Join(t.TempDir(), "codex-usage")
	script := `#!/bin/sh
printf '%s\n' '{"type":"thread.started","thread_id":"t1"}'
printf '%s\n' '{"type":"item.completed","item":{"type":"agent_message","text":"Done"}}'
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":20,"output_tokens":10}}'
exit 0
`
	_ = os.WriteFile(fake, []byte(script), 0o755)
	a := NewCodexAdapter(CLIAgentConfig{
		DataRoot: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake)
		},
	})
	a.detect()
	resp, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "u2",
		Messages: []WireMessage{{Role: "user", Content: "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.TokenTotal != 30 {
		t.Fatalf("tokens %d", resp.TokenTotal)
	}
}

func TestCodexArgs_NoAskForApproval_AllModes(t *testing.T) {
	a := NewCodexAdapter(CLIAgentConfig{DataTemp: t.TempDir()})
	cases := []struct {
		mode      string
		exec, res []string
	}{
		{"", []string{"--sandbox", "read-only", "-c", `approval_policy="never"`}, []string{"-c", `sandbox_mode="read-only"`, "-c", `approval_policy="never"`}},
		{"bypass", []string{"--dangerously-bypass-approvals-and-sandbox"}, []string{"--dangerously-bypass-approvals-and-sandbox"}},
		{"read-only", []string{"--sandbox", "read-only", "-c", `approval_policy="never"`}, []string{"-c", `sandbox_mode="read-only"`, "-c", `approval_policy="never"`}},
		{"workspace-write", []string{"--sandbox", "workspace-write", "-c", `approval_policy="never"`}, []string{"-c", `sandbox_mode="workspace-write"`, "-c", `approval_policy="never"`}},
		// 6.19.18.4：approve-for-me 新开发 --approve-for-me；续聊退回 workspace-write。
		{"approve-for-me", []string{"--approve-for-me"}, []string{"-c", `sandbox_mode="workspace-write"`, "-c", `approval_policy="never"`}},
		{"approve", []string{"--dangerously-bypass-approvals-and-sandbox"}, []string{"--dangerously-bypass-approvals-and-sandbox"}},
		{"full", []string{"--dangerously-bypass-approvals-and-sandbox"}, []string{"--dangerously-bypass-approvals-and-sandbox"}},
		{"plan", []string{"--sandbox", "read-only", "-c", `approval_policy="never"`}, []string{"-c", `sandbox_mode="read-only"`, "-c", `approval_policy="never"`}},
		{"default", []string{"--sandbox", "read-only", "-c", `approval_policy="never"`}, []string{"-c", `sandbox_mode="read-only"`, "-c", `approval_policy="never"`}},
		{"normal", []string{"--sandbox", "read-only", "-c", `approval_policy="never"`}, []string{"-c", `sandbox_mode="read-only"`, "-c", `approval_policy="never"`}},
		{"auto", []string{"--approve-for-me"}, []string{"-c", `sandbox_mode="workspace-write"`, "-c", `approval_policy="never"`}},
	}
	contains := func(args, sub []string) bool {
		for i := 0; i+len(sub) <= len(args); i++ {
			ok := true
			for j := range sub {
				if args[i+j] != sub[j] {
					ok = false
					break
				}
			}
			if ok {
				return true
			}
		}
		return false
	}
	for _, c := range cases {
		for _, resume := range []bool{false, true} {
			args := a.buildCodexArgs(TurnOptions{PermissionMode: c.mode, ModelID: "gpt-6-astra"}, "你好", "/proj", "sess-1", resume)
			for _, bad := range []string{"--ask-for-approval", "-a"} {
				for _, x := range args {
					if x == bad {
						t.Fatalf("mode %q resume=%v has %s: %q", c.mode, resume, bad, args)
					}
				}
			}
			want := c.exec
			if resume {
				want = c.res
				if args[1] != "resume" || contains(args, []string{"--sandbox"}) || contains(args, []string{"-C"}) {
					t.Fatalf("resume args invalid for exec resume: %q", args)
				}
				if args[len(args)-2] != "sess-1" {
					t.Fatalf("resume session id position: %q", args)
				}
			}
			if !contains(args, want) {
				t.Fatalf("mode %q resume=%v want %q in %q", c.mode, resume, want, args)
			}
		}
	}
}
