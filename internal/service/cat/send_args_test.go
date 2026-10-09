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

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

// 端到端：SendCatMessage 的 modelId / thinkLevelId → Service → BuildAdapter → 假 grok 的 argv。
// 覆盖「服务层没把选择传进适配器」「续聊丢参数」「没选也乱传」三类断点。
func TestSendPassesModelAndEffortEveryTurn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv.log")
	fake := filepath.Join(dir, "grok")
	script := `#!/bin/sh
case "$1" in
  -v) echo "grok 1.0.40"; exit 0 ;;
  models) printf 'Default model: grok-4.6\n\nAvailable models:\n  * grok-4.6 (default)\n  - grok-4.5\n'; exit 0 ;;
esac
for a in "$@"; do printf '%s\n' "$a" >> '` + argvLog + `'; done
printf '%s\n' '----' >> '` + argvLog + `'
printf '%s\n' '{"type":"text","data":"ok"}'
printf '%s\n' '{"type":"end","stopReason":"end_turn"}'
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	temp := filepath.Join(dir, "tmp")
	_ = os.MkdirAll(temp, 0o755)
	reg := catagent.NewRegistry()
	build := catagent.NewBuildAdapter(catagent.BuildConfig{
		ComponentDir: filepath.Join(dir, "components"),
		DataTemp:     temp,
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
	reg.Register(build)
	build.Start()
	svc := cat.New(cat.Config{Store: st, Registry: reg})
	ctx := context.Background()

	deadline := time.Now().Add(3 * time.Second)
	for {
		s, _ := svc.GetCatStatus(ctx)
		if s.State == catagent.StateReady {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not ready: %+v", s)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 模型列表：真实名字，id 与 label 一致，默认在前。
	models, err := svc.ListCatModels(ctx, catagent.KindCatBuild)
	if err != nil || len(models) != 2 || models[0].ID != "grok-4.6" || models[0].DisplayName != "grok-4.6" || models[1].DisplayName != "grok-4.5" {
		t.Fatalf("models %+v %v", models, err)
	}
	thinks, err := svc.ListCatThinkLevels(ctx, catagent.KindCatBuild)
	if err != nil || len(thinks) == 0 {
		t.Fatalf("thinks %+v %v", thinks, err)
	}
	hasX := false
	for _, th := range thinks {
		hasX = hasX || th.ID == "xhigh"
	}
	if !hasX {
		t.Fatalf("thinks missing xhigh: %+v", thinks)
	}

	c, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if err != nil {
		t.Fatal(err)
	}
	send := func(model, think string) {
		t.Helper()
		if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "hi", ModelID: model, ThinkLevelID: think}); err != nil {
			t.Fatal(err)
		}
		svc.Wait()
	}
	send("grok-4.5", "high") // 首轮
	send("grok-4.5", "low")  // 续聊
	send("", "")             // 没选：不传
	// 新会话：xhigh 首轮（--session-id）+ 续聊（--resume）
	c2, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c2.ID, Content: "hi", ModelID: "grok-4.6", ThinkLevelID: "xhigh"}); err != nil {
			t.Fatal(err)
		}
		svc.Wait()
	}

	raw, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSuffix(string(raw), "----\n"), "----\n")
	if len(calls) != 5 {
		t.Fatalf("want 5 turn calls, got %d:\n%s", len(calls), raw)
	}
	argv := func(i int) []string { return strings.Split(strings.TrimRight(calls[i], "\n"), "\n") }
	val := func(a []string, flag string) (string, bool) {
		for j := 0; j+1 < len(a); j++ {
			if a[j] == flag {
				return a[j+1], true
			}
		}
		return "", false
	}

	first := argv(0)
	if _, ok := val(first, "--session-id"); !ok {
		t.Fatalf("turn1 missing --session-id: %v", first)
	}
	if v, _ := val(first, "-m"); v != "grok-4.5" {
		t.Fatalf("turn1 -m = %q: %v", v, first)
	}
	if v, _ := val(first, "--reasoning-effort"); v != "high" {
		t.Fatalf("turn1 --reasoning-effort = %q: %v", v, first)
	}

	second := argv(1)
	if _, ok := val(second, "--resume"); !ok {
		t.Fatalf("turn2 missing --resume: %v", second)
	}
	if v, _ := val(second, "-m"); v != "grok-4.5" {
		t.Fatalf("turn2 -m = %q: %v", v, second)
	}
	if v, _ := val(second, "--reasoning-effort"); v != "low" {
		t.Fatalf("turn2 --reasoning-effort = %q: %v", v, second)
	}

	for i, flag := range map[int]string{3: "--session-id", 4: "--resume"} { // xhigh：首轮与续聊都带
		a := argv(i)
		if v, _ := val(a, "--reasoning-effort"); v != "xhigh" {
			t.Fatalf("turn%d --reasoning-effort = %q: %v", i+1, v, a)
		}
		if _, ok := val(a, flag); !ok {
			t.Fatalf("turn%d missing %s: %v", i+1, flag, a)
		}
	}

	third := argv(2)
	if _, ok := val(third, "-m"); ok {
		t.Fatalf("turn3 should not pass -m: %v", third)
	}
	if _, ok := val(third, "--reasoning-effort"); ok {
		t.Fatalf("turn3 should not pass --reasoning-effort: %v", third)
	}
}

// 用户没选强度 → 首轮 / 续聊都不带 --reasoning-effort；`grok models` 失败 → 空列表、不带 -m。
func TestSendNoEffortNoModelWhenUnselectedOrProbeFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv.log")
	fake := filepath.Join(dir, "grok")
	script := `#!/bin/sh
