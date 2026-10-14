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

// 写库失败：IO_ERROR「保存失败，请重试。」（契约 6.19.12.7 第 1 条）。
func TestSaveCatSettingsStoreFailure(t *testing.T) {
	st := openStore(t)
	svc, _ := newProjSvc(t, st, nil, cat.Config{})
	st.Close()
	_, err := svc.SaveCatSettings(context.Background(), cat.Settings{CustomInstruction: "x"})
	if !apperr.Is(err, apperr.IOError) || apperr.From(err).Message != "保存失败，请重试。" {
		t.Fatalf("got %v", err)
	}
}

// 端到端：设置里的自定义指令经 Service → BuildAdapter 进到 CLI 的 argv（--rules=，以 - 开头也不当开关），
// 首轮和续聊都带；清空后不带。
func TestCustomInstructionReachesCLIArgv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	argv := filepath.Join(dir, "argv.log")
	fake := filepath.Join(dir, "grok")
	script := "#!/bin/sh\ncase \"$1\" in\n  -v) echo 'grok 1.0.40'; exit 0 ;;\n  usage) exit 1 ;;\n  models) exit 0 ;;\nesac\n" +
		`for a in "$@"; do printf '%s\n' "$a" >> '` + argv + `'; done
printf '%s\n' '----' >> '` + argv + `'
printf '%s\n' '{"type":"text","data":"ok"}'
printf '%s\n' '{"type":"end","stopReason":"end_turn"}'
`
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
	for deadline := time.Now().Add(3 * time.Second); b.Status().State != catagent.StateReady; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("not ready")
		}
	}
	reg := catagent.NewRegistry()
	reg.Register(b)
	svc := cat.New(cat.Config{Store: openStore(t), Registry: reg, DataRoot: filepath.Join(dir, "data")})
	ctx := context.Background()
	if _, err := svc.SaveCatSettings(ctx, cat.Settings{CustomInstruction: "-用中文回答"}); err != nil {
		t.Fatal(err)
	}
	c, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if err != nil {
		t.Fatal(err)
	}
	send := func() {
		if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "hi"}); err != nil {
			t.Fatal(err)
		}
		svc.Wait()
	}
	send()
	send()
	if _, err := svc.SaveCatSettings(ctx, cat.Settings{}); err != nil {
		t.Fatal(err)
	}
	send()
	raw, _ := os.ReadFile(argv)
	turns := strings.Split(strings.TrimSuffix(string(raw), "----\n"), "----\n")
	if len(turns) != 3 {
		t.Fatalf("turns %q", raw)
	}
	for i := 0; i < 2; i++ {
		if !strings.Contains("\n"+turns[i], "\n--rules=-用中文回答\n") {
			t.Fatalf("turn %d missing --rules: %q", i, turns[i])
		}
	}
	if strings.Contains(turns[2], "--rules") {
		t.Fatalf("cleared instruction still sent: %q", turns[2])
	}
}
