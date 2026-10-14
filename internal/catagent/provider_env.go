package catagent

import (
	"regexp"

	"FFmpegFree/internal/apperr"
	"net/url"
	"strconv"
	"strings"
)

// ProviderOverlay 是 v0.38 自定义服务商注入到子进程的参数。
// 仅用于 cmd.Env / CLI 子进程，禁止 os.Setenv 写进父进程（契约 6.19.12.8）。
type ProviderOverlay struct {
	Enabled   bool
	AgentKind string
	Protocol  string // openai | anthropic；空 = 该家默认
	ApiURL    string
	ApiKey    string // 明文，仅内存；不得写入日志 / 事件 / settings JSON
}

// CodexProviderID 是 v0.46 注入 Codex 的自定义服务商 id（-c model_providers.ffcat.*）。
const CodexProviderID = "ffcat"

// CodexProviderKeyEnv 是 Codex 自定义服务商读 Key 的子进程环境变量（model_providers.ffcat.env_key 指向它）。
const CodexProviderKeyEnv = "FFCAT_CODEX_API_KEY"

// isAnthropicOfficialHost 报告 apiUrl 是否指向 Anthropic 官方域名（C2：官方用 x-api-key）。
func isAnthropicOfficialHost(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Hostname(), "api.anthropic.com")
}

// ExtraEnv 按家返回 KEY=VALUE（契约 6.19.20.1 C1–C3，v0.46 已核）。
// 空 Key / URL 的项不产出；调用方在 Enabled 时仍应注入（缺 Key 由 CLI 失败）。
//
// 依据（每一项都在 provider_env_test.go 的白名单测试里再核一次）：
//   - Grok：GROK_MODELS_BASE_URL + XAI_API_KEY —— ~/.grok/docs/user-guide/11-custom-models.md「Custom Models Endpoint」、
//     26-config-reference.md「endpoints.models_base_url … Also GROK_MODELS_BASE_URL」；真机 grok 1.0.40 `grok models` 已打到网关。
//   - Claude：ANTHROPIC_BASE_URL + ANTHROPIC_AUTH_TOKEN（网关）/ ANTHROPIC_API_KEY（官方域名）——
//     https://code.claude.com/docs/en/llm-gateway-connect 、https://code.claude.com/docs/en/env-vars
//   - Codex：只放 Key（FFCAT_CODEX_API_KEY），地址走 ExtraArgs 的 -c model_providers.ffcat.*。
func (o ProviderOverlay) ExtraEnv() []string {
	if !o.Enabled {
		return nil
	}
	key := strings.TrimSpace(o.ApiKey)
	u := strings.TrimSpace(o.ApiURL)
	var out []string
	switch o.AgentKind {
	case KindCatBuild:
		if u != "" {
			out = append(out, "GROK_MODELS_BASE_URL="+u)
		}
		if key != "" {
			out = append(out, "XAI_API_KEY="+key)
		}
	case KindClaudeCLI:
		if u != "" {
			out = append(out, "ANTHROPIC_BASE_URL="+u)
		}
		if key != "" {
			if isAnthropicOfficialHost(u) {
				out = append(out, "ANTHROPIC_API_KEY="+key)
			} else {
				out = append(out, "ANTHROPIC_AUTH_TOKEN="+key)
			}
		}
	case KindCodexCLI:
		if key != "" {
			out = append(out, CodexProviderKeyEnv+"="+key)
		}
	}
	return out
}

// ExtraArgs 是要加到 CLI argv 里的参数（目前只有 Codex，exec 与 exec resume 都带）。
// 依据：https://developers.openai.com/codex/config-reference（model_provider、model_providers.<id>.name/base_url/env_key/wire_api，
// 「responses is the only supported value」）；`-c, --config <key=value>` 见 testdata/help/codex-exec.txt / codex-exec-resume.txt；
// 真机 codex 0.162.1：exec 与 exec resume 带这组参数都向假网关发 POST /v1/responses、Authorization: Bearer。
// URL 为空时不产出（避免把 Codex 指到空地址）。
func (o ProviderOverlay) ExtraArgs() []string {
	if !o.Enabled || o.AgentKind != KindCodexCLI {
		return nil
	}
	u := strings.TrimSpace(o.ApiURL)
	if u == "" {
		return nil
	}
	p := "model_providers." + CodexProviderID + "."
	return []string{
		"-c", `model_provider="` + CodexProviderID + `"`,
		"-c", p + `name="FFmpegFree"`,
		"-c", p + "base_url=" + strconv.Quote(u),
		"-c", p + `env_key="` + CodexProviderKeyEnv + `"`,
		"-c", p + `wire_api="responses"`,
	}
}

// authEnvKeysToStrip 是启用自定义时要从父进程 env 剥掉的鉴权相关键，
// 避免 CLI 登录 / shell 环境变量「补洞」（契约 6.19.12.8 优先级 1）。
func authEnvKeysToStrip(agentKind string) []string {
	switch agentKind {
	case KindCatBuild:
		// GROK_CODE_XAI_API_KEY：11-custom-models.md「Grok also accepts GROK_CODE_XAI_API_KEY」；GROK_MODELS_LIST_URL 同节。
		return []string{"XAI_API_KEY", "GROK_CODE_XAI_API_KEY", "GROK_MODELS_BASE_URL", "GROK_MODELS_LIST_URL"}
	case KindClaudeCLI:
		return []string{"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"}
	case KindCodexCLI:
		return []string{CodexProviderKeyEnv}
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

// MsgCodexNoResponses 是 Codex 自定义服务商缺 /responses 时的提示（契约 6.19.20.5）。
const MsgCodexNoResponses = "这个服务商不支持 Codex 需要的接口，请换一个服务商或关闭自定义"

// ReasonProviderNoResponses 是对应的 error.detail。
const ReasonProviderNoResponses = "reason=provider_responses_unsupported"

// providerResponsesUnsupportedErr 与 cli_outdated 同形：detail 多行 reason / agentKind；服务层据此置 retryable=false。
func providerResponsesUnsupportedErr(kind string) *apperr.AppError {
	return apperr.New(apperr.CatReplyFailed, MsgCodexNoResponses).WithDetail(ReasonProviderNoResponses + "\nagentKind=" + kind)
}

// codexResponsesUnsupportedRe 认 codex 0.162.1 真机输出（样本 testdata/codex-provider-responses-404.txt /
// codex-provider-responses-405.txt，exec 与 exec resume 一致）：
// `unexpected status 404 Not Found: Unknown error, url: http://…/v1/responses`。认不出 → 走普通失败。
var codexResponsesUnsupportedRe = regexp.MustCompile(`unexpected status (404|405) [^,]*, url: \S*/responses\b`)

func codexResponsesMissing(text string) bool {
	return codexResponsesUnsupportedRe.MatchString(text)
}
