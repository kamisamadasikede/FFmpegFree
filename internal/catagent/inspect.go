package catagent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"
)

// InspectItem 是一项技能 / 插件 / 钩子 / MCP 服务 / 子代理（契约 6.19.12.1 CatInspectItem）。
// 只有名字、说明、来源和启用状态，不带路径、命令、环境变量或密钥。
type InspectItem struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// InspectResult 是 CatInspectResult。Reason 空 = 成功；not_ready | timeout | parse。各列表始终是数组。
type InspectResult struct {
	Skills     []InspectItem `json:"skills"`
	Plugins    []InspectItem `json:"plugins"`
	Hooks      []InspectItem `json:"hooks"`
	MCPServers []InspectItem `json:"mcpServers"`
	Agents     []InspectItem `json:"agents"`
	Version    string        `json:"version,omitempty"`
	Reason     string        `json:"reason,omitempty"`
}

// Inspect 失败原因。
const (
	InspectNotReady = "not_ready"
	InspectTimeout  = "timeout"
	InspectParse    = "parse"
)

// InspectTimeoutDur 是 `inspect --json` 的最长等待（契约 15 秒；测试可改小）。
var InspectTimeoutDur = 15 * time.Second

// EmptyInspect 返回全部为空数组的结果。
func EmptyInspect(reason string) InspectResult {
	return InspectResult{
		Skills: []InspectItem{}, Plugins: []InspectItem{}, Hooks: []InspectItem{},
		MCPServers: []InspectItem{}, Agents: []InspectItem{}, Reason: reason,
	}
}

// Inspect 在 dir 下只读执行 `inspect --json`（走 runCmd，Windows 隐藏窗口）。失败返回空列表加原因，不报错。
func (a *BuildAdapter) Inspect(ctx context.Context, dir string) InspectResult {
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
	runCtx, cancel := context.WithTimeout(ctx, InspectTimeoutDur)
	defer cancel()
	cmd := a.runCmd(runCtx, exe, "inspect", "--json")
	if dir != "" {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			cmd.Dir = dir
		}
	}
	cmd.Env = grokTurnEnv(os.Environ())
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()
	if runCtx.Err() != nil {
		return EmptyInspect(InspectTimeout)
	}
	if errb.Len() > 0 && a.cfg.Logf != nil {
		a.cfg.Logf("cat inspect stderr: %s", truncate(errb.String(), 500))
	}
	res, ok := ParseInspect(out.Bytes())
	if !ok {
		if runErr != nil && a.cfg.Logf != nil {
			a.cfg.Logf("cat inspect: %v", runErr)
		}
		return EmptyInspect(InspectParse)
	}
	return res
}

// ParseInspect 只取五类条目的名字 / 说明 / 来源 / 启用；其余字段（path、target、command、env…）一律丢弃。
func ParseInspect(raw []byte) (InspectResult, bool) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(raw), &doc); err != nil || doc == nil {
		return InspectResult{}, false
	}
	res := EmptyInspect("")
	ok := true
	pick := func(key string, dst *[]InspectItem, hook bool) {
		v, has := doc[key]
		if !has || string(v) == "null" {
			return
		}
		items, good := parseInspectList(v, hook)
		if !good {
			ok = false
			return
		}
		*dst = items
	}
	pick("skills", &res.Skills, false)
	pick("plugins", &res.Plugins, false)
	pick("hooks", &res.Hooks, true)
	pick("mcpServers", &res.MCPServers, false)
	pick("agents", &res.Agents, false)
	if !ok {
		return InspectResult{}, false
	}
	var ver string
	if v, has := doc["grokVersion"]; has {
		_ = json.Unmarshal(v, &ver)
	} else if v, has := doc["version"]; has {
		_ = json.Unmarshal(v, &ver)
	}
	res.Version = strings.TrimSpace(ver)
	return res, true
}

func parseInspectList(v json.RawMessage, hook bool) ([]InspectItem, bool) {
	var list []map[string]json.RawMessage
	if err := json.Unmarshal(v, &list); err != nil {
		// 对象形式 {name: {...}}
		var obj map[string]map[string]json.RawMessage
		if err2 := json.Unmarshal(v, &obj); err2 != nil {
			return nil, false
		}
		for name, m := range obj {
			if m == nil {
				m = map[string]json.RawMessage{}
			}
			if _, has := m["name"]; !has {
				b, _ := json.Marshal(name)
				m["name"] = b
			}
			list = append(list, m)
		}
	}
	out := make([]InspectItem, 0, len(list))
	for _, m := range list {
		name := jsonStr(m, "name")
		if name == "" && hook {
			name = jsonStr(m, "event")
			if name == "" {
				name = jsonStr(m, "hookEvent")
			}
			if mt := jsonStr(m, "matcher"); mt != "" && name != "" {
				name += " · " + mt
			}
		}
		if name == "" {
			name = jsonStr(m, "id")
		}
		if name == "" {
			continue
		}
		it := InspectItem{Name: clip(name, 200), Description: clip(jsonStr(m, "description"), 500), Enabled: true}
		if src, has := m["source"]; has {
			var s string
			if json.Unmarshal(src, &s) == nil {
				it.Source = s
			} else {
				var so map[string]json.RawMessage
				if json.Unmarshal(src, &so) == nil {
					it.Source = jsonStr(so, "type")
				}
			}
		}
		if b, has := jsonBool(m, "enabled"); has {
			it.Enabled = b
		} else if b, has := jsonBool(m, "disabled"); has {
			it.Enabled = !b
		}
		out = append(out, it)
	}
	return out, true
}

func jsonStr(m map[string]json.RawMessage, k string) string {
	v, ok := m[k]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

func jsonBool(m map[string]json.RawMessage, k string) (bool, bool) {
	v, ok := m[k]
	if !ok {
		return false, false
	}
	var b bool
	if json.Unmarshal(v, &b) != nil {
		return false, false
	}
	return b, true
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
