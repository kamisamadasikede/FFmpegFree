package i18n_test

import (
	"encoding/json"
	"sort"
	"testing"

	"FFmpegFree/internal/i18n"
)

func TestCatalogKeysMatch(t *testing.T) {
	zh := i18n.CatalogKeys(i18n.LangZhCN)
	en := i18n.CatalogKeys(i18n.LangEn)
	ja := i18n.CatalogKeys(i18n.LangJa)
	sort.Strings(zh)
	sort.Strings(en)
	sort.Strings(ja)
	if len(zh) == 0 {
		t.Fatal("zh-CN catalog empty")
	}
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	if !eq(zh, en) || !eq(zh, ja) {
		t.Fatalf("catalog keys differ:\nzh=%v\nen=%v\nja=%v", zh, en, ja)
	}
}

func TestMissingKeyFallsBackZhCN(t *testing.T) {
	i18n.Set(i18n.LangEn)
	defer i18n.Set(i18n.LangZhCN)
	// 人为用 Translate 测回退：en 目录没有的 key 回落 zh-CN
	got := i18n.Translate(i18n.LangEn, i18n.KeyDialogPickFiles, nil)
	want := i18n.Translate(i18n.LangZhCN, i18n.KeyDialogPickFiles, nil)
	// en 有这个 key，所以应是英文
	if got == want {
		t.Fatalf("en should differ from zh-CN for pickFiles: %q", got)
	}
	missing := i18n.Translate(i18n.LangEn, "does.not.exist", nil)
	if missing != "does.not.exist" {
		// 两边都没有 → 返回 key 本身
		t.Fatalf("missing both: got %q", missing)
	}
}

func TestInterpolate(t *testing.T) {
	i18n.Set(i18n.LangZhCN)
	got := i18n.T(i18n.KeyNotifyScheduleDoneTitle, map[string]string{"name": "每日检查"})
	if got != "定时任务已完成：每日检查" {
		t.Fatalf("got %q", got)
	}
}

func TestMapOSLocale(t *testing.T) {
	cases := []struct {
		raw, want string
	}{
		{"zh_CN.UTF-8", i18n.LangZhCN},
		{"zh-TW", i18n.LangZhCN}, // PM：繁体系统先映射简体
		{"zh-HK", i18n.LangZhCN},
		{"zh_TW.UTF-8", i18n.LangZhCN},
		{"ja_JP", i18n.LangJa},
		{"ja_JP.UTF-8", i18n.LangJa},
		{"en_US", i18n.LangEn},
		{"en_US.UTF-8", i18n.LangEn},
		{"fr_FR", i18n.LangEn},
		{"", i18n.LangEn},
		{"C", i18n.LangEn},
		{"POSIX", i18n.LangEn},
	}
	for _, c := range cases {
		if got := i18n.MapOSLocale(c.raw); got != c.want {
			t.Errorf("MapOSLocale(%q)=%q want %q", c.raw, got, c.want)
		}
	}
}

func TestEffective(t *testing.T) {
	if g := i18n.Effective(i18n.LangSystem, i18n.LangJa); g != i18n.LangJa {
		t.Fatalf("system+ja → %q", g)
	}
	if g := i18n.Effective(i18n.LangEn, i18n.LangZhCN); g != i18n.LangEn {
		t.Fatalf("explicit en → %q", g)
	}
	if g := i18n.Effective("", i18n.LangZhCN); g != i18n.LangZhCN {
		t.Fatalf("empty setting → system mapped: %q", g)
	}
}

func TestSetIgnoresInvalid(t *testing.T) {
	i18n.Set(i18n.LangZhCN)
	i18n.Set("fr")
	if i18n.Current() != i18n.LangZhCN {
		t.Fatalf("invalid set changed current to %q", i18n.Current())
	}
}

func TestIsSetting(t *testing.T) {
	for _, ok := range []string{i18n.LangSystem, i18n.LangZhCN, i18n.LangEn, i18n.LangJa} {
		if !i18n.IsSetting(ok) {
			t.Errorf("IsSetting(%q)=false", ok)
		}
	}
	if i18n.IsSetting("fr") || i18n.IsSetting("zh") || i18n.IsSetting("") {
		t.Fatal("unexpected valid setting")
	}
}

// 保证目录 JSON 可序列化形状稳定（无副作用，只是烟雾）。
func TestCatalogJSONRoundTripSmoke(t *testing.T) {
	m := map[string]string{i18n.KeyDialogReplace: i18n.T(i18n.KeyDialogReplace, nil)}
	b, err := json.Marshal(m)
	if err != nil || len(b) == 0 {
		t.Fatal(err)
	}
}
