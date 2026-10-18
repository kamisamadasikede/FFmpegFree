package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCatGroupsCRUD(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(context.Background(), filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	proj, _, err := st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "p", Path: dir, PathKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := st.InsertCatGroup(ctx, CatGroup{ProjectID: proj.ID, Name: "开发群", AutoRoute: true})
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.InsertCatGroupMember(ctx, CatGroupMember{
		GroupID: g.ID, Name: "后端工程师", Title: "后端", AgentKind: "claude_cli",
		Charter: CatCharter{Status: "generating", Dirs: []string{}, Edited: []string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertCatGroupMember(ctx, CatGroupMember{
		GroupID: g.ID, Name: "后端工程师", Title: "后端", AgentKind: "claude_cli",
		Charter: CatCharter{Status: "none"},
	}); !IsMemberNameTaken(err) {
		t.Fatalf("expected name taken, got %v", err)
	}
	msg, err := st.InsertCatGroupMessage(ctx, CatGroupMessage{
		GroupID: g.ID, Kind: "user", Text: "做登录", ChainID: "c1", Mentions: []string{m.ID},
	})
	if err != nil || msg.Seq != 1 {
		t.Fatalf("%+v %v", msg, err)
	}
	list, err := st.ListCatGroupMessages(ctx, g.ID, 0, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("%v %v", list, err)
	}
	members, err := st.ListCatGroupMembers(ctx, g.ID)
	if err != nil || len(members) != 1 {
		t.Fatal(err)
	}
}

func TestUpdateCatGroupMessage_ReplaceLoading(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(context.Background(), filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	proj, _, err := st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "p", Path: dir, PathKey: "k2"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := st.InsertCatGroup(ctx, CatGroup{ProjectID: proj.ID, Name: "群", AutoRoute: true})
	if err != nil {
		t.Fatal(err)
	}
	loading, err := st.InsertCatGroupMessage(ctx, CatGroupMessage{
		GroupID: g.ID, Kind: "system", Text: "正在安排谁来接…", ChainID: "c1",
		SystemKind: "route", Route: &CatRouteInfo{Mode: "ai"},
	})
	if err != nil {
		t.Fatal(err)
	}
	route := &CatRouteInfo{Mode: "ai", MemberID: "m1", MemberIDs: []string{"m1", "m2"}, Everyone: false, Reason: "一起介绍"}
	upd := loading
	upd.Text = "一起介绍 @前端 @后端"
	upd.SystemKind = "coord"
	upd.Mentions = []string{"m1", "m2"}
	upd.Route = route
	got, err := st.UpdateCatGroupMessage(ctx, upd)
	if err != nil {
		t.Fatal(err)
	}
	if got.Seq != loading.Seq || got.SystemKind != "coord" || got.Text != "一起介绍 @前端 @后端" {
		t.Fatalf("replace: %+v", got)
	}
	listed, err := st.ListCatGroupMessages(ctx, g.ID, 0, 10)
	if err != nil || len(listed) != 1 {
		t.Fatalf("want 1 msg after replace, got %d err=%v", len(listed), err)
	}
	if listed[0].SystemKind != "coord" || len(listed[0].Mentions) != 2 || listed[0].Route == nil || !equalStr(listed[0].Route.MemberIDs, []string{"m1", "m2"}) {
		t.Fatalf("listed=%+v route=%+v", listed[0], listed[0].Route)
	}
}

func equalStr(a, b []string) bool {
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
