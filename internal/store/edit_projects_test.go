package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestEditProjectsCRUD(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	if err := s.SaveEditProject(ctx, EditProjectRow{ID: "a", Name: "甲", Project: `{"x":1}`, DurationSec: 3.5, ClipCount: 2, UpdatedAt: 100}, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveEditProject(ctx, EditProjectRow{ID: "b", Name: "乙", Project: `{}`, UpdatedAt: 200}, true); err != nil {
		t.Fatal(err)
	}
	// 主键冲突
	if err := s.SaveEditProject(ctx, EditProjectRow{ID: "a", Name: "x", Project: `{}`}, true); err == nil {
		t.Fatal("重复 id 应失败")
	}
	// 更新
	if err := s.SaveEditProject(ctx, EditProjectRow{ID: "a", Name: "甲2", Project: `{"x":2}`, DurationSec: 9, ClipCount: 4, UpdatedAt: 300}, false); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetEditProject(ctx, "a")
	if err != nil || got.Name != "甲2" || got.Project != `{"x":2}` || got.DurationSec != 9 || got.ClipCount != 4 || got.UpdatedAt != 300 {
		t.Fatalf("%+v %v", got, err)
	}
	// 更新不存在
	if err := s.SaveEditProject(ctx, EditProjectRow{ID: "zz"}, false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("%v", err)
	}
	if _, err := s.GetEditProject(ctx, "zz"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("%v", err)
	}
	// 列表：updated_at 倒序
	list, err := s.ListEditProjects(ctx, 0)
	if err != nil || len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("%+v %v", list, err)
	}
	list, _ = s.ListEditProjects(ctx, 1)
	if len(list) != 1 {
		t.Fatal("limit")
	}
	if err := s.DeleteEditProject(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEditProject(ctx, "a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("%v", err)
	}
	list, _ = s.ListEditProjects(ctx, 10)
	if len(list) != 1 || list[0].ID != "b" {
		t.Fatalf("%+v", list)
	}
}

func TestEditProjectsLimitClamp(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	for i := 0; i < 205; i++ {
		id := string(rune('A'+i/26)) + string(rune('a'+i%26))
		if err := s.SaveEditProject(ctx, EditProjectRow{ID: id, Name: id, Project: `{}`, UpdatedAt: int64(i)}, true); err != nil {
			t.Fatal(err)
		}
	}
	if l, _ := s.ListEditProjects(ctx, 0); len(l) != 50 {
		t.Fatalf("默认 50，实际 %d", len(l))
	}
	if l, _ := s.ListEditProjects(ctx, 1000); len(l) != 200 {
		t.Fatalf("最大 200，实际 %d", len(l))
	}
	if l, _ := s.ListEditProjects(ctx, -5); len(l) != 50 {
		t.Fatalf("负数取默认，实际 %d", len(l))
	}
}

// 迁移 0002 给旧库（只有 0001）加列，已有数据保留。
func TestMigration0002UpgradesExistingDB(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+p)
	if err != nil {
		t.Fatal(err)
	}
	ms, _ := loadMigrations()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ms[0].sql); err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO schema_migrations VALUES (1, 1)`)
	if _, err := db.Exec(`INSERT INTO edit_projects (id,name,project,updated_at) VALUES ('old','旧','{}',5)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, _ := s.SchemaVersion(ctx)
	if v < 2 {
		t.Fatalf("version=%d", v)
	}
	r, err := s.GetEditProject(ctx, "old")
	if err != nil || r.Name != "旧" || r.ClipCount != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}
