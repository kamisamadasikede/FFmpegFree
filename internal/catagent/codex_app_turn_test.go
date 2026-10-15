package catagent

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A real subprocess exercises stdio framing, early notifications, rejection,
// cleanup and resume, without sending requests to any AI provider.
func TestCodexAppServerHelper(t *testing.T) {
	if os.Getenv("FFC_TEST_APP_SERVER") != "1" {
		return
	}
	w := json.NewEncoder(os.Stdout)
	notify := func(method string, params any) { _ = w.Encode(map[string]any{"method": method, "params": params}) }
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var m struct {
			ID     json.RawMessage            `json:"id"`
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			os.Exit(21)
		}
		result := any(map[string]any{})
		switch m.Method {
		case "initialize":
			for i := 0; i < 300; i++ {
				notify("server/status", map[string]any{"i": i})
			}
		case "initialized":
			continue
		case "thread/start", "thread/resume":
			if string(m.Params["developerInstructions"]) != `"internal preview credential"` || string(m.Params["sandbox"]) != `"workspace-write"` {
				os.Exit(22)
			}
			if m.Method == "thread/resume" && string(m.Params["threadId"]) != `"thr-test"` {
				os.Exit(23)
			}
			result = map[string]any{"thread": map[string]any{"id": "thr-test"}, "modelProvider": os.Getenv("FFC_TEST_APP_PROVIDER")}
		case "turn/start":
			if strings.Contains(string(m.Params["input"]), "credential") || !strings.Contains(string(m.Params["input"]), "你好") {
				os.Exit(24)
			}
			result = map[string]any{"turn": map[string]any{"id": "turn-test"}}
		case "turn/steer":
			if string(m.Params["expectedTurnId"]) != `"turn-test"` {
				os.Exit(25)
			}
			if strings.Contains(string(m.Params["input"]), "reject") {
				_ = w.Encode(map[string]any{"id": m.ID, "error": map[string]any{"code": -1, "message": "turn rejected"}})
				continue
			}
		default:
			os.Exit(26)
		}
		_ = w.Encode(map[string]any{"id": m.ID, "result": result})
		if m.Method == "turn/start" {
			if mode := os.Getenv("FFC_TEST_APP_ERROR"); mode != "" {
				notify("error", map[string]any{"threadId": "other", "turnId": "turn-test", "error": map[string]any{"message": "unrelated"}, "willRetry": false})
				notify("error", map[string]any{"threadId": "thr-test", "turnId": "turn-test", "error": map[string]any{"message": "Reconnecting...", "additionalDetails": "gateway unavailable secret-key bridge-token"}, "willRetry": true})
				if mode == "fatal" {
					notify("error", map[string]any{"threadId": "thr-test", "turnId": "turn-test", "error": map[string]any{"message": "gateway failed"}, "willRetry": false})
				} else {
					notify("item/agentMessage/delta", map[string]any{"threadId": "thr-test", "turnId": "turn-test", "itemId": "a", "delta": "recovered"})
					notify("turn/completed", map[string]any{"threadId": "thr-test", "turn": map[string]any{"id": "turn-test", "status": "completed"}})
				}
				continue
			}
			notify("item/agentMessage/delta", map[string]any{"threadId": "thr-test", "turnId": "turn-test", "itemId": "a", "delta": "你好"})
		}
		if m.Method == "turn/steer" {
			notify("item/agentMessage/delta", map[string]any{"threadId": "thr-test", "turnId": "turn-test", "itemId": "a", "delta": "，已收到补充"})
			notify("item/completed", map[string]any{"threadId": "thr-test", "turnId": "turn-test", "item": map[string]any{"id": "a", "type": "agentMessage", "text": "你好，已收到补充"}})
			notify("thread/tokenUsage/updated", map[string]any{"threadId": "thr-test", "tokenUsage": map[string]any{"last": map[string]any{"totalTokens": 35, "outputTokens": 5, "inputTokens": 30}, "modelContextWindow": 128000}})
			notify("turn/completed", map[string]any{"threadId": "thr-test", "turn": map[string]any{"id": "turn-test", "status": "completed"}})
		}
	}
	os.Exit(0)
}

func TestCodexAppTurnReportsRetryAndFatal(t *testing.T) {
	for _, mode := range []string{"fatal", "recover"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("FFC_TEST_APP_SERVER", "1")
			t.Setenv("FFC_TEST_APP_ERROR", mode)
			a := NewCodexAdapter(CLIAgentConfig{DataRoot: t.TempDir(), Exec: func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				if !strings.Contains(strings.Join(args, " "), "features.unbounded_connection_retries=false") {
					t.Error("headless reconnects are not bounded")
				}
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCodexAppServerHelper$")
			}})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var retry, fatal string
			resp, err := a.runCodexAppTurn(TurnOptions{Ctx: ctx, ProjectPath: t.TempDir(), Rules: "internal preview credential", PermissionMode: ModeCodexWorkspace, Provider: ProviderOverlay{ApiKey: "secret-key"}, ToolEnv: map[string]string{"FFC_OFFICE_TOKEN": "bridge-token"}, Messages: []WireMessage{{Role: "user", Content: "你好"}}, OnActivity: func(kind, _, text string, _ bool) {
				if kind == "retry" {
					retry = text
				}
			}, OnError: func(text string) { fatal = text }}, "fake")
			if !strings.Contains(retry, "gateway unavailable") || strings.Contains(retry, "secret-key") || strings.Contains(retry, "bridge-token") {
				t.Fatalf("retry not safely reported: %q", retry)
			}
			if mode == "fatal" {
				if err == nil || fatal != "gateway failed" {
					t.Fatalf("fatal=%q err=%v", fatal, err)
				}
			} else if err != nil || resp.Message.Content != "recovered" || fatal != "" {
				t.Fatalf("recovery %+v %v fatal=%q", resp, err, fatal)
			}
		})
	}
}

