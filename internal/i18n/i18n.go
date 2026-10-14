// Package i18n 提供后端只能自己产生的文案翻译（契约 6.20.3）。
// 前端能按 code/reason 映射的错误 message 仍保持中文兜底，不走这里。
package i18n

import (
	"strings"
	"sync"
)

var catalogs = map[string]map[string]string{
	LangZhCN: catalogZhCN,
	LangEn:   catalogEn,
	LangJa:   catalogJa,
}

var (
	mu      sync.RWMutex
	current = LangZhCN
)

// Supported 返回除 system 外可持久化的语言码。
func Supported() []string { return []string{LangZhCN, LangEn, LangJa} }

// IsLang 判断是否为有效的生效语言（不含 system）。
func IsLang(lang string) bool {
	_, ok := catalogs[lang]
	return ok
}

// IsSetting 判断是否为合法的设置值：system | zh-CN | en | ja。
func IsSetting(lang string) bool {
	return lang == LangSystem || IsLang(lang)
}

// Current 返回当前生效语言。
func Current() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Set 设置当前生效语言；非法值忽略并保持原值。线程安全。
func Set(lang string) {
	if !IsLang(lang) {
		return
	}
	mu.Lock()
	current = lang
	mu.Unlock()
}

// T 按当前语言取文案；缺 key 回退 zh-CN；再用 params 做 {name} 插值。
func T(key string, params map[string]string) string {
	mu.RLock()
	lang := current
	mu.RUnlock()
	return Translate(lang, key, params)
}

// Translate 按指定语言取文案（测试 / 离线渲染用）；缺 key 回退 zh-CN。
func Translate(lang, key string, params map[string]string) string {
	s := lookup(lang, key)
	if s == "" {
		s = lookup(LangZhCN, key)
	}
	if s == "" {
		return key
	}
	return interpolate(s, params)
}

func lookup(lang, key string) string {
	if c, ok := catalogs[lang]; ok {
		return c[key]
	}
	return ""
}

func interpolate(s string, params map[string]string) string {
	if len(params) == 0 {
		return s
	}
	for k, v := range params {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// CatalogKeys 返回某语言目录的全部 key（测试用）。
func CatalogKeys(lang string) []string {
	c, ok := catalogs[lang]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(c))
	for k := range c {
		out = append(out, k)
	}
	return out
}
