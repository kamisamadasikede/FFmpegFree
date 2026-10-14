package catagent

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

// 契约 v0.48.1 §6.19.22：Codex / Claude 的上下文占用（与 Grok 同一个 cat:turn contextUsed / contextWindow）。
//
// Codex（codex-cli 0.162.1）：`codex exec --json` 的 stdout 不带窗口大小，turn.completed.usage 是整轮多次请求的合计，
// 不能当“当前上下文”。改读同一会话的 rollout（$CODEX_HOME/sessions/.../rollout-*-<thread_id>.jsonl）：
// 最后一条 event_msg.payload.type=token_count 的 info.last_token_usage.total_tokens = 已用，
// info.model_context_window = 容量；没有 token_count 时容量退回 task_started.model_context_window（真机样本有）。
//
// Claude（claude-code 2.1.296）：`-p --output-format stream-json` 的 result.modelUsage[<model>].contextWindow = 容量
// （CLI 按模型算出，二进制里 modelUsage 带 contextWindow:dm(model)）；result.usage 是整轮合计，不用。
// 已用 = 会话 transcript 里最后一条主线 assistant 的 message.usage：
// input_tokens + cache_creation_input_tokens + cache_read_input_tokens + output_tokens。

// parseCodexRolloutContext 取 rollout 里最后一次 token_count 的已用与容量。
func parseCodexRolloutContext(r interface{ Read([]byte) (int, error) }) (used, window int64) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var line struct {
			Type    string `json:"type"`
			Payload struct {
				Type   string `json:"type"`
				Window int64  `json:"model_context_window"`
				Info   *struct {
					Last struct {
						Total  int64 `json:"total_tokens"`
						Input  int64 `json:"input_tokens"`
						Output int64 `json:"output_tokens"`
					} `json:"last_token_usage"`
					Window int64 `json:"model_context_window"`
				} `json:"info"`
			} `json:"payload"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil || line.Type != "event_msg" {
			continue
		}
		switch line.Payload.Type {
		case "task_started":
			if line.Payload.Window > 0 {
				window = line.Payload.Window
			}
		case "token_count":
			if line.Payload.Info == nil {
				continue
			}
			if line.Payload.Info.Window > 0 {
				window = line.Payload.Info.Window
			}
			n := line.Payload.Info.Last.Total
			if n <= 0 {
				n = line.Payload.Info.Last.Input + line.Payload.Info.Last.Output
			}
			if n > 0 {
				used = n
			}
		}
	}
	return used, window
}

// parseClaudeTranscriptContext 取 transcript 最后一条主线 assistant 的上下文占用和模型名。
func parseClaudeTranscriptContext(r interface{ Read([]byte) (int, error) }) (used int64, model string) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var line struct {
			Type      string `json:"type"`
			Sidechain bool   `json:"isSidechain"`
			Message   struct {
				Model string `json:"model"`
				Usage *struct {
					Input       int64 `json:"input_tokens"`
					CacheCreate int64 `json:"cache_creation_input_tokens"`
					CacheRead   int64 `json:"cache_read_input_tokens"`
					Output      int64 `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil || line.Type != "assistant" || line.Sidechain || line.Message.Usage == nil {
			continue
		}
		if line.Message.Model == "<synthetic>" {
			continue
		}
		u := line.Message.Usage
		if n := u.Input + u.CacheCreate + u.CacheRead + u.Output; n > 0 {
			used, model = n, line.Message.Model
		}
	}
	return used, model
}

// claudeResultContextWindow 读 result.modelUsage 的 contextWindow：优先与 model 同名的项，否则取最大值。
func claudeResultContextWindow(raw map[string]json.RawMessage, model string) int64 {
	mu, ok := raw["modelUsage"]
	if !ok {
		return 0
	}
	var m map[string]struct {
		ContextWindow int64 `json:"contextWindow"`
	}
	if json.Unmarshal(mu, &m) != nil {
		return 0
	}
	if v, ok := m[model]; ok && v.ContextWindow > 0 {
		return v.ContextWindow
	}
	var best int64
	for _, v := range m {
		if v.ContextWindow > best {
			best = v.ContextWindow
		}
	}
	return best
}

// turnContextUsage 在一轮结束后取该会话当前上下文占用；拿不到返回 0, 0（前端不画圆环）。
// claudeWindow 为本轮 stream result.modelUsage 里读到的容量。
func (a *CLIAgentAdapter) turnContextUsage(sessionID, cwd string, claudeWindow int64) (int64, int64) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, 0
	}
	var path string
	if a.kind == cliKindCodex {
		path = findCodexRollout(a.codexHome(), sessionID)
	} else {
		path = findClaudeTranscript(a.claudeConfigDir(), cwd, sessionID)
	}
	if path == "" {
		return 0, 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	if a.kind == cliKindCodex {
		used, window := parseCodexRolloutContext(f)
		if used <= 0 || window <= 0 {
			return 0, 0
		}
		return used, window
	}
	used, _ := parseClaudeTranscriptContext(f)
	if used <= 0 || claudeWindow <= 0 {
		return 0, 0
	}
	return used, claudeWindow
}
