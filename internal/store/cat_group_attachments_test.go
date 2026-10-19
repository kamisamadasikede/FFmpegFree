package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCatGroupImagesUpgradeAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "app.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := s.InsertOrGetCatProject(ctx, CatProjectRow{Name: "project", Path: t.TempDir(), PathKey: "project"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.InsertCatGroup(ctx, CatGroup{ProjectID: p.ID, Name: "group"})
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.InsertCatGroupMessage(ctx, CatGroupMessage{GroupID: g.ID, Kind: "user", ChainID: "old", Text: "existing message"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`ALTER TABLE cat_group_messages DROP COLUMN files_json; DELETE FROM schema_migrations WHERE version=21`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	back, err := s.GetCatGroupMessage(ctx, g.ID, old.Seq)
	if err != nil || len(back.Files) != 0 || back.Text != old.Text {
		t.Fatalf("existing history changed: %+v %v", back, err)
	}
	files := []CatGroupAttachment{{Name: "reference.png", Path: filepath.Join(t.TempDir(), "reference.png")}}
	msg, err := s.InsertCatGroupMessage(ctx, CatGroupMessage{GroupID: g.ID, Kind: "user", ChainID: "images", Files: files})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list, err := s.ListCatGroupMessages(ctx, g.ID, 0, 50)
	if err != nil || len(list) != 2 || !reflect.DeepEqual(list[1].Files, files) {
		t.Fatalf("images lost on reopen: %+v %v", list, err)
	}
	origin, err := s.GetCatGroupChainOrigin(ctx, g.ID, msg.ChainID)
	if err != nil || !reflect.DeepEqual(origin.Files, files) {
		t.Fatalf("origin images lost: %+v %v", origin, err)
	}
}
