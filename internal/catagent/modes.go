package catagent

import (
	"regexp"
	"strings"
)

// v0.44（契约 6.19.18）：按 CLI 下发模式列表、按模型下发思考强度。
// 中文名 / 短名 / 说明是产品定稿文案（6.19.18.3；Codex read-only 与 Grok default/acceptEdits/auto 的说明为产品 10-14 二次定稿），只能在这里改，并同步契约。

// 模式风险档（6.19.18.2）：safe ⇔ writesFiles=false。
const (
	RiskSafe   = "safe"
	RiskEdits  = "edits"
	RiskDanger = "danger"
)

// AgentMode 是 CatAgentMode（契约 6.19.18.2）。
type AgentMode struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	WritesFiles bool   `json:"writesFiles"`
	IsDefault   bool   `json:"isDefault"`
	ShortLabel  string `json:"shortLabel"`
	Risk        string `json:"risk"`
}

// 各家模式 id（6.19.18.3）。
const (
	ModeClaudeManual = "manual"
	ModeClaudePlan   = "plan"
	ModeAcceptEdits  = "acceptEdits" // Claude / Grok 同名
	ModeAuto         = "auto"        // Claude / Grok 同名
	ModeClaudeBypass = "bypassPermissions"

	ModeCodexReadOnly     = "read-only"
	ModeCodexWorkspace    = "workspace-write"
	ModeCodexApproveForMe = "approve-for-me"
	ModeCodexBypass       = "bypass"

	ModeGrokDefault       = "default"
	ModeGrokAlwaysApprove = "always-approve"
)

const (
	descReadOnly = "只看文件，不改动"
	descBypass   = "不做任何检查，仅建议在隔离环境使用"
)

// Claude Code 2.1.296：H claude.txt `--permission-mode` choices；D https://code.claude.com/docs/en/permission-modes
var claudeModes = []AgentMode{
	{ID: ModeClaudeManual, Label: "只读", ShortLabel: "只读", Description: descReadOnly, Risk: RiskSafe, IsDefault: true},
	{ID: ModeClaudePlan, Label: "计划", ShortLabel: "计划", Description: "先分析并给出方案，不改文件", Risk: RiskSafe},
	{ID: ModeAcceptEdits, Label: "自动改文件", ShortLabel: "改文件", Description: "直接修改文件，其他命令不执行", WritesFiles: true, Risk: RiskEdits},
	{ID: ModeAuto, Label: "自动", ShortLabel: "自动", Description: "自动执行，经过安全检查", WritesFiles: true, Risk: RiskEdits},
	{ID: ModeClaudeBypass, Label: "全部放开", ShortLabel: "全部放开", Description: descBypass, WritesFiles: true, Risk: RiskDanger},
}

// Codex 0.162.1：H codex-exec.txt `--sandbox` / `--approve-for-me` / `--dangerously-bypass-approvals-and-sandbox`；
// D https://developers.openai.com/codex/config-reference ；默认只读 D https://developers.openai.com/codex/noninteractive
var codexModes = []AgentMode{
	{ID: ModeCodexReadOnly, Label: "只读", ShortLabel: "只读", Description: "只读取文件，不修改，命令在只读沙箱中运行", Risk: RiskSafe, IsDefault: true},
	{ID: ModeCodexWorkspace, Label: "改项目文件", ShortLabel: "改文件", Description: "可修改项目内文件，命令在沙箱中运行", WritesFiles: true, Risk: RiskEdits},
	{ID: ModeCodexApproveForMe, Label: "自动审查", ShortLabel: "自动审查", Description: "可修改项目文件，需要批准的操作交给自动审查", WritesFiles: true, Risk: RiskEdits},
	{ID: ModeCodexBypass, Label: "全部放开", ShortLabel: "全部放开", Description: descBypass, WritesFiles: true, Risk: RiskDanger},
}

// Grok Build 1.0.40：H grok.txt `--permission-mode` / `--always-approve`；D 二进制内嵌官方文档 "Available modes"。
var grokModes = []AgentMode{
	{ID: ModeGrokDefault, Label: "只读", ShortLabel: "只读", Description: "只用只读工具和只读命令，不修改文件", Risk: RiskSafe, IsDefault: true},
	{ID: ModeAcceptEdits, Label: "自动改文件", ShortLabel: "改文件", Description: "直接修改文件，不再询问", WritesFiles: true, Risk: RiskEdits},
	{ID: ModeAuto, Label: "自动", ShortLabel: "自动", Description: "安全检查放行的操作直接执行，其他的会被拦下", WritesFiles: true, Risk: RiskEdits},
	{ID: ModeGrokAlwaysApprove, Label: "全部放开", ShortLabel: "全部放开", Description: descBypass, WritesFiles: true, Risk: RiskDanger},
}

