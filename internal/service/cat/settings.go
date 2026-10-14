package cat

import (
	"context"
	"database/sql"
	"errors"
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
	DefaultAgentKind string                      `json:"defaultAgentKind"` // cat_build|claude_cli|codex_cli；默认 cat_build
	DefaultsByAgent  map[string]AgentDefaults    `json:"defaultsByAgent"`  // key = agentKind
	ProvidersByAgent map[string]ProviderSettings `json:"providersByAgent"` // v0.38：key = agentKind
	// DefaultModeByAgent（v0.44.4，6.19.18.9）：key = agentKind；value = 该家 modes[].id 或 ""（= 该家默认）。
	// 取代已废弃的 defaultApprovalMode（只在读库时兼容，Save 不再写）。
	DefaultModeByAgent map[string]string `json:"defaultModeByAgent"`
	CustomInstruction  string            `json:"customInstruction"`
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
	DefaultModeByAgent  map[string]string           `json:"defaultModeByAgent"`
	DefaultApprovalMode string                      `json:"defaultApprovalMode"` // v0.44.4 起只读兼容
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
	if s.DefaultModeByAgent == nil {
		s.DefaultModeByAgent = map[string]string{}
	} else {
		out := make(map[string]string, len(s.DefaultModeByAgent))
		for k, m := range s.DefaultModeByAgent {
			out[strings.TrimSpace(k)] = strings.TrimSpace(m)
		}
		s.DefaultModeByAgent = out
	}
	if s.NotifyOnFinish == nil {
		t := true
		s.NotifyOnFinish = &t
	}
	return s
}

