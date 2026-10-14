package catagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// 契约 v0.45 §6.19.19.1：Claude 记录读回。
// 位置：D https://code.claude.com/docs/en/sessions「Where transcripts are stored」——
// $CLAUDE_CONFIG_DIR（未设则 ~/.claude）/projects/<目录名>/<sessionId>.jsonl。
// 真机核对（claude 2.1.296，box）：cwd=/tmp/ccprobe → projects/-tmp-ccprobe/<session_id>.jsonl，
// 文件名就是 stream-json system/init 里的 session_id（testdata/claude/061a686f-….jsonl）。

// claudeConfigDir 与 Claude Code 一致：$CLAUDE_CONFIG_DIR，否则 ~/.claude。
func (a *CLIAgentAdapter) claudeConfigDir() string {
	if h := strings.TrimSpace(a.cfg.ClaudeConfigDir); h != "" {
		return h
	}
	if h := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// claudeProjectDirName 按文档规则把工作目录换成目录名：非字母数字换成 '-'（真机 /tmp/ccprobe → -tmp-ccprobe）。
// 超长截断 / CLAUDE_CODE_PROJECT_DIR_NAME 改名的情况不自己拼，交给 findClaudeTranscript 扫一层兜底。
func claudeProjectDirName(cwd string) string {
	var b strings.Builder
	for _, r := range cwd {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func validClaudeSessionID(sid string) bool {
	return sid != "" && !strings.ContainsAny(sid, `/\`) && !strings.Contains(sid, "..")
}

// findClaudeTranscript 先试按规则拼出的目录，找不到再在 projects/*/ 下按文件名查。
func findClaudeTranscript(configDir, cwd, sid string) string {
	if configDir == "" || !validClaudeSessionID(sid) {
		return ""
	}
	root := filepath.Join(configDir, "projects")
	name := sid + ".jsonl"
	if cwd != "" {
		p := filepath.Join(root, claudeProjectDirName(cwd), name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name(), name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func (a *CLIAgentAdapter) loadClaudeTranscript(convID, cwd string) ([]WireMessage, error) {
	sid := a.loadSessionID(convID)
	if sid == "" {
		return []WireMessage{}, nil // 从没存过 sessionId：新对话，不算缺失（v0.45.2）
	}
	p := findClaudeTranscript(a.claudeConfigDir(), cwd, sid)
	if p == "" {
		return []WireMessage{}, ErrHistoryMissing
	}
	f, err := os.Open(p)
	if err != nil {
		a.cfg.Logf("cat claude transcript open: %v", err)
		if os.IsNotExist(err) {
			return []WireMessage{}, ErrHistoryMissing
		}
		return []WireMessage{}, ErrHistoryReadFailed
	}
	defer f.Close()
	return ParseClaudeTranscript(f), nil
}

type claudeLine struct {
	Type              string `json:"type"`
	IsSidechain       bool   `json:"isSidechain"`
	IsMeta            bool   `json:"isMeta"`
	IsAPIErrorMessage bool   `json:"isApiErrorMessage"`
	Message           *struct {
		ID      string          `json:"id"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type claudeBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Content  json.RawMessage `json:"content"`
	IsError  bool            `json:"is_error"`
}

// claudeBlocks 把 message.content（字符串或 block 数组）统一成 block 列表。
func claudeBlocks(raw json.RawMessage) []claudeBlock {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []claudeBlock{{Type: "text", Text: s}}
	}
	var list []claudeBlock
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	return list
}

// isClaudeInjectedUser：CLI 自己以 user 角色记下的斜杠命令 / 本地命令回显，不是用户说的话
// （真机 2.1.296 testdata/claude/slash-command-2.1.296.jsonl：<local-command-caveat>、<command-name>）。
func isClaudeInjectedUser(text string) bool {
	t := strings.TrimSpace(text)
	for _, pre := range []string{"<local-command-", "<command-name>", "<command-message>", "<command-args>", "<system-reminder>"} {
		if strings.HasPrefix(t, pre) {
			return true
		}
	}
	return false
}

func claudeToolResultText(raw json.RawMessage) string {
	var parts []string
	for _, b := range claudeBlocks(raw) {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// ParseClaudeTranscript 宽松解析 Claude 会话 jsonl（官方说明行格式是内部格式、随版本变化）：
// 坏行 / 未知 type 跳过；只认 user / assistant；跳过 sidechain、meta；同一 message.id 的 assistant 分片合并。
func ParseClaudeTranscript(r io.Reader) []WireMessage {
	out := []WireMessage{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	lastAssistantID := ""
	lastAssistantIdx := -1
	add := func(role, text string) int {
		if text = strings.TrimSpace(text); text == "" {
			return -1
		}
		out = append(out, WireMessage{Role: role, Content: text})
		return len(out) - 1
	}
	for sc.Scan() {
		var ln claudeLine
		if json.Unmarshal(sc.Bytes(), &ln) != nil || ln.Message == nil {
			continue
		}
		if ln.IsSidechain || ln.IsMeta {
			continue
		}
		blocks := claudeBlocks(ln.Message.Content)
		switch ln.Type {
		case "user":
			lastAssistantID, lastAssistantIdx = "", -1
			var texts []string
			for _, b := range blocks {
				switch b.Type {
				case "text":
					if !isClaudeInjectedUser(b.Text) {
						texts = append(texts, b.Text)
					}
				case "tool_result":
					if b.IsError {
						if t := claudeToolResultText(b.Content); t != "" {
							add("tool", truncate("error: "+t, 200))
						}
					}
				}
			}
			add("user", strings.Join(texts, "\n"))
		case "assistant":
			if ln.IsAPIErrorMessage {
				// CLI 自己合成的失败行（真机：model=<synthetic>，isApiErrorMessage=true）→ 状态行。
				var texts []string
				for _, b := range blocks {
					texts = append(texts, b.Text)
				}
				add("status", strings.Join(texts, "\n"))
				lastAssistantID, lastAssistantIdx = "", -1
				continue
			}
			mid := ln.Message.ID
			for _, b := range blocks {
				switch b.Type {
				case "text":
					if strings.TrimSpace(b.Text) == "" {
						continue
					}
					if mid != "" && mid == lastAssistantID && lastAssistantIdx >= 0 && lastAssistantIdx == len(out)-1 && out[lastAssistantIdx].Role == "assistant" {
						out[lastAssistantIdx].Content += b.Text
						continue
					}
					if i := add("assistant", b.Text); i >= 0 {
						lastAssistantIdx = i
					}
				case "thinking":
					add("thought", b.Thinking)
				case "tool_use":
					args := strings.TrimSpace(string(b.Input))
					if args == "{}" || args == "null" {
						args = ""
					}
					add("tool", truncate(strings.TrimSpace(b.Name+" "+args), 200))
				}
			}
			lastAssistantID = mid
		}
	}
	for i := range out {
		out[i].Content = strings.TrimSpace(out[i].Content)
	}
	return out
}

// firstUserLabel 逐行找第一条用户话就停（侧栏每条都要取，不整份解析）。
func firstUserLabel(path string, parse func(io.Reader) []WireMessage) string {
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		for _, m := range parse(bytes.NewReader(sc.Bytes())) {
			if m.Role == "user" {
				if l := shortLabel(m.Content); l != "" {
					return l
				}
			}
		}
	}
	return ""
}

// SessionLabel（Claude / Codex，v0.45 §6.19.19.1 第 5 条）：从 CLI 自己的会话文件取首句用户话，
// 只当次展示，调用方不回写库。
func (a *CLIAgentAdapter) SessionLabel(convID, cwd string) string {
	sid := a.loadSessionID(strings.TrimSpace(convID))
	if a.kind == cliKindClaude {
		return firstUserLabel(findClaudeTranscript(a.claudeConfigDir(), cwd, sid), ParseClaudeTranscript)
	}
	return firstUserLabel(findCodexRollout(a.codexHome(), sid), func(r io.Reader) []WireMessage { return ParseCodexRollout(r) })
}
