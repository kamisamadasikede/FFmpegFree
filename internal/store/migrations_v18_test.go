package store

import (
	"context"
	"path/filepath"
	"testing"
)

// v0.55 群聊迁移改号为 0018（老板的 0016_teams / 0017_team_artifacts 原样保留，0019 留给 v0.55.6）。
func TestMigrationsFreshAndFromTeams17(t *testing.T) {
	ctx := context.Background()
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err) // 版本号重复会在这里报错
	}
	want := map[int]string{16: "0016_teams.sql", 17: "0017_team_artifacts.sql", 18: "0018_cat_groups.sql"}
	got := map[int]string{}
	for _, m := range ms {
		got[m.version] = m.name
	}
	for v, n := range want {
		if got[v] != n {
			t.Fatalf("version %d = %q, want %q", v, got[v], n)
		}
	}
	p := filepath.Join(t.TempDir(), "app.db")
	s, err := Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	hasTable := func(name string) bool {
		var n int
		_ = s.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
		return n == 1
	}
	if v, _ := s.SchemaVersion(ctx); v != ms[len(ms)-1].version {
		t.Fatalf("fresh version %d", v)
	}
	for _, tb := range []string{"team_rooms", "team_runs", "cat_groups", "cat_group_members", "cat_group_messages", "cat_group_routes", "cat_group_turn_meta"} {
		if !hasTable(tb) {
			t.Fatalf("missing %s", tb)
		}
	}
	// 老板本机库：已跑到 17、没有群聊表 → 再打开只补 18。
	for _, tb := range []string{"cat_group_workflows", "cat_group_turn_meta", "cat_group_routes", "cat_group_messages", "cat_group_members", "cat_groups"} {
		if _, err := s.DB().Exec(`DROP TABLE ` + tb); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB().Exec(`DELETE FROM schema_migrations WHERE version >= 18`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !hasTable("cat_groups") || !hasTable("team_rooms") {
		t.Fatal("upgrade from 17 did not add group tables")
	}
}