// migrateFromWire 把旧顶层 defaultModel/defaultEffort 迁进 defaultsByAgent["cat_build"]（不覆盖已有非空）。
func migrateFromWire(w settingsWire) Settings {
	s := Settings{
		DefaultAgentKind:   w.DefaultAgentKind,
		DefaultsByAgent:    w.DefaultsByAgent,
		ProvidersByAgent:   w.ProvidersByAgent,
		DefaultModeByAgent: w.DefaultModeByAgent,
		CustomInstruction:  w.CustomInstruction,
		NotifyOnFinish:     w.NotifyOnFinish,
	}
	// 6.19.18.9 第 3 条：库里只有旧 defaultApprovalMode 时，按各家「旧值」列映射给三家；
	// 映射不到 / 未知 → 该家最保守默认。只改返回值，不回写。
	if legacy := strings.TrimSpace(w.DefaultApprovalMode); legacy != "" && len(w.DefaultModeByAgent) == 0 {
		s.DefaultModeByAgent = LegacyDefaultModeByAgent(legacy)
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

// ValidEffortFormat：v0.44 起写死集合作废（6.19.18.6），这里只校验格式；按模型的校验见 validEffortFor。
func ValidEffortFormat(v string) bool { return v == "" || catagent.EffortFormatOK(v) }

// LegacyDefaultModeByAgent 把旧全局 defaultApprovalMode 按每家旧值列转成 defaultModeByAgent（6.19.18.9 第 3 条）。
func LegacyDefaultModeByAgent(legacy string) map[string]string {
	out := map[string]string{}
	for _, k := range catagent.CreatableKinds() {
		out[k] = catagent.NormalizeMode(k, legacy)
	}
	return out
}

// DefaultModeFor 是新对话该家的模式（6.19.18.9 第 4 条）：空或已不在该家 modes[] → 该家最保守默认。
func (s Settings) DefaultModeFor(kind string) string {
	if v := s.DefaultModeByAgent[kind]; catagent.IsModeID(kind, v) {
		return v
	}
	return catagent.DefaultModeID(kind)
}

// validateDefaultModeByAgent 是 6.19.18.9 第 2 条：key 只能是三家之一；value 为 "" 或该家 modes[].id。
// danger 项（全部放开）后端不要求确认。
func validateDefaultModeByAgent(m map[string]string) error {
	for k, v := range m {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !catagent.Phase1Creatable(k) {
			return apperr.New(apperr.InvalidArgument, "默认模式设置不正确").WithDetail("reason=default_mode_key")
		}
		if v != "" && !catagent.IsModeID(k, v) {
			return apperr.New(apperr.InvalidArgument, "默认模式设置不正确").WithDetail("reason=default_mode")
		}
	}
	return nil
}

// validEffortFor 是 6.19.18.6：空合法；模型为空或列表探测不到该模型 → 只校验格式；否则须在该模型 efforts 内。
func (s *Service) validEffortFor(ctx context.Context, kind, model, effort string) bool {
	effort, model = strings.TrimSpace(effort), strings.TrimSpace(model)
	if effort == "" {
		return true
	}
	if !catagent.EffortFormatOK(effort) {
		return false
	}
	if model == "" || model == "default" {
		return true
	}
	list, err := s.ListCatModels(ctx, kind)
	if err != nil {
		return true
	}
	m, ok := catagent.FindModel(list, model)
	if !ok {
		return true
	}
	return catagent.ModelSupportsEffort(m, effort)
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
	// 只校验这次改动的项：库里已有、这次没改的默认值不再拦整次保存（否则一项旧值失效，
	// 连开自定义服务商、填地址这类无关改动都会「保存失败」）。
	prev, _ := s.GetCatSettings(ctx)
	if len(req.DefaultsByAgent) > 0 {
		cp := make(map[string]AgentDefaults, len(req.DefaultsByAgent))
		for k, d := range req.DefaultsByAgent {
			cp[k] = d
		}
		req.DefaultsByAgent = cp
	}
	for k, d := range req.DefaultsByAgent {
		kind := strings.TrimSpace(k)
		if !catagent.Phase1Creatable(kind) {
			return Settings{}, apperr.New(apperr.InvalidArgument, "助手模式设置不正确").WithDetail("reason=defaults_by_agent_key")
		}
		old := prev.DefaultsByAgent[kind]
		unchanged := strings.TrimSpace(old.Model) == strings.TrimSpace(d.Model) && strings.TrimSpace(old.Effort) == strings.TrimSpace(d.Effort)
		// v0.46（6.19.20.2 ⑧）：该家自定义开时允许手填模型 id，按格式校验；服务商列表不带档位。
		// 新提交的档位仍拒绝；库里原有的旧档位（CLI 模式下选的）在打开开关时清空，不拦保存。
		if req.ProvidersByAgent[kind].EnabledCustom {
			m := strings.TrimSpace(d.Model)
			if strings.TrimSpace(d.Effort) != "" {
				if strings.TrimSpace(d.Effort) != strings.TrimSpace(old.Effort) {
					return Settings{}, apperr.New(apperr.InvalidArgument, "思考强度不对").WithDetail("reason=effort")
				}
				d.Effort = ""
				req.DefaultsByAgent[k] = d
			}
			if m != "" && m != "default" && !ValidCustomModelID(m) {
				return Settings{}, apperr.New(apperr.InvalidArgument, "模型 ID 不正确").WithDetail("reason=model")
			}
			continue
		}
		if unchanged {
			continue
		}
		if !s.validEffortFor(ctx, kind, d.Model, d.Effort) {
			return Settings{}, apperr.New(apperr.InvalidArgument, "思考强度不对").WithDetail("reason=effort")
		}
	}
	if err := validateProvidersByAgent(req.ProvidersByAgent); err != nil {
		return Settings{}, err
	}
	// 打开自定义必须有地址；只拦这次改动的那家（库里原样未改的不拦整次保存）。
	for k, p := range req.ProvidersByAgent {
		if !p.EnabledCustom || strings.TrimSpace(p.ApiUrl) != "" {
			continue
		}
		o := prev.ProvidersByAgent[strings.TrimSpace(k)]
		if o.EnabledCustom && strings.TrimSpace(o.ApiUrl) == "" {
			continue
		}
		return Settings{}, apperr.New(apperr.InvalidArgument, "请先填写 API 地址，再打开自定义。").WithDetail("reason=api_url")
	}
	v := req.normalized()
	// 落库前清掉只读 / 误传字段；apiKeySet 以密钥存储为准，不写入 cat.settings。
	for k, p := range v.ProvidersByAgent {
		p.ApiKeySet = false
		p.ApiKey = ""
		v.ProvidersByAgent[k] = p
	}
	// 没改动的项（库里原值）不拦保存，只校验这次改的。
	modes := map[string]string{}
	for k, m := range v.DefaultModeByAgent {
		if prev.DefaultModeByAgent[k] == m && catagent.Phase1Creatable(k) {
			continue
		}
		modes[k] = m
	}
	if err := validateDefaultModeByAgent(modes); err != nil {
		return Settings{}, err
	}
	if utf8.RuneCountInString(v.CustomInstruction) > MaxCustomInstructionRunes {
		return Settings{}, apperr.New(apperr.InvalidArgument, "最多 4000 字。").WithDetail("reason=too_long")
	}
	if err := s.cfg.Store.SetSetting(ctx, SettingsKey, v); err != nil {
		return Settings{}, apperr.Wrap(apperr.IOError, "保存失败，请重试。", err)
	}
	// v0.46：改了 enabledCustom / protocol / apiUrl 的家清模型列表缓存（6.19.20.2 ⑤）。
	for _, kind := range catagent.CreatableKinds() {
		a, b := prev.ProvidersByAgent[kind], v.ProvidersByAgent[kind]
		if a.EnabledCustom != b.EnabledCustom || a.Protocol != b.Protocol || a.ApiUrl != b.ApiUrl {
			s.invalidateProviderModels(kind)
		}
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

// InspectCatConfig 列出技能、插件、钩子、MCP、子代理（契约 6.19.12.5 / v0.39 可管理）。
// 无 projectId → reason=no_project（省略路径字段）；项目 missing → CAT_PROJECT_MISSING；
// 有项目时始终填 skillsRoot / mcpConfigPath / skillsRootExists（即便 reason=unsupported/not_ready）。
func (s *Service) InspectCatConfig(ctx context.Context, req InspectRequest) (InspectResult, error) {
	kind := strings.TrimSpace(req.AgentKind)
	if kind == "" {
		kind = catagent.KindCatBuild
	}
	pid := strings.TrimSpace(req.ProjectID)
	if pid == "" {
		return catagent.EmptyInspect(catagent.InspectNoProject), nil
	}
	if s.cfg.Store == nil {
		return InspectResult{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	row, err := s.cfg.Store.GetCatProject(ctx, pid)
	if errors.Is(err, sql.ErrNoRows) {
		return InspectResult{}, errProjectNotFound()
	}
	if err != nil {
		return InspectResult{}, apperr.Wrap(apperr.IOError, "读取项目失败", err)
	}
	if s.checkProject(row).Missing {
		return InspectResult{}, errProjectMissing()
	}
	return s.inspectForProject(ctx, row, kind)
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
