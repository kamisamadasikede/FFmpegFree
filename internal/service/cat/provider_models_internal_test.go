package cat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchProviderModelsHeadersAndPaths(t *testing.T) {
	var gotPath, gotQuery string
	var hdr http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, hdr = r.URL.Path, r.URL.RawQuery, r.Header.Clone()
		_, _ = w.Write([]byte(`{"data":[{"id":"b"},{"id":"a","display_name":"A 模型"},{"id":"b"},{"id":"-bad"},{"id":"has space"}]}`))
	}))
	defer srv.Close()

	st, _, ms := fetchProviderModels(context.Background(), "openai", srv.URL+"/v1", "k1")
	if st != ProviderModelsOK || gotPath != "/v1/models" || hdr.Get("Authorization") != "Bearer k1" || hdr.Get("x-api-key") != "" {
		t.Fatalf("openai: %s %s %v", st, gotPath, hdr)
	}
	if len(ms) != 2 || ms[0].ID != "b" || ms[0].DisplayName != "b" || ms[1].DisplayName != "A 模型" || ms[0].Efforts == nil || len(ms[0].Efforts) != 0 {
		t.Fatalf("parse/dedupe/order: %+v", ms)
	}
	st, _, _ = fetchProviderModels(context.Background(), "anthropic", srv.URL, "k2")
	if st != ProviderModelsOK || gotPath != "/v1/models" || gotQuery != "limit=1000" ||
		hdr.Get("x-api-key") != "k2" || hdr.Get("Authorization") != "Bearer k2" || hdr.Get("anthropic-version") != "2023-06-01" {
		t.Fatalf("anthropic: %s %s?%s %v", st, gotPath, gotQuery, hdr)
	}
}

func TestFetchProviderModelsStatusMapping(t *testing.T) {
	cases := []struct {
		code int
		body string
		want string
	}{
		{200, `{"data":[]}`, ProviderModelsOK},
		{200, `{"object":"list"}`, ProviderModelsBadResponse},
		{200, `<html>`, ProviderModelsBadResponse},
		{401, ``, ProviderModelsUnauthorized},
		{403, ``, ProviderModelsUnauthorized},
		{404, ``, ProviderModelsNotSupported},
		{405, ``, ProviderModelsNotSupported},
		{501, ``, ProviderModelsNotSupported},
		{500, ``, ProviderModelsUnreachable},
		{502, ``, ProviderModelsUnreachable},
		{503, ``, ProviderModelsUnreachable},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.code)
			_, _ = w.Write([]byte(tc.body))
		}))
		st, _, ms := fetchProviderModels(context.Background(), "openai", srv.URL, "k")
		srv.Close()
		if st != tc.want {
			t.Errorf("%d %q: got %s want %s", tc.code, tc.body, st, tc.want)
		}
		if tc.want == ProviderModelsOK && ms == nil {
			t.Errorf("ok must give non-nil slice")
		}
	}
}

func TestFetchProviderModelsRedirectNotFollowed(t *testing.T) {
	hit := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		_, _ = w.Write([]byte(`{"data":[{"id":"x"}]}`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/models", http.StatusFound)
	}))
	defer srv.Close()
	st, _, _ := fetchProviderModels(context.Background(), "openai", srv.URL, "k")
	if st != ProviderModelsUnreachable || hit {
		t.Fatalf("redirect followed? state=%s hit=%v", st, hit)
	}
}

func TestFetchProviderModelsBodyCap(t *testing.T) {
	big := `{"data":[{"id":"x","pad":"` + strings.Repeat("a", providerModelsMaxBody) + `"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(big)) }))
	defer srv.Close()
	if st, _, _ := fetchProviderModels(context.Background(), "openai", srv.URL, "k"); st != ProviderModelsBadResponse {
		t.Fatalf("oversized: %s", st)
	}
	// 刚好 2MB 以内能解析。
	ok := `{"data":[{"id":"x","pad":"` + strings.Repeat("a", providerModelsMaxBody-100) + `"}]}`
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(ok)) }))
	defer srv2.Close()
	if st, _, _ := fetchProviderModels(context.Background(), "openai", srv2.URL, "k"); st != ProviderModelsOK {
		t.Fatalf("under cap: %s", st)
	}
}

func TestFetchProviderModelsTimeout(t *testing.T) {
	old := providerModelsTimeout
	providerModelsTimeout = 200 * time.Millisecond
	defer func() { providerModelsTimeout = old }()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	start := time.Now()
	st, _, _ := fetchProviderModels(context.Background(), "openai", srv.URL, "k")
	if st != ProviderModelsUnreachable || time.Since(start) > 3*time.Second {
		t.Fatalf("timeout: %s after %v", st, time.Since(start))
	}
	if providerModelsTimeout != 200*time.Millisecond || old != 8*time.Second {
		t.Fatalf("default timeout must be 8s, got %v", old)
	}
}

func TestFetchProviderModelsDNSFailure(t *testing.T) {
	if st, _, _ := fetchProviderModels(context.Background(), "openai", "http://nonexistent.invalid", "k"); st != ProviderModelsUnreachable {
		t.Fatalf("dns: %s", st)
	}
}

func TestValidCustomModelID(t *testing.T) {
	good := []string{"gpt-5.1", "my/gw:model@v2", "模型", strings.Repeat("a", 200)}
	bad := []string{"", "  ", "-m", "--model", "a b", "a\tb", "a\x00b", strings.Repeat("a", 201)}
	for _, g := range good {
		if !ValidCustomModelID(g) {
			t.Errorf("want ok %q", g)
		}
	}
	for _, b := range bad {
		if ValidCustomModelID(b) {
			t.Errorf("want bad %q", b)
		}
	}
}
