package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PartPath 返回最终输出 final 对应的临时文件名：<name>.part.<原扩展名>（契约 v0.4）。
// 例如 /out/a.mp4 → /out/a.part.mp4。保留扩展名是为了让 ffmpeg 能按扩展名识别封装格式。
func PartPath(final string) string {
	ext := filepath.Ext(final)
	base := strings.TrimSuffix(final, ext)
	return base + ".part" + ext
}

// UniquePath 在目标已存在时依次尝试 name(1).ext、name(2).ext ……直到找到不存在的路径（契约 6.5）。
// 注意：它只检查最终文件；.part 文件也被视为占用，避免两个并发任务写同一个 .part。
func UniquePath(final string) string {
	if !exists(final) && !exists(PartPath(final)) {
		return final
	}
	ext := filepath.Ext(final)
	base := strings.TrimSuffix(final, ext)
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s(%d)%s", base, i, ext)
		if !exists(cand) && !exists(PartPath(cand)) {
			return cand
		}
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// RunWithPart 为写文件的任务提供统一的输出流程：
//
//  1. 选出不冲突的最终路径（重名追加 (1)、(2)）并创建输出目录；
//  2. 调用 produce(partPath) 让任务写到 <name>.part.<ext>；
//  3. produce 成功后原子改名为最终路径；失败或取消则删除 .part。
//
// 返回最终路径。produce 必须把完整输出写到 partPath。
func RunWithPart(desired string, produce func(partPath string) error) (string, error) {
	final := UniquePath(desired)
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return "", fmt.Errorf("创建输出目录失败: %w", err)
	}
	part := PartPath(final)
	if err := produce(part); err != nil {
		os.Remove(part)
		return "", err
	}
	if err := os.Rename(part, final); err != nil {
		os.Remove(part)
		return "", fmt.Errorf("重命名输出文件失败: %w", err)
	}
	return final, nil
}
