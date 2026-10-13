package catagent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
)

// init 必须早于 testing 的 flag.Parse。子进程的参数是 grok 的 --cwd / --prompt-file，
// 若拖到 TestMain，测试框架会把它们当成未知测试开关直接退出。
func init() {
	if os.Getenv("FFMPEG_CAT_FAKE_GROK") == "1" {
		os.Exit(fakeGrokProcess())
	}
}

// fakeGrokProcess 让测试二进制冒充 grok：记下参数和环境，再吐一行能被回合认下的流。
func fakeGrokProcess() int {
	out := os.Getenv("FFMPEG_CAT_FAKE_OUT")
	if out == "" {
		return 2
	}
	var b strings.Builder
	for _, a := range os.Args[1:] {
		b.WriteString(a)
		b.WriteByte('\n')
	}
	b.WriteString("ENV_SANDBOX=" + os.Getenv("GROK_SANDBOX") + "\n")
	b.WriteString("ENV_WRITE=" + os.Getenv("GROK_WRITE_FILE") + "\n")
	b.WriteString("ENV_UPDATER=" + os.Getenv("GROK_DISABLE_AUTOUPDATER") + "\n")
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		return 1
	}
	if dest := os.Getenv("FFMPEG_CAT_FAKE_PROMPT"); dest != "" {
		for i, a := range os.Args {
			if a == "--prompt-file" && i+1 < len(os.Args) {
				body, err := os.ReadFile(os.Args[i+1])
				if err != nil {
					return 1
				}
				if err := os.WriteFile(dest, body, 0o644); err != nil {
					return 1
				}
				break
			}
		}
	}
	if msg := os.Getenv("FFMPEG_CAT_FAKE_STDERR"); msg != "" {
		os.Stderr.WriteString(msg + "\n")
	}
	os.Stdout.WriteString("{\"type\":\"text\",\"data\":\"ok\"}\n")
	os.Stdout.WriteString("{\"type\":\"end\",\"stopReason\":\"end_turn\"}\n")
	if os.Getenv("FFMPEG_CAT_FAKE_EXIT") == "1" {
		return 1
	}
	return 0
}

func TestGrokTurnEnv_DoesNotLockTools(t *testing.T) {
	env := grokTurnEnv([]string{
		"PATH=/bin",
		"GROK_SANDBOX=off",
		"GROK_WRITE_FILE=1",
		"GROK_DISABLE_AUTOUPDATER=0",
	})
	got := map[string]string{}
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if _, dup := got[k]; dup && (k == "GROK_SANDBOX" || k == "GROK_WRITE_FILE" || k == "GROK_DISABLE_AUTOUPDATER") {
			t.Fatalf("duplicated %s in %v", k, env)
		}
		got[k] = v
	}
	if got["GROK_SANDBOX"] != "off" || got["GROK_WRITE_FILE"] != "1" {
		t.Fatalf("caller tool env was rewritten: %v", env)
	}
	if got["GROK_DISABLE_AUTOUPDATER"] != "1" {
		t.Fatalf("autoupdater: %v", env)
	}
}

func TestRunTurn_KeepsFullCLITools(t *testing.T) {
	out := t.TempDir() + string(os.PathSeparator) + "argv.txt"
	t.Setenv("FFMPEG_CAT_FAKE_GROK", "1")
	t.Setenv("FFMPEG_CAT_FAKE_OUT", out)
	t.Setenv("GROK_SANDBOX", "")
	t.Setenv("GROK_WRITE_FILE", "")

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return exe, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	a.detect()

	turns := [][]WireMessage{
		{{Role: "user", Content: "first"}},
		{{Role: "user", Content: "first"}, {Role: "assistant", Content: "ok"}, {Role: "user", Content: "second"}},
	}
	banned := []string{"--tools", "--disallowed-tools", "--deny", "--no-subagents", "--disable-web-search"}
	for i, msgs := range turns {
		if _, err := a.RunTurn(TurnOptions{Ctx: context.Background(), ConversationID: "conv-full", Messages: msgs}); err != nil {
			t.Fatalf("turn %d: %v", i, err)
		}
		raw, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		argv := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
		has := func(flag string) bool {
			for _, a := range argv {
				if a == flag {
					return true
				}
			}
			return false
		}
		if !has("--always-approve") || !has("--output-format") || !has("--no-auto-update") {
			t.Fatalf("turn %d: missing launch flags:\n%s", i, raw)
		}
		if has("--permission-mode") {
			t.Fatalf("turn %d: empty mode must not pass --permission-mode:\n%s", i, raw)
		}
		for _, flag := range banned {
			if has(flag) {
				t.Fatalf("turn %d: still passes %s:\n%s", i, flag, raw)
			}
		}
		if i == 0 && !has("--session-id") {
			t.Fatalf("turn 0: missing --session-id:\n%s", raw)
		}
		if i == 1 && !has("--resume") {
			t.Fatalf("turn 1: missing --resume:\n%s", raw)
		}
		text := string(raw)
		if !strings.Contains(text, "\nENV_SANDBOX=\n") || !strings.Contains(text, "\nENV_WRITE=\n") {
			t.Fatalf("turn %d: tool env was forced:\n%s", i, raw)
		}
		if !strings.Contains(text, "\nENV_UPDATER=1\n") && !strings.HasSuffix(text, "\nENV_UPDATER=1") {
			t.Fatalf("turn %d: autoupdater:\n%s", i, raw)
		}
	}
}

