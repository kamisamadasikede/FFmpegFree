package cat_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

// P0：Codex 对话刷新（重建服务 + 重开数据库）后，侧栏还在，打开记录页能读回 rollout 正文。
// 样本是 codex-cli 0.162.1 真机写出的 rollout（未登录，见 catagent/testdata/codex/README.md）。
func TestCodexHistorySurvivesReloadAndLoads(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dataRoot := filepath.Join(dir, "data")
	codexHome := filepath.Join(dir, "codex")
	dbPath := filepath.Join(dir, "app.db")
	const thread = "01a13845-acb6-7382-a6de-f154547ff010"
	sample, err := os.ReadFile(filepath.Join("..", "..", "catagent", "testdata", "codex",
		"rollout-2026-10-14T10-37-32-"+thread+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	day := filepath.Join(codexHome, "sessions", "2026", "10", "14")
	_ = os.MkdirAll(day, 0o755)
	if err := os.WriteFile(filepath.Join(day, "rollout-2026-10-14T10-37-32-"+thread+".jsonl"), sample, 0o644); err != nil {
		t.Fatal(err)
	}

	boot := func() (*cat.Service, *store.Store) {
		st, err := store.Open(ctx, dbPath)
		if err != nil {
			t.Fatal(err)
		}
		reg := catagent.NewRegistry()
		reg.Register(&fakeAdapter{})
		reg.Register(catagent.NewCodexAdapter(catagent.CLIAgentConfig{DataRoot: dataRoot, CodexHome: codexHome}))
		return cat.New(cat.Config{Store: st, Registry: reg, DataRoot: dataRoot}), st
	}

	svc1, st1 := boot()
	c, err := svc1.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCodexCLI})
	if err != nil {
		t.Fatal(err)
	}
	// RunTurn 收到 thread.started 后写的就是这个文件（见 catagent 的同名测试）。
	sp := filepath.Join(dataRoot, "cat", "cli-sessions", catagent.KindCodexCLI, c.ID)
	_ = os.MkdirAll(filepath.Dir(sp), 0o755)
	if err := os.WriteFile(sp, []byte(thread), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = st1.Close()

	// 刷新。
	svc2, st2 := boot()
	defer st2.Close()
	list, err := svc2.ListCatConversations(ctx)
	if err != nil || len(list) != 1 || list[0].ID != c.ID || list[0].AgentKind != catagent.KindCodexCLI {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	d, err := svc2.GetCatConversation(ctx, c.ID)
	if err != nil {
		t.Fatalf("打开记录页失败: %v", err)
	}
	if len(d.Messages) != 2 || d.Messages[0].Role != "user" || d.Messages[0].Content != "say hi" || d.Messages[1].Role != "status" {
		t.Fatalf("messages=%+v", d.Messages)
	}
}
