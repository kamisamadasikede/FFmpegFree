package cat

import (
	"path/filepath"
	"strings"

	"FFmpegFree/internal/agentexec"
)

// projectConfigKey 是共享执行运行时（agentexec）按项目区分原生配置的键。
func projectConfigKey(root string) string {
	if root == "" {
		return "global"
	}
	p, e := agentexec.CanonicalRoot(root)
	if e != nil {
		p = filepath.Clean(root)
	}
	return strings.ToLower(p)
}
