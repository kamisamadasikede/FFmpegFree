package catagent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

func TestDetect_ReadyWhenGrokOnPATH(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "grok")
	if runtime.GOOS == "windows" {
		fake += ".exe"
	}
	_ = os.WriteFile(fake, []byte("x"), 0o755)
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(file string) (string, error) {
			if file == "grok" || file == "grok.exe" {
				return fake, nil
			}
			return "", exec.ErrNotFound
		},
	})
	a.detect()
	st := a.Status()
	if st.State != StateReady || st.Version != "path" {
		t.Fatalf("status %+v", st)
	}
	models, err := a.ListModels()
	// 假二进制跑不起 `grok models` → 解析失败 → 空列表（无写死回退）。
	if err != nil || models == nil || len(models) != 0 {
		t.Fatalf("models %v %v", models, err)
	}
	thinks, err := a.ListThinkLevels()
	if err != nil || len(thinks) != 4 {
		t.Fatalf("thinks %v %v", thinks, err)
	}
}

func TestDetect_MissingWithoutGrok(t *testing.T) {
	a := NewBuildAdapter(BuildConfig{
		ComponentDir: t.TempDir(),
		DataTemp:     t.TempDir(),
		LookPath:     func(string) (string, error) { return "", exec.ErrNotFound },
	})
	a.detect()
	st := a.Status()
	if st.State != StateMissing || st.CanDownload || st.Error == nil || st.Error.Message != MsgNotReady {
		t.Fatalf("%+v", st)
	}
	_, err := a.RunTurn(TurnOptions{Ctx: context.Background(), Messages: []WireMessage{{Role: "user", Content: "x"}}})
	if !apperr.Is(err, apperr.CatNotReady) {
		t.Fatalf("%v", err)
	}
}

func TestRunTurn_StreamingJSON(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "grok-fake")
	script := `#!/bin/sh
# echo NDJSON text deltas then end
printf '%s\n' '{"type":"text","data":"你"}'
printf '%s\n' '{"type":"text","data":"好"}'
printf '%s\n' '{"type":"thought","data":"ignore"}'
printf '%s\n' '{"type":"end","stopReason":"end_turn","sessionId":"s1"}'
exit 0
`
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake is unix-only; LookPath+Exec covered on Windows via other tests")
	}
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var deltas []string
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake)
		},
	})
	a.detect()
	if a.Status().State != StateReady {
		t.Fatalf("%+v", a.Status())
	}
	resp, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "c1",
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
}

func TestRunTurn_CLIErrorEvent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	fake := filepath.Join(t.TempDir(), "grok-err")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"error\",\"message\":\"boom\"}'\nexit 0\n"
	_ = os.WriteFile(fake, []byte(script), 0o755)
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake)
		},
	})
	a.detect()
	_, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "c1",
		Messages: []WireMessage{{Role: "user", Content: "x"}},
		Timeout:  3 * time.Second,
	})
	ae := apperr.From(err)
	if !apperr.Is(err, apperr.CatReplyFailed) || ae == nil || !strings.Contains(ae.Message, "boom") {
		t.Fatalf("%v", err)
	}
}

