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

// groupExecutionScope 是群聊成员回合的执行范围：项目 + groupId + memberId。
// 群成员各跑各的（@ 两人真并行），不拿普通对话用的项目目录锁；同一成员仍串行。
func groupExecutionScope(root, groupID, memberID string) string {
	return "group:" + projectConfigKey(root) + "|" + groupID + "|" + memberID
}
