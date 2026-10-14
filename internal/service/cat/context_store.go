package cat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/paths"
)

// 契约 v0.48.1 §6.19.22：非 Grok 助手的上下文占用记录，放在应用自己的对话目录（不写进用户项目文件夹）。
const contextUsageFile = ".cat-context.json"

type contextUsageDoc struct {
	Used   int64 `json:"contextUsed"`
	Window int64 `json:"contextWindow"`
}

func contextUsagePath(dataRoot, convID string) string {
	if strings.TrimSpace(dataRoot) == "" || strings.TrimSpace(convID) == "" {
		return ""
	}
	return filepath.Join(paths.ResolveCatConvDir(dataRoot, convID), contextUsageFile)
}

func saveContextUsage(dataRoot, convID string, used, window int64) {
	p := contextUsagePath(dataRoot, convID)
	if p == "" || used <= 0 || window <= 0 {
		return
	}
	b, err := json.Marshal(contextUsageDoc{Used: used, Window: window})
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(p), 0o755) != nil {
		return
	}
	tmp := p + ".tmp"
	if os.WriteFile(tmp, b, 0o644) != nil {
		return
	}
	_ = os.Rename(tmp, p)
}

func loadContextUsage(dataRoot, convID string) (int64, int64, bool) {
	p := contextUsagePath(dataRoot, convID)
	if p == "" {
		return 0, 0, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return 0, 0, false
	}
	var d contextUsageDoc
	if json.Unmarshal(b, &d) != nil || d.Used <= 0 || d.Window <= 0 {
		return 0, 0, false
	}
	return d.Used, d.Window, true
}
