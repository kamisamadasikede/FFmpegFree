package doc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/store"
)

// TaskLister 列任务（启动时清理 interrupted 的 office_pdf 任务遗留的 .part），*store.Store 实现它。
type TaskLister interface {
	ListTasks(ctx context.Context, f store.TaskFilter) (store.TaskPage, error)
}

// CleanupInterruptedParts 启动时调用，删除被中断的 office_pdf 任务遗留的 .part 文件。只在同时满足下面五条时才删：
//  1. 文件名是 <name>.part.pdf 或 <name>(n).part.pdf（n=1..99），<name>.pdf 是某条 interrupted 的 office_pdf 任务记录的 outputPath；
//  2. 只看任务记录里登记的输出目录（outputPath 必须是绝对路径且扩展名是 .pdf），不扫描其他目录；
//  3. 修改时间早于本次启动（不会误删本次启动后新任务正在写的 .part）；
//  4. 只删普通文件（Lstat，不跟随符号链接；目录、链接、设备都不碰）；
//  5. 不递归、不通配：候选文件名是精确拼出来的 100 个。
//
// 返回删除个数。
func (s *Service) CleanupInterruptedParts(ctx context.Context) int {
	if s.cfg.Lister == nil {
		return 0
	}
	removed := 0
	for offset := 0; offset < 5000; {
		page, err := s.cfg.Lister.ListTasks(ctx, store.TaskFilter{
			Types: []store.TaskType{store.TypeOfficePDF}, Statuses: []store.TaskStatus{store.StatusInterrupted}, Limit: 200, Offset: offset})
		if err != nil || len(page.Items) == 0 {
			break
		}
		for _, t := range page.Items {
			out := t.OutputPath
			if out == "" || !filepath.IsAbs(out) || !strings.EqualFold(filepath.Ext(out), ".pdf") {
				continue
			}
			ext := filepath.Ext(out)
			base := strings.TrimSuffix(out, ext)
			cands := []string{base + ".part" + ext}
			for i := 1; i <= 99; i++ {
				cands = append(cands, fmt.Sprintf("%s(%d).part%s", base, i, ext))
			}
			for _, c := range cands {
				if fi, err := os.Lstat(c); err == nil && fi.Mode().IsRegular() && fi.ModTime().Before(s.started) {
					if os.Remove(c) == nil {
						removed++
					}
				}
			}
		}
		offset += len(page.Items)
	}
	return removed
}
