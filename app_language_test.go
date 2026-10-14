package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/i18n"
	"FFmpegFree/internal/store"
)

func TestGetSetAppLanguagePersistence(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{store: st}
	a.loadAppLanguage(context.Background())

	got, err := a.GetAppLanguage()
	if err != nil {
		t.Fatal(err)
	}
	if got.Lang != i18n.LangSystem {
		t.Fatalf("default lang=%q", got.Lang)
	}
	if !i18n.IsLang(got.System) || got.Effective != i18n.Effective(got.Lang, got.System) {
		t.Fatalf("got %+v", got)
	}

	out, err := a.SetAppLanguage(SetAppLanguageRequest{Lang: i18n.LangEn})
	if err != nil {
		t.Fatal(err)
	}
	if out.Lang != i18n.LangEn || out.Effective != i18n.LangEn {
		t.Fatalf("set en: %+v", out)
	}
	if i18n.Current() != i18n.LangEn {
		t.Fatalf("i18n.Current=%q", i18n.Current())
	}

	// 持久化
	var saved string
	found, err := st.GetSetting(context.Background(), SettingAppLanguage, &saved)
	if err != nil || !found || saved != i18n.LangEn {
		t.Fatalf("saved=%q found=%v err=%v", saved, found, err)
	}

	// 重新 load
	a2 := &App{store: st}
	a2.loadAppLanguage(context.Background())
	got2, _ := a2.GetAppLanguage()
	if got2.Lang != i18n.LangEn || i18n.Current() != i18n.LangEn {
		t.Fatalf("reload: %+v current=%q", got2, i18n.Current())
	}

	// system
	out, err = a.SetAppLanguage(SetAppLanguageRequest{Lang: i18n.LangSystem})
	if err != nil || out.Lang != i18n.LangSystem {
		t.Fatalf("system: %+v %v", out, err)
	}
	if out.Effective != out.System {
		t.Fatalf("system effective should equal system: %+v", out)
	}

	// 恢复默认，避免污染其它包测试
	i18n.Set(i18n.LangZhCN)
}

func TestSetAppLanguageRejectsInvalid(t *testing.T) {
	a := &App{}
	_, err := a.SetAppLanguage(SetAppLanguageRequest{Lang: "fr"})
	if !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("want INVALID_ARGUMENT, got %v", err)
	}
	ae := apperr.From(err)
	if ae.Detail != "reason=bad_language" {
		t.Fatalf("detail=%q", ae.Detail)
	}
	if ae.Params["lang"] != "fr" {
		t.Fatalf("params=%v", ae.Params)
	}
	raw := ae.Error()
	var m map[string]any
	if e := json.Unmarshal([]byte(raw), &m); e != nil {
		t.Fatal(e)
	}
	if m["params"].(map[string]any)["lang"] != "fr" {
		t.Fatalf("json params: %v", m)
	}
	i18n.Set(i18n.LangZhCN)
}

func TestDialogTitlesFollowLanguage(t *testing.T) {
	i18n.Set(i18n.LangZhCN)
	zh := i18n.T(i18n.KeyDialogPickFiles, nil)
	if zh != "选择文件" {
		t.Fatalf("zh=%q", zh)
	}
	i18n.Set(i18n.LangEn)
	en := i18n.T(i18n.KeyDialogPickFiles, nil)
	if en != "Select Files" {
		t.Fatalf("en=%q", en)
	}
	i18n.Set(i18n.LangJa)
	ja := i18n.T(i18n.KeyDialogConfirmReplaceTitle, nil)
	if ja != "置き換えの確認" {
		t.Fatalf("ja=%q", ja)
	}
	i18n.Set(i18n.LangZhCN)
}

func TestNotifyTitlesFollowLanguage(t *testing.T) {
	i18n.Set(i18n.LangEn)
	defer i18n.Set(i18n.LangZhCN)
	title := i18n.T(i18n.KeyNotifyScheduleDoneTitle, map[string]string{"name": "Daily"})
	if title != "Scheduled task completed: Daily" {
		t.Fatalf("%q", title)
	}
}
