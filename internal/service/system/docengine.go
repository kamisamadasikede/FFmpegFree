package system

import (
	"context"
	"strings"
)

func normalizeDocEngine(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "", "auto":
		return "auto"
	case "office", "wps", "component":
		return v
	default:
		return "" // invalid
	}
}

// DocEngine 返回设置里的文档引擎（空 / 无效按 auto）。
func (m *Manager) DocEngine(ctx context.Context) string {
	m.mu.Lock()
	st := m.cfg.Settings
	m.mu.Unlock()
	if st == nil {
		return "auto"
	}
	var v string
	if _, err := st.GetSetting(ctx, SettingDocEngine, &v); err != nil {
		return "auto"
	}
	if n := normalizeDocEngine(v); n != "" {
		return n
	}
	return "auto"
}

func (m *Manager) setDocEngine(ctx context.Context, v string) error {
	m.mu.Lock()
	st := m.cfg.Settings
	m.mu.Unlock()
	if st == nil {
		return nil
	}
	if v == "" {
		v = "auto"
	}
	return st.SetSetting(ctx, SettingDocEngine, v)
}

// SetDocEngineHook 在 docEngine 变更时回调（发 doc:component）。
func (m *Manager) SetDocEngineHook(fn func(string)) {
	m.mu.Lock()
	m.onDocEngine = fn
	m.mu.Unlock()
}
