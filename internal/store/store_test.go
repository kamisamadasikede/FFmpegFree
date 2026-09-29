package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "app.db")
	s, err := Open(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}

func TestOpenAppliesMigrationsOnce(t *testing.T) {
	ctx := context.Background()
	s, p := openTemp(t)
	v, err := s.SchemaVersion(ctx)
	if err != nil || v != 2 {
		t.Fatalf("期望迁移版本 2，实际 %d, err=%v", v, err)
	}
	for _, table := range []string{"media", "tasks", "presets", "edit_projects", "settings", "doc_recent"} {
		var n int
		if err := s.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil || n != 1 {
			t.Fatalf("缺少表 %s", table)
		}
	}
	s.Close()

	// 再次打开不应重复执行迁移。
	s2, err := Open(ctx, p)
	if err != nil {
		t.Fatalf("重复打开失败: %v", err)
	}
	defer s2.Close()
	var rows int
	s2.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&rows)
	if rows != 2 {
		t.Fatalf("schema_migrations 应只有 2 行（0001、0002），实际 %d", rows)
	}
}

func TestWALEnabled(t *testing.T) {
	s, _ := openTemp(t)
	var mode string
	if err := s.DB().QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("期望 WAL 模式，实际 %s", mode)
	}
}

func TestMediaPathKeyUnique(t *testing.T) {
	s, _ := openTemp(t)
	ins := `INSERT INTO media (id, path, path_key) VALUES (?, ?, ?)`
	if _, err := s.DB().Exec(ins, "1", `C:\Videos\A.mp4`, `c:\videos\a.mp4`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(ins, "2", `C:\videos\a.MP4`, `c:\videos\a.mp4`); err == nil {
		t.Fatal("相同 path_key 应被唯一约束拦下")
	}
}

func TestMarkInterrupted(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	ins := `INSERT INTO tasks (id, type, status, created_at) VALUES (?, 'convert', ?, 1)`
	for id, st := range map[string]string{"q": "queued", "r": "running", "ok": "succeeded", "f": "failed"} {
		if _, err := s.DB().Exec(ins, id, st); err != nil {
			t.Fatal(err)
		}
	}
	now := time.UnixMilli(1_700_000_000_000)
	n, err := s.MarkInterrupted(ctx, now)
	if err != nil || n != 2 {
		t.Fatalf("期望标记 2 个任务，实际 %d, err=%v", n, err)
	}
	check := func(id, wantStatus string, wantVersion int) {
		var st string
		var ver int
		s.DB().QueryRow(`SELECT status, version FROM tasks WHERE id=?`, id).Scan(&st, &ver)
		if st != wantStatus || ver != wantVersion {
			t.Fatalf("任务 %s: 期望 %s/v%d，实际 %s/v%d", id, wantStatus, wantVersion, st, ver)
		}
	}
	check("q", "interrupted", 2)
	check("r", "interrupted", 2)
	check("ok", "succeeded", 1)
	check("f", "failed", 1)
}
