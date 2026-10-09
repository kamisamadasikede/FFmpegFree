package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func openCatTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "app.db")
	st, err := Open(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, p
}

func TestCatProjectsMigrationIdempotentAndOldConversations(t *testing.T) {
	ctx := context.Background()
	st, p := openCatTestStore(t)
	old, err := st.InsertCatConversation(ctx, CatConversation{AgentKind: "cat_build", ProjectPath: "/ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st2, err := Open(ctx, p) // 再开一次：迁移不重复执行
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	var n int
	if err := st2.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = 11`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("0011 应只记录一次，得 %d", n)
	}
	c, err := st2.GetCatConversation(ctx, old.ID)
	if err != nil || c.ProjectID != "" || c.ProjectPath != "" {
		t.Fatalf("旧会话 project_id 应为空且 projectPath 不再读写: %+v %v", c, err)
	}
	var pp string
	_ = st2.db.QueryRowContext(ctx, `SELECT project_path FROM cat_conversations WHERE id = ?`, old.ID).Scan(&pp)
	if pp != "" {
		t.Fatalf("v0.31 不应写 project_path: %q", pp)
	}
}

func TestCatProjectInsertOrGetDedup(t *testing.T) {
	ctx := context.Background()
	st, _ := openCatTestStore(t)
	a, existed, err := st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "A", Path: "/x/a", PathKey: "/x/a"})
	if err != nil || existed {
		t.Fatalf("%v %v", existed, err)
	}
	b, existed, err := st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "B", Path: "/x/a/", PathKey: "/x/a"})
	if err != nil || !existed || b.ID != a.ID || b.Name != "A" {
		t.Fatalf("重复 path_key 应返回已有行且不改名: %+v %v %v", b, existed, err)
	}
	if _, err := st.GetCatProject(ctx, "nope"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
}

func TestCatProjectListOrder(t *testing.T) {
	ctx := context.Background()
	st, _ := openCatTestStore(t)
	p1, _, _ := st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "1", Path: "/1", PathKey: "/1", CreatedAt: 100})
	p2, _, _ := st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "2", Path: "/2", PathKey: "/2", CreatedAt: 200})
	p3, _, _ := st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "3", Path: "/3", PathKey: "/3", CreatedAt: 300})
	// p1 有一条最近活动的对话 → 排第一；p3 无对话用 createdAt=300；p2 对话活动 250。
	if _, err := st.InsertCatConversation(ctx, CatConversation{AgentKind: "cat_build", ProjectID: p1.ID, CreatedAt: 400, UpdatedAt: 500}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertCatConversation(ctx, CatConversation{AgentKind: "cat_build", ProjectID: p2.ID, CreatedAt: 250, UpdatedAt: 250}); err != nil {
		t.Fatal(err)
	}
	// 改名不改变排序
	if _, err := st.RenameCatProject(ctx, p2.ID, "二"); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListCatProjects(ctx)
	if err != nil || len(list) != 3 {
		t.Fatalf("%v %v", list, err)
	}
	if list[0].ID != p1.ID || list[1].ID != p3.ID || list[2].ID != p2.ID {
		t.Fatalf("排序不对: %v", []string{list[0].Name, list[1].Name, list[2].Name})
	}
}

func TestCatProjectDeleteTransaction(t *testing.T) {
	ctx := context.Background()
	st, _ := openCatTestStore(t)
	p, _, _ := st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "p", Path: "/p", PathKey: "/p"})
	in, _ := st.InsertCatConversation(ctx, CatConversation{AgentKind: "cat_build", ProjectID: p.ID})
	out, _ := st.InsertCatConversation(ctx, CatConversation{AgentKind: "cat_build"})
	for _, c := range []string{in.ID, out.ID} {
		if _, err := st.InsertCatMessage(ctx, c, CatMessage{Role: "user", Content: "hi"}); err != nil {
			t.Fatal(err)
		}
	}
	// 中途失败整体回滚
	boom := errors.New("boom")
	if _, err := st.deleteCatProject(ctx, p.ID, func(step string) error {
		if step == "conversations" {
			return boom
		}
		return nil
	}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if _, err := st.GetCatConversation(ctx, in.ID); err != nil {
		t.Fatalf("回滚后对话应还在: %v", err)
	}
	if msgs, _ := st.ListCatMessages(ctx, in.ID); len(msgs) != 1 {
		t.Fatalf("回滚后消息应还在: %v", msgs)
	}
	deleted, err := st.DeleteCatProject(ctx, p.ID)
	if err != nil || !deleted {
		t.Fatalf("%v %v", deleted, err)
	}
	if _, err := st.GetCatConversation(ctx, in.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("项目下对话应删除: %v", err)
	}
	var n int
	_ = st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cat_messages WHERE conversation_id = ?`, in.ID).Scan(&n)
	if n != 0 {
		t.Fatalf("项目下消息应删除: %d", n)
	}
	if msgs, _ := st.ListCatMessages(ctx, out.ID); len(msgs) != 1 {
		t.Fatal("不属于项目的对话不受影响")
	}
	if deleted, err := st.DeleteCatProject(ctx, p.ID); err != nil || deleted {
		t.Fatalf("重复删除应幂等: %v %v", deleted, err)
	}
}

func TestCatProjectRelocateStore(t *testing.T) {
	ctx := context.Background()
	st, _ := openCatTestStore(t)
	a, _, _ := st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "A", Path: "/a", PathKey: "/a"})
	b, _, _ := st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "B", Path: "/b", PathKey: "/b"})
	if _, err := st.RelocateCatProject(ctx, a.ID, "/b/", "/b"); !errors.Is(err, ErrCatProjectDuplicate) {
		t.Fatal(err)
	}
	if _, err := st.RelocateCatProject(ctx, "nope", "/c", "/c"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	got, err := st.RelocateCatProject(ctx, a.ID, "/c", "/c")
	if err != nil || got.Path != "/c" || got.PathKey != "/c" || got.Name != "A" {
		t.Fatalf("%+v %v", got, err)
	}
	_ = b
}

func TestSetCatProjectPathKey(t *testing.T) {
	ctx := context.Background()
	st, _ := openCatTestStore(t)
	a, _, _ := st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "A", Path: "/A", PathKey: "/A"})
	_, _, _ = st.InsertOrGetCatProject(ctx, CatProjectRow{Name: "b", Path: "/b", PathKey: "/b"})
	if err := st.SetCatProjectPathKey(ctx, a.ID, "/b"); !errors.Is(err, ErrCatProjectDuplicate) {
		t.Fatal(err)
	}
	if err := st.SetCatProjectPathKey(ctx, "nope", "/x"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if err := st.SetCatProjectPathKey(ctx, a.ID, "/a"); err != nil {
		t.Fatal(err)
	}
	if g, _ := st.GetCatProject(ctx, a.ID); g.PathKey != "/a" || g.Path != "/A" || g.UpdatedAt != a.UpdatedAt {
		t.Fatalf("%+v", g)
	}
}
