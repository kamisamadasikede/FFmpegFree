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
