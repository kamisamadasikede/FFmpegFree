package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CatConvDir 是普通对话的工作目录（契约 6.19.11.4）：<dataRoot>/cat/conversations/<convId>。
// 适配器的 --cwd、ListCatFiles、RevealCatConversationFolder 共用这一处，不各算各的。
// convId 会收成目录名安全片段；dataRoot 或 id 为空时返回 ""。
func CatConvDir(dataRoot, convID string) string {
	seg := convSegment(convID)
	root := strings.TrimSpace(dataRoot)
	if root == "" || seg == "" {
		return ""
	}
	return filepath.Join(root, "cat", "conversations", seg)
}

// LegacyCatCwd 是 v0.32 之前的工作目录：<dataRoot>/tmp/cat/cwd-<convId>。
// CLI 按这个目录续会话。旧目录还在时这个对话继续用它，不搬、不复制。
func LegacyCatCwd(dataRoot, convID string) string {
	seg := convSegment(convID)
	root := strings.TrimSpace(dataRoot)
	if root == "" || seg == "" {
		return ""
	}
	return filepath.Join(root, "tmp", "cat", "cwd-"+seg)
}

// ResolveCatConvDir 选择这个对话实际使用的文件夹：旧目录是真实文件夹就用旧的，否则用 CatConvDir。
// 不创建目录。旧路径若是符号链接则不用，避免跟着链接走。
func ResolveCatConvDir(dataRoot, convID string) string {
	legacy := LegacyCatCwd(dataRoot, convID)
	if legacy != "" && realDir(legacy) {
		return legacy
	}
	return CatConvDir(dataRoot, convID)
}

// SamePath 比较两条路径。Windows / macOS 不区分大小写。
func SamePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if CaseInsensitiveGOOS(runtime.GOOS) {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// IsCatConvDir 报告 dir 是不是这个对话自己的文件夹（新位置或旧 cwd，精确相等，不是父目录）。
func IsCatConvDir(dataRoot, convID, dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	return SamePath(dir, CatConvDir(dataRoot, convID)) || SamePath(dir, LegacyCatCwd(dataRoot, convID))
}

// convSegment 与历史 cwd 目录名同一规则：只留字母数字和 - _，其余换成 _，最长 80。
// ULID 会话 id 经这步不变，所以旧的 cwd-<id> 仍能对上。
func convSegment(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return ""
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

func realDir(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.IsDir() && fi.Mode()&os.ModeSymlink == 0
}
