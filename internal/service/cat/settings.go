package cat

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
)

// SettingsKey 是 settings 表里 Cat 设置的键（契约 6.19.12.1 第 1 条）。
const SettingsKey = "cat.settings"

// MaxCustomInstructionRunes 是全局自定义指令的字数上限。
const MaxCustomInstructionRunes = 4000

// AgentDefaults 是某一家的默认模型 / 思考强度（契约 v0.36 CatAgentDefaults）。
type AgentDefaults struct {
	Model  string `json:"model"`  // 空 = 不传 -m
	Effort string `json:"effort"` // 空 = 不传思考强度；否则 low|medium|high|xhigh
}

// Settings 是 Cat 设置（契约 6.19.12.1 CatSettings，v0.36 多服务商默认；v0.38 providersByAgent）。
type Settings struct {
	DefaultAgentKind    string                     `json:"defaultAgentKind"` // cat_build|claude_cli|codex_cli；默认 cat_build
	DefaultsByAgent     map[string]AgentDefaults   `json:"defaultsByAgent"`  // key = agentKind
	ProvidersByAgent    map[string]ProviderSettings `json:"providersByAgent"` // v0.38：key = agentKind
	DefaultApprovalMode string                     `json:"defaultApprovalMode"`
	CustomInstruction   string                     `json:"customInstruction"`
	// NotifyOnFinish 为 nil 视为 true；Get / Save 的返回值总是填好。
	NotifyOnFinish *bool `json:"notifyOnFinish"`
}

// ProviderSettings 是一家的自定义服务商视图（契约 CatProviderSettings）。
// ApiKeySet 只读回显；Get/Save 永不带明文 Key。Save 忽略请求里的 ApiKeySet / ApiKey。
type ProviderSettings struct {
	EnabledCustom bool   `json:"enabledCustom"`
	Protocol      string `json:"protocol"` // openai | anthropic；空 = defaultProtocol
	ApiUrl        string `json:"apiUrl"`
	ApiKeySet     bool   `json:"apiKeySet"`
	// ApiKey 若客户端误传则忽略，永不落库（契约 6.19.12.8）。
	ApiKey string `json:"apiKey,omitempty"`
}

// ProviderCapability 是适配器声明，不落库（契约 CatProviderCapability）。
type ProviderCapability struct {
	AgentKind         string   `json:"agentKind"`
	DefaultProtocol   string   `json:"defaultProtocol"`
	AllowProtocolPick bool     `json:"allowProtocolPick"`
	AllowedProtocols  []string `json:"allowedProtocols"`
}

// ProviderSecretsKey 是 settings 表里各家 API Key 的键（明文 map，不进 cat.settings）。
const ProviderSecretsKey = "cat.providerSecrets"

// MaxApiUrlRunes 是 apiUrl 长度上限（契约 6.19.12.3）。
const MaxApiUrlRunes = 2048

// SaveProviderKeyRequest / ClearProviderKeyRequest 是 v0.38 密钥绑定入参。
type SaveProviderKeyRequest struct {
	AgentKind string `json:"agentKind"`
	ApiKey    string `json:"apiKey"`
}

type ClearProviderKeyRequest struct {
	AgentKind string `json:"agentKind"`
}

// settingsWire 用于读库：同时接受 v0.35 顶层 defaultModel/defaultEffort 与 v0.36 新形状。
type settingsWire struct {
	DefaultAgentKind    string                      `json:"defaultAgentKind"`
	DefaultsByAgent     map[string]AgentDefaults    `json:"defaultsByAgent"`
	ProvidersByAgent    map[string]ProviderSettings `json:"providersByAgent"`
	DefaultApprovalMode string                      `json:"defaultApprovalMode"`
	CustomInstruction   string                      `json:"customInstruction"`
	NotifyOnFinish      *bool                       `json:"notifyOnFinish"`
	// Legacy v0.35：
	DefaultModel  string `json:"defaultModel"`
	DefaultEffort string `json:"defaultEffort"`
}

// NotifyEnabled 报告是否要发完成通知（nil = 开）。
func (s Settings) NotifyEnabled() bool { return s.NotifyOnFinish == nil || *s.NotifyOnFinish }

func (s Settings) normalized() Settings {
	s.DefaultAgentKind = strings.TrimSpace(s.DefaultAgentKind)
	if s.DefaultAgentKind == "" {
		s.DefaultAgentKind = catagent.KindCatBuild
	}
	if s.DefaultsByAgent == nil {
		s.DefaultsByAgent = map[string]AgentDefaults{}
	} else {
		out := make(map[string]AgentDefaults, len(s.DefaultsByAgent))
		for k, d := range s.DefaultsByAgent {
			d.Model = strings.TrimSpace(d.Model)
			d.Effort = strings.TrimSpace(d.Effort)
			out[k] = d
		}
		s.DefaultsByAgent = out
	}
	if s.ProvidersByAgent == nil {
		s.ProvidersByAgent = map[string]ProviderSettings{}
	} else {
		out := make(map[string]ProviderSettings, len(s.ProvidersByAgent))
		for k, p := range s.ProvidersByAgent {
			p.Protocol = strings.TrimSpace(p.Protocol)
			p.ApiUrl = strings.TrimSpace(p.ApiUrl)
			p.ApiKey = "" // 永不回显 / 落库明文
			out[k] = p
		}
		s.ProvidersByAgent = out
	}
	s.DefaultApprovalMode = strings.TrimSpace(s.DefaultApprovalMode)
	if s.NotifyOnFinish == nil {
		t := true
		s.NotifyOnFinish = &t
	}
	return s
}

