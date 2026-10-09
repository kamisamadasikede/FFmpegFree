package catagent

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/paths"
)

// TurnCwd 是这一轮传给助手的工作目录。项目对话用项目文件夹，普通对话用会话目录。不创建目录。
func TurnCwd(dataRoot, projectPath, convID string) string {
	if p := strings.TrimSpace(projectPath); p != "" {
		return p
	}
	return paths.ResolveCatConvDir(dataRoot, convID)
}

// ReadContextUsage 读取该会话最近一次的上下文占用（已用 token、窗口容量）。
// 没有记录、已用为 0 或容量非法时 ok 为 false，避免把空会话画成 0%。
func ReadContextUsage(cwd, convID string) (used, window int64, ok bool) {
	return readContextUsageIn(defaultSessionsRoot(), cwd, convID)
}

func defaultSessionsRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".grok", "sessions")
}

func readContextUsageIn(root, cwd, convID string) (int64, int64, bool) {
	path := contextSignalsPath(root, cwd, convID)
	if path == "" {
		return 0, 0, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, false
	}
	var doc struct {
		Used   int64 `json:"contextTokensUsed"`
		Window int64 `json:"contextWindowTokens"`
	}
	if json.Unmarshal(b, &doc) != nil {
		return 0, 0, false
	}
	if doc.Used <= 0 || doc.Window <= 0 {
		return 0, 0, false
	}
	return doc.Used, doc.Window, true
}

// contextSignalsPath 拼出会话用量文件。目录名与助手落盘一致：QueryEscape 后把空格的 + 换成 %20。
func contextSignalsPath(root, cwd, convID string) string {
	root = strings.TrimSpace(root)
	cwd = strings.TrimSpace(cwd)
	convID = strings.TrimSpace(convID)
	if root == "" || cwd == "" || convID == "" {
		return ""
	}
	name := strings.ReplaceAll(url.QueryEscape(cwd), "+", "%20")
	return filepath.Join(root, name, sessionIDFromConv(convID), "signals.json")
}
