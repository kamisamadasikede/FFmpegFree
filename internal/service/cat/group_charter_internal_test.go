package cat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/store"
)

func TestAddCatGroupMember_CallsGenerateCharterFull(t *testing.T) {
	// Source guard: first charter gen must be full=true (§6.21.4).
	b, err := os.ReadFile("groups.go")
	if err != nil {
		b, err = os.ReadFile(filepath.Join("internal", "service", "cat", "groups.go"))
	}
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "s.generateCharter(context.Background(), m.ID, true)") {
		t.Fatal("AddCatGroupMember must call generateCharter(..., true)")
	}
	if strings.Contains(s, "s.generateCharter(context.Background(), m.ID, false)") {
		t.Fatal("AddCatGroupMember must not call generateCharter(..., false)")
	}
}

func TestFailCharter_StoresFailReason(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	proj, _, err := st.InsertOrGetCatProject(ctx, store.CatProjectRow{Name: "p", Path: dir, PathKey: "k-charter"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := st.InsertCatGroup(ctx, store.CatGroup{ProjectID: proj.ID, Name: "群"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.InsertCatGroupMember(ctx, store.CatGroupMember{
		GroupID: g.ID, Name: "甲", Title: "工程师", AgentKind: "cat_build",
		Charter: store.CatCharter{Status: "generating", Dirs: []string{}, Edited: []string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(Config{Store: st, Emit: func(string, any) {}})
	svc.failCharter(ctx, m, true, "CLI 返回空正文（结构化输出未解出）")
	got, err := st.GetCatGroupMember(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Charter.Status != "failed" {
		t.Fatalf("status=%q", got.Charter.Status)
	}
	if got.Charter.FailReason != "CLI 返回空正文（结构化输出未解出）" {
		t.Fatalf("failReason=%q", got.Charter.FailReason)
	}
}

func TestCharterFailReason_Timeout(t *testing.T) {
	r := charterFailReason(context.DeadlineExceeded, "")
	if r != "职责生成超时" {
		t.Fatalf("got %q", r)
	}
}

func TestCharterFailReason_EmptyBody(t *testing.T) {
	r := charterFailReason(nil, "")
	if r != "CLI 返回空正文（结构化输出未解出）" {
		t.Fatalf("got %q", r)
	}
}
