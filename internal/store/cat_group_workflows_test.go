package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCatGroupWorkflowsUpgradePersistenceAndCascade(t *testing.T) {
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
	// Simulate an existing v19 installation with group data already present.
	if _, err = s.DB().Exec(`DROP TABLE cat_group_assignments; DROP TABLE cat_group_requests; DROP TABLE cat_group_workflows; DROP INDEX cat_group_messages_chain; ALTER TABLE cat_group_messages DROP COLUMN files_json; DELETE FROM schema_migrations WHERE version>=20`); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	w := CatGroupWorkflow{ChainID: "active", GroupID: g.ID, Request: "build feature", Status: "blocked", Tasks: []CatGroupWorkTask{{ID: "api", MemberID: "employee", Kind: "develop", Status: "done", Result: "API ready", Evidence: "go test passed"}, {ID: "verify", Kind: "verify", Status: "pending", DependsOn: []string{"api"}}}}
	if err := s.SaveCatGroupWorkflow(ctx, &w); err != nil {
		t.Fatal(err)
	}
	if w.Revision != 1 || w.UpdatedAt == 0 {
		t.Fatalf("missing revision/time: %+v", w)
	}
	w.Tasks[1].Status = "blocked"
	if err := s.SaveCatGroupWorkflow(ctx, &w); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	back, err := s.GetCatGroupWorkflow(ctx, w.ChainID)
	if err != nil || !reflect.DeepEqual(back, w) || back.Revision != 2 {
		t.Fatalf("workflow lost on reopen: %+v, %v", back, err)
	}
	// Old active workflows must survive a large amount of terminal history.
	for i := 0; i < 40; i++ {
		other := CatGroupWorkflow{ChainID: fmt.Sprint("completed-", i), GroupID: g.ID, Status: "completed"}
		if err := s.SaveCatGroupWorkflow(ctx, &other); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListCatGroupWorkflows(ctx, g.ID)
	if err != nil || len(list) != 11 {
		t.Fatalf("active + 10 terminal: %d, %v", len(list), err)
	}
	found := false
	for _, item := range list {
		found = found || item.ChainID == w.ChainID
	}
	if !found {
		t.Fatal("active workflow hidden by terminal history")
	}
	if _, err := s.DB().Exec(`DELETE FROM cat_groups WHERE id=?`, g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCatGroupWorkflow(ctx, w.ChainID); err != sql.ErrNoRows {
		t.Fatalf("workflow survived group deletion: %v", err)
	}
}