func TestRunTurn_DashPromptGoesToFile(t *testing.T) {
	out := t.TempDir() + string(os.PathSeparator) + "argv.txt"
	copied := t.TempDir() + string(os.PathSeparator) + "prompt.txt"
	t.Setenv("FFMPEG_CAT_FAKE_GROK", "1")
	t.Setenv("FFMPEG_CAT_FAKE_OUT", out)
	t.Setenv("FFMPEG_CAT_FAKE_PROMPT", copied)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return exe, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	a.detect()
	prompt := "----- 结束 -----\n第 5 页接口已打印\n  File \"C:\\a.py\", line 1\n    listing\n"
	if _, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "conv-dash",
		Messages: []WireMessage{{Role: "user", Content: prompt}},
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "----- 结束 -----") || strings.Contains(text, "\n-p\n") {
		t.Fatalf("prompt leaked onto the command line:\n%s", text)
	}
	if !strings.Contains(text, "--prompt-file\n") {
		t.Fatalf("missing --prompt-file:\n%s", text)
	}
	body, err := os.ReadFile(copied)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != prompt {
		t.Fatalf("file %q", body)
	}
}

func TestRunTurn_ImageUsesPromptJSON(t *testing.T) {
	out := t.TempDir() + string(os.PathSeparator) + "argv.txt"
	t.Setenv("FFMPEG_CAT_FAKE_GROK", "1")
	t.Setenv("FFMPEG_CAT_FAKE_OUT", out)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return exe, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	a.detect()
	dir := t.TempDir()
	img := dir + string(os.PathSeparator) + "shot.png"
	writePNG(t, img, 4, 4)
	note := dir + string(os.PathSeparator) + "n.txt"
	if err := os.WriteFile(note, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prompt := "----- 结束 -----"
	if _, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "conv-img",
		Messages: []WireMessage{{Role: "user", Content: prompt}},
		Files:    []PromptFile{{Name: "shot.png", Path: img}, {Name: "n.txt", Path: note}},
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "--prompt-file\n") || strings.Contains(text, "\n-p\n") {
		t.Fatalf("image turn used the text prompt flag:\n%s", text)
	}
	if !strings.Contains(text, "--prompt-json\n") {
		t.Fatalf("missing --prompt-json:\n%s", text)
	}
	var payload string
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line == "--prompt-json" && i+1 < len(lines) {
			payload = lines[i+1]
		}
	}
	var blocks []contentBlock
	if err := json.Unmarshal([]byte(payload), &blocks); err != nil {
		t.Fatalf("payload: %v\n%s", err, payload)
	}
	sawText, sawImage := false, false
	for _, b := range blocks {
		if b.Type == "text" && strings.Contains(b.Text, prompt) && strings.Contains(b.Text, note) {
			sawText = true
		}
		if b.Type == "image" && b.MimeType == "image/png" && b.Data != "" {
			sawImage = true
		}
	}
	if !sawText || !sawImage {
		t.Fatalf("blocks %+v", blocks)
	}
}

func TestRunTurn_PlanOmitsAlwaysApprove(t *testing.T) {
	out := t.TempDir() + string(os.PathSeparator) + "argv.txt"
	t.Setenv("FFMPEG_CAT_FAKE_GROK", "1")
	t.Setenv("FFMPEG_CAT_FAKE_OUT", out)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return exe, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	a.detect()
	if _, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "conv-plan", PermissionMode: "plan",
		Messages: []WireMessage{{Role: "user", Content: "plan this"}},
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "--permission-mode\nplan\n") {
		t.Fatalf("missing plan mode:\n%s", text)
	}
	if strings.Contains(text, "--always-approve") {
		t.Fatalf("plan mode still auto-approves:\n%s", text)
	}
}

func TestRunTurn_ExitAfterTextShowsStderr(t *testing.T) {
	out := t.TempDir() + string(os.PathSeparator) + "argv.txt"
	t.Setenv("FFMPEG_CAT_FAKE_GROK", "1")
	t.Setenv("FFMPEG_CAT_FAKE_OUT", out)
	t.Setenv("FFMPEG_CAT_FAKE_STDERR", "read tcp 10.0.0.1:443: i/o timeout")
	t.Setenv("FFMPEG_CAT_FAKE_EXIT", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return exe, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	a.detect()
	var noted []string
	var deltas []string
	_, err = a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "conv-timeout",
		Messages:    []WireMessage{{Role: "user", Content: "hi"}},
		OnTextDelta: func(d string) { deltas = append(deltas, d) },
		OnError:     func(m string) { noted = append(noted, m) },
	})
	ae := apperr.From(err)
	if ae == nil || !strings.Contains(ae.Message, "i/o timeout") {
		t.Fatalf("err=%v deltas=%v noted=%v", err, deltas, noted)
	}
	if strings.Join(deltas, "") != "ok" {
		t.Fatalf("deltas %v", deltas)
	}
	if len(noted) == 0 || !strings.Contains(noted[0], "i/o timeout") {
		t.Fatalf("noted %v", noted)
	}
}
