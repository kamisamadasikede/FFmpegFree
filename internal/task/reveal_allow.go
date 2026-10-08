package task

import (
	"os"
	"path/filepath"
	"time"
)

// 契约 v0.23.3：DeleteRecords / DeleteSource 返回的文件类失败（in_use / permission / not_task_output / io）里的路径，
// 在同一次运行里、删除调用之后 10 分钟内可以用 RevealInFolder 打开所在文件夹（记录已删，不再是“登记的输出”）。
// 只放行记录登记的输出路径本身（按 Clean 后的路径精确匹配，大小写规则同 nameKey），只在内存里，最多保留最新的 100 个。
const (
	revealAllowTTL = 10 * time.Minute
	revealAllowMax = 100
)

type revealItem struct {
	key     string
	expires time.Time
}

// revealAllow 是放行表；零值可用，由 Manager.mu 保护。按加入顺序排列，最旧的在前。
type revealAllow struct {
	items []revealItem
}

func (m *Manager) now() time.Time {
	if m.cfg.Now != nil {
		return m.cfg.Now()
	}
	return time.Now()
}

// allowRevealOfFailure 把删除失败留下的文件加入放行表。path 必须就是记录登记的输出路径 recorded（绝对路径），
// 否则不加——放行表永远只收我们自己记录过的输出路径，不收任意路径。
func (m *Manager) allowRevealOfFailure(path, recorded string) {
	if path == "" || recorded == "" || !filepath.IsAbs(path) || !filepath.IsAbs(recorded) {
		return
	}
	k := nameKey(path)
	if k != nameKey(recorded) {
		return
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	items := m.reveal.items[:0]
	for _, it := range m.reveal.items { // 去掉过期的和同一路径的旧项
		if it.key != k && now.Before(it.expires) {
			items = append(items, it)
		}
	}
	items = append(items, revealItem{key: k, expires: now.Add(revealAllowTTL)})
	if n := len(items) - revealAllowMax; n > 0 {
		items = append([]revealItem(nil), items[n:]...)
	}
	m.reveal.items = items
}

// IsRecentDeleteFailure 判断 path（Clean 后精确匹配）是不是 10 分钟内删除失败、留在磁盘上的记录输出文件。
// 符号链接不算（同 IsTaskOutput：RevealInFolder 打开的是真实路径，链接可能指到别处）。
func (m *Manager) IsRecentDeleteFailure(path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	k := nameKey(path)
	now := m.now()
	m.mu.Lock()
	found := false
	for _, it := range m.reveal.items {
		if it.key == k && now.Before(it.expires) {
			found = true
			break
		}
	}
	m.mu.Unlock()
	if !found {
		return false
	}
	fi, err := os.Lstat(filepath.Clean(path))
	return err == nil && fi.Mode()&os.ModeSymlink == 0
}
