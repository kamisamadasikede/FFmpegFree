package system

import (
	"context"
	"strings"

	"FFmpegFree/internal/apperr"
)

// SettingAsrTier 是语音识别档位（契约 6.18.5）：standard | hd，默认 standard。
const SettingAsrTier = "asrTier"

const (
	AsrTierStandard = "standard"
	AsrTierHD       = "hd"
)

func normalizeAsrTier(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "", AsrTierStandard:
		return AsrTierStandard
	case AsrTierHD:
		return AsrTierHD
	default:
		return ""
	}
}

// AsrTier 返回设置里的识别档位（默认 standard）。
func (m *Manager) AsrTier(ctx context.Context) string {
	m.mu.Lock()
	st := m.cfg.Settings
	m.mu.Unlock()
	if st == nil {
		m.memMu.Lock()
		defer m.memMu.Unlock()
		if m.memAsrTier == AsrTierHD {
			return AsrTierHD
		}
		return AsrTierStandard
	}
	var v string
	if _, err := st.GetSetting(ctx, SettingAsrTier, &v); err != nil {
		return AsrTierStandard
	}
	if n := normalizeAsrTier(v); n != "" {
		return n
	}
	return AsrTierStandard
}

func (m *Manager) setAsrTier(ctx context.Context, v string) error {
	m.mu.Lock()
	st := m.cfg.Settings
	hook := m.onAsrTier
	m.mu.Unlock()
	if st == nil {
		m.memMu.Lock()
		m.memAsrTier = v
		m.memMu.Unlock()
		if hook != nil {
			hook(v)
		}
		return nil
	}
	if err := st.SetSetting(ctx, SettingAsrTier, v); err != nil {
		return err
	}
	if hook != nil {
		hook(v)
	}
	return nil
}

// SetAsrTierHook 在 asrTier 变更时回调（刷新引导体积，不自动下载）。
func (m *Manager) SetAsrTierHook(fn func(string)) {
	m.mu.Lock()
	m.onAsrTier = fn
	m.mu.Unlock()
}

// validateAsrTier 供 UpdateSettings：非法返回错误。
func validateAsrTier(v string) (string, error) {
	n := normalizeAsrTier(v)
	if n == "" {
		return "", apperr.New(apperr.InvalidArgument, "识别档位不正确").WithDetail("asrTier=" + v)
	}
	return n, nil
}
