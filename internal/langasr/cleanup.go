package langasr

import (
	"os"
	"path/filepath"
	"strings"
)

// CleanupTaskDir 删除单个任务临时目录 <tempRoot>/<taskId>。
func CleanupTaskDir(tempRoot, taskID string) {
	if tempRoot == "" || taskID == "" || strings.Contains(taskID, "..") || strings.ContainsAny(taskID, `/\`) {
		return
	}
	_ = os.RemoveAll(filepath.Join(tempRoot, taskID))
}

// CleanupResidues 启动时清理 tmp/lang 下残留（契约 6.18.2）。
func CleanupResidues(tempRoot string) {
	if tempRoot == "" {
		return
	}
	ents, err := os.ReadDir(tempRoot)
	if err != nil {
		return
	}
	for _, e := range ents {
		_ = os.RemoveAll(filepath.Join(tempRoot, e.Name()))
	}
}
