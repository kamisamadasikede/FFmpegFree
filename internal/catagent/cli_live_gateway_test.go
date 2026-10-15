package catagent

import (
	"context"
	"encoding/json"
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

// Opt-in integration with installed CLIs, isolated homes and a local mock API.
// No provider credentials, paid requests, or user configuration are involved.
func TestLiveCLIGateway(t *testing.T) {
	if os.Getenv("FFC_TEST_LIVE_CLI") != "1" {
		t.Skip("set FFC_TEST_LIVE_CLI=1 to exercise installed CLIs")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	wrapper, err := filepath.Abs("../service/cat/pptskill/scripts/office.cjs")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{KindCodexCLI, KindCatBuild} {
		t.Run(kind, func(t *testing.T) {
			name := "codex"
			if kind == KindCatBuild {
				name = "grok"
			}
			exe, err := exec.LookPath(name)
			if err != nil {
				t.Fatal(err)
			}
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"data":[{"id":"local-model","object":"model","owned_by":"local"}]}`)
					return
				}
				if r.Header.Get("Authorization") != "Bearer local-test-only" {
					t.Errorf("provider key not forwarded")
				}
				requests.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				if strings.HasSuffix(r.URL.Path, "/chat/completions") {
					for _, chunk := range []string{`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"local-model","choices":[{"index":0,"delta":{"role":"assistant","content":"local gateway OK"},"finish_reason":null}]}`, `{"id":"c1","object":"chat.completion.chunk","created":1,"model":"local-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, `[DONE]`} {
						fmt.Fprintf(w, "data: %s\n\n", chunk)
					}
					return
				}
				item := map[string]any{"id": "msg_test", "type": "message", "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "local gateway OK", "annotations": []any{}}}}
				response := map[string]any{"id": "resp_test", "object": "response", "created_at": 1, "status": "completed", "model": "local-model", "output": []any{item}, "usage": map[string]any{"input_tokens": 10, "output_tokens": 3, "total_tokens": 13}}
				for _, event := range []map[string]any{
					{"type": "response.created", "response": map[string]any{"id": "resp_test", "object": "response", "status": "in_progress", "output": []any{}}},
					{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"id": "msg_test", "type": "message", "role": "assistant", "content": []any{}}},
					{"type": "response.content_part.added", "output_index": 0, "item_id": "msg_test", "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}},
					{"type": "response.output_text.delta", "output_index": 0, "item_id": "msg_test", "content_index": 0, "delta": "local gateway OK"},
					{"type": "response.output_item.done", "output_index": 0, "item": item},
					{"type": "response.completed", "response": response},
				} {
					b, _ := json.Marshal(event)
					fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], b)
				}
			}))
			defer server.Close()
			home, root := t.TempDir(), t.TempDir()
			mcp := fmt.Sprintf("[mcp_servers.ffc_ffc-office]\ncommand=%q\nargs=[%q,\"--mcp\"]\n", node, wrapper)
			config := mcp
			if kind == KindCatBuild {
				t.Setenv("GROK_HOME", home)
				config = fmt.Sprintf("[cli]\nuse_leader=false\n[model.local-model]\nmodel=\"local-model\"\nbase_url=%q\nenv_key=\"XAI_API_KEY\"\napi_backend=\"chat_completions\"\n", server.URL+"/v1") + mcp
			} else {
				t.Setenv("CODEX_HOME", home)
			}
			t.Setenv("NO_PROXY", "localhost,127.0.0.1")
			t.Setenv("no_proxy", "localhost,127.0.0.1")
			if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			opts := TurnOptions{Ctx: ctx, ConversationID: "local-gateway-conversation", ProjectPath: root, ModelID: "local-model", PermissionMode: ModeCodexReadOnly, Rules: "Reply briefly. Internal Office tools are available; do not call them for this test.", Provider: ProviderOverlay{Enabled: true, AgentKind: kind, ApiURL: server.URL + "/v1", ApiKey: "local-test-only"}, ToolEnv: map[string]string{"FFC_OFFICE_TOKEN": "local-bridge-token", "FFC_OFFICE_ENDPOINT": server.URL + "/office"}, Messages: []WireMessage{{Role: "user", Content: "Reply OK"}}}
			var run func(TurnOptions) (TurnResponse, error)
			if kind == KindCodexCLI {
				a := NewCodexAdapter(CLIAgentConfig{DataRoot: home})
				a.exe, a.st = exe, Status{State: StateReady}
				opts.LiveSteer = true
				run = a.RunTurn
			} else {
				a := NewBuildAdapter(BuildConfig{DataRoot: home, SessionsRoot: filepath.Join(home, "sessions")})
				a.exe, a.st = exe, Status{State: StateReady}
				opts.PermissionMode = "dontAsk"
				run = a.RunTurn
			}
			for i := 0; i < 2; i++ {
				before := requests.Load()
				resp, err := run(opts)
				if err != nil || resp.Message.Content != "local gateway OK" || requests.Load() <= before {
					t.Fatalf("run %d: response=%+v error=%v requests=%d", i, resp, err, requests.Load())
				}
				opts.Resume = true
			}
		})
	}
}
