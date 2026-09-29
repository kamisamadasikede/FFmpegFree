package app

import (
	"context"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/edit"
)

func TestEditServiceNotReadyIsInternal(t *testing.T) {
	s := NewEditService(func() *edit.Service { return nil }, nil)
	if _, err := s.ValidateProject(edit.EditProject{}); !apperr.Is(err, apperr.Internal) {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := s.Export(edit.EditProject{}, edit.EditExportOptions{}); !apperr.Is(err, apperr.Internal) {
		t.Fatalf("Export: %v", err)
	}
	if _, err := s.GetPreviewURL("/a.mp4"); !apperr.Is(err, apperr.Internal) {
		t.Fatalf("GetPreviewURL: %v", err)
	}
	if _, err := s.SaveProject(edit.EditProject{}); !apperr.Is(err, apperr.Internal) {
		t.Fatalf("Save: %v", err)
	}
	if _, err := s.LoadProject("x"); !apperr.Is(err, apperr.Internal) {
		t.Fatalf("Load: %v", err)
	}
	if _, err := s.ListProjects(0); !apperr.Is(err, apperr.Internal) {
		t.Fatalf("List: %v", err)
	}
	if err := s.DeleteProject("x"); !apperr.Is(err, apperr.Internal) {
		t.Fatalf("Delete: %v", err)
	}
}

func TestEditServiceRootCtx(t *testing.T) {
	if NewEditService(nil, nil).rootCtx() == nil {
		t.Fatal("rootCtx 为 nil 时应退回 Background")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := NewEditService(nil, func() context.Context { return ctx })
	cancel()
	if s.rootCtx().Err() == nil {
		t.Fatal("应使用应用根 ctx")
	}
}
