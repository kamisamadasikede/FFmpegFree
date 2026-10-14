package cat_test

import (
	"context"
	"sync"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

// 契约 6.19.18.7：只重试最后一轮失败的 turn；原用户消息、model / effort / mode 不变；不新存用户消息；新 turnId。
func TestRetryCatTurnInPlace(t *testing.T) {
	var mu sync.Mutex
	var calls []catagent.TurnOptions
	fail := true
	svc, _, conv := setupFake(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		mu.Lock()
		calls = append(calls, o)
		f := fail
		mu.Unlock()
		if f {
			return catagent.TurnResponse{}, apperr.New(apperr.CatReplyFailed, "boom")
		}
		o.OnTextDelta("ok")
		return catagent.TurnResponse{}, nil
	})
	ctx := context.Background()
	first, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{
		ConversationID: conv, Content: "hi", ModelID: "grok-4.5", ThinkLevelID: "low", PermissionMode: "acceptEdits",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.Wait()

	if _, err := svc.RetryCatTurn(ctx, cat.RetryCatTurnRequest{ConversationID: conv, TurnID: "other"}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("wrong turn must be rejected: %v", err)
	}
	mu.Lock()
	fail = false
	mu.Unlock()
	res, err := svc.RetryCatTurn(ctx, cat.RetryCatTurnRequest{ConversationID: conv, TurnID: first.TurnID})
	if err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	if res.TurnID == "" || res.TurnID == first.TurnID || res.UserMessage.ID != first.UserMessage.ID {
		t.Fatalf("retry result %+v vs %+v", res, first)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("calls %d", len(calls))
	}
	r := calls[1]
	if r.ModelID != "grok-4.5" || r.ThinkLevelID != "low" || r.PermissionMode != "acceptEdits" || r.Messages[0].Content != "hi" {
		t.Fatalf("retry opts %+v", r)
	}
	// 已成功的轮不能再重试。
	if _, err := svc.RetryCatTurn(ctx, cat.RetryCatTurnRequest{ConversationID: conv, TurnID: res.TurnID}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("completed turn retry: %v", err)
	}
}

// 6.19.18.6 末句：effort 不在当前模型列表 → 不传；模式旧值映射、空 → 默认。
func TestSendEffortFilteredAndModeMapped(t *testing.T) {
	var mu sync.Mutex
	var got []catagent.TurnOptions
	svc, _, conv := setupFake(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		mu.Lock()
		got = append(got, o)
		mu.Unlock()
		o.OnTextDelta("ok")
		return catagent.TurnResponse{}, nil
	})
	ctx := context.Background()
	for _, r := range []cat.SendMessageRequest{
		{ModelID: "grok-4.5", ThinkLevelID: "xhigh", PermissionMode: "approve"},
		{ModelID: "grok-4.6", ThinkLevelID: "xhigh", PermissionMode: ""},
		{ModelID: "", ThinkLevelID: "high", PermissionMode: "auto"},
	} {
		r.ConversationID, r.Content = conv, "hi"
		if _, err := svc.SendCatMessage(ctx, r); err != nil {
			t.Fatal(err)
		}
		svc.Wait()
	}
	mu.Lock()
	defer mu.Unlock()
	want := []struct{ effort, mode string }{{"", "always-approve"}, {"xhigh", "default"}, {"", "auto"}}
	for i, w := range want {
		if got[i].ThinkLevelID != w.effort || got[i].PermissionMode != w.mode {
			t.Errorf("%d: effort %q mode %q want %+v", i, got[i].ThinkLevelID, got[i].PermissionMode, w)
		}
	}
}

func TestStatusCarriesModes(t *testing.T) {
	svc, _, _ := setupFake(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) { return catagent.TurnResponse{}, nil })
	st, _ := svc.GetCatStatus(context.Background())
	if len(st.Agents) != 3 {
		t.Fatalf("%+v", st.Agents)
	}
	for _, a := range st.Agents {
		if len(a.Modes) == 0 || !a.Modes[0].IsDefault || a.Modes[len(a.Modes)-1].Risk != catagent.RiskDanger {
			t.Errorf("%s modes %+v", a.AgentKind, a.Modes)
		}
	}
	// 保存：max 在模型列表里就放行（这里 grok 没有 max → 拒）；模型为空只看格式。
	if _, err := svc.SaveCatSettings(context.Background(), cat.Settings{DefaultsByAgent: map[string]cat.AgentDefaults{catagent.KindCatBuild: {Effort: "max"}}}); err != nil {
		t.Fatalf("empty model + max must pass: %v", err)
	}
	if _, err := svc.SaveCatSettings(context.Background(), cat.Settings{DefaultsByAgent: map[string]cat.AgentDefaults{catagent.KindCatBuild: {Model: "grok-4.6", Effort: "max"}}}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("grok-4.6 max must fail: %v", err)
	}
	if _, err := svc.SaveCatSettings(context.Background(), cat.Settings{DefaultModeByAgent: map[string]string{catagent.KindCodexCLI: catagent.ModeCodexApproveForMe}}); err != nil {
		t.Fatalf("mode id must be accepted: %v", err)
	}
}