// migrateFromWire 把旧顶层 defaultModel/defaultEffort 迁进 defaultsByAgent["cat_build"]（不覆盖已有非空）。
func migrateFromWire(w settingsWire) Settings {
	s := Settings{
		DefaultAgentKind:    w.DefaultAgentKind,
		DefaultsByAgent:     w.DefaultsByAgent,
		ProvidersByAgent:    w.ProvidersByAgent,
		DefaultApprovalMode: w.DefaultApprovalMode,
		CustomInstruction:   w.CustomInstruction,
		NotifyOnFinish:      w.NotifyOnFinish,
	}
	if s.DefaultsByAgent == nil {
		s.DefaultsByAgent = map[string]AgentDefaults{}
	}
	legacyModel := strings.TrimSpace(w.DefaultModel)
	legacyEffort := strings.TrimSpace(w.DefaultEffort)
	if legacyModel != "" || legacyEffort != "" {
		cur := s.DefaultsByAgent[catagent.KindCatBuild]
		if cur.Model == "" && legacyModel != "" {
			cur.Model = legacyModel
		}
		if cur.Effort == "" && legacyEffort != "" {
			cur.Effort = legacyEffort
		}
		s.DefaultsByAgent[catagent.KindCatBuild] = cur
	}
	return s
}

// ValidEffort 报告思考强度是否合法（空 = 默认）。
func ValidEffort(v string) bool {
	switch v {
	case "", "low", "medium", "high", "xhigh":
		return true
	}
	return false
}

// ValidApprovalMode 报告批准方式是否合法（空 = 全部批准）。
func ValidApprovalMode(v string) bool {
	switch v {
	case "", "normal", "plan", "auto", "approve":
		return true
	}
	return false
}

// ValidDefaultAgentKind 报告 defaultAgentKind 是否合法（空视为 cat_build）。
func ValidDefaultAgentKind(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return true
	}
	return catagent.Phase1Creatable(v)
}

