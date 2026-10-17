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

func TestLiveTeamMCPRoundTrip(t *testing.T) {
	app := os.Getenv("FFC_TEST_TEAM_APP")
	if os.Getenv("FFC_TEST_LIVE_CLI") != "1" || app == "" {
		t.Skip("requires opt-in and a built FFC application")
	}
	for _, kind := range []string{KindCodexCLI, KindClaudeCLI, KindCatBuild} {
		t.Run(kind, func(t *testing.T) {
			home, root := t.TempDir(), t.TempDir()
			var calls, tools atomic.Int32
			bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/tools/team_members" || r.Header.Get("Authorization") != "Bearer local-capability" {
					t.Errorf("unexpected team request %s", r.URL.Path)
					w.WriteHeader(403)
					return
				}
				tools.Add(1)
				fmt.Fprint(w, `{"members":[{"memberId":"owner","kind":"human","name":"负责人"}]}`)
			}))
			defer bridge.Close()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					fmt.Fprint(w, `{"data":[{"id":"local-model","object":"model"}]}`)
					return
				}
				if strings.Contains(r.URL.Path, "count_tokens") {
					fmt.Fprint(w, `{"input_tokens":10}`)
					return
				}
				var request map[string]any
				_ = json.NewDecoder(r.Body).Decode(&request)
				name := ""
				namespace := ""
				if list, ok := request["tools"].([]any); ok {
					for _, item := range list {
						v, _ := item.(map[string]any)
						n, _ := v["name"].(string)
						if f, ok := v["function"].(map[string]any); ok {
							n, _ = f["name"].(string)
						}
						if strings.Contains(n, "team_members") {
							name = n
						}
						if sub, ok := v["tools"].([]any); ok {
							for _, entry := range sub {
								f, _ := entry.(map[string]any)
								if f["name"] == "team_members" {
									name = "team_members"
									namespace = n
								}
							}
						}
					}
				}
				first := calls.Add(1) == 1
				if kind == KindCatBuild {
					if list, ok := request["tools"].([]any); ok {
						for _, entry := range list {
							v, _ := entry.(map[string]any)
							f, _ := v["function"].(map[string]any)
							if f["name"] == "use_tool" || v["name"] == "use_tool" {
								name = "use_tool"
							}
						}
					}
				}
				if kind == KindCatBuild {
					if list, ok := request["tools"].([]any); ok && len(list) == 1 {
						v, _ := list[0].(map[string]any)
						f, _ := v["function"].(map[string]any)
						if f["name"] == "session_title" || v["name"] == "session_title" {
							calls.Add(-1)
							first = false
						}
					}
				}
				if first && name == "" {
					names := []string{}
					var walk func(any)
					walk = func(value any) {
						switch v := value.(type) {
						case []any:
							for _, item := range v {
								walk(item)
							}
						case map[string]any:
							if n, ok := v["name"].(string); ok {
								names = append(names, n)
							}
							for _, key := range []string{"tools", "function", "functions"} {
								walk(v[key])
							}
						}
					}
					walk(request["tools"])
					t.Errorf("team_members absent from native model tools: %v", names)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				write := func(v any) { b, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", b) }
				if kind == KindClaudeCLI {
					event := func(typ string, v map[string]any) {
						v["type"] = typ
						b, _ := json.Marshal(v)
						fmt.Fprintf(w, "event: %s\ndata: %s\n\n", typ, b)
					}
					event("message_start", map[string]any{"message": map[string]any{"id": "local", "type": "message", "role": "assistant", "model": "local-model", "content": []any{}, "stop_reason": nil, "usage": map[string]int{"input_tokens": 10, "output_tokens": 0}}})
					stop := "end_turn"
					if first {
						event("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "tool_use", "id": "call1", "name": name, "input": map[string]any{}}})
						event("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": "{}"}})
						stop = "tool_use"
					} else {
						event("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
						event("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "text_delta", "text": "roundtrip OK"}})
					}
					event("content_block_stop", map[string]any{"index": 0})
					event("message_delta", map[string]any{"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": map[string]int{"output_tokens": 3}})
					event("message_stop", map[string]any{})
					return
				}
				if kind == KindCatBuild {
					delta := map[string]any{"role": "assistant", "content": "roundtrip OK"}
					stop := "stop"
					if first {
						delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call1", "type": "function", "function": map[string]string{"name": name, "arguments": `{"tool_name":"ffc_team__team_members","tool_input":{}}`}}}}
						stop = "tool_calls"
					}
					write(map[string]any{"id": "local", "object": "chat.completion.chunk", "created": 1, "model": "local-model", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
					write(map[string]any{"id": "local", "object": "chat.completion.chunk", "created": 1, "model": "local-model", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": stop}}})
					fmt.Fprint(w, "data: [DONE]\n\n")
					return
				}
				item := map[string]any{"id": "msg", "type": "message", "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "roundtrip OK", "annotations": []any{}}}}
				if first {
					item = map[string]any{"id": "fc1", "type": "function_call", "status": "completed", "call_id": "call1", "name": name, "arguments": "{}"}
					if namespace != "" {
						item["namespace"] = namespace
					}
				}
				for _, ev := range []map[string]any{{"type": "response.created", "response": map[string]any{"id": "local", "object": "response", "status": "in_progress", "output": []any{}}}, {"type": "response.output_item.added", "output_index": 0, "item": item}, {"type": "response.output_item.done", "output_index": 0, "item": item}, {"type": "response.completed", "response": map[string]any{"id": "local", "object": "response", "status": "completed", "model": "local-model", "output": []any{item}, "usage": map[string]int{"input_tokens": 10, "output_tokens": 3, "total_tokens": 13}}}} {
					b, _ := json.Marshal(ev)
					fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev["type"], b)
				}
			}))
			defer api.Close()
			t.Setenv("NO_PROXY", "localhost,127.0.0.1")
			t.Setenv("no_proxy", "localhost,127.0.0.1")
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			opts := TurnOptions{Ctx: ctx, ConversationID: "team-mcp-local", ProjectPath: root, ModelID: "local-model", LiveSteer: true, Provider: ProviderOverlay{Enabled: true, AgentKind: kind, ApiURL: api.URL + "/v1", ApiKey: "local-test-only"}, ToolEnv: map[string]string{"FFC_TEAM_URL": bridge.URL, "FFC_TEAM_TOKEN": "local-capability", "FFC_TEAM_CLI": app}, Messages: []WireMessage{{Role: "user", Content: "Read current team members then reply briefly"}}}
			opts.OnActivity = func(kind, key, text string, replace bool) {
				if key == "permission" {
					t.Log(text)
				}
			}
			var run func(TurnOptions) (TurnResponse, error)
			name := "codex"
			switch kind {
			case KindCodexCLI:
				t.Setenv("CODEX_HOME", home)
				opts.Transport = "app-server"
				opts.PermissionMode = ModeCodexReadOnly
			case KindClaudeCLI:
				name = "claude"
				t.Setenv("CLAUDE_CONFIG_DIR", home)
				t.Setenv("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1")
				opts.Transport = "stream-json"
				opts.PermissionMode = ModeClaudeManual
			case KindCatBuild:
				name = "grok"
				t.Setenv("GROK_HOME", home)
				opts.Transport = "stdio"
				opts.PermissionMode = ModeGrokDefault
				_ = os.WriteFile(filepath.Join(home, "config.toml"), []byte(fmt.Sprintf("[cli]\nuse_leader=false\n[model.local-model]\nmodel=\"local-model\"\nbase_url=%q\nenv_key=\"XAI_API_KEY\"\napi_backend=\"chat_completions\"\n", api.URL+"/v1")), 0600)
			}
			exe, e := exec.LookPath(name)
			if e != nil {
				t.Fatal(e)
			}
			if kind == KindCatBuild {
				a := NewBuildAdapter(BuildConfig{DataRoot: home})
				a.exe, a.st = exe, Status{State: StateReady}
				run = a.RunTurn
			} else {
				a := NewCodexAdapter(CLIAgentConfig{DataRoot: home, DataTemp: t.TempDir()})
				if kind == KindClaudeCLI {
					a = NewClaudeAdapter(CLIAgentConfig{DataRoot: home, DataTemp: t.TempDir()})
				}
				a.exe, a.st, a.version = exe, Status{State: StateReady}, "2.1.300"
				run = a.RunTurn
			}
			resp, e := run(opts)
			if e != nil || tools.Load() != 1 || !strings.Contains(resp.Message.Content, "roundtrip OK") {
				t.Fatalf("calls=%d tools=%d response=%q err=%v", calls.Load(), tools.Load(), resp.Message.Content, e)
			}
		})
	}
}
