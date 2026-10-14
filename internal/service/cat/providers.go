package cat

import (
	"context"
	"strings"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
)

// ListCatProviderCapabilities 返回三家固定顺序的协议能力（契约 6.19.12.8）。
// allowProtocolPick 默认 false（附录 C4 GUESS）；allowedProtocols 至少含 defaultProtocol。
func (s *Service) ListCatProviderCapabilities(ctx context.Context) ([]ProviderCapability, error) {
	_ = ctx
	_ = s
	return defaultProviderCapabilities(), nil
}

func defaultProviderCapabilities() []ProviderCapability {
	// 产品默认：cat_build→openai，claude_cli→anthropic，codex_cli→openai。
	return []ProviderCapability{
		{AgentKind: catagent.KindCatBuild, DefaultProtocol: "openai", AllowProtocolPick: false, AllowedProtocols: []string{"openai"}},
		{AgentKind: catagent.KindClaudeCLI, DefaultProtocol: "anthropic", AllowProtocolPick: false, AllowedProtocols: []string{"anthropic"}},
		{AgentKind: catagent.KindCodexCLI, DefaultProtocol: "openai", AllowProtocolPick: false, AllowedProtocols: []string{"openai"}},
	}
}

func capabilityByKind(kind string) (ProviderCapability, bool) {
	for _, c := range defaultProviderCapabilities() {
		if c.AgentKind == kind {
			return c, true
		}
	}
	return ProviderCapability{}, false
}

