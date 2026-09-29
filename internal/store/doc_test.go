package store

import (
	"context"
	"fmt"
	"testing"
)

func TestDocRecentUpsertListDelete(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	a, err := s.UpsertDocRecent(ctx, "/a/1.pdf", DocRecent{ID: "D1", Path: "/A/1.pdf", Name: "1.pdf", Size: 10, OpenedAt: 100})
	if err != nil || a.ID != "D1" {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := s.UpsertDocRecent(ctx, "/a/2.pdf", DocRecent{ID: "D2", Path: "/a/2.pdf", Name: "2.pdf", OpenedAt: 200}); err != nil {
		t.Fatal(err)
	}
	b, err := s.UpsertDocRecent(ctx, "/a/1.pdf", DocRecent{ID: "D9", Path: "/a/1.pdf", Name: "1.pdf", Size: 20, OpenedAt: 300})
	if err != nil || b.ID != "D1" {
		t.Fatalf("应保留原 id: %+v %v", b, err)
	}
	list, err := s.ListDocRecent(ctx, 0)
	if err != nil || len(list) != 2 || list[0].ID != "D1" || list[0].Size != 20 || list[0].Path != "/a/1.pdf" || list[1].ID != "D2" {
		t.Fatalf("按 opened_at 倒序且已更新: %+v %v", list, err)
	}
	if l, _ := s.ListDocRecent(ctx, 1); len(l) != 1 {
		t.Fatalf("limit: %d", len(l))
	}
	keys, err := s.DeleteDocRecent(ctx, []string{"D1", "nope"})
	if err != nil || len(keys) != 1 || keys[0] != "/a/1.pdf" {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
	if list, _ = s.ListDocRecent(ctx, 0); len(list) != 1 || list[0].ID != "D2" {
		t.Fatalf("%+v", list)
	}
	if keys, err := s.DeleteDocRecent(ctx, nil); err != nil || keys != nil {
		t.Fatal(keys, err)
	}
}

func TestDocRecentKeepsLatest(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	s.SetDocRecentKeep(3)
	for i := 1; i <= 5; i++ {
		k := fmt.Sprintf("/p/%d.pdf", i)
		if _, err := s.UpsertDocRecent(ctx, k, DocRecent{ID: fmt.Sprintf("D%d", i), Path: k, Name: "x", OpenedAt: int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := s.ListDocRecent(ctx, 200)
	if len(list) != 3 || list[0].ID != "D5" || list[2].ID != "D3" {
		t.Fatalf("%+v", list)
	}
}

func TestDocRecentDefaultKeep(t *testing.T) {
	if DefaultDocRecentKeep != 1000 {
		t.Fatal("契约：保留最近 1000 条")
	}
}
