package system

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
)

// FileFilter 是文件选择对话框的过滤器（契约第 3 节）。Patterns 是 glob 列表，如 ["*.mp4", "*.mkv"]；
// 单个元素里也可以用分号写多个（"*.mp4;*.mkv"）。Patterns 为空表示不过滤（所有文件）。
type FileFilter struct {
	Name     string   `json:"name"`     // 对话框里显示的名字，如 "视频文件"；为空时用 Patterns 拼成
	Patterns []string `json:"patterns"` // 如 ["*.mp4", "*.mkv"]
}

const (
	maxFilterPatterns = 100
	maxFilterNameLen  = 100
)

// NormalizePatterns 校验并展开过滤器里的 glob：拆分分号 / 逗号、去空白、去重；
// 每项只允许「*.扩展名」形式（扩展名由字母数字、_ - + ? * 组成）或单独的 "*" / "*.*"。返回 nil 表示不过滤。
func (f FileFilter) NormalizePatterns() ([]string, error) {
	if len(f.Patterns) > maxFilterPatterns {
		return nil, apperr.New(apperr.InvalidArgument, "过滤器的扩展名太多")
	}
	if utf8.RuneCountInString(f.Name) > maxFilterNameLen {
		return nil, apperr.New(apperr.InvalidArgument, "过滤器名称太长")
	}
	var out []string
	seen := map[string]bool{}
	for _, raw := range f.Patterns {
		for _, p := range strings.FieldsFunc(raw, func(r rune) bool { return r == ';' || r == ',' }) {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if !validPattern(p) {
				return nil, apperr.New(apperr.InvalidArgument, "过滤器格式不对，应为 *.mp4 这样的通配符").WithDetail(p)
			}
			if k := strings.ToLower(p); !seen[k] {
				seen[k] = true
				out = append(out, p)
			}
		}
	}
	for _, p := range out {
		if p == "*" || p == "*.*" {
			return nil, nil // 包含"所有文件"就等于不过滤
		}
	}
	return out, nil
}

func validPattern(p string) bool {
	ext, ok := strings.CutPrefix(p, "*.")
	if !ok || ext == "" {
		return p == "*"
	}
	for _, r := range ext {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '+', r == '?', r == '*':
		default:
			return false
		}
	}
	return true
}

// DialogPattern 返回传给 Wails 的过滤器：显示名和分号分隔的模式串；不过滤时返回 ok=false。
func (f FileFilter) DialogPattern() (name, pattern string, ok bool, err error) {
	pats, err := f.NormalizePatterns()
	if err != nil || len(pats) == 0 {
		return "", "", false, err
	}
	pattern = strings.Join(pats, ";")
	name = strings.TrimSpace(f.Name)
	if name == "" {
		name = pattern
	}
	return name, pattern, true, nil
}

// CleanPickedPaths 把对话框返回的路径规整成绝对的、Clean 过的路径，丢弃空串和重复项；始终返回非 nil 切片。
func CleanPickedPaths(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, p := range in {
		if strings.TrimSpace(p) == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}
