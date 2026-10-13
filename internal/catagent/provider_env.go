package catagent

import "strings"

// ProviderOverlay 是 v0.38 自定义服务商注入到子进程的参数。
// 仅用于 cmd.Env / CLI 子进程，禁止 os.Setenv 写进父进程（契约 6.19.12.8）。
type ProviderOverlay struct {
	Enabled   bool
	AgentKind string
	Protocol  string // openai | anthropic；空 = 该家默认
	ApiURL    string
	ApiKey    string // 明文，仅内存；不得写入日志 / 事件 / settings JSON
}

// ExtraEnv 按家返回 KEY=VALUE（附录 6.19.14 C1–C3 GUESS）。
// 空 Key / URL 的项不产出；调用方在 Enabled 时仍应注入（缺 Key 由 CLI 失败）。
func (o ProviderOverlay) ExtraEnv() []string {
	if !o.Enabled {
		return nil
	}
	key := strings.TrimSpace(o.ApiKey)
	url := strings.TrimSpace(o.ApiURL)
	var out []string
	switch o.AgentKind {
	case KindCatBuild:
		// GUESS C1：Build / Grok 继承 XAI_API_KEY；Base URL 类环境变量名未确认。
		if key != "" {
			out = append(out, "XAI_API_KEY="+key)
		}
		if url != "" {
			// GUESS C1：常见兼容网关 Base URL 名；真机确认前不得写进用户可见文案。
			out = append(out, "XAI_BASE_URL="+url)
			out = append(out, "OPENAI_BASE_URL="+url) // 部分 OpenAI 兼容网关只认此名
		}
	case KindClaudeCLI:
		// GUESS C2：Anthropic 官方常见 ANTHROPIC_API_KEY + ANTHROPIC_BASE_URL。
		if key != "" {
			out = append(out, "ANTHROPIC_API_KEY="+key)
		}
		if url != "" {
			out = append(out, "ANTHROPIC_BASE_URL="+url)
		}
	case KindCodexCLI:
		// GUESS C3：OpenAI / Codex 常见 OPENAI_API_KEY + OPENAI_BASE_URL。
		if key != "" {
			out = append(out, "OPENAI_API_KEY="+key)
		}
		if url != "" {
			out = append(out, "OPENAI_BASE_URL="+url)
		}
	}
	return out
}

// authEnvKeysToStrip 是启用自定义时要从父进程 env 剥掉的鉴权相关键，
// 避免 CLI 登录 / shell 环境变量「补洞」（契约 6.19.12.8 优先级 1）。
func authEnvKeysToStrip(agentKind string) []string {
	switch agentKind {
	case KindCatBuild:
		return []string{"XAI_API_KEY", "XAI_BASE_URL", "OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_API_BASE"}
	case KindClaudeCLI:
		return []string{"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"}
	case KindCodexCLI:
		return []string{"OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_API_BASE"}
	}
	return nil
}

// MergeProviderEnv 把自定义服务商叠加进子进程环境。
// Enabled=false：原样返回 base（v0.37 路径）。
// Enabled=true：剥掉该家鉴权相关父进程变量后追加 ExtraEnv；从不调用 os.Setenv。
func MergeProviderEnv(base []string, o ProviderOverlay) []string {
	if !o.Enabled {
		return base
	}
	strip := map[string]struct{}{}
	for _, k := range authEnvKeysToStrip(o.AgentKind) {
		strip[k] = struct{}{}
	}
	// 也剥掉即将覆盖的键，避免重复。
	for _, kv := range o.ExtraEnv() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			strip[kv[:i]] = struct{}{}
		}
	}
	out := make([]string, 0, len(base)+8)
	for _, e := range base {
		if i := strings.IndexByte(e, '='); i > 0 {
			if _, ok := strip[e[:i]]; ok {
				continue
			}
		}
		out = append(out, e)
	}
	out = append(out, o.ExtraEnv()...)
	return out
}
