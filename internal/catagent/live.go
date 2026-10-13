package catagent

import "strings"

// processCommand 是一个还在跑的进程和它的命令行。
type processCommand struct {
	pid  uint32
	line string
}

// readProcessCommands 列出可能属于这场回复的进程。测试替换它。
var readProcessCommands = readOSProcessCommands

// killProcessTree 结束一个进程和它拉起来的子进程。测试替换它。
var killProcessTree = killOSProcessTree

// LiveConvIDs 返回命令行里还挂着这场会话的对话。本进程自己记着的回合不必再问这里。
func LiveConvIDs(convIDs []string) []string {
	if len(convIDs) == 0 {
		return nil
	}
	want := make(map[string]string, len(convIDs))
	for _, id := range convIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		want[sessionIDFromConv(id)] = id
	}
	if len(want) == 0 {
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	for _, p := range readProcessCommands() {
		for sid, conv := range want {
			if _, ok := seen[conv]; ok {
				continue
			}
			if commandHasSession(p.line, sid) {
				seen[conv] = struct{}{}
				out = append(out, conv)
			}
		}
	}
	return out
}

// StopLiveConv 结束刷新后还挂在这场会话上的进程。本进程自己的回合走原来的取消，不从这里杀。
func StopLiveConv(convID string) {
	sid := sessionIDFromConv(strings.TrimSpace(convID))
	if sid == "" {
		return
	}
	for _, p := range readProcessCommands() {
		if commandHasSession(p.line, sid) {
			_ = killProcessTree(p.pid)
		}
	}
}

// commandHasSession 只认 --resume / --session-id 后面的会话号，避免正文里偶然出现同一串字符。
func commandHasSession(line, sessionID string) bool {
	if line == "" || sessionID == "" {
		return false
	}
	fields := splitCmd(line)
	for i, f := range fields {
		switch {
		case f == "--resume" || f == "--session-id":
			if i+1 < len(fields) && strings.EqualFold(fields[i+1], sessionID) {
				return true
			}
		case strings.HasPrefix(strings.ToLower(f), "--resume="):
			if strings.EqualFold(f[len("--resume="):], sessionID) {
				return true
			}
		case strings.HasPrefix(strings.ToLower(f), "--session-id="):
			if strings.EqualFold(f[len("--session-id="):], sessionID) {
				return true
			}
		}
	}
	return false
}

func splitCmd(line string) []string {
	var out []string
	var b strings.Builder
	quote := false
	for _, r := range line {
		switch {
		case r == '"':
			quote = !quote
		case !quote && (r == ' ' || r == '\t'):
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}