// GetCatSettings 读 Cat 设置；没有记录 = 全部空值（通知默认开，defaultAgentKind=cat_build）。
// 读路径迁旧字段；不回写。cat.settings 缺 defaultAgentKind 时回落系统键 catDefaultAgentKind。
// v0.38：providersByAgent[].apiKeySet 由密钥存储计算；永不回明文 Key。
func (s *Service) GetCatSettings(ctx context.Context) (Settings, error) {
	if s.cfg.Store == nil {
		return Settings{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	var w settingsWire
	found, err := s.cfg.Store.GetSetting(ctx, SettingsKey, &w)
	if err != nil {
		return Settings{}.normalized(), apperr.Wrap(apperr.IOError, "读取设置失败", err)
	}
	v := migrateFromWire(w)
	// 仅当 JSON 未带 defaultAgentKind（或空）时，回落系统旧键。
	if strings.TrimSpace(w.DefaultAgentKind) == "" {
		if s.cfg.SystemDefaultAgentKind != nil {
			if k := strings.TrimSpace(s.cfg.SystemDefaultAgentKind(ctx)); k != "" && catagent.Phase1Creatable(k) {
				v.DefaultAgentKind = k
			}
		}
	}
	_ = found
	v = v.normalized()
	s.fillProviderKeyFlags(ctx, &v)
	return v, nil
}

// SaveCatSettings 整体保存并返回保存后的值（新形状，不含旧 defaultModel/defaultEffort）。
// 校验失败 INVALID_ARGUMENT；写入失败 IO_ERROR，库里仍是旧值（前端回滚，6.19.12.7）。
// v0.38：可写 providersByAgent 的 enabledCustom/protocol/apiUrl；忽略 apiKeySet 与明文 Key；不因 Save 清除已存 Key。
func (s *Service) SaveCatSettings(ctx context.Context, req Settings) (Settings, error) {
	if s.cfg.Store == nil {
		return Settings{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	if !ValidDefaultAgentKind(req.DefaultAgentKind) {
		return Settings{}, apperr.New(apperr.InvalidArgument, "助手模式设置不正确").WithDetail("reason=default_agent_kind")
	}
	for k, d := range req.DefaultsByAgent {
		if !catagent.Phase1Creatable(strings.TrimSpace(k)) {
			return Settings{}, apperr.New(apperr.InvalidArgument, "助手模式设置不正确").WithDetail("reason=defaults_by_agent_key")
		}
		if !ValidEffort(strings.TrimSpace(d.Effort)) {
			return Settings{}, apperr.New(apperr.InvalidArgument, "思考强度不对").WithDetail("reason=effort")
		}
	}
	if err := validateProvidersByAgent(req.ProvidersByAgent); err != nil {
		return Settings{}, err
	}
	v := req.normalized()
	// 落库前清掉只读 / 误传字段；apiKeySet 以密钥存储为准，不写入 cat.settings。
	for k, p := range v.ProvidersByAgent {
		p.ApiKeySet = false
		p.ApiKey = ""
		v.ProvidersByAgent[k] = p
	}
	if !ValidApprovalMode(v.DefaultApprovalMode) {
		return Settings{}, apperr.New(apperr.InvalidArgument, "批准方式不对").WithDetail("reason=approval_mode")
	}
	if utf8.RuneCountInString(v.CustomInstruction) > MaxCustomInstructionRunes {
		return Settings{}, apperr.New(apperr.InvalidArgument, "最多 4000 字。").WithDetail("reason=too_long")
	}
	if err := s.cfg.Store.SetSetting(ctx, SettingsKey, v); err != nil {
		return Settings{}, apperr.Wrap(apperr.IOError, "保存失败，请重试。", err)
	}
	s.fillProviderKeyFlags(ctx, &v)
	return v, nil
}

// rulesForTurn 是这一轮要带的全局自定义指令；读不到设置就不带（不让发送失败）。
func (s *Service) rulesForTurn(ctx context.Context) string {
	if s.cfg.Store == nil {
		return ""
	}
	v, err := s.GetCatSettings(ctx)
	if err != nil {
		s.cfg.Logf("cat settings read: %v", err)
		return ""
	}
	return strings.TrimSpace(v.CustomInstruction)
}

// InspectRequest 是 InspectCatConfig 的参数。
type InspectRequest struct {
	ProjectID string `json:"projectId"`
	AgentKind string `json:"agentKind,omitempty"` // v0.36：空 = cat_build
}

// InspectResult / InspectItem 复用适配器的类型（契约 CatInspectResult / CatInspectItem）。
type InspectResult = catagent.InspectResult
type InspectItem = catagent.InspectItem

// InspectCatConfig 只读列出技能、插件、钩子、MCP、子代理（契约 6.19.12.5）。失败返回空列表 + reason，error 为 nil。
// v0.36：可选 agentKind（空 = cat_build）；其它家暂无 inspect → reason=unsupported。
func (s *Service) InspectCatConfig(ctx context.Context, req InspectRequest) (InspectResult, error) {
	kind := strings.TrimSpace(req.AgentKind)
	if kind == "" {
		kind = catagent.KindCatBuild
	}
	a, err := s.adapter(kind)
	if err != nil {
		if kind == catagent.KindCatBuild {
			return catagent.EmptyInspect(catagent.InspectNotReady), nil
		}
		return catagent.EmptyInspect(catagent.InspectUnsupported), nil
	}
	insp, ok := a.(interface {
		Inspect(context.Context, string) catagent.InspectResult
	})
	if !ok {
		// cat_build 应有 inspect；未实现且未就绪时沿用 not_ready。其它家暂无适配器 → unsupported。
		if kind == catagent.KindCatBuild {
			return catagent.EmptyInspect(catagent.InspectNotReady), nil
		}
		return catagent.EmptyInspect(catagent.InspectUnsupported), nil
	}
	if a.Status().State != catagent.StateReady {
		return catagent.EmptyInspect(catagent.InspectNotReady), nil
	}
	cwd := ""
	if pid := strings.TrimSpace(req.ProjectID); pid != "" && s.cfg.Store != nil {
		row, err := s.cfg.Store.GetCatProject(ctx, pid)
		if err == nil && !s.checkProject(row).Missing {
			cwd = row.Path
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			s.cfg.Logf("cat inspect project: %v", err)
		}
	}
	if cwd == "" && s.cfg.DataRoot != "" {
		cwd = filepath.Join(s.cfg.DataRoot, "cat")
	}
	return insp.Inspect(ctx, cwd), nil
}

// ClearAllCatConversations 删掉所有对话（含定时任务产生的），先取消进行中的回复；
// 普通对话在数据目录里的自有文件夹尽力删（失败只记日志）；项目文件夹、定时任务和运行记录不动（6.19.13.7-4 / 6.19.13.10-5）。
func (s *Service) ClearAllCatConversations(ctx context.Context) error {
	if s.cfg.Store == nil {
		return apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	list, err := s.cfg.Store.ListCatConversations(ctx)
	if err != nil {
		return apperr.Wrap(apperr.IOError, "清空失败，请重试。", err)
	}
	for _, c := range list {
		s.cancelTurnAndWait(c.ID)
	}
	var firstErr error
	for _, c := range list {
		if err := s.cfg.Store.DeleteCatConversation(ctx, c.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// 删除与取消之间若又起了一轮，再停一次。
		s.cancelTurn(c.ID, "")
		if c.ProjectID == "" {
			s.removeConvDir(c.ID)
		}
	}
	if firstErr != nil {
		return apperr.Wrap(apperr.IOError, "清空失败，请重试。", firstErr)
	}
	return nil
}
