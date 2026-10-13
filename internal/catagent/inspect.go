package catagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

// rulesCLIArgs 把全局自定义指令变成 --rules=<指令>（契约 v0.35 6.19.12.4）。
// 用等号连写，指令以 - 开头时也不会被当成别的开关。空白指令不带。
func rulesCLIArgs(rules string) []string {
	rules = strings.TrimSpace(rules)
	if rules == "" {
		return nil
	}
	return []string{"--rules=" + rules}
}

// InspectTimeout 是 `grok inspect --json` 的超时（契约 6.19.12.5：15 秒）。测试可改小。
var InspectTimeout = 15 * time.Second

// Inspect 失败原因（契约 6.19.12.1 CatInspectResult.reason）。
const (
	InspectNotReady    = "not_ready"
	InspectTimedOut    = "timeout"
	InspectParse       = "parse"
	InspectUnsupported = "unsupported" // v0.36：该家暂无 inspect 适配器
	InspectNoProject   = "no_project"   // v0.39：未选项目
)

// InspectItem 是技能 / 插件 / 钩子 / MCP / 子代理里的一项。只有名字、说明、来源和是否启用，不带路径、命令或密钥。
type InspectItem struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// InspectResult 是 InspectCatConfig 的返回（契约 6.19.12.1 / v0.39）。列表永远非 nil。
type InspectResult struct {
	Skills           []InspectItem `json:"skills"`
	Plugins          []InspectItem `json:"plugins"`
	Hooks            []InspectItem `json:"hooks"`
	MCPServers       []InspectItem `json:"mcpServers"`
	Agents           []InspectItem `json:"agents"`
	Version          string        `json:"version,omitempty"`
	Reason           string        `json:"reason,omitempty"` // 空 = 成功；not_ready|timeout|parse|unsupported|no_project
	SkillsRoot       string        `json:"skillsRoot,omitempty"`    // v0.39：绝对路径；无项目时省略
	McpConfigPath    string        `json:"mcpConfigPath,omitempty"` // v0.39
	SkillsRootExists bool          `json:"skillsRootExists"`        // v0.39
}

// EmptyInspect 是失败时的返回：五个空列表 + 原因。
func EmptyInspect(reason string) InspectResult {
	return InspectResult{
		Skills: []InspectItem{}, Plugins: []InspectItem{}, Hooks: []InspectItem{},
		MCPServers: []InspectItem{}, Agents: []InspectItem{}, Reason: reason,
	}
}

// Inspect 在 cwd 下跑 `grok inspect --json`（走 runCmd：Windows 隐藏窗口）。只读，不改任何配置。
// 未就绪 / 超时 / 输出认不出时返回空列表 + reason，不返回错误。
func (a *BuildAdapter) Inspect(ctx context.Context, cwd string) InspectResult {
	if a.Status().State != StateReady {
		return EmptyInspect(InspectNotReady)
	}
	a.mu.Lock()
	exe := a.exe
	a.mu.Unlock()
	if exe == "" {
		return EmptyInspect(InspectNotReady)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithTimeout(ctx, InspectTimeout)
	defer cancel()
	cmd := a.runCmd(runCtx, exe, "inspect", "--json")
	if strings.TrimSpace(cwd) != "" {
		if st, err := os.Stat(cwd); err == nil && st.IsDir() {
			cmd.Dir = cwd
		}
	}
	// v0.39：有项目 cwd 时把技能根 / MCP 路径注入子进程（附录 D1/D4 GUESS）；无项目不注入（D6）。
	env := grokTurnEnv(os.Environ())
	if proj := strings.TrimSpace(cwd); proj != "" {
		skillsRoot, mcpPath := ProjectCatPaths(proj)
		env = MergeSkillsEnv(env, SkillsOverlay{
			AgentKind: KindCatBuild, SkillsRoot: skillsRoot, McpConfigPath: mcpPath,
		})
	}
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return EmptyInspect(InspectTimedOut)
	}
	if runErr != nil && a.cfg.Logf != nil {
		a.cfg.Logf("cat inspect: %v %s", runErr, truncate(errb.String(), 300))
	}
	res, ok := ParseInspect(out.Bytes())
	if !ok {
		return EmptyInspect(InspectParse)
	}
	return res
}

// ParseInspect 解析 `grok inspect --json`（CLI 1.0.40 实测：skills / agents / hooks / plugins / mcpServers 是数组，
// source 是 {type, path} 对象）。只取名字、说明、来源类型、是否启用；路径、命令、环境变量一律丢掉。
func ParseInspect(raw []byte) (InspectResult, bool) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(raw), &top); err != nil || top == nil {
		return InspectResult{}, false
	}
	res := EmptyInspect("")
	// 至少要认出一类列表，否则当作认不出。
	known := false
	pick := func(key string) []InspectItem {
		v, ok := top[key]
		if !ok {
			return []InspectItem{}
		}
		items, ok := parseInspectList(v)
		if ok {
			known = true
		}
		return items
	}
	res.Skills = pick("skills")
	res.Plugins = pick("plugins")
	res.Hooks = pick("hooks")
	res.MCPServers = pick("mcpServers")
	res.Agents = pick("agents")
	if !known {
		return InspectResult{}, false
	}
	if v, ok := top["grokVersion"]; ok {
		_ = json.Unmarshal(v, &res.Version)
	}
	return res, true
}

func parseInspectList(raw json.RawMessage) ([]InspectItem, bool) {
	out := []InspectItem{}
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		// 也接受 {名字: {...}} 形式。
		var obj map[string]map[string]json.RawMessage
		if err2 := json.Unmarshal(raw, &obj); err2 != nil {
			return out, false
		}
		for name, m := range obj {
			if _, has := m["name"]; !has {
				b, _ := json.Marshal(name)
				m["name"] = b
			}
			arr = append(arr, m)
		}
	}
	for _, m := range arr {
		it := InspectItem{Enabled: true}
		it.Name = firstString(m, "name", "id", "event", "matcher")
		if it.Name == "" {
			continue
		}
		it.Description = truncate(firstString(m, "description", "summary"), 500)
		it.Source = inspectSource(m)
		if v, ok := m["enabled"]; ok {
			var b bool
			if json.Unmarshal(v, &b) == nil {
				it.Enabled = b
			}
		}
		if v, ok := m["disabled"]; ok {
			var b bool
			if json.Unmarshal(v, &b) == nil && b {
				it.Enabled = false
			}
		}
		out = append(out, it)
	}
	return out, true
}

func firstString(m map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// inspectSource 只取来源类型（bundled / user / project / plugin / builtin 等），不取路径。
func inspectSource(m map[string]json.RawMessage) string {
	for _, k := range []string{"source", "scope"} {
		v, ok := m[k]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			// 字符串来源若像路径就丢掉。
			if strings.ContainsAny(s, `/\`) {
				return ""
			}
			return s
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(v, &obj) == nil {
			return firstString(obj, "type", "kind", "scope")
		}
	}
	return ""
}
