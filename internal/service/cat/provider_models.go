package cat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
)

// 契约 6.19.20.2 ListCatProviderModels。

// ListCatProviderModelsRequest 是 ListCatProviderModels 的参数。
type ListCatProviderModelsRequest struct {
	AgentKind string `json:"agentKind"`
	Refresh   bool   `json:"refresh"`
}

// CatProviderModelList 是 ListCatProviderModels 的结果。
type CatProviderModelList struct {
	State     string  `json:"state"`
	Models    []Model `json:"models"`
	FetchedAt string  `json:"fetchedAt"`
}

// 状态值（6.19.20.2 ④）。
const (
	ProviderModelsOK            = "ok"
	ProviderModelsNotConfigured = "not_configured"
	ProviderModelsUnauthorized  = "unauthorized"
	ProviderModelsUnreachable   = "unreachable"
	ProviderModelsNotSupported  = "not_supported"
	ProviderModelsBadResponse   = "bad_response"
)

// 测试可改（provider_models_internal_test.go）。
var (
	providerModelsTimeout  = 8 * time.Second
	providerModelsCacheTTL = 10 * time.Minute
)

const (
	providerModelsMaxBody  = 2 << 20 // 2 MB
	anthropicVersionHeader = "2023-06-01"
	// MaxModelIDRunes 是手填模型 id 上限（6.19.20.2 ⑧）。
	MaxModelIDRunes = 200
)

type providerModelsCacheEntry struct {
	key       string
	list      CatProviderModelList
	expiresAt time.Time
}

type providerModelsCache struct {
	mu sync.Mutex
	m  map[string]providerModelsCacheEntry // agentKind → entry
}

var errRedirect = errors.New("redirect not followed")

// providerHTTP 是拉模型列表用的 client；不跟随重定向（防 Key 被带走），整体 8 秒。测试可替换。
var providerHTTP = func() *http.Client {
	return &http.Client{
		Timeout: providerModelsTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errRedirect
		},
	}
}

var providerNow = time.Now

func (s *Service) pmCache() *providerModelsCache {
	s.pmOnce.Do(func() { s.pm = &providerModelsCache{m: map[string]providerModelsCacheEntry{}} })
	return s.pm
}

// invalidateProviderModels 清一家的缓存（改设置 / 存删 Key 时调）。
func (s *Service) invalidateProviderModels(kind string) {
	c := s.pmCache()
	c.mu.Lock()
	delete(c.m, kind)
	c.mu.Unlock()
}

func emptyProviderList(state string) CatProviderModelList {
	return CatProviderModelList{State: state, Models: []Model{}}
}

// ListCatProviderModels 按协议调服务商 /models。只有参数错误返回 error；其他放 state。
func (s *Service) ListCatProviderModels(ctx context.Context, req ListCatProviderModelsRequest) (CatProviderModelList, error) {
	kind := strings.TrimSpace(req.AgentKind)
	if !catagent.Phase1Creatable(kind) {
		return CatProviderModelList{}, apperr.New(apperr.InvalidArgument, "助手模式设置不正确").WithDetail("reason=agent_kind")
	}
	o := s.providerOverlayFor(ctx, kind)
	if !o.Enabled {
		return emptyProviderList(ProviderModelsNotConfigured), nil
	}
	base := strings.TrimRight(strings.TrimSpace(o.ApiURL), "/")
	u, err := url.Parse(base)
	if base == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.TrimSpace(o.ApiKey) == "" {
		return emptyProviderList(ProviderModelsNotConfigured), nil
	}
	proto := o.Protocol
	if proto == "" {
		proto = "openai"
	}
	sum := sha256.Sum256([]byte(o.ApiKey))
	cacheKey := base + "\x00" + proto + "\x00" + hex.EncodeToString(sum[:])

	c := s.pmCache()
	if !req.Refresh {
		c.mu.Lock()
		e, ok := c.m[kind]
		c.mu.Unlock()
		if ok && e.key == cacheKey && providerNow().Before(e.expiresAt) {
			out := e.list
			out.Models = append([]Model{}, e.list.Models...)
			return out, nil
		}
	}

	start := providerNow()
	state, status, models := fetchProviderModels(ctx, proto, base, o.ApiKey)
	// 只记 agentKind / host / 状态码 / state / 耗时（6.19.20.2 ⑥）。
	s.cfg.Logf("cat provider models: agent=%s host=%s status=%d state=%s ms=%d", kind, u.Host, status, state, providerNow().Sub(start).Milliseconds())
	if state != ProviderModelsOK {
		return emptyProviderList(state), nil
	}
	out := CatProviderModelList{State: ProviderModelsOK, Models: models, FetchedAt: providerNow().UTC().Format(time.RFC3339)}
	c.mu.Lock()
	c.m[kind] = providerModelsCacheEntry{key: cacheKey, list: out, expiresAt: providerNow().Add(providerModelsCacheTTL)}
	c.mu.Unlock()
	ret := out
	ret.Models = append([]Model{}, models...)
	return ret, nil
}