case "$1" in
  -v) echo "grok 1.0.40"; exit 0 ;;
  models) echo "Error: Not signed in." >&2; exit 1 ;;
esac
for a in "$@"; do printf '%s\n' "$a" >> '` + argvLog + `'; done
printf '%s\n' '----' >> '` + argvLog + `'
printf '%s\n' '{"type":"text","data":"ok"}'
printf '%s\n' '{"type":"end","stopReason":"end_turn"}'
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	temp := filepath.Join(dir, "tmp")
	_ = os.MkdirAll(temp, 0o755)
	reg := catagent.NewRegistry()
	build := catagent.NewBuildAdapter(catagent.BuildConfig{
		ComponentDir: filepath.Join(dir, "components"),
		DataTemp:     temp,
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
	reg.Register(build)
	build.Start()
	svc := cat.New(cat.Config{Store: st, Registry: reg})
	ctx := context.Background()
	deadline := time.Now().Add(3 * time.Second)
	for {
		s, _ := svc.GetCatStatus(ctx)
		if s.State == catagent.StateReady {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not ready: %+v", s)
		}
		time.Sleep(20 * time.Millisecond)
	}
	models, err := svc.ListCatModels(ctx, catagent.KindCatBuild)
	if err != nil || models == nil || len(models) != 0 {
		t.Fatalf("models %+v %v, want empty", models, err)
	}
	thinks, err := svc.ListCatThinkLevels(ctx, catagent.KindCatBuild)
	if err != nil || len(thinks) == 0 {
		t.Fatalf("thinks %+v %v (list must stay)", thinks, err)
	}
	c, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // 首轮 + 续聊，前端没选模型也没选强度
		if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "hi"}); err != nil {
			t.Fatal(err)
		}
		svc.Wait()
	}
	raw, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSuffix(string(raw), "----\n"), "----\n")
	if len(calls) != 2 {
		t.Fatalf("want 2 turn calls, got %d:\n%s", len(calls), raw)
	}
	for i, call := range calls {
		argv := strings.Split(strings.TrimRight(call, "\n"), "\n")
		has := func(f string) bool {
			for _, a := range argv {
				if a == f {
					return true
				}
			}
			return false
		}
		if has("-m") || has("--reasoning-effort") || has("--effort") {
			t.Fatalf("turn %d must not pass -m / effort: %v", i+1, argv)
		}
		if i == 0 && !has("--session-id") || i == 1 && !has("--resume") {
			t.Fatalf("turn %d session flag wrong: %v", i+1, argv)
		}
	}
}
