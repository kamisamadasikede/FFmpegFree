package cat_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/service/system"
)

func TestCatSettingsRoundTripAndValidation(t *testing.T) {
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	got, err := svc.GetCatSettings(ctx)
	if err != nil || got.DefaultAgentKind != catagent.KindCatBuild || got.DefaultsByAgent == nil || !got.NotifyEnabled() || got.NotifyOnFinish == nil {
		t.Fatalf("empty settings %+v %v", got, err)
	}
	off := false
	saved, err := svc.SaveCatSettings(ctx, cat.Settings{
		DefaultAgentKind:    catagent.KindClaudeCLI,
		DefaultsByAgent:     map[string]cat.AgentDefaults{catagent.KindCatBuild: {Model: " grok-4.6 ", Effort: "xhigh"}},
		DefaultApprovalMode: "plan",
		CustomInstruction:   "用中文",
		NotifyOnFinish:      &off,
	})
	if err != nil || saved.DefaultAgentKind != catagent.KindClaudeCLI || saved.DefaultsByAgent[catagent.KindCatBuild].Model != "grok-4.6" || saved.NotifyEnabled() {
		t.Fatalf("%+v %v", saved, err)
	}
	got, _ = svc.GetCatSettings(ctx)
	if got.DefaultsByAgent[catagent.KindCatBuild].Effort != "xhigh" || got.DefaultApprovalMode != "plan" || got.CustomInstruction != "用中文" || got.NotifyEnabled() {
		t.Fatalf("%+v", got)
	}
	for _, bad := range []cat.Settings{
		{DefaultsByAgent: map[string]cat.AgentDefaults{catagent.KindCatBuild: {Effort: "MAX!"}}}, // v0.44：模型为空只校验格式
		{DefaultApprovalMode: "yolo"},
		{CustomInstruction: strings.Repeat("字", 4001)},
		{DefaultAgentKind: "cat_cli"},
		{DefaultsByAgent: map[string]cat.AgentDefaults{"cat_cli": {Model: "x"}}},
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

func TestCatSettingsLegacyMigrateOnRead(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	// 写入 v0.35 旧形状。
	legacy := map[string]any{
		"defaultModel": "grok-4.6", "defaultEffort": "high",
		"defaultApprovalMode": "auto", "customInstruction": "旧指令",
	}
	if err := st.SetSetting(ctx, cat.SettingsKey, legacy); err != nil {
		t.Fatal(err)
	}
	svc, _ := newProjSvc(t, st, nil, cat.Config{})
	got, err := svc.GetCatSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	d := got.DefaultsByAgent[catagent.KindCatBuild]
	if got.DefaultAgentKind != catagent.KindCatBuild || d.Model != "grok-4.6" || d.Effort != "high" || got.CustomInstruction != "旧指令" {
		t.Fatalf("%+v", got)
	}
	// Get 不回写：原始仍含旧字段。
	var wire map[string]json.RawMessage
	if _, err := st.GetSetting(ctx, cat.SettingsKey, &wire); err != nil {
		t.Fatal(err)
	}
	if _, ok := wire["defaultModel"]; !ok {
		t.Fatal("expected legacy key still in store after Get")
	}
	// 已有 defaultsByAgent.cat_build.model 时不覆盖。
	mixed := map[string]any{
		"defaultModel": "legacy-model", "defaultEffort": "low",
		"defaultsByAgent": map[string]any{
			catagent.KindCatBuild: map[string]string{"model": "keep-me", "effort": ""},
		},
	}
	if err := st.SetSetting(ctx, cat.SettingsKey, mixed); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.GetCatSettings(ctx)
	d = got.DefaultsByAgent[catagent.KindCatBuild]
	if d.Model != "keep-me" || d.Effort != "low" {
		t.Fatalf("migrate overwrite: %+v", d)
	}
	// Save 后旧字段消失。
	if _, err := svc.SaveCatSettings(ctx, got); err != nil {
		t.Fatal(err)
	}
	var stored map[string]json.RawMessage
	if _, err := st.GetSetting(ctx, cat.SettingsKey, &stored); err != nil {
		t.Fatal(err)
	}
	if _, ok := stored["defaultModel"]; ok {
		t.Fatal("defaultModel still stored after Save")
	}
	if _, ok := stored["defaultEffort"]; ok {
		t.Fatal("defaultEffort still stored after Save")
	}
}

func TestCatSettingsSystemKeyFallback(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	if err := st.SetSetting(ctx, system.SettingCatDefaultAgentKind, catagent.KindCodexCLI); err != nil {
		t.Fatal(err)
	}
	svc, _ := newProjSvc(t, st, nil, cat.Config{
		SystemDefaultAgentKind: func(context.Context) string { return catagent.KindCodexCLI },
	})
	got, err := svc.GetCatSettings(ctx)
	if err != nil || got.DefaultAgentKind != catagent.KindCodexCLI {
		t.Fatalf("%+v %v", got, err)
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
	ctx := context.Background()
	proj := filepath.Join(t.TempDir(), "proj")
	_ = os.MkdirAll(proj, 0o755)
	svc, _ := newProjSvc(t, openStore(t), &notReadyAdapter{}, cat.Config{})
	pr, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: proj})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.InspectCatConfig(ctx, cat.InspectRequest{ProjectID: pr.Project.ID})
	if err != nil || res.Reason != "not_ready" || res.Skills == nil || res.MCPServers == nil {
		t.Fatalf("%+v %v", res, err)
	}
	if res.SkillsRoot == "" || res.McpConfigPath == "" {
		t.Fatalf("v0.39 path fields required: %+v", res)
	}
}

func TestInspectUnsupportedForStubKind(t *testing.T) {
	ctx := context.Background()
	proj := filepath.Join(t.TempDir(), "proj")
	_ = os.MkdirAll(proj, 0o755)
	reg := catagent.NewRegistry()
	reg.Register(&fakeAdapter{})
	reg.Register(catagent.NewStubAdapter(catagent.KindClaudeCLI))
	svc := cat.New(cat.Config{Store: openStore(t), Registry: reg})
	pr, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: proj})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.InspectCatConfig(ctx, cat.InspectRequest{ProjectID: pr.Project.ID, AgentKind: catagent.KindClaudeCLI})
	if err != nil || res.Reason != catagent.InspectUnsupported {
		t.Fatalf("%+v %v", res, err)
	}
	if res.SkillsRoot == "" {
		t.Fatalf("paths still required: %+v", res)
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