// fetchProviderModels 发请求并映射状态；返回 (state, httpStatus, models)。
func fetchProviderModels(ctx context.Context, proto, base, key string) (string, int, []Model) {
	var target string
	if proto == "anthropic" {
		target = base + "/v1/models?limit=1000"
	} else {
		target = base + "/models"
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), providerModelsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, target, nil)
	if err != nil {
		return ProviderModelsNotConfigured, 0, nil
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	if proto == "anthropic" {
		// https://docs.anthropic.com/en/api/models-list：x-api-key + anthropic-version；
		// 网关按 code.claude.com/docs/en/llm-gateway-protocol「Model discovery」两个凭据头都发。
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", anthropicVersionHeader)
	}
	resp, err := providerHTTP().Do(req)
	if err != nil {
		// 连接 / DNS / TLS / 超时 / 重定向（CheckRedirect）→ unreachable。错误原文不外传。
		return ProviderModelsUnreachable, 0, nil
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ProviderModelsUnauthorized, resp.StatusCode, nil
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented:
		return ProviderModelsNotSupported, resp.StatusCode, nil
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return ProviderModelsUnreachable, resp.StatusCode, nil
	case resp.StatusCode >= 500:
		return ProviderModelsUnreachable, resp.StatusCode, nil
	case resp.StatusCode != http.StatusOK:
		return ProviderModelsBadResponse, resp.StatusCode, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, providerModelsMaxBody+1))
	if err != nil {
		return ProviderModelsUnreachable, resp.StatusCode, nil
	}
	if len(body) > providerModelsMaxBody {
		return ProviderModelsBadResponse, resp.StatusCode, nil
	}
	models, ok := parseProviderModels(body)
	if !ok {
		return ProviderModelsBadResponse, resp.StatusCode, nil
	}
	return ProviderModelsOK, resp.StatusCode, models
}

// parseProviderModels 读 {"data":[{"id":..., "display_name"?:...}]}；id 去重、保序。
func parseProviderModels(body []byte) ([]Model, bool) {
	var doc struct {
		Data *[]struct {
			ID          *string `json:"id"`
			DisplayName string  `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &doc); err != nil || doc.Data == nil {
		return nil, false
	}
	out := []Model{}
	seen := map[string]bool{}
	for _, d := range *doc.Data {
		if d.ID == nil {
			continue
		}
		id := strings.TrimSpace(*d.ID)
		if id == "" || seen[id] || !ValidCustomModelID(id) {
			continue
		}
		seen[id] = true
		name := strings.TrimSpace(d.DisplayName)
		if name == "" {
			name = id
		}
		out = append(out, Model{ID: id, DisplayName: name, Efforts: []string{}, DefaultEffort: ""})
	}
	return out, true
}

// ValidCustomModelID：去空格后 1–200 字符、无空白 / 控制字符、不以 - 开头（6.19.20.2 ⑧）。
func ValidCustomModelID(id string) bool {
	id = strings.TrimSpace(id)
	n := utf8.RuneCountInString(id)
	if n < 1 || n > MaxModelIDRunes || strings.HasPrefix(id, "-") || !utf8.ValidString(id) {
		return false
	}
	for _, r := range id {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// customEnabled 报告该家是否开了自定义服务商。
func (s *Service) customEnabled(ctx context.Context, kind string) bool {
	return s.providerOverlayFor(ctx, kind).Enabled
}
