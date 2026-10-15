package cat

import (
	"context"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
)

// CheckCatCLIUpdates runs after installation detection. Offline checks are quiet
// and must never turn an installed CLI into a missing or outdated one.
func (s *Service) CheckCatCLIUpdates(ctx context.Context) ([]catagent.CLIUpdate, error) {
	kinds := s.cfg.Registry.Kinds()
	rows := make([]catagent.CLIUpdate, len(kinds))
	var wg sync.WaitGroup
	for i, kind := range kinds {
		a, _ := s.cfg.Registry.Get(kind)
		u, ok := a.(catagent.CLIUpdater)
		if !ok || a.Status().State != catagent.StateReady {
			continue
		}
		wg.Add(1)
		go func(i int, kind string, u catagent.CLIUpdater) {
			defer wg.Done()
			checkCtx, cancel := context.WithTimeout(ctx, 18*time.Second)
			defer cancel()
			r, err := u.CheckCLIUpdate(checkCtx)
			if err != nil {
				s.cfg.Logf("cat update check %s: %v", kind, err)
				return
			}
			rows[i] = r
		}(i, kind, u)
	}
	wg.Wait()
	out := []catagent.CLIUpdate{}
	for _, row := range rows {
		if row.AgentKind != "" {
			out = append(out, row)
		}
	}
	return out, nil
}

// UpdateCatCLI is only called after a user chooses Update. Different CLIs can
// install concurrently, without blocking ongoing conversations or new turns.
func (s *Service) UpdateCatCLI(ctx context.Context, agentKind string) (Status, error) {
	agentKind = strings.TrimSpace(agentKind)
	a, ok := s.cfg.Registry.Get(agentKind)
	if !ok || a == nil {
		return Status{}, apperr.New(apperr.InvalidArgument, "不认识这个助手")
	}
	u, ok := a.(catagent.CLIUpdater)
	if !ok {
		return Status{}, apperr.New(apperr.Unsupported, "这个助手暂不支持自动更新。")
	}
	s.cliUpdateMu.Lock()
	if s.cliUpdating[agentKind] {
		s.cliUpdateMu.Unlock()
		return Status{}, apperr.New(apperr.TaskConflict, "正在更新，请稍候。")
	}
	if s.cliUpdating == nil {
		s.cliUpdating = make(map[string]bool)
	}
	s.cliUpdating[agentKind] = true
	s.cliUpdateMu.Unlock()
	defer func() {
		s.cliUpdateMu.Lock()
		delete(s.cliUpdating, agentKind)
		s.cliUpdateMu.Unlock()
	}()
	updateCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := u.UpdateCLI(updateCtx); err != nil {
		s.cfg.Logf("cat update %s: %v", agentKind, err)
		return Status{}, apperr.New(apperr.IOError, "更新失败，请检查网络或安装权限后重试。")
	}
	return s.RecheckCat(ctx, agentKind)
}