// ModesFor 返回该 kind 的模式列表副本（保守→放开；未知 kind → 空）。
func ModesFor(kind string) []AgentMode {
	var src []AgentMode
	switch kind {
	case KindClaudeCLI:
		src = claudeModes
	case KindCodexCLI:
		src = codexModes
	case KindCatBuild:
		src = grokModes
	}
	return append([]AgentMode{}, src...)
}

// DefaultModeID 返回该 kind 的默认模式 id。
func DefaultModeID(kind string) string {
	for _, m := range ModesFor(kind) {
		if m.IsDefault {
			return m.ID
		}
	}
	return ""
}

// IsModeID 报告 id 是否是该 kind 下发的模式之一。
func IsModeID(kind, id string) bool {
	for _, m := range ModesFor(kind) {
		if m.ID == id {
			return true
		}
	}
	return false
}

// legacyModes 是 v0.43 及以前的批准方式值（6.19.18.4「旧值」列）。
var legacyModes = map[string]map[string]string{
	KindClaudeCLI: {"normal": ModeClaudeManual, "default": ModeClaudeManual, "ask": ModeClaudeManual, "plan": ModeClaudePlan, "auto": ModeAuto, "auto-review": ModeAuto, "approve": ModeClaudeBypass, "always-approve": ModeClaudeBypass, "full": ModeClaudeBypass},
	KindCodexCLI:  {"normal": ModeCodexReadOnly, "default": ModeCodexReadOnly, "ask": ModeCodexReadOnly, "plan": ModeCodexReadOnly, "auto": ModeCodexApproveForMe, "auto-review": ModeCodexApproveForMe, "approve": ModeCodexBypass, "always-approve": ModeCodexBypass, "full": ModeCodexBypass},
	KindCatBuild:  {"normal": ModeGrokDefault, "ask": ModeGrokDefault, "plan": ModeGrokDefault, "auto-review": ModeAuto, "approve": ModeGrokAlwaysApprove, "full": ModeGrokAlwaysApprove, "bypassPermissions": ModeGrokAlwaysApprove},
}

// IsKnownModeInput 报告 v 是否是该 kind 的模式 id 或旧值（空也算，= 默认）。
func IsKnownModeInput(kind, v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || IsModeID(kind, v) {
		return true
	}
	_, ok := legacyModes[kind][v]
	return ok
}

// NormalizeMode 把保存的 / 请求里的模式值映射到该 kind 的模式 id（读时映射，6.19.18.2）。
// 空或不认识 → 该 kind 的默认（最保守）模式。
func NormalizeMode(kind, v string) string {
	v = strings.TrimSpace(v)
	if IsModeID(kind, v) {
		return v
	}
	if id, ok := legacyModes[kind][v]; ok {
		return id
	}
	return DefaultModeID(kind)
}

// ---------- 思考强度 ----------

var effortFormat = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// EffortFormatOK 是模型未知时的格式校验（6.19.18.6）。
func EffortFormatOK(v string) bool { return effortFormat.MatchString(v) }

// EffortDisplayName 是 6.19.18.5a 的显示名（仅给兼容的 ListCatThinkLevels；前端按 id 映射）。未知 id 原样。
func EffortDisplayName(id string) string {
	switch id {
	case "low":
		return "低"
	case "medium":
		return "中"
	case "high":
		return "高"
	case "xhigh":
		return "很高"
	case "max":
		return "最高"
	case "ultra":
		return "极限"
	}
	return id
}

// FindModel 在列表里找模型。
func FindModel(list []Model, id string) (Model, bool) {
	for _, m := range list {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// ModelSupportsEffort 报告模型是否列出该档位。
func ModelSupportsEffort(m Model, effort string) bool {
	for _, e := range m.Efforts {
		if e == effort {
			return true
		}
	}
	return false
}

func withEfforts(m Model, efforts []string, def string) Model {
	m.Efforts = append([]string{}, efforts...)
	m.DefaultEffort = def
	return m
}

// Grok Build 1.0.40 每模型档位（6.19.18.5；D 二进制内嵌模型目录）。目录里没有的模型 → []。
var grokEffortTable = map[string]struct {
	efforts []string
	def     string
}{
	"grok-4.6": {[]string{"low", "medium", "high", "xhigh"}, "high"},
	"grok-4.5": {[]string{"low", "medium", "high"}, "high"},
}

func grokModelWithEfforts(id string) Model {
	m := Model{ID: id, DisplayName: id, Efforts: []string{}}
	if t, ok := grokEffortTable[id]; ok {
		m = withEfforts(m, t.efforts, t.def)
	}
	return m
}
