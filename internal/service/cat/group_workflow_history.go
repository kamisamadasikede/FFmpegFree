package cat

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
)

type DismissGroupWorkflowRequest struct {
	GroupID string `json:"groupId"`
	ChainID string `json:"chainId"`
}

// A blocked graph may still have independent tasks executing. Keep those in
// progress until every queued/running task has settled.
func groupWorkflowActive(w store.CatGroupWorkflow) bool {
	if w.Status == "completed" || w.Status == "stopped" {
		return false
	}
	if w.Status == "running" {
		return true
	}
	for _, t := range w.Tasks {
		if t.Status == "running" || t.Status == "queued" {
			return true
		}
	}
	return false
}

// Dismiss only the task card. Keep messages and execution records available.
func (s *Service) DismissCatGroupWorkflow(ctx context.Context, req DismissGroupWorkflowRequest) error {
	if _, err := s.requireGroup(ctx, req.GroupID); err != nil {
		return err
	}
	s.groupWorkMu.Lock()
	w, err := s.cfg.Store.GetCatGroupWorkflow(ctx, req.ChainID)
	if err != nil {
		s.groupWorkMu.Unlock()
		return err
	}
	if w.GroupID != req.GroupID {
		s.groupWorkMu.Unlock()
		return s.errGroupNotFound()
	}
	if w.Dismissed {
		s.groupWorkMu.Unlock()
		return nil
	}
	if groupWorkflowActive(w) {
		s.groupWorkMu.Unlock()
		return apperr.New(apperr.TaskConflict, "任务仍在处理中，请先停止再从记录中移除")
	}
	w.Dismissed = true
	err = s.cfg.Store.SaveCatGroupWorkflow(ctx, &w)
	if err == nil {
		delete(s.groupWorkContexts, w.ChainID)
	}
	s.groupWorkMu.Unlock()
	if err == nil {
		s.emitGroupWorkflow(w)
	}
	return err
}

// Clear every settled card, including records outside the recent-history window.
// Return saved IDs so the UI never hides a task resumed concurrently.
func (s *Service) ClearCatGroupWorkflowHistory(ctx context.Context, groupID string) ([]string, error) {
	if _, err := s.requireGroup(ctx, groupID); err != nil {
		return nil, err
	}
	s.groupWorkMu.Lock()
	works, err := s.cfg.Store.ListAllCatGroupWorkflows(ctx, groupID)
	if err != nil {
		s.groupWorkMu.Unlock()
		return nil, err
	}
	changed := []store.CatGroupWorkflow{}
	ids := []string{}
	for _, w := range works {
		if groupWorkflowActive(w) {
			continue
		}
		w.Dismissed = true
		if err = s.cfg.Store.SaveCatGroupWorkflow(ctx, &w); err != nil {
			break
		}
		delete(s.groupWorkContexts, w.ChainID)
		changed = append(changed, w)
		ids = append(ids, w.ChainID)
	}
	s.groupWorkMu.Unlock()
	for _, w := range changed {
		s.emitGroupWorkflow(w)
	}
	return ids, err
}