func TestCodexAppResumeUsesSelectedProvider(t *testing.T) {
	p := codexAppThreadParams(TurnOptions{Provider: ProviderOverlay{Enabled: true, AgentKind: KindCodexCLI, ApiURL: "https://gateway.invalid"}}, t.TempDir())
	if p["modelProvider"] != CodexProviderID {
		t.Fatal("resumed thread may retain its old provider", p)
	}
	p = codexAppThreadParams(TurnOptions{}, t.TempDir())
	if _, ok := p["modelProvider"]; ok {
		t.Fatal("native CLI provider overridden")
	}
}

func TestCodexAppSelectsExecBeforeSendingPrompt(t *testing.T) {
	t.Setenv("FFC_TEST_APP_SERVER", "1")
	t.Setenv("FFC_TEST_APP_PROVIDER", "custom")
	a := NewCodexAdapter(CLIAgentConfig{DataRoot: t.TempDir(), Exec: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCodexAppServerHelper$")
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	delta, ready := false, false
	_, err := a.runCodexAppTurn(TurnOptions{Ctx: ctx, ConversationID: "c", ProjectPath: t.TempDir(), Rules: "internal preview credential", PermissionMode: ModeCodexWorkspace, Messages: []WireMessage{{Role: "user", Content: "你好"}}, OnTextDelta: func(string) { delta = true }, OnSteerReady: func(f SteerFunc) { ready = f != nil }}, "fake")
	if err != errCodexExecTransport || delta || ready || a.loadSessionID("c") != "" {
		t.Fatalf("input submitted or session replaced before transport selection: %v", err)
	}
}

func TestCodexAppTurnNativeSteerAndResume(t *testing.T) {
	t.Setenv("FFC_TEST_APP_SERVER", "1")
	a := NewCodexAdapter(CLIAgentConfig{DataRoot: t.TempDir(), Exec: func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		if len(args) < 3 || args[0] != "app-server" || args[2] != "stdio://" {
			t.Errorf("args %v", args)
		}
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCodexAppServerHelper$")
	}})
	for run := 0; run < 2; run++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var live SteerFunc
		var text strings.Builder
		cleared := false
		r, err := a.runCodexAppTurn(TurnOptions{Ctx: ctx, ConversationID: "conv", ProjectPath: t.TempDir(), Rules: "internal preview credential", PermissionMode: ModeCodexWorkspace, Messages: []WireMessage{{Role: "user", Content: "你好"}}, OnTextDelta: func(s string) { text.WriteString(s) }, OnSteerReady: func(f SteerFunc) {
			if f == nil {
				cleared = true
				return
			}
			live = f
			if err := f(ctx, "reject", nil); err == nil {
				t.Error("rejected steer accepted")
			}
			if err := f(ctx, "补充", nil); err != nil {
				t.Error(err)
			}
		}}, "fake")
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if !cleared || r.Message.Content != "你好，已收到补充" || text.String() != r.Message.Content || r.TokenTotal != 35 || r.ContextWindow != 128000 {
			t.Fatalf("%+v text=%q cleared=%v", r, text.String(), cleared)
		}
		if a.loadSessionID("conv") != "thr-test" {
			t.Fatal("session not persisted")
		}
		if err := live(context.Background(), "too late", nil); err == nil {
			t.Fatal("steer after exit accepted")
		}
	}
}

func TestCodexAppTurnCancelClosesProcess(t *testing.T) {
	t.Setenv("FFC_TEST_APP_SERVER", "1")
	a := NewCodexAdapter(CLIAgentConfig{DataRoot: t.TempDir(), Exec: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCodexAppServerHelper$")
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := a.runCodexAppTurn(TurnOptions{Ctx: ctx, ConversationID: "c", ProjectPath: t.TempDir(), Rules: "internal preview credential", PermissionMode: ModeCodexWorkspace, Messages: []WireMessage{{Role: "user", Content: "你好"}}, OnSteerReady: func(f SteerFunc) {
		if f != nil {
			cancel()
		}
	}}, "fake")
	if err == nil || ctx.Err() == nil {
		t.Fatalf("%v", err)
	}
}

func TestCodexRPCReplyBeforeEOF(t *testing.T) {
	for i := 0; i < 25; i++ {
		reader, writer := io.Pipe()
		r := newCodexRPC(reader, io.Discard)
		go func() { _, _ = io.WriteString(writer, "{\"id\":1,\"result\":{\"ok\":true}}\n"); _ = writer.Close() }()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		var got struct {
			OK bool `json:"ok"`
		}
		err := r.call(ctx, "probe", nil, &got)
		cancel()
		if err != nil || !got.OK {
			t.Fatalf("%+v %v", got, err)
		}
		for range r.events {
		}
	}
}

func TestCodexAppInputSeparatesImagesAndUserText(t *testing.T) {
	p := filepath.Join(t.TempDir(), "image.png")
	_ = os.WriteFile(p, []byte("image"), 0600)
	input, err := codexAppInput("你好", []PromptFile{{Path: p}})
	if err != nil || len(input) != 2 || input[0]["text"] != "你好" || input[1]["type"] != "localImage" {
		t.Fatalf("%+v %v", input, err)
	}
}