// SaveCatProviderKey 写入一家的 API Key（唯一明文入口）；空串 → INVALID_ARGUMENT。
// 响应含 apiKeySet=true，无明文 Key。
func (s *Service) SaveCatProviderKey(ctx context.Context, req SaveProviderKeyRequest) (ProviderSettings, error) {
	if s.cfg.Store == nil {
		return ProviderSettings{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	kind := strings.TrimSpace(req.AgentKind)
	if !catagent.Phase1Creatable(kind) {
		return ProviderSettings{}, apperr.New(apperr.InvalidArgument, "助手模式设置不正确").WithDetail("reason=agent_kind")
	}
	key := strings.TrimSpace(req.ApiKey)
	if key == "" {
		return ProviderSettings{}, apperr.New(apperr.InvalidArgument, "请填写 API Key。").WithDetail("reason=api_key")
	}
	secrets, err := s.loadProviderSecrets(ctx)
	if err != nil {
		return ProviderSettings{}, err
	}
	secrets[kind] = key
	if err := s.cfg.Store.SetSetting(ctx, ProviderSecretsKey, secrets); err != nil {
		return ProviderSettings{}, apperr.Wrap(apperr.IOError, "保存失败，请重试。", err)
	}
	s.invalidateProviderModels(kind)
	return s.providerView(ctx, kind)
}

// ClearCatProviderKey 删除一家已存 Key → apiKeySet=false；不改 enabledCustom / apiUrl。
func (s *Service) ClearCatProviderKey(ctx context.Context, req ClearProviderKeyRequest) (ProviderSettings, error) {
	if s.cfg.Store == nil {
		return ProviderSettings{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	kind := strings.TrimSpace(req.AgentKind)
	if !catagent.Phase1Creatable(kind) {
		return ProviderSettings{}, apperr.New(apperr.InvalidArgument, "助手模式设置不正确").WithDetail("reason=agent_kind")
	}
	secrets, err := s.loadProviderSecrets(ctx)
	if err != nil {
		return ProviderSettings{}, err
	}
	delete(secrets, kind)
	if err := s.cfg.Store.SetSetting(ctx, ProviderSecretsKey, secrets); err != nil {
		return ProviderSettings{}, apperr.Wrap(apperr.IOError, "保存失败，请重试。", err)
	}
	s.invalidateProviderModels(kind)
	return s.providerView(ctx, kind)
}

func (s *Service) providerView(ctx context.Context, kind string) (ProviderSettings, error) {
	st, err := s.GetCatSettings(ctx)
	if err != nil {
		return ProviderSettings{}, err
	}
	p, ok := st.ProvidersByAgent[kind]
	if !ok {
		p = ProviderSettings{}
	}
	// GetCatSettings 已填 apiKeySet；再保险一次。
	secrets, _ := s.loadProviderSecrets(ctx)
	_, p.ApiKeySet = secrets[kind]
	p.ApiKey = ""
	return p, nil
}

func (s *Service) loadProviderSecrets(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	if s.cfg.Store == nil {
		return out, nil
	}
	found, err := s.cfg.Store.GetSetting(ctx, ProviderSecretsKey, &out)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "读取设置失败", err)
	}
	if !found || out == nil {
		return map[string]string{}, nil
	}
	// 去掉空值。
	clean := make(map[string]string, len(out))
	for k, v := range out {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k != "" && v != "" {
			clean[k] = v
		}
	}
	return clean, nil
}

func (s *Service) fillProviderKeyFlags(ctx context.Context, v *Settings) {
	secrets, err := s.loadProviderSecrets(ctx)
	if err != nil {
		s.cfg.Logf("cat provider secrets: %v", err)
		secrets = map[string]string{}
	}
	if v.ProvidersByAgent == nil {
		v.ProvidersByAgent = map[string]ProviderSettings{}
	}
	for k, p := range v.ProvidersByAgent {
		_, p.ApiKeySet = secrets[k]
		p.ApiKey = ""
		v.ProvidersByAgent[k] = p
	}
	// 已存 Key 但尚未写过 providers 条目的家：回显 apiKeySet=true（缺 key 其它字段默认关/空）。
	for k := range secrets {
		if !catagent.Phase1Creatable(k) {
			continue
		}
		if _, ok := v.ProvidersByAgent[k]; !ok {
			v.ProvidersByAgent[k] = ProviderSettings{ApiKeySet: true}
		}
	}
}

func validateProvidersByAgent(m map[string]ProviderSettings) error {
	if m == nil {
		return nil
	}
	caps := map[string]ProviderCapability{}
	for _, c := range defaultProviderCapabilities() {
		caps[c.AgentKind] = c
	}
	for k, p := range m {
		kind := strings.TrimSpace(k)
		if !catagent.Phase1Creatable(kind) {
			return apperr.New(apperr.InvalidArgument, "助手模式设置不正确").WithDetail("reason=providers_by_agent_key")
		}
		proto := strings.TrimSpace(p.Protocol)
		if proto != "" && proto != "openai" && proto != "anthropic" {
			return apperr.New(apperr.InvalidArgument, "协议设置不正确").WithDetail("reason=protocol")
		}
		cap, ok := caps[kind]
		if !ok {
			return apperr.New(apperr.InvalidArgument, "助手模式设置不正确").WithDetail("reason=providers_by_agent_key")
		}
		// C4 GUESS：allowProtocolPick=false 时，非空且 ≠ defaultProtocol → INVALID_ARGUMENT。
		if !cap.AllowProtocolPick && proto != "" && proto != cap.DefaultProtocol {
			return apperr.New(apperr.InvalidArgument, "协议设置不正确").WithDetail("reason=protocol_locked")
		}
		if utf8.RuneCountInString(strings.TrimSpace(p.ApiUrl)) > MaxApiUrlRunes {
			return apperr.New(apperr.InvalidArgument, "地址太长。").WithDetail("reason=api_url_too_long")
		}
	}
	return nil
}

// providerOverlayFor 按 agentKind 组装子进程注入参数（启用自定义时带 Key/URL）。
func (s *Service) providerOverlayFor(ctx context.Context, agentKind string) catagent.ProviderOverlay {
	kind := strings.TrimSpace(agentKind)
	if kind == "" {
		kind = catagent.KindCatBuild
	}
	st, err := s.GetCatSettings(ctx)
	if err != nil {
		s.cfg.Logf("cat provider overlay settings: %v", err)
		return catagent.ProviderOverlay{AgentKind: kind}
	}
	p := st.ProvidersByAgent[kind]
	if !p.EnabledCustom {
		return catagent.ProviderOverlay{AgentKind: kind}
	}
	secrets, err := s.loadProviderSecrets(ctx)
	if err != nil {
		s.cfg.Logf("cat provider overlay secrets: %v", err)
	}
	key := ""
	if secrets != nil {
		key = secrets[kind]
	}
	proto := strings.TrimSpace(p.Protocol)
	if proto == "" {
		if cap, ok := capabilityByKind(kind); ok {
			proto = cap.DefaultProtocol
		}
	}
	return catagent.ProviderOverlay{
		Enabled:   true,
		AgentKind: kind,
		Protocol:  proto,
		ApiURL:    strings.TrimSpace(p.ApiUrl),
		ApiKey:    key,
	}
}

// applyProviderOverlay 把自定义服务商写到适配器（List/Recheck 探测用）；无则清空。
func (s *Service) applyProviderOverlay(ctx context.Context, a catagent.Adapter, agentKind string) {
	if a == nil {
		return
	}
	o := s.providerOverlayFor(ctx, agentKind)
	if set, ok := a.(interface{ SetProviderOverlay(catagent.ProviderOverlay) }); ok {
		set.SetProviderOverlay(o)
	}
}

// applyAllProviderOverlays 在状态 / 重检前把三家自定义服务商写到适配器。
func (s *Service) applyAllProviderOverlays(ctx context.Context) {
	for _, kind := range catagent.CreatableKinds() {
		if a, ok := s.cfg.Registry.Get(kind); ok && a != nil {
			s.applyProviderOverlay(ctx, a, kind)
		}
	}
}
