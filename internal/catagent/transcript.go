package catagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
)

// SessionStarted 表示这个对话在 CLI 自己的会话目录里已经有记录。
// 只用来决定下一句用 --resume 还是 --session-id，不把正文抄进应用数据库。
func SessionStarted(cwd, convID string) bool {
	return sessionStartedIn(defaultSessionsRoot(), cwd, convID)
}

func sessionStartedIn(root, cwd, convID string) bool {
	p := contextSignalsPath(root, cwd, convID)
	if p == "" {
		return false
	}
	st, err := os.Stat(filepath.Dir(p))
	return err == nil && st.IsDir()
}

// SessionLabel 从 CLI 自己的会话文件里取侧栏用的短标题（第一句用户话）。
// 只用于当次展示，调用方不要写回应用数据库。
func SessionLabel(cwd, convID string) string {
	return sessionLabelIn(defaultSessionsRoot(), cwd, convID)
}

func sessionLabelIn(root, cwd, convID string) string {
	p := contextSignalsPath(root, cwd, convID)
	if p == "" {
		return ""
	}
	f, err := os.Open(filepath.Join(filepath.Dir(p), "chat_history.jsonl"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var row struct {
			Type    string          `json:"type"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(sc.Bytes(), &row) != nil || row.Type != "user" {
			continue
		}
		var text string
		if json.Unmarshal(row.Content, &text) != nil {
			continue
		}
		if label := shortLabel(text); label != "" {
			return label
		}
	}
	return ""
}

func shortLabel(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 18 {
		return string(r[:18]) + "…"
	}
	return s
}

func (a *BuildAdapter) sessionRoot() string {
	if r := strings.TrimSpace(a.cfg.SessionsRoot); r != "" {
		return r
	}
	return defaultSessionsRoot()
}

// LoadTranscript 读取 CLI 自己的会话。优先用 updates.jsonl，按和当场流式相同的规则收成消息；
// 没有这份事件时才退回 `grok export` 的 Markdown。会话目录还不存在时直接空列表，不启进程。
func (a *BuildAdapter) LoadTranscript(ctx context.Context, convID, cwd string) ([]WireMessage, error) {
	convID = strings.TrimSpace(convID)
	if convID == "" || !sessionStartedIn(a.sessionRoot(), cwd, convID) {
		return []WireMessage{}, nil
	}
	if p := sessionUpdatesPath(a.sessionRoot(), cwd, convID); p != "" {
		if msgs, err := projectSessionFile(p); err == nil && len(msgs) > 0 {
			return msgs, nil
		}
	}
	if a.Status().State != StateReady {
		return nil, apperr.New(apperr.CatNotReady, MsgNotReady)
	}
	a.mu.Lock()
	exe := a.exe
	a.mu.Unlock()
	if exe == "" {
		return nil, apperr.New(apperr.CatNotReady, MsgNotReady)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := a.runCmd(runCtx, exe, "export", sessionIDFromConv(convID))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(strings.ToLower(stderr.String()), "not found") {
			return []WireMessage{}, nil
		}
		return nil, apperr.Wrap(apperr.IOError, "读取对话记录失败", err)
	}
	return ParseExportMarkdown(stdout.String()), nil
}

func projectSessionFile(path string) ([]WireMessage, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ProjectSessionUpdates(b)
}

// ParseExportMarkdown 把 `grok export` 的 Markdown 拆成用户 / 助手 / 工具。
// 只有单独一行的 ## User、## Assistant、## Tools、## Thinking 是分段；
// 助手正文里的标题留在正文里。围栏代码块里的同名行也不分段。
func ParseExportMarkdown(s string) []WireMessage {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out []WireMessage
	var role string
	var b strings.Builder
	flush := func() {
		if role == "" {
			return
		}
		text := strings.TrimSpace(b.String())
		b.Reset()
		if text == "" {
			return
		}
		out = append(out, WireMessage{Role: role, Content: text})
	}
	inFence := false
	for _, line := range strings.Split(s, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
		}
		if !inFence {
			if next, ok := exportRole(trim); ok {
				flush()
				role = next
				continue
			}
		}
		if role == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	flush()
	if out == nil {
		out = []WireMessage{}
	}
	return out
}

func exportRole(line string) (string, bool) {
	switch line {
	case "## User":
		return "user", true
	case "## Assistant":
		return "assistant", true
	case "## Tools":
		return "tool", true
	case "## Thinking":
		return "thought", true
	default:
		return "", false
	}
}
