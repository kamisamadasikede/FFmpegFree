package cat_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func TestProviderSettingsRoundTripAndKeyLifecycle(t *testing.T) {
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})

	got, err := svc.GetCatSettings(ctx)
	if err != nil || got.ProvidersByAgent == nil {
		t.Fatalf("%+v %v", got, err)
	}

	saved, err := svc.SaveCatSettings(ctx, cat.Settings{
		ProvidersByAgent: map[string]cat.ProviderSettings{
			catagent.KindCatBuild: {
				EnabledCustom: true,
				Protocol:      "openai",
				ApiUrl:        "https://example.test/v1",
				ApiKeySet:     true, // 客户端伪造：必须忽略
				ApiKey:        "should-never-persist",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := saved.ProvidersByAgent[catagent.KindCatBuild]
	if !p.EnabledCustom || p.Protocol != "openai" || p.ApiUrl != "https://example.test/v1" || p.ApiKeySet || p.ApiKey != "" {
		t.Fatalf("save view %+v", p)
	}

	// Key 保存 → apiKeySet true；Save 设置不得清掉 Key。
	view, err := svc.SaveCatProviderKey(ctx, cat.SaveProviderKeyRequest{AgentKind: catagent.KindCatBuild, ApiKey: "sk-secret-test"})
	if err != nil || !view.ApiKeySet || view.ApiKey != "" {
		t.Fatalf("save key %+v %v", view, err)
	}
	got, _ = svc.GetCatSettings(ctx)
	p = got.ProvidersByAgent[catagent.KindCatBuild]
	if !p.ApiKeySet || !p.EnabledCustom || p.ApiUrl != "https://example.test/v1" || p.ApiKey != "" {
		t.Fatalf("after key %+v", p)
	}

	// 再 Save 设置不擦 Key。
	if _, err := svc.SaveCatSettings(ctx, cat.Settings{
		ProvidersByAgent: map[string]cat.ProviderSettings{
			catagent.KindCatBuild: {EnabledCustom: true, Protocol: "openai", ApiUrl: "https://example.test/v1"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.GetCatSettings(ctx)
	if !got.ProvidersByAgent[catagent.KindCatBuild].ApiKeySet {
		t.Fatal("Save wiped key")
	}

	// Clear → apiKeySet false；enabledCustom / apiUrl 保留。
	cleared, err := svc.ClearCatProviderKey(ctx, cat.ClearProviderKeyRequest{AgentKind: catagent.KindCatBuild})
	if err != nil || cleared.ApiKeySet || !cleared.EnabledCustom || cleared.ApiUrl != "https://example.test/v1" {
		t.Fatalf("clear %+v %v", cleared, err)
	}
	got, _ = svc.GetCatSettings(ctx)
	p = got.ProvidersByAgent[catagent.KindCatBuild]
	if p.ApiKeySet || !p.EnabledCustom {
		t.Fatalf("after clear %+v", p)
	}
}

func TestProviderValidation(t *testing.T) {
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})

	cases := []struct {
		name string
		req  cat.Settings
	}{
		{"unknown kind", cat.Settings{ProvidersByAgent: map[string]cat.ProviderSettings{"cat_cli": {}}}},
		{"bad protocol", cat.Settings{ProvidersByAgent: map[string]cat.ProviderSettings{catagent.KindCatBuild: {Protocol: "gemini"}}}},
		{"locked protocol", cat.Settings{ProvidersByAgent: map[string]cat.ProviderSettings{catagent.KindCatBuild: {Protocol: "anthropic"}}}},
		{"url too long", cat.Settings{ProvidersByAgent: map[string]cat.ProviderSettings{catagent.KindCatBuild: {ApiUrl: strings.Repeat("u", 2049)}}}},
	}
	for _, tc := range cases {
		if _, err := svc.SaveCatSettings(ctx, tc.req); !apperr.Is(err, apperr.InvalidArgument) {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
	if _, err := svc.SaveCatProviderKey(ctx, cat.SaveProviderKeyRequest{AgentKind: catagent.KindCatBuild, ApiKey: "  "}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("empty key: %v", err)
	}
	if _, err := svc.SaveCatProviderKey(ctx, cat.SaveProviderKeyRequest{AgentKind: "nope", ApiKey: "x"}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("bad kind: %v", err)
	}
}

func TestListCatProviderCapabilitiesOrderAndDefaults(t *testing.T) {
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	list, err := svc.ListCatProviderCapabilities(context.Background())
	if err != nil || len(list) != 3 {
		t.Fatalf("%v %v", list, err)
	}
	want := []struct {
		kind, proto string
	}{
		{catagent.KindCatBuild, "openai"},
		{catagent.KindClaudeCLI, "anthropic"},
		{catagent.KindCodexCLI, "openai"},
	}
	for i, w := range want {
		c := list[i]
		if c.AgentKind != w.kind || c.DefaultProtocol != w.proto || c.AllowProtocolPick || len(c.AllowedProtocols) == 0 || c.AllowedProtocols[0] != w.proto {
			t.Fatalf("[%d] %+v", i, c)
		}
	}
}

func TestGetCatSettingsNeverLeaksKey(t *testing.T) {
	ctx := context.Background()
	secret := "sk-super-secret-never-leak"
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	if _, err := svc.SaveCatProviderKey(ctx, cat.SaveProviderKeyRequest{AgentKind: catagent.KindClaudeCLI, ApiKey: secret}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveCatSettings(ctx, cat.Settings{
		ProvidersByAgent: map[string]cat.ProviderSettings{
			catagent.KindClaudeCLI: {EnabledCustom: true, ApiUrl: "https://api.example"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetCatSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "super-secret") {
		t.Fatalf("settings JSON leaked key: %s", raw)
	}
	p := got.ProvidersByAgent[catagent.KindClaudeCLI]
	if !p.ApiKeySet || p.ApiKey != "" {
		t.Fatalf("%+v", p)
	}

	view, _ := svc.SaveCatProviderKey(ctx, cat.SaveProviderKeyRequest{AgentKind: catagent.KindClaudeCLI, ApiKey: secret + "-2"})
	raw2, _ := json.Marshal(view)
	if strings.Contains(string(raw2), secret) {
		t.Fatalf("key response leaked: %s", raw2)
	}
}

func TestSendInjectsProviderOverlay(t *testing.T) {
	ctx := context.Background()
	var got catagent.ProviderOverlay
	a := &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		got = o.Provider
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "ok"}}, nil
	}}
	svc, _ := newProjSvc(t, openStore(t), a, cat.Config{})
	if _, err := svc.SaveCatSettings(ctx, cat.Settings{
		ProvidersByAgent: map[string]cat.ProviderSettings{
			catagent.KindCatBuild: {EnabledCustom: true, Protocol: "openai", ApiUrl: "https://gw.test/v1"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveCatProviderKey(ctx, cat.SaveProviderKeyRequest{AgentKind: catagent.KindCatBuild, ApiKey: "sk-run"}); err != nil {
		t.Fatal(err)
	}
	c, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	if !got.Enabled || got.ApiURL != "https://gw.test/v1" || got.ApiKey != "sk-run" || got.AgentKind != catagent.KindCatBuild {
		t.Fatalf("overlay %+v", got)
	}
}
