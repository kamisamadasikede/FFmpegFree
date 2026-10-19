package store

import (
	"context"
	"path/filepath"
	"testing"
)

// v0.55.6：0019_cat_group_route_timing.sql 给 cat_group_routes 加 started_at / finished_at（契约 6.21.17 ③）。
func TestMigrationsV19RouteTiming(t *testing.T) {
	ctx := context.Background()
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]string{}
	for _, m := range ms {
		got[m.version] = m.name
	}
	if got[18] != "0018_cat_groups.sql" || got[19] != "0019_cat_group_route_timing.sql" {
		t.Fatalf("versions 18/19 = %q / %q", got[18], got[19])
	}
	p := filepath.Join(t.TempDir(), "app.db")
	s, err := Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	hasCol := func(col string) bool {
		var n int
		_ = s.DB().QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cat_group_routes') WHERE name = ?`, col).Scan(&n)
		return n == 1
	}
	if v, _ := s.SchemaVersion(ctx); v != ms[len(ms)-1].version {
		t.Fatalf("fresh version %d", v)
	}
	if !hasCol("started_at") || !hasCol("finished_at") {
		t.Fatal("fresh db missing timing columns")
	}

	// 只跑到 18 的库（#330 合入后的开发库）：表里已有旧派活记录 → 再打开补 19，旧行两列为 0。
	if _, err := s.DB().Exec(`DROP TABLE cat_group_workflows; DROP INDEX cat_group_messages_chain; ALTER TABLE cat_group_messages DROP COLUMN files_json`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`DROP TABLE cat_group_routes`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`CREATE TABLE cat_group_routes (
		route_seq INTEGER PRIMARY KEY, group_id TEXT NOT NULL, chain_id TEXT NOT NULL, mode TEXT NOT NULL,
		member_id TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '', input_text TEXT NOT NULL DEFAULT '',
		output_text TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT INTO cat_group_routes (group_id, chain_id, mode, created_at) VALUES ('g', 'c', 'ai', 5)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`DELETE FROM schema_migrations WHERE version >= 19`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !hasCol("started_at") || !hasCol("finished_at") {
		t.Fatal("upgrade from 18 did not add timing columns")
	}
	old, err := s.GetCatGroupRoute(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if old.StartedAt != 0 || old.FinishedAt != 0 || old.CreatedAt != 5 {
		t.Fatalf("old row %+v", old)
	}
	d, err := s.InsertCatGroupRoute(ctx, CatGroupRouteDetail{GroupID: "g", ChainID: "c2", Mode: "ai", StartedAt: 1000, FinishedAt: 1750})
	if err != nil {
		t.Fatal(err)
	}
	back, err := s.GetCatGroupRoute(ctx, d.RouteSeq)
	if err != nil {
		t.Fatal(err)
	}
	if back.StartedAt != 1000 || back.FinishedAt != 1750 {
		t.Fatalf("round trip %+v", back)
	}
}

func TestCatGroupMemberBriefsAndReports(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := Open(ctx, filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	proj, _, err := st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "p", Path: dir, PathKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	g1, _ := st.InsertCatGroup(ctx, CatGroup{ProjectID: proj.ID, Name: "一群", AutoRoute: true})
	g2, _ := st.InsertCatGroup(ctx, CatGroup{ProjectID: proj.ID, Name: "二群", AutoRoute: true})
	g3, _ := st.InsertCatGroup(ctx, CatGroup{ProjectID: proj.ID, Name: "空群", AutoRoute: true})
	if g3.Members == nil || len(g3.Members) != 0 {
		t.Fatalf("new group members must be [] not nil: %#v", g3.Members)
	}
	// 故意倒序插入，检查按 order 升序；order 5 → avatarIndex 1（循环）。
	for i, o := range []int{5, 1, 0} {
		if _, err := st.InsertCatGroupMember(ctx, CatGroupMember{GroupID: g1.ID, Name: "m" + string(rune('a'+i)), Title: "t", AgentKind: "claude_cli", Order: o}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.InsertCatGroupMember(ctx, CatGroupMember{GroupID: g2.ID, Name: "x", Title: "t2", AgentKind: "codex_cli", Order: 0}); err != nil {
		t.Fatal(err)
	}
	b, err := st.ListCatGroupMemberBriefs(ctx, []string{g1.ID, g2.ID, g3.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(b[g1.ID]) != 3 || len(b[g2.ID]) != 1 || b[g3.ID] == nil || len(b[g3.ID]) != 0 {
		t.Fatalf("briefs %+v", b)
	}
	wantOrder, wantAvatar := []int{0, 1, 5}, []int{1, 2, 1}
	for i, m := range b[g1.ID] {
		if m.Order != wantOrder[i] || m.AvatarIndex != wantAvatar[i] {
			t.Fatalf("brief %d = %+v", i, m)
		}
	}
	if b[g2.ID][0].AgentKind != "codex_cli" || b[g2.ID][0].Title != "t2" {
		t.Fatalf("g2 brief %+v", b[g2.ID][0])
	}
	if got, _ := st.ListCatGroupMemberBriefs(ctx, nil); len(got) != 0 {
		t.Fatal("empty ids")
	}

	// 汇报消息：只取 kind=member 且有 report 的。
	_, _ = st.InsertCatGroupMessage(ctx, CatGroupMessage{GroupID: g1.ID, Kind: "user", Text: "u", ChainID: "c"})
	_, _ = st.InsertCatGroupMessage(ctx, CatGroupMessage{GroupID: g1.ID, Kind: "member", Text: "r", ChainID: "c",
		Report: &CatGroupReport{Did: "d", Files: []CatGroupFileTag{{Path: "a.go"}}, Parsed: true}})
	_, _ = st.InsertCatGroupMessage(ctx, CatGroupMessage{GroupID: g1.ID, Kind: "system", Text: "s", ChainID: "c"})
	_, _ = st.InsertCatGroupMessage(ctx, CatGroupMessage{GroupID: g2.ID, Kind: "member", Text: "r2", ChainID: "c",
		Report: &CatGroupReport{Did: "d2", Files: []CatGroupFileTag{}, Parsed: true}})
	reps, err := st.ListCatGroupReportMessages(ctx, g1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reps) != 1 || reps[0].Report == nil || reps[0].Report.Files[0].Path != "a.go" {
		t.Fatalf("reports %+v", reps)
	}
	// 成员行默认运行时字段
	ms, _ := st.ListCatGroupMembers(ctx, g1.ID)
	if ms[0].Status != "idle" || ms[0].Queue == nil || ms[0].QueueLength != 0 {
		t.Fatalf("runtime defaults %+v", ms[0])
	}
}