func TestRunTurn_PassesArgsWithoutToolLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	t.Setenv("GROK_SANDBOX", "")
	t.Setenv("GROK_WRITE_FILE", "")
	dir := t.TempDir()
	fake := filepath.Join(dir, "grok-args")
	out := filepath.Join(dir, "args.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$0\" \"$@\" > '" + out + "'\nprev=\nfor a in \"$@\"; do\n  if [ \"$prev\" = \"--prompt-file\" ]; then body=$(cat \"$a\"); printf 'PROMPT:%s' \"$body\" >> '" + out + "'; fi\n  prev=$a\ndone\necho \"ENV_SANDBOX=$GROK_SANDBOX\" >> '" + out + "'\necho \"ENV_WRITE=$GROK_WRITE_FILE\" >> '" + out + "'\nprintf '%s\\n' '{\"type\":\"text\",\"data\":\"ok\"}'\nprintf '%s\\n' '{\"type\":\"end\",\"stopReason\":\"end_turn\"}'\n"
	_ = os.WriteFile(fake, []byte(script), 0o755)
	proj := filepath.Join(dir, "proj")
	_ = os.MkdirAll(proj, 0o755)
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	a.detect()
	_, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "conv-abc",
		ProjectPath: proj, ModelID: "grok-build",
		Messages: []WireMessage{{Role: "user", Content: "hello world"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{"--prompt-file", "--output-format", "streaming-json", "--cwd", proj, "--session-id", "-m", "grok-build", "--no-auto-update", "--always-approve"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "\n-p\n") || strings.Contains(s, "\nhello world\n") {
		t.Fatalf("prompt must stay in the file, not on the command line:\n%s", s)
	}
	if !strings.Contains(s, "PROMPT:hello world") {
		t.Fatalf("prompt file was not passed:\n%s", s)
	}
	for _, banned := range []string{"--tools", "--disallowed-tools", "--deny", "--no-subagents", "--disable-web-search", "ENV_SANDBOX=read-only", "ENV_WRITE=0"} {
		if strings.Contains(s, banned) {
			t.Fatalf("must not pass %q:\n%s", banned, s)
		}
	}
	if !strings.Contains(s, "--session-id") {
		t.Fatalf("must pass --session-id:\n%s", s)
	}
}

func TestHandleToolRequests_RefuseWrite(t *testing.T) {
	res := HandleToolRequests("/tmp", []ToolRequest{{ID: "1", Kind: "write_file", Path: "a.txt", Content: "x"}})
	if len(res) != 1 || res[0].OK || res[0].Content != MsgToolWriteRef {
		t.Fatalf("%+v", res)
	}
}

func TestHandleToolRequests_ListDir(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644)
	res := HandleToolRequests(dir, []ToolRequest{{ID: "1", Kind: "list_dir", Path: "."}})
	if len(res) != 1 || !res[0].OK || res[0].Content == "" {
		t.Fatalf("%+v", res)
	}
}

func TestScanStreamingJSON_Lenient(t *testing.T) {
	var texts, thoughts, tools []string
	var errMsg string
	in := strings.NewReader(strings.Join([]string{
		`{"type":"text","data":"A"}`,
		`not-json`,
		`{"type":"thought","data":"x"}`,
		`{"type":"tool_call","name":"read_file","title":"读取文件"}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_message_chunk","content":{"text":"B"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_thought_chunk","content":{"text":"想"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"tool_call","toolCallId":"t1","title":"grep","status":"in_progress"}}}`,
		`{"type":"error","message":"nope"}`,
		`{"type":"end","stopReason":"end_turn"}`,
	}, "\n"))
	if err := scanStreamingJSON(in, func(p streamPiece) {
		switch p.Kind {
		case "thought":
			thoughts = append(thoughts, p.Text)
		case "tool":
			tools = append(tools, p.Text)
		default:
			texts = append(texts, p.Text)
		}
	}, func(m string) { errMsg = m }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(texts, "") != "AB" || strings.Join(thoughts, "") != "x想" || strings.Join(tools, ",") != "读取文件,grep" || errMsg != "nope" {
		t.Fatalf("text=%v thought=%v tool=%v err=%q", texts, thoughts, tools, errMsg)
	}
}

func TestScanStreamingJSON_ToolCallFiles(t *testing.T) {
	var tools []string
	in := strings.NewReader(strings.Join([]string{
		`{"type":"tool_call","toolCallId":"call_1","toolName":"read_file","kind":"read","status":"in_progress","rawInput":{"path":"src/main.rs"},"locations":[]}`,
		`{"type":"tool_call_update","toolCallId":"call_1","status":"completed","locations":[{"path":"src/main.rs"},{"path":"src/lib.rs"}]}`,
		`{"type":"tool_call","toolCallId":"call_2","toolName":"grep","status":"in_progress","rawInput":{"pattern":"foo","path":"src"}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"tool_call","toolCallId":"call_3","title":"Edit","kind":"edit","status":"completed","rawInput":{"file_path":"a.go"}}}}`,
	}, "\n"))
	if err := scanStreamingJSON(in, func(p streamPiece) {
		if p.Kind == "tool" {
			tools = append(tools, p.Text)
		}
	}, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if len(tools) != 4 {
		t.Fatalf("%v", tools)
	}
	if tools[0] != "read_file\nsrc/main.rs" {
		t.Fatalf("start %q", tools[0])
	}
	if tools[1] != "read_file\nsrc/main.rs\nsrc/lib.rs" {
		t.Fatalf("update %q", tools[1])
	}
	if tools[2] != "grep · foo\nsrc" {
		t.Fatalf("search %q", tools[2])
	}
	if tools[3] != "Edit\na.go" {
		t.Fatalf("edit %q", tools[3])
	}
}

func TestToolCallKeepsWriteAndEditDiff(t *testing.T) {
	line := func(update any) string {
		b, err := json.Marshal(map[string]any{"method": "session/update", "params": map[string]any{"update": update}})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	var tools []string
	in := strings.NewReader(strings.Join([]string{
		line(map[string]any{
			"sessionUpdate": "tool_call", "toolCallId": "w1", "title": "write",
			"rawInput": map[string]any{"file_path": "a.yml", "content": "name: nginx\nports:\n"},
		}),
		line(map[string]any{
			"sessionUpdate": "tool_call_update", "toolCallId": "w1", "kind": "edit", "title": "Write `a.yml`",
			"content":   []any{map[string]any{"type": "diff", "path": "a.yml", "oldText": "", "newText": "name: nginx\n"}},
			"locations": []any{map[string]any{"path": "a.yml"}},
		}),
		line(map[string]any{
			"sessionUpdate": "tool_call", "toolCallId": "e1", "title": "search_replace",
			"rawInput": map[string]any{"file_path": "deploy.ps1", "old_string": "HTTP_PORT=8080", "new_string": "HTTP_PORT=8081"},
		}),
		line(map[string]any{
			"sessionUpdate": "tool_call", "toolCallId": "r1", "title": "read_file",
			"rawInput": map[string]any{"target_file": "deploy.ps1", "content": "should-not-show"},
		}),
	}, "\n"))
	if err := scanStreamingJSON(in, func(p streamPiece) {
		if p.Kind == "tool" {
			tools = append(tools, p.Text)
		}
	}, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if len(tools) != 4 {
		t.Fatalf("%v", tools)
	}
	if tools[0] != "write\na.yml\n\n+name: nginx\n+ports:" {
		t.Fatalf("write %q", tools[0])
	}
	if tools[1] != "Write `a.yml`\na.yml\n\n+name: nginx" {
		t.Fatalf("write update %q", tools[1])
	}
	if tools[2] != "search_replace\ndeploy.ps1\n\n-HTTP_PORT=8080\n+HTTP_PORT=8081" {
		t.Fatalf("edit %q", tools[2])
	}
	if tools[3] != "read_file\ndeploy.ps1" || strings.Contains(tools[3], "should-not-show") {
		t.Fatalf("read %q", tools[3])
	}
}

func TestPermissionCLIArgs(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ban  string
	}{
		{"", "--always-approve", "--permission-mode"},
		{"approve", "--always-approve", "--permission-mode"},
		{"always-approve", "--always-approve", "--permission-mode"},
		{"full", "--always-approve", "--permission-mode"},
		{"plan", "--permission-mode\x00plan", "--always-approve"},
		{"auto", "--permission-mode\x00auto", "--always-approve"},
		{"auto-review", "--permission-mode\x00auto", "--always-approve"},
		{"normal", "--permission-mode\x00default", "--always-approve"},
		{"ask", "--permission-mode\x00default", "--always-approve"},
		{"default", "--permission-mode\x00default", "--always-approve"},
	}
	for _, tc := range cases {
		got := strings.Join(permissionCLIArgs(tc.in), "\x00")
		if got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
		if strings.Contains(got, tc.ban) {
			t.Fatalf("%q still has %s: %q", tc.in, tc.ban, got)
		}
	}
}

func TestScanStreamingJSON_ResultError(t *testing.T) {
	var errMsg string
	var texts []string
	in := strings.NewReader(strings.Join([]string{
		`{"type":"text","data":"已写到一半"}`,
		`{"type":"result","subtype":"error_during_execution","is_error":true,"errors":["network timeout"]}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"不要当成失败"}`,
	}, "\n"))
	if err := scanStreamingJSON(in, func(p streamPiece) {
		if p.Kind == "" || p.Kind == "text" {
			texts = append(texts, p.Text)
		}
	}, func(m string) { errMsg = m }); err != nil {
		t.Fatal(err)
	}
	if errMsg != "network timeout" || strings.Join(texts, "") != "已写到一半" {
		t.Fatalf("err=%q text=%v", errMsg, texts)
	}
}

func TestStderrSummary_TimeoutNotUpdater(t *testing.T) {
	strong, other := stderrSummary("new version available: 1.2.3\nread tcp 10.0.0.1:443: i/o timeout\n")
	if !strings.Contains(strong, "i/o timeout") {
		t.Fatalf("strong %q", strong)
	}
	if strings.Contains(strong, "new version") || strings.Contains(other, "new version") || strings.Contains(other, "i/o timeout") {
		t.Fatalf("strong %q other %q", strong, other)
	}
}

func TestScanStreamingJSON_RoundTrip(t *testing.T) {
	// 确保事件能被 json 编解码（防字段漂移）。
	lines := []streamEvent{
		{Type: "text", Data: "hi"},
		{Type: "end"},
	}
	for _, ev := range lines {
		b, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		var back streamEvent
		if err := json.Unmarshal(b, &back); err != nil || back.Type != ev.Type {
			t.Fatalf("%v %v", back, err)
		}
	}
}

func TestRunTurn_AuthFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	fake := filepath.Join(t.TempDir(), "grok-auth")
	script := `#!/bin/sh
printf '%s\n' '{"type":"error","message":"unauthorized: invalid api key"}'
exit 1
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake)
		},
	})
	a.detect()
	_, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "c1",
		Messages: []WireMessage{{Role: "user", Content: "x"}},
		Timeout:  3 * time.Second,
	})
	if !apperr.Is(err, apperr.CatNotReady) || apperr.From(err).Message != MsgAuthInvalid {
		t.Fatalf("%v", err)
	}
	if d := apperr.From(err).Detail; d == "" || !strings.Contains(d, "reason=auth") {
		t.Fatalf("detail %q", d)
	}
	st := a.Status()
	if st.State != StateFailed || st.Error == nil || st.Error.Message != MsgAuthInvalid {
		t.Fatalf("status %+v", st)
	}
}
