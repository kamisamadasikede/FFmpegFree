package cat_test

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
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func TestCatSettingsDefaultsValidateAndRoundTrip(t *testing.T) {
	st := openStore(t)
	svc := cat.New(cat.Config{Store: st})
	ctx := context.Background()
	got, err := svc.GetCatSettings(ctx)
	if err != nil || got.NotifyOnFinish == nil || !*got.NotifyOnFinish || got.DefaultModel != "" || got.CustomInstruction != "" {
		t.Fatalf("defaults: %+v %v", got, err)
	}
	for _, bad := range []cat.CatSettings{
		{DefaultEffort: "ultra"}, {DefaultApprovalMode: "yolo"}, {CustomInstruction: strings.Repeat("字", 4001)},
	} {
		if _, err := svc.SaveCatSettings(ctx, bad); !apperr.Is(err, apperr.InvalidArgument) {
			t.Fatalf("want INVALID_ARGUMENT for %+v, got %v", bad, err)
		}
	}
	off := false
	saved, err := svc.SaveCatSettings(ctx, cat.CatSettings{DefaultModel: " grok-4.5 ", DefaultEffort: "xhigh", DefaultApprovalMode: "plan", CustomInstruction: strings.Repeat("字", 4000), NotifyOnFinish: &off})
	if err != nil || saved.DefaultModel != "grok-4.5" || *saved.NotifyOnFinish {
		t.Fatalf("save: %+v %v", saved, err)
	}
	again, _ := svc.GetCatSettings(ctx)
	if again.DefaultEffort != "xhigh" || again.DefaultApprovalMode != "plan" || *again.NotifyOnFinish {
		t.Fatalf("reload: %+v", again)
	}
	// 写库失败：IO_ERROR「保存失败，请重试。」，库里仍是旧值。
	st.Close()
	if _, err := svc.SaveCatSettings(ctx, cat.CatSettings{}); !apperr.Is(err, apperr.IOError) || apperr.From(err).Message != cat.MsgSaveFailed {
		t.Fatalf("closed store: %v", err)
	}
}

