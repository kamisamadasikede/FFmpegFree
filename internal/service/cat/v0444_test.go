package cat_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

// 6.19.18.9 第 3 条：只有旧 defaultApprovalMode → Get 时按各家旧值列映射；未知 → 最保守；不回写；Save 写新形状删旧字段。
func TestDefaultModeByAgentLegacyRead(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	svc, _ := newProjSvc(t, st, nil, cat.Config{})
	cases := map[string]map[string]string{
		"auto":         {catagent.KindClaudeCLI: catagent.ModeAuto, catagent.KindCodexCLI: catagent.ModeCodexApproveForMe, catagent.KindCatBuild: catagent.ModeAuto},
		"plan":         {catagent.KindClaudeCLI: catagent.ModeClaudePlan, catagent.KindCodexCLI: catagent.ModeCodexReadOnly, catagent.KindCatBuild: catagent.ModeGrokDefault},
		"approve":      {catagent.KindClaudeCLI: catagent.ModeClaudeBypass, catagent.KindCodexCLI: catagent.ModeCodexBypass, catagent.KindCatBuild: catagent.ModeGrokAlwaysApprove},
		"yolo-unknown": {catagent.KindClaudeCLI: catagent.DefaultModeID(catagent.KindClaudeCLI), catagent.KindCodexCLI: catagent.ModeCodexReadOnly, catagent.KindCatBuild: catagent.ModeGrokDefault},
	}
	for legacy, want := range cases {
		if err := st.SetSetting(ctx, cat.SettingsKey, map[string]any{"defaultApprovalMode": legacy}); err != nil {
			t.Fatal(err)
		}
		got, err := svc.GetCatSettings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range want {
			if got.DefaultModeByAgent[k] != v {
				t.Errorf("%s/%s = %q want %q", legacy, k, got.DefaultModeByAgent[k], v)
			}
		}
	}
	var wire map[string]json.RawMessage
	_, _ = st.GetSetting(ctx, cat.SettingsKey, &wire)
	if _, ok := wire["defaultApprovalMode"]; !ok {
		t.Fatal("Get must not rewrite")
	}
	// 新形状优先于旧字段。
	_ = st.SetSetting(ctx, cat.SettingsKey, map[string]any{"defaultApprovalMode": "approve", "defaultModeByAgent": map[string]string{catagent.KindCodexCLI: catagent.ModeCodexWorkspace}})
	got, _ := svc.GetCatSettings(ctx)
	if got.DefaultModeByAgent[catagent.KindCodexCLI] != catagent.ModeCodexWorkspace || got.DefaultModeByAgent[catagent.KindClaudeCLI] != "" {
		t.Fatalf("%+v", got.DefaultModeByAgent)
	}
	if got.DefaultModeFor(catagent.KindClaudeCLI) != catagent.DefaultModeID(catagent.KindClaudeCLI) {
		t.Fatal("empty → conservative default")
	}
	// 设置页选 danger 不需要确认：直接保存成功；保存后旧字段被删。
	if _, err := svc.SaveCatSettings(ctx, cat.Settings{DefaultModeByAgent: map[string]string{catagent.KindCatBuild: catagent.ModeGrokAlwaysApprove, catagent.KindClaudeCLI: ""}}); err != nil {
		t.Fatal(err)
	}
	wire = nil
	_, _ = st.GetSetting(ctx, cat.SettingsKey, &wire)
	if _, ok := wire["defaultApprovalMode"]; ok {
		t.Fatal("Save must drop legacy defaultApprovalMode")
	}
	if _, ok := wire["defaultModeByAgent"]; !ok {
		t.Fatal("Save must write defaultModeByAgent")
	}
}

// 6.19.18.7 第 7 条：retryable 只在最后一轮失败且内存里仍有记录时为 true（失败事件 + GetCatConversation）。
func TestTurnRetryable(t *testing.T) {
	var mu sync.Mutex
	fail := true
	svc, rec, conv := setupFake(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		mu.Lock()
		f := fail
		mu.Unlock()
		if f {
			return catagent.TurnResponse{}, apperr.New(apperr.CatReplyFailed, "boom")
		}
		o.OnTextDelta("ok")
		return catagent.TurnResponse{}, nil
	})
	ctx := context.Background()
	d, _ := svc.GetCatConversation(ctx, conv)
	if d.LastTurn != nil {
		t.Fatalf("fresh conv lastTurn %+v", d.LastTurn)
	}
	first, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: conv, Content: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	lastFailed := func(turnID string) catagent.TurnEvent {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		for i := len(rec.turns) - 1; i >= 0; i-- {
			if e := rec.turns[i]; e.TurnID == turnID && e.Status != catagent.TurnRunning {
				return e
			}
		}
		t.Fatalf("no terminal event for %s", turnID)
		return catagent.TurnEvent{}
	}
	if ev := lastFailed(first.TurnID); ev.Status != catagent.TurnFailed || !ev.Retryable {
		t.Fatalf("failed event %+v", ev)
	}
	d, _ = svc.GetCatConversation(ctx, conv)
	if d.LastTurn == nil || d.LastTurn.TurnID != first.TurnID || !d.LastTurn.Retryable {
		t.Fatalf("lastTurn %+v", d.LastTurn)
	}
	// 新一轮开始（且成功）→ 前一失败轮不再可重试；成功轮也是 false。
	mu.Lock()
	fail = false
	mu.Unlock()
	second, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: conv, Content: "again"})
	if err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	if ev := lastFailed(second.TurnID); ev.Retryable {
		t.Fatalf("completed event retryable: %+v", ev)
	}
	d, _ = svc.GetCatConversation(ctx, conv)
	if d.LastTurn == nil || d.LastTurn.TurnID != second.TurnID || d.LastTurn.Retryable {
		t.Fatalf("lastTurn %+v", d.LastTurn)
	}
	// JSON 里 retryable 始终输出。
	b, _ := json.Marshal(catagent.TurnEvent{})
	if !json.Valid(b) || !contains(string(b), `"retryable":false`) {
		t.Fatalf("%s", b)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
