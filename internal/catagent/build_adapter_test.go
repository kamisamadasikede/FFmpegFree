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
	if err != nil || len(models) != 1 || models[0].DisplayName != "Cat 助手" {
		t.Fatalf("models %v %v", models, err)
	}
	thinks, err := a.ListThinkLevels()
	if err != nil || len(thinks) != 0 {
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
		Exec:     func(ctx context.Context, name string, args ...string) *exec.Cmd { return exec.CommandContext(ctx, fake) },
	})
	a.detect()
	_, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "c1",
		Messages: []WireMessage{{Role: "user", Content: "x"}},
		Timeout:  3 * time.Second,
	})
	if !apperr.Is(err, apperr.CatReplyFailed) || apperr.From(err).Message != MsgReplyFailed {
		t.Fatalf("%v", err)
	}
}

func TestRunTurn_PassesArgsAndReadonlyEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "grok-args")
	out := filepath.Join(dir, "args.txt")
	script := "#!/bin/sh\necho \"$0\" \"$@\" > '" + out + "'\necho \"ENV_SANDBOX=$GROK_SANDBOX\" >> '" + out + "'\necho \"ENV_WRITE=$GROK_WRITE_FILE\" >> '" + out + "'\nprintf '%s\\n' '{\"type\":\"text\",\"data\":\"ok\"}'\nprintf '%s\\n' '{\"type\":\"end\",\"stopReason\":\"end_turn\"}'\n"
	_ = os.WriteFile(fake, []byte(script), 0o755)
	proj := filepath.Join(dir, "proj")
	_ = os.MkdirAll(proj, 0o755)
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec:     func(ctx context.Context, name string, args ...string) *exec.Cmd { return exec.CommandContext(ctx, name, args...) },
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
	for _, want := range []string{"-p", "hello world", "--output-format", "streaming-json", "--cwd", proj, "-s", "conv-abc", "-m", "grok-build", "--no-auto-update", "ENV_SANDBOX=read-only", "ENV_WRITE=0"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "--always-approve") {
		t.Fatalf("must not pass --always-approve:\n%s", s)
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
	var got []string
	var errMsg string
	in := strings.NewReader(strings.Join([]string{
		`{"type":"text","data":"A"}`,
		`not-json`,
		`{"type":"thought","data":"x"}`,
		`{"type":"text","data":"B"}`,
		`{"type":"error","message":"nope"}`,
		`{"type":"end","stopReason":"end_turn"}`,
	}, "\n"))
	if err := scanStreamingJSON(in, func(d string) { got = append(got, d) }, func(m string) { errMsg = m }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "") != "AB" || errMsg != "nope" {
		t.Fatalf("got=%v err=%q", got, errMsg)
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
		Exec:     func(ctx context.Context, name string, args ...string) *exec.Cmd { return exec.CommandContext(ctx, fake) },
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
