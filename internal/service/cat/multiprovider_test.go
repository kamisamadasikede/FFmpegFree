package cat_test

import (
	"context"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func TestCreateClaudeAndCodexAllowed(t *testing.T) {
	ctx := context.Background()
	reg := catagent.NewRegistry()
	reg.Register(&fakeAdapter{})
	reg.Register(catagent.NewStubAdapter(catagent.KindClaudeCLI))
	reg.Register(catagent.NewStubAdapter(catagent.KindCodexCLI))
	svc := cat.New(cat.Config{Store: openStore(t), Registry: reg})
	for _, kind := range []string{catagent.KindClaudeCLI, catagent.KindCodexCLI} {
		c, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: kind})
		if err != nil || c.AgentKind != kind {
			t.Fatalf("%s: %+v %v", kind, c, err)
		}
	}
	if _, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: "cat_cli"}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("cat_cli: %v", err)
	}
}

func TestListModelsEmptyForUnknownAndStub(t *testing.T) {
	ctx := context.Background()
	reg := catagent.NewRegistry()
	reg.Register(catagent.NewStubAdapter(catagent.KindClaudeCLI))
	svc := cat.New(cat.Config{Store: openStore(t), Registry: reg})
	for _, kind := range []string{"nope", catagent.KindClaudeCLI, catagent.KindCodexCLI} {
		models, err := svc.ListCatModels(ctx, kind)
		if err != nil || models == nil || len(models) != 0 {
			t.Fatalf("%s models: %v %v", kind, models, err)
		}
		thinks, err := svc.ListCatThinkLevels(ctx, kind)
		if err != nil || thinks == nil || len(thinks) != 0 {
			t.Fatalf("%s thinks: %v %v", kind, thinks, err)
		}
	}
}

func TestGetCatStatusAgentsOrder(t *testing.T) {
	ctx := context.Background()
	reg := catagent.NewRegistry()
	reg.Register(&fakeAdapter{})
	reg.Register(catagent.NewStubAdapter(catagent.KindClaudeCLI))
	reg.Register(catagent.NewStubAdapter(catagent.KindCodexCLI))
	svc := cat.New(cat.Config{Store: openStore(t), Registry: reg})
	st, err := svc.GetCatStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != catagent.StateReady {
		t.Fatalf("top state %+v", st)
	}
	want := []string{catagent.KindCatBuild, catagent.KindClaudeCLI, catagent.KindCodexCLI}
	if len(st.Agents) != 3 {
		t.Fatalf("agents %d", len(st.Agents))
	}
	for i, kind := range want {
		if st.Agents[i].AgentKind != kind {
			t.Fatalf("order[%d]=%s want %s", i, st.Agents[i].AgentKind, kind)
		}
	}
	if !st.Agents[0].Ready || st.Agents[1].Ready || st.Agents[2].Ready {
		t.Fatalf("ready flags %+v", st.Agents)
	}
	var got *catagent.Status
	svc2 := cat.New(cat.Config{
		Store: openStore(t), Registry: reg,
		Emit: func(event string, payload any) {
			if event == catagent.EventStatus {
				s := payload.(catagent.Status)
				got = &s
			}
		},
	})
	out, err := svc2.RecheckCat(ctx, "")
	if err != nil || len(out.Agents) != 3 {
		t.Fatalf("%+v %v", out, err)
	}
	if got == nil || len(got.Agents) != 3 {
		t.Fatalf("emit %+v", got)
	}
}

func TestCreateUsesSettingsDefaultAgentKind(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	reg := catagent.NewRegistry()
	reg.Register(&fakeAdapter{})
	reg.Register(catagent.NewStubAdapter(catagent.KindCodexCLI))
	var svc *cat.Service
	svc = cat.New(cat.Config{
		Store: st, Registry: reg,
		DefaultAgentKind: func(ctx context.Context) string {
			s, err := svc.GetCatSettings(ctx)
			if err != nil || s.DefaultAgentKind == "" {
				return catagent.KindCatBuild
			}
			return s.DefaultAgentKind
		},
	})
	if _, err := svc.SaveCatSettings(ctx, cat.Settings{DefaultAgentKind: catagent.KindCodexCLI}); err != nil {
		t.Fatal(err)
	}
	c, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{})
	if err != nil || c.AgentKind != catagent.KindCodexCLI {
		t.Fatalf("%+v %v", c, err)
	}
}
