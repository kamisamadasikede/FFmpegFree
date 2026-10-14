package main

import (
	"context"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/i18n"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SettingAppLanguage 是应用语言设置键（契约 6.20.3），默认 system。
const SettingAppLanguage = "app.language"

// EventAppLanguage 是语言变更事件名（payload = AppLanguage）。
const EventAppLanguage = "app:language"

// AppLanguage 是 Get/SetAppLanguage 的返回（契约 6.20.3）。
type AppLanguage struct {
	Lang      string `json:"lang"`      // 设置值：system | zh-CN | en | ja
	System    string `json:"system"`    // OS 映射结果：zh-CN | en | ja
	Effective string `json:"effective"` // 实际生效
}

// SetAppLanguageRequest 是 SetAppLanguage 的入参。
type SetAppLanguageRequest struct {
	Lang string `json:"lang"`
}

// GetAppLanguage 返回当前语言设置、系统映射与生效语言。
func (a *App) GetAppLanguage() (AppLanguage, error) {
	return a.readAppLanguage(context.Background()), nil
}

// SetAppLanguage 保存语言设置并立即生效；非法值返回 INVALID_ARGUMENT reason=bad_language。
func (a *App) SetAppLanguage(req SetAppLanguageRequest) (AppLanguage, error) {
	lang := strings.TrimSpace(req.Lang)
	if !i18n.IsSetting(lang) {
		return AppLanguage{}, apperr.New(apperr.InvalidArgument, "语言设置不正确").
			WithDetail("reason=bad_language").
			WithParams(map[string]string{"lang": lang})
	}
	ctx := context.Background()
	if a.store != nil {
		if err := a.store.SetSetting(ctx, SettingAppLanguage, lang); err != nil {
			return AppLanguage{}, apperr.Wrap(apperr.IOError, "保存语言设置失败", err)
		}
	} else {
		a.memLang = lang
	}
	out := a.applyLanguage(lang)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, EventAppLanguage, out)
	}
	return out, nil
}

// loadAppLanguage 在启动时从设置读出并应用到 i18n（默认 system）。
func (a *App) loadAppLanguage(ctx context.Context) {
	setting := i18n.LangSystem
	if a.store != nil {
		var v string
		if found, err := a.store.GetSetting(ctx, SettingAppLanguage, &v); err == nil && found {
			if i18n.IsSetting(v) {
				setting = v
			}
		}
	} else if a.memLang != "" && i18n.IsSetting(a.memLang) {
		setting = a.memLang
	}
	a.applyLanguage(setting)
}

func (a *App) readAppLanguage(ctx context.Context) AppLanguage {
	setting := i18n.LangSystem
	if a.store != nil {
		var v string
		if found, err := a.store.GetSetting(ctx, SettingAppLanguage, &v); err == nil && found && i18n.IsSetting(v) {
			setting = v
		}
	} else if a.memLang != "" && i18n.IsSetting(a.memLang) {
		setting = a.memLang
	}
	sys := i18n.DetectSystemLang()
	return AppLanguage{
		Lang:      setting,
		System:    sys,
		Effective: i18n.Effective(setting, sys),
	}
}

func (a *App) applyLanguage(setting string) AppLanguage {
	sys := i18n.DetectSystemLang()
	eff := i18n.Effective(setting, sys)
	i18n.Set(eff)
	return AppLanguage{Lang: setting, System: sys, Effective: eff}
}
