package cat

import (
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
	"context"
	"fmt"
	"testing"
)

func TestGroupWorkflowHistoryDismissalPersistsAndProtectsActiveWork(t *testing.T) {
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		t.Error("dismissal must never start execution")
		return catagent.TurnResponse{}, nil
	})
	ctx := context.Background()
	for _, status := range []string{"running", "queued", "failed", "blocked", "stopped", "done"} {
		w := store.CatGroupWorkflow{ChainID: status, GroupID: g.ID, Status: "blocked", Tasks: []store.CatGroupWorkTask{{ID: "a", MemberID: ms[0].ID, Status: status}, {ID: "b", MemberID: ms[1].ID, Status: "pending", DependsOn: []string{"a"}}}}
		if status == "done" {
			w.Status = "completed"
		}
		if status == "stopped" {
			w.Status = "stopped"
		}
		if err := s.cfg.Store.SaveCatGroupWorkflow(ctx, &w); err != nil {
			t.Fatal(err)
		}
		req := DismissGroupWorkflowRequest{GroupID: g.ID, ChainID: w.ChainID}
		err := s.DismissCatGroupWorkflow(ctx, req)
		active := status == "running" || status == "queued"
		if (err != nil) != active {
			t.Fatalf("status=%s active=%v err=%v", status, active, err)
		}
		saved, err := s.cfg.Store.GetCatGroupWorkflow(ctx, w.ChainID)
		if err != nil || saved.Dismissed == active {
			t.Fatalf("dismissal not persisted: %+v %v", saved, err)
		}
		if !active {
			if err := s.DismissCatGroupWorkflow(ctx, req); err != nil {
				t.Fatal("repeated dismissal should be idempotent", err)
			}
			handled, err := s.retryGroupWorkTask(ctx, g, w.ChainID, ms[0].ID, "a")
			if !handled || err == nil {
				t.Fatal("dismissed task was allowed to resume")
			}
		}
	}
	list, err := s.cfg.Store.ListCatGroupWorkflows(ctx, g.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("hidden cards returned on reload: %+v %v", list, err)
	}
	for _, w := range list {
		if w.Dismissed {
			t.Fatal("hidden card listed")
		}
	}
	other, err := s.cfg.Store.InsertCatGroup(ctx, store.CatGroup{ProjectID: g.ProjectID, Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DismissCatGroupWorkflow(ctx, DismissGroupWorkflowRequest{GroupID: other.ID, ChainID: "running"}); err == nil {
		t.Fatal("allowed cross-group dismissal")
	}
}

func TestGroupWorkflowHistoryClearIncludesOlderRecords(t *testing.T) {
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		t.Error("clearing history must never start execution")
		return catagent.TurnResponse{}, nil
	})
	ctx := context.Background()
	for i := 0; i < 15; i++ {
		w := store.CatGroupWorkflow{ChainID: fmt.Sprintf("old-%d", i), GroupID: g.ID, Status: "completed"}
		if err := s.cfg.Store.SaveCatGroupWorkflow(ctx, &w); err != nil {
			t.Fatal(err)
		}
	}
	active := store.CatGroupWorkflow{ChainID: "active", GroupID: g.ID, Status: "blocked", Tasks: []store.CatGroupWorkTask{{ID: "a", MemberID: ms[0].ID, Status: "queued"}}}
	if err := s.cfg.Store.SaveCatGroupWorkflow(ctx, &active); err != nil {
		t.Fatal(err)
	}
	other, err := s.cfg.Store.InsertCatGroup(ctx, store.CatGroup{ProjectID: g.ProjectID, Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	untouched := store.CatGroupWorkflow{ChainID: "other", GroupID: other.ID, Status: "completed"}
	if err := s.cfg.Store.SaveCatGroupWorkflow(ctx, &untouched); err != nil {
		t.Fatal(err)
	}
	before, err := s.cfg.Store.ListCatGroupWorkflows(ctx, g.ID)
	if err != nil || len(before) != 11 {
		t.Fatalf("expected recent window + active, got %d: %v", len(before), err)
	}
	ids, err := s.ClearCatGroupWorkflowHistory(ctx, g.ID)
	if err != nil || len(ids) != 15 {
		t.Fatalf("must clear all settled records: %d %v", len(ids), err)
	}
	after, err := s.cfg.Store.ListCatGroupWorkflows(ctx, g.ID)
	if err != nil || len(after) != 1 || after[0].ChainID != active.ChainID {
		t.Fatalf("history returned on reload or active work lost: %+v %v", after, err)
	}
	saved, err := s.cfg.Store.GetCatGroupWorkflow(ctx, untouched.ChainID)
	if err != nil || saved.Dismissed {
		t.Fatalf("other group affected: %+v %v", saved, err)
	}
	ids, err = s.ClearCatGroupWorkflowHistory(ctx, g.ID)
	if err != nil || len(ids) != 0 {
		t.Fatalf("clear must be idempotent: %v %v", ids, err)
	}
}
