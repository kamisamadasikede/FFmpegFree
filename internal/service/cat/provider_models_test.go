package cat_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

type logRec struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (l *logRec) logf(f string, a ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(&l.buf, f+"\n", a...)
}
func (l *logRec) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.buf.String() }

const secretKey = "sk-very-secret-123"

func setupProvider(t *testing.T, url string) (*cat.Service, *logRec) {
	t.Helper()
	ctx := context.Background()
	lr := &logRec{}
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{Logf: lr.logf})
	if _, err := svc.SaveCatSettings(ctx, cat.Settings{ProvidersByAgent: map[string]cat.ProviderSettings{
		catagent.KindCodexCLI: {EnabledCustom: true, ApiUrl: url},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveCatProviderKey(ctx, cat.SaveProviderKeyRequest{AgentKind: catagent.KindCodexCLI, ApiKey: secretKey}); err != nil {
		t.Fatal(err)
	}
	return svc, lr
}

func TestListCatProviderModelsFlowCacheAndNoSecretsInLogs(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"data":[{"id":"gw-1","display_name":"GW One"},{"id":"gw-2"}],"secret_body_marker":"zzz"}`))
	}))
	defer srv.Close()
	ctx := context.Background()
	svc, lr := setupProvider(t, srv.URL+"/v1/")

	r, err := svc.ListCatProviderModels(ctx, cat.ListCatProviderModelsRequest{AgentKind: catagent.KindCodexCLI})
	if err != nil || r.State != "ok" || len(r.Models) != 2 || r.Models[0].DisplayName != "GW One" || r.FetchedAt == "" {
		t.Fatalf("%+v %v", r, err)
	}
	// 缓存：再拉不打服务；refresh 打。
	_, _ = svc.ListCatProviderModels(ctx, cat.ListCatProviderModelsRequest{AgentKind: catagent.KindCodexCLI})
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("cache miss: %d", hits)
	}
	_, _ = svc.ListCatProviderModels(ctx, cat.ListCatProviderModelsRequest{AgentKind: catagent.KindCodexCLI, Refresh: true})
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("refresh: %d", hits)
	}
	// ListCatModels 跟着走服务商列表（同一缓存）。
	ms, err := svc.ListCatModels(ctx, catagent.KindCodexCLI)
	if err != nil || len(ms) != 2 || ms[1].ID != "gw-2" || atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("ListCatModels %+v %v hits=%d", ms, err, hits)
	}
	// 换 Key → 清缓存。
	if _, err := svc.SaveCatProviderKey(ctx, cat.SaveProviderKeyRequest{AgentKind: catagent.KindCodexCLI, ApiKey: "other-key"}); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.ListCatProviderModels(ctx, cat.ListCatProviderModelsRequest{AgentKind: catagent.KindCodexCLI})
	if atomic.LoadInt32(&hits) != 3 {
		t.Fatalf("key change must invalidate: %d", hits)
	}
	// 改地址 → 清缓存。
	if _, err := svc.SaveCatSettings(ctx, cat.Settings{ProvidersByAgent: map[string]cat.ProviderSettings{
		catagent.KindCodexCLI: {EnabledCustom: true, ApiUrl: srv.URL + "/v1"},
	}}); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.ListCatProviderModels(ctx, cat.ListCatProviderModelsRequest{AgentKind: catagent.KindCodexCLI})
	if atomic.LoadInt32(&hits) != 4 {
		t.Fatalf("url change must invalidate: %d", hits)
	}
	logs := lr.String()
	for _, bad := range []string{secretKey, "other-key", "Bearer", "secret_body_marker", "zzz", "Authorization"} {
		if strings.Contains(logs, bad) {
			t.Fatalf("log leaked %q:\n%s", bad, logs)
		}
	}
	if !strings.Contains(logs, "state=ok") {
		t.Fatalf("expected summary log:\n%s", logs)
	}
}

func TestListCatProviderModelsFailuresNotCached(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"bad key ` + secretKey + `"}`))
	}))
	defer srv.Close()
	svc, _ := setupProvider(t, srv.URL)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		r, err := svc.ListCatProviderModels(ctx, cat.ListCatProviderModelsRequest{AgentKind: catagent.KindCodexCLI})
		if err != nil || r.State != "unauthorized" || r.Models == nil || len(r.Models) != 0 || r.FetchedAt != "" {
			t.Fatalf("%+v %v", r, err)
		}
	}
	if hits != 2 {
		t.Fatalf("failure cached: %d", hits)
	}
	if ms, err := svc.ListCatModels(ctx, catagent.KindCodexCLI); err != nil || ms == nil || len(ms) != 0 {
		t.Fatalf("ListCatModels on failure: %+v %v", ms, err)
	}
}

func TestListCatProviderModelsNotConfiguredAndBadArg(t *testing.T) {
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	if _, err := svc.ListCatProviderModels(ctx, cat.ListCatProviderModelsRequest{AgentKind: "nope"}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("bad kind: %v", err)
	}
	r, err := svc.ListCatProviderModels(ctx, cat.ListCatProviderModelsRequest{AgentKind: catagent.KindClaudeCLI})
	if err != nil || r.State != "not_configured" || r.Models == nil {
		t.Fatalf("%+v %v", r, err)
	}
	// 开了但没 Key / 地址非法 → not_configured。
	for _, u := range []string{"https://gw.example.com", "ftp://x", "not a url", ""} {
		if _, err := svc.SaveCatSettings(ctx, cat.Settings{ProvidersByAgent: map[string]cat.ProviderSettings{
			catagent.KindClaudeCLI: {EnabledCustom: true, ApiUrl: u},
		}}); err != nil {
			t.Fatal(err)
		}
		r, _ = svc.ListCatProviderModels(ctx, cat.ListCatProviderModelsRequest{AgentKind: catagent.KindClaudeCLI})
		if r.State != "not_configured" {
			t.Fatalf("%q: %+v", u, r)
		}
	}
}

func TestSaveSettingsManualModelWhenCustom(t *testing.T) {
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	prov := map[string]cat.ProviderSettings{catagent.KindCodexCLI: {EnabledCustom: true, ApiUrl: "https://gw.example.com/v1"}}
	save := func(model, effort string) error {
		_, err := svc.SaveCatSettings(ctx, cat.Settings{
			ProvidersByAgent: prov,
			DefaultsByAgent:  map[string]cat.AgentDefaults{catagent.KindCodexCLI: {Model: model, Effort: effort}},
		})
		return err
	}
	if err := save("  my-private-model  ", ""); err != nil {
		t.Fatalf("manual id rejected: %v", err)
	}
	for _, bad := range []string{"-c", "a b", strings.Repeat("x", 201)} {
		if err := save(bad, ""); !apperr.Is(err, apperr.InvalidArgument) {
			t.Fatalf("%q accepted: %v", bad, err)
		}
	}
	if err := save("my-private-model", "high"); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("effort on manual model accepted: %v", err)
	}
	// 关自定义不清模型（6.19.20.2 ⑨）。
	got, err := svc.SaveCatSettings(ctx, cat.Settings{
		ProvidersByAgent: map[string]cat.ProviderSettings{catagent.KindCodexCLI: {EnabledCustom: false, ApiUrl: "https://gw.example.com/v1"}},
		DefaultsByAgent:  map[string]cat.AgentDefaults{catagent.KindCodexCLI: {Model: "my-private-model"}},
	})
	if err != nil || got.DefaultsByAgent[catagent.KindCodexCLI].Model != "my-private-model" {
		t.Fatalf("%+v %v", got.DefaultsByAgent, err)
	}
}
