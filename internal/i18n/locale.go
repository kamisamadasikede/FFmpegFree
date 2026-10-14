package i18n

import (
	"strings"
)

// MapOSLocale 把 OS 原始 locale 字符串映射为生效语言（契约 6.20.3）。
// zh*（含 zh-TW / zh-HK）→ zh-CN；ja* → ja；其余（含空）→ en。
func MapOSLocale(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return LangEn
	}
	// 常见形式：zh_CN.UTF-8、zh-TW、ja_JP、en_US.UTF-8、C、POSIX
	s = strings.Split(s, ".")[0] // 去掉 .UTF-8
	s = strings.Split(s, "@")[0] // 去掉 @euro 等
	s = strings.ReplaceAll(s, "_", "-")
	lower := strings.ToLower(s)
	if lower == "c" || lower == "posix" {
		return LangEn
	}
	// 取主语言标签
	primary := lower
	if i := strings.IndexByte(lower, '-'); i >= 0 {
		primary = lower[:i]
	}
	switch primary {
	case "zh":
		return LangZhCN
	case "ja":
		return LangJa
	default:
		return LangEn
	}
}

// Effective 根据设置值与系统映射结果算实际生效语言。
func Effective(setting, systemMapped string) string {
	if setting == LangSystem || setting == "" {
		if IsLang(systemMapped) {
			return systemMapped
		}
		return LangEn
	}
	if IsLang(setting) {
		return setting
	}
	return LangEn
}
