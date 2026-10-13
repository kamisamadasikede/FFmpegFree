package cat_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/cat"
)

func TestCatSettingsRoundTripAndValidation(t *testing.T) {
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	got, err := svc.GetCatSettings(ctx)
	if err != nil || got.DefaultModel != "" || !got.NotifyEnabled() || got.NotifyOnFinish == nil {
		t.Fatalf("empty settings %+v %v", got, err)
	}
	off := false
	saved, err := svc.SaveCatSettings(ctx, cat.Settings{DefaultModel: " grok-4.6 ", DefaultEffort: "xhigh", DefaultApprovalMode: "plan", CustomInstruction: "用中文", NotifyOnFinish: &off})
	if err != nil || saved.DefaultModel != "grok-4.6" || saved.NotifyEnabled() {
		t.Fatalf("%+v %v", saved, err)
	}
	got, _ = svc.GetCatSettings(ctx)
	if got.DefaultEffort != "xhigh" || got.DefaultApprovalMode != "plan" || got.CustomInstruction != "用中文" || got.NotifyEnabled() {
		t.Fatalf("%+v", got)
	}
	for _, bad := range []cat.Settings{
		{DefaultEffort: "max"}, {DefaultApprovalMode: "yolo"}, {CustomInstruction: strings.Repeat("字", 4001)},
	} {
		if _, err := svc.SaveCatSettings(ctx, bad); !apperr.Is(err, apperr.InvalidArgument) {
			t.Fatalf("%+v → %v", bad, err)
		}
	}
	// 校验失败不改库里的值。
	if got2, _ := svc.GetCatSettings(ctx); got2.CustomInstruction != "用中文" {
		t.Fatalf("%+v", got2)
	}
	if _, err := svc.SaveCatSettings(ctx, cat.Settings{CustomInstruction: strings.Repeat("字", 4000)}); err != nil {
		t.Fatal(err)
	}
}

func TestSendCarriesRulesEveryTurn(t *testing.T) {
	ctx := context.Background()
	var rules []string
	a := &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		rules = append(rules, o.Rules)
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "ok"}}, nil
	}}
	svc, _ := newProjSvc(t, openStore(t), a, cat.Config{})
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
	if _, err := svc.SaveCatSettings(ctx, cat.Settings{CustomInstruction: "  先列计划  "}); err != nil {
		t.Fatal(err)
	}
	send()
	send()
	if len(rules) != 3 || rules[0] != "" || rules[1] != "先列计划" || rules[2] != "先列计划" {
		t.Fatalf("%q", rules)
	}
}

func TestInspectNotReadyReturnsEmpty(t *testing.T) {
	svc, _ := newProjSvc(t, openStore(t), &notReadyAdapter{}, cat.Config{})
	res, err := svc.InspectCatConfig(context.Background(), cat.InspectRequest{})
	if err != nil || res.Reason != "not_ready" || res.Skills == nil || res.MCPServers == nil {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestClearAllConversationsKeepsProjectFolders(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	projDir := filepath.Join(t.TempDir(), "proj")
	_ = os.MkdirAll(projDir, 0o755)
	keep := filepath.Join(projDir, "keep.txt")
	_ = os.WriteFile(keep, []byte("x"), 0o644)
	svc, _ := newProjSvc(t, openStore(t), &fakeAdapter{}, cat.Config{DataRoot: root})
	pr, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: projDir})
	if err != nil {
		t.Fatal(err)
	}
	pc, _ := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: pr.Project.ID})
	_ = pc
	plain, _ := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	legacy, _ := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	legacyDir := paths.LegacyCatCwd(root, legacy.ID)
	_ = os.MkdirAll(legacyDir, 0o755)
	plainDir := paths.CatConvDir(root, plain.ID)
	if _, err := os.Stat(plainDir); err != nil {
		t.Fatalf("plain dir: %v", err)
	}
	if err := svc.ClearAllCatConversations(ctx); err != nil {
		t.Fatal(err)
	}
	list, _ := svc.ListCatConversations(ctx)
	if len(list) != 0 {
		t.Fatalf("left %d", len(list))
	}
	for _, d := range []string{plainDir, legacyDir} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Fatalf("%s not removed: %v", d, err)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("project file touched: %v", err)
	}
	projs, _ := svc.ListCatProjects(ctx)
	if len(projs) != 1 {
		t.Fatalf("projects %d", len(projs))
	}
}
