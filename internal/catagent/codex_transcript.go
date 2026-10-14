package catagent

import (
	"bufio"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SessionResumable：这条对话是否已有可续的 CLI 会话（Codex 看存下的 thread_id）。
func (a *CLIAgentAdapter) SessionResumable(convID string) bool {
	return a.loadSessionID(convID) != ""
}

// codexHome 与 codex-cli 一致：$CODEX_HOME，否则 ~/.codex。
func (a *CLIAgentAdapter) codexHome() string {
	if h := strings.TrimSpace(a.cfg.CodexHome); h != "" {
		return h
	}
	if h := strings.TrimSpace(os.Getenv("CODEX_HOME")); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// findCodexRollout 在 sessions/YYYY/MM/DD/ 和 archived_sessions/ 下找 rollout-*-<threadID>.jsonl。
func findCodexRollout(home, threadID string) string {
	if home == "" || threadID == "" || strings.ContainsAny(threadID, `/\`) {
		return ""
	}
	suffix := "-" + threadID + ".jsonl"
	var found string
	for _, sub := range []string{"sessions", "archived_sessions"} {
		root := filepath.Join(home, sub)
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || found != "" {
				return nil
			}
			n := d.Name()
			if !d.IsDir() && strings.HasPrefix(n, "rollout-") && strings.HasSuffix(n, suffix) {
				found = p
				return fs.SkipAll
			}
			return nil
		})
		if found != "" {
			return found
		}
	}
	return ""
}

// LoadTranscript 读取 Codex 自己的 rollout 会话文件（codex_cli）。正文不进 SQLite，刷新后从这里读回。
// 没有 thread_id 或文件已不在时返回空列表，不报错，保证记录页能打开。Claude 暂不支持（返回空列表）。
func (a *CLIAgentAdapter) LoadTranscript(_ context.Context, convID, _ string) ([]WireMessage, error) {
	out := []WireMessage{}
	if a.kind != cliKindCodex {
		return out, nil
	}
	sid := a.loadSessionID(strings.TrimSpace(convID))
	p := findCodexRollout(a.codexHome(), sid)
	if p == "" {
		return out, nil
	}
	f, err := os.Open(p)
	if err != nil {
		a.cfg.Logf("cat codex transcript open: %v", err)
		return out, nil
	}
	defer f.Close()
	return ParseCodexRollout(f), nil
}

// ParseCodexRollout 把 rollout jsonl 收成消息。只认 response_item 的 message/reasoning/工具调用
// 和 event_msg 里的错误；坏行、未知类型、developer 指令、注入的上下文块一律跳过。
func ParseCodexRollout(r interface{ Read([]byte) (int, error) }) []WireMessage {
	out := []WireMessage{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var line struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil || len(line.Payload) == 0 {
			continue
		}
		var p struct {
			Type      string `json:"type"`
			Role      string `json:"role"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Input     string `json:"input"`
			Message   string `json:"message"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Summary []struct {
				Text string `json:"text"`
			} `json:"summary"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal(line.Payload, &p) != nil {
			continue
		}
		add := func(role, text string) {
			if text = strings.TrimSpace(text); text != "" {
				out = append(out, WireMessage{Role: role, Content: text})
			}
		}
		switch line.Type {
		case "response_item":
			switch p.Type {
			case "message":
				var parts []string
				for _, c := range p.Content {
					if c.Text == "" || (p.Role == "user" && isCodexInjected(c.Text)) {
						continue
					}
					parts = append(parts, c.Text)
				}
				switch p.Role {
				case "user":
					add("user", strings.Join(parts, "\n"))
				case "assistant":
					add("assistant", strings.Join(parts, "\n"))
				}
			case "reasoning":
				var parts []string
				for _, s := range p.Summary {
					parts = append(parts, s.Text)
				}
				add("thought", strings.Join(parts, "\n"))
			case "function_call", "custom_tool_call", "local_shell_call":
				args := p.Arguments
				if args == "" {
					args = p.Input
				}
				name := p.Name
				if name == "" {
					name = p.Type
				}
				add("tool", truncate(strings.TrimSpace(name+" "+args), 200))
			}
		case "event_msg":
			switch p.Type {
			case "error":
				add("status", p.Message)
			case "task_complete", "turn_aborted":
				if msg := codexErrMessage(p.Error); msg != "" {
					add("status", msg)
				}
			}
		}
	}
	return out
}

func codexErrMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &o)
	return o.Message
}

// isCodexInjected：codex 以 user 角色注入的环境/指令块，不是用户说的话。
func isCodexInjected(text string) bool {
	t := strings.TrimSpace(text)
	for _, pre := range []string{"<environment_context>", "<user_instructions>", "# AGENTS.md instructions", "<permissions instructions>", "<turn_aborted>", "<user_shell_command>"} {
		if strings.HasPrefix(t, pre) {
			return true
		}
	}
	return false
}
