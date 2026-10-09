package system

import (
	"context"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
)

// SettingCatDefaultAgentKind 是新建 Cat 会话预填的 agentKind（契约 6.19.5），默认 cat_build。
const SettingCatDefaultAgentKind = "catDefaultAgentKind"

func normalizeCatDefaultAgentKind(v string) string {
	v = strings.TrimSpace(v)
	switch v {
	case "", catagent.KindCatBuild:
		return catagent.KindCatBuild
	default:
		// 一期只允许 cat_build；其它值视为非法（二期再放开）。
		return ""
	}
}

// CatDefaultAgentKind 返回新建会话预填 kind。
func (m *Manager) CatDefaultAgentKind(ctx context.Context) string {
	m.mu.Lock()
	st := m.cfg.Settings
	m.mu.Unlock()
	if st == nil {
		m.memMu.Lock()
		defer m.memMu.Unlock()
		if m.memCatDefault != "" {
			return m.memCatDefault
		}
		return catagent.KindCatBuild
	}
	var v string
	if _, err := st.GetSetting(ctx, SettingCatDefaultAgentKind, &v); err != nil {
		return catagent.KindCatBuild
	}
	if n := normalizeCatDefaultAgentKind(v); n != "" {
		return n
	}
	return catagent.KindCatBuild
}

func (m *Manager) setCatDefaultAgentKind(ctx context.Context, v string) error {
	m.mu.Lock()
	st := m.cfg.Settings
	m.mu.Unlock()
	if st == nil {
		m.memMu.Lock()
		m.memCatDefault = v
		m.memMu.Unlock()
		return nil
	}
	return st.SetSetting(ctx, SettingCatDefaultAgentKind, v)
}

func validateCatDefaultAgentKind(v string) (string, error) {
	n := normalizeCatDefaultAgentKind(v)
	if n == "" {
		return "", apperr.New(apperr.InvalidArgument, "助手模式设置不正确")
	}
	return n, nil
}