func fakeGrok(t *testing.T, dir, body string) *catagent.BuildAdapter {
	t.Helper()
	fake := filepath.Join(dir, "grok")
	script := "#!/bin/sh\ncase \"$1\" in\n  -v) echo \"grok 1.0.40\"; exit 0 ;;\n  models) printf 'Available models:\\n  * grok-4.6 (default)\\n'; exit 0 ;;\nesac\n" + body
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	b := catagent.NewBuildAdapter(catagent.BuildConfig{
		ComponentDir: filepath.Join(dir, "components"), DataRoot: filepath.Join(dir, "data"), DataTemp: filepath.Join(dir, "tmp"),
		LookPath: func(file string) (string, error) {
			if file == "grok" || file == "grok.exe" {
				return fake, nil
			}
			return "", exec.ErrNotFound
		},
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	b.Start()
	deadline := time.Now().Add(3 * time.Second)
	for b.Status().State != catagent.StateReady {
		if time.Now().After(deadline) {
			t.Fatalf("not ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return b
}

// 自定义指令每一轮（首轮和续聊）都以 --rules=<文本> 带上；清空后不再带；以 - 开头也不会被当成开关。
func TestCustomInstructionAddsRulesEveryTurn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	argv := filepath.Join(dir, "argv.log")
	b := fakeGrok(t, dir, `for a in "$@"; do printf '%s\n' "$a" >> '`+argv+`'; done
printf '%s\n' '----' >> '`+argv+`'
printf '%s\n' '{"type":"text","data":"ok"}'
printf '%s\n' '{"type":"end","stopReason":"end_turn"}'
`)
	reg := catagent.NewRegistry()
	reg.Register(b)
	st := openStore(t)
	svc := cat.New(cat.Config{Store: st, Registry: reg, DataRoot: filepath.Join(dir, "data")})
	ctx := context.Background()
	if _, err := svc.SaveCatSettings(ctx, cat.CatSettings{CustomInstruction: "-用中文回答"}); err != nil {
		t.Fatal(err)
	}
	c, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "hi"}); err != nil {
			t.Fatal(err)
		}
		svc.Wait()
	}
	_, _ = svc.SaveCatSettings(ctx, cat.CatSettings{})
	if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	raw, _ := os.ReadFile(argv)
	turns := strings.Split(strings.TrimSpace(string(raw)), "----")
	if len(turns) < 3 {
		t.Fatalf("turns: %q", raw)
	}
	for i := 0; i < 2; i++ {
		if !strings.Contains(turns[i], "\n--rules=-用中文回答\n") && !strings.HasPrefix(turns[i], "--rules=-用中文回答\n") {
			t.Fatalf("turn %d missing --rules: %q", i, turns[i])
		}
	}
	if strings.Contains(turns[2], "--rules") {
		t.Fatalf("cleared instruction still sent: %q", turns[2])
	}
}

func TestInspectCatConfigParsesAndHidesSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	b := fakeGrok(t, dir, `cat <<'J'
{"grokVersion":"1.0.40","skills":[{"name":"build-with-ai","description":"d","source":{"type":"bundled","path":"/secret/SKILL.md"}}],
"mcpServers":[{"name":"demo","transport":"stdio","target":"echo SECRET","source":{"type":"mcpJson","path":"/p/.mcp.json"}}],
"plugins":[{"name":"p1","enabled":false,"source":"user"}],"hooks":[{"event":"PreToolUse","matcher":"Bash","command":"rm -rf"}],
"agents":[{"name":"general-purpose","source":{"type":"builtin"}}]}
J
`)
	reg := catagent.NewRegistry()
	reg.Register(b)
	svc := cat.New(cat.Config{Store: openStore(t), Registry: reg, DataRoot: filepath.Join(dir, "data")})
	r, err := svc.InspectCatConfig(context.Background(), cat.InspectRequest{})
	if err != nil || r.Reason != "" || r.Version != "1.0.40" {
		t.Fatalf("%+v %v", r, err)
	}
	if len(r.Skills) != 1 || r.Skills[0].Source != "bundled" || !r.Skills[0].Enabled ||
		len(r.MCPServers) != 1 || r.MCPServers[0].Name != "demo" || r.MCPServers[0].Source != "mcpJson" ||
		len(r.Plugins) != 1 || r.Plugins[0].Enabled || len(r.Hooks) != 1 || r.Hooks[0].Name != "PreToolUse · Bash" || len(r.Agents) != 1 {
		t.Fatalf("parsed: %+v", r)
	}
	all := strings.Join([]string{r.Skills[0].Description, r.MCPServers[0].Description, r.Hooks[0].Description}, "|")
	for _, leak := range []string{"SECRET", "/secret", ".mcp.json", "rm -rf"} {
		if strings.Contains(all, leak) {
			t.Fatalf("leaked %q", leak)
		}
	}
}

func TestInspectCatConfigFailures(t *testing.T) {
	// 未就绪
	svc := cat.New(cat.Config{Store: openStore(t), Registry: func() *catagent.Registry { r := catagent.NewRegistry(); r.Register(&notReadyAdapter{}); return r }()})
	r, err := svc.InspectCatConfig(context.Background(), cat.InspectRequest{})
	if err != nil || r.Reason != catagent.InspectNotReady || r.Skills == nil || r.MCPServers == nil {
		t.Fatalf("not ready: %+v %v", r, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	dir := t.TempDir()
	b := fakeGrok(t, dir, "echo 'not json'\n")
	reg := catagent.NewRegistry()
	reg.Register(b)
	svc = cat.New(cat.Config{Store: openStore(t), Registry: reg})
	if r, _ := svc.InspectCatConfig(context.Background(), cat.InspectRequest{}); r.Reason != catagent.InspectParse {
		t.Fatalf("parse: %+v", r)
	}
	dir2 := t.TempDir()
	b2 := fakeGrok(t, dir2, "sleep 5\n")
	reg2 := catagent.NewRegistry()
	reg2.Register(b2)
	old := catagent.InspectTimeoutDur
	catagent.InspectTimeoutDur = 200 * time.Millisecond
	defer func() { catagent.InspectTimeoutDur = old }()
	svc = cat.New(cat.Config{Store: openStore(t), Registry: reg2})
	if r, _ := svc.InspectCatConfig(context.Background(), cat.InspectRequest{}); r.Reason != catagent.InspectTimeout {
		t.Fatalf("timeout: %+v", r)
	}
}
