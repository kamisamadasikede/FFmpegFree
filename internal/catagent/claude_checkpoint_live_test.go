package catagent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Real installed CLI, isolated Claude settings and a local Anthropic-compatible
// stream. An echoed UUID acknowledges native receipt, not active-turn steering.
func TestLiveClaudeCheckpoint(t *testing.T) {
	if os.Getenv("FFC_TEST_LIVE_CLI") != "1" {
		t.Skip("opt-in installed CLI test")
	}
	exe, e := exec.LookPath("claude")
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1")
	t.Setenv("no_proxy", "localhost,127.0.0.1")
	first := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/messages") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"input_tokens":10}`)
			return
		}
		n := calls.Add(1)
		if n == 1 {
			close(first)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`{"type":"message_start","message":{"id":"local","type":"message","role":"assistant","model":"local-model","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"local checkpoint OK"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":3}}`,
			`{"type":"message_stop"}`,
		} {
			typ := strings.SplitN(strings.TrimPrefix(event, `{"type":"`), `"`, 2)[0]
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", typ, event)
		}
	}))
	defer server.Close()
	a := NewClaudeAdapter(CLIAgentConfig{DataRoot: t.TempDir(), DataTemp: t.TempDir()})
	a.exe, a.st, a.version = exe, Status{State: StateReady}, "2.1.300"
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	ready := make(chan SteerFunc, 1)
	mode := ""
	result := make(chan struct {
		r TurnResponse
		e error
	}, 1)
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".git"), 0700)
	go func() {
		r, e := a.RunTurn(TurnOptions{Ctx: ctx, ConversationID: "team-claude-local", ProjectPath: root, Transport: "stream-json", LiveSteer: true, PermissionMode: ModeClaudeManual, ModelID: "local-model", Provider: ProviderOverlay{Enabled: true, AgentKind: KindClaudeCLI, ApiURL: server.URL, ApiKey: "local-test-only"}, Messages: []WireMessage{{Role: "user", Content: "Reply briefly"}}, OnInputMode: func(s string) { mode = s }, OnSteerReady: func(f SteerFunc) {
			if f != nil {
				select {
				case ready <- f:
				default:
				}
			}
		}})
		result <- struct {
			r TurnResponse
			e error
		}{r, e}
	}()
	var steer SteerFunc
	select {
	case steer = <-ready:
	case out := <-result:
		close(release)
		t.Fatalf("before ready: %v", out.e)
	case <-ctx.Done():
		close(release)
		t.Fatal("native initialization timed out")
	}
	select {
	case <-first:
	case <-ctx.Done():
		close(release)
		t.Fatal("no local request")
	}
	accepted := make(chan error, 1)
	go func() { accepted <- steer(ctx, "Additional instruction", nil) }()
	// Receipt can be echoed at the native checkpoint after the first result.
	close(release)
	select {
	case e := <-accepted:
		if e != nil {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("no native input acknowledgement")
	}
	select {
	case out := <-result:
		if out.e != nil || mode != "checkpoint" || calls.Load() < 2 || strings.Count(out.r.Message.Content, "local checkpoint OK") != 2 {
			t.Fatalf("mode=%s calls=%d result=%+v err=%v", mode, calls.Load(), out.r, out.e)
		}
	case <-ctx.Done():
		t.Fatal("native result timed out")
	}
}
