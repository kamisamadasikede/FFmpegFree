package catagent

import (
	"encoding/json"
	"strings"
)

// v0.48.2（契约 6.19.23）：Codex / Claude 回复过程中实时显示工具调用。
// 标签格式与重新加载会话记录（claude_transcript.go / codex_transcript.go）一致：「名字 参数」截断 200 字。

// claudeActivityPieces 处理 Claude stream-json 的 type=assistant / type=user 整条消息：
// assistant 的 tool_use / thinking 块 → tool / thought；user 的 tool_result 且 is_error → tool「error: …」。
// 文字块不在这里出（已由 stream_event 的 text_delta 增量送达，避免重复）。
// ok=false 表示不是这两类行，交回原解析。
func claudeActivityPieces(line []byte) (pieces []streamPiece, sid string, ok bool) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(line, &raw) != nil {
		return nil, "", false
	}
	typ, _ := rawStr(raw, "type")
	typ = strings.ToLower(typ)
	if typ != "assistant" && typ != "user" {
		return nil, "", false
	}
	sid, _ = rawStr(raw, "session_id")
	var msg struct {
		ID      string          `json:"id"`
		Content json.RawMessage `json:"content"`
	}
	if len(raw["message"]) == 0 || json.Unmarshal(raw["message"], &msg) != nil {
		return nil, sid, true
	}
	var blocks []struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     json.RawMessage `json:"input"`
		Thinking  string          `json:"thinking"`
		ToolUseID string          `json:"tool_use_id"`
		IsError   bool            `json:"is_error"`
		Content   json.RawMessage `json:"content"`
	}
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return nil, sid, true
	}
	for i, b := range blocks {
		switch {
		case typ == "assistant" && b.Type == "tool_use":
			args := strings.TrimSpace(string(b.Input))
			if args == "{}" || args == "null" {
				args = ""
			}
			text := truncate(strings.TrimSpace(b.Name+" "+args), 200)
			if text == "" {
				continue
			}
			pieces = append(pieces, streamPiece{Kind: "tool", ID: firstNonEmpty(b.ID, msg.ID+":"+fmtInt64(int64(i))), Text: text, Replace: true})
		case typ == "assistant" && b.Type == "thinking":
			if t := strings.TrimSpace(b.Thinking); t != "" {
				pieces = append(pieces, streamPiece{Kind: "thought", ID: "think:" + msg.ID + ":" + fmtInt64(int64(i)), Text: t, Replace: true})
			}
		case typ == "user" && b.Type == "tool_result" && b.IsError:
			if t := strings.TrimSpace(claudeToolResultText(b.Content)); t != "" {
				pieces = append(pieces, streamPiece{Kind: "tool", ID: firstNonEmpty(b.ToolUseID, "tool") + ":error", Text: truncate("error: "+t, 200), Replace: true})
			}
		}
	}
	return pieces, sid, true
}

// codexToolLabel 把 codex exec --json 的工具类 item 变成一行标签。
// 字段按 codex-rs exec/src/exec_events.rs：command_execution{command,status}、
// file_change{changes[{path,kind}],status}、mcp_tool_call{server,tool,arguments,status}、web_search{query}。
func codexToolLabel(itemType string, item map[string]json.RawMessage) string {
	status := strings.ToLower(rawStrVal(item, "status"))
	var label string
	switch itemType {
	case "command_execution":
		label = strings.TrimSpace("shell " + rawStrVal(item, "command"))
	case "file_change":
		var changes []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(item["changes"], &changes)
		var b strings.Builder
		b.WriteString("apply_patch")
		for _, c := range changes {
			if p := strings.TrimSpace(c.Path); p != "" {
				b.WriteByte('\n')
				if c.Kind != "" {
					b.WriteString(c.Kind + " ")
				}
				b.WriteString(p)
			}
		}
		label = b.String()
	case "mcp_tool_call":
		name := strings.Trim(rawStrVal(item, "server")+"."+rawStrVal(item, "tool"), ".")
		if name == "" {
			name = "mcp_tool_call"
		}
		args := strings.TrimSpace(string(item["arguments"]))
		if args == "{}" || args == "null" {
			args = ""
		}
		label = strings.TrimSpace(name + " " + args)
	case "web_search":
		label = strings.TrimSpace("web_search " + rawStrVal(item, "query"))
	}
	if label == "" {
		label = itemType
	}
	if status == "failed" || status == "declined" {
		label += " · 失败"
	}
	return truncate(label, 200)
}
