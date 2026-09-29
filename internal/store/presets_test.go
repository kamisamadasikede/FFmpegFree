package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestPresetsSeedSaveDelete(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	seed := []PresetRow{{ID: "b1", Name: "内置1", Options: `{"container":"mp4"}`, Sort: 1}, {ID: "b2", Name: "内置2", Options: `{}`, Sort: 2}}
	if err := s.SeedPresets(ctx, seed); err != nil {
		t.Fatal(err)
	}
	// 再次 seed 更新内置（升级调整名称），不重复
	seed[0].Name = "内置1改"
	if err := s.SeedPresets(ctx, seed); err != nil {
		t.Fatal(err)
	}
	u, err := s.SaveUserPreset(ctx, PresetRow{ID: "u1", Name: "我的", Options: `{"container":"mkv"}`}, true)
	if err != nil || u.BuiltIn || u.Sort != 1 {
		t.Fatalf("%+v %v", u, err)
	}
	list, _ := s.ListPresets(ctx)
	if len(list) != 3 || list[0].ID != "b1" || list[0].Name != "内置1改" || !list[0].BuiltIn || list[2].ID != "u1" {
		t.Fatalf("内置在前且已更新: %+v", list)
	}
	// 更新用户预设
	if _, err := s.SaveUserPreset(ctx, PresetRow{ID: "u1", Name: "改名", Options: `{}`}, false); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.GetPreset(ctx, "u1"); r.Name != "改名" || r.Options != "{}" {
		t.Fatalf("%+v", r)
	}
	// 内置不可改删
	if _, err := s.SaveUserPreset(ctx, PresetRow{ID: "b1", Name: "x", Options: `{}`}, false); !errors.Is(err, ErrPresetBuiltIn) {
		t.Fatalf("%v", err)
	}
	if err := s.DeleteUserPreset(ctx, "b1"); !errors.Is(err, ErrPresetBuiltIn) {
		t.Fatalf("%v", err)
	}
	// 不存在
	if _, err := s.SaveUserPreset(ctx, PresetRow{ID: "zz", Name: "x", Options: `{}`}, false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("%v", err)
	}
	if err := s.DeleteUserPreset(ctx, "zz"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("%v", err)
	}
	if err := s.DeleteUserPreset(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountUserPresets(ctx); n != 0 {
		t.Fatal(n)
	}
	// 同 id 的用户预设不被 seed 覆盖
	s.SaveUserPreset(ctx, PresetRow{ID: "b3", Name: "用户占用", Options: `{}`}, true)
	s.SeedPresets(ctx, []PresetRow{{ID: "b3", Name: "内置3", Options: `{"x":1}`}})
	if r, _ := s.GetPreset(ctx, "b3"); r.Name != "用户占用" || r.BuiltIn {
		t.Fatalf("%+v", r)
	}
}
