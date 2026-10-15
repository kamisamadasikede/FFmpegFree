package cat

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
)

type testCLIUpdater struct {
	*fakeDetAdapter
	updates atomic.Int32
	fail    bool
	started chan struct{}
	finish  chan struct{}
}

func (f *testCLIUpdater) CheckCLIUpdate(context.Context) (catagent.CLIUpdate, error) {
	if f.fail {
		return catagent.CLIUpdate{}, errors.New("offline")
	}
	return catagent.CLIUpdate{AgentKind: f.kind, CurrentVersion: "1.0", LatestVersion: "2.0", Available: true, CanUpdate: true}, nil
}
func (f *testCLIUpdater) UpdateCLI(context.Context) error {
	f.updates.Add(1)
	if f.started != nil {
		close(f.started)
		<-f.finish
	}
	if f.fail {
		return errors.New("installer failed")
	}
	return nil
}
func updateTestService() (*Service, *testCLIUpdater) {
	f := &testCLIUpdater{fakeDetAdapter: newFake(catagent.KindCodexCLI, true)}
	f.set(Status{State: catagent.StateReady, Version: "1.0"}, true)
	reg := catagent.NewRegistry()
	reg.Register(f)
	return New(Config{Registry: reg, StatusDebounce: -1}), f
}
func TestCLIUpdatesQuietCheckAndExplicitInstall(t *testing.T) {
	s, f := updateTestService()
	rows, err := s.CheckCatCLIUpdates(context.Background())
	if err != nil || len(rows) != 1 || f.updates.Load() != 0 {
		t.Fatalf("check ran installer: %v %v", rows, err)
	}
	f.fail = true
	rows, err = s.CheckCatCLIUpdates(context.Background())
	if err != nil || len(rows) != 0 || f.Status().State != catagent.StateReady {
		t.Fatal("offline check changed readiness")
	}
	f.fail = false
	if _, err := s.UpdateCatCLI(context.Background(), catagent.KindCodexCLI); err != nil {
		t.Fatal(err)
	}
	if f.updates.Load() != 1 || f.rechecks.Load() != 1 {
		t.Fatal("install must recheck")
	}
}
func TestCLIUpdatesConcurrentAndChatRemainsUsable(t *testing.T) {
	s, f := updateTestService()
	s.cfg.Store = openDetStore(t, filepath.Join(t.TempDir(), "updates.db"))
	c, err := s.CreateCatConversation(context.Background(), CreateConversationRequest{AgentKind: catagent.KindCodexCLI})
	if err != nil {
		t.Fatal(err)
	}
	s.turns["busy"] = &turnState{agentKind: catagent.KindCodexCLI}
	f.started, f.finish = make(chan struct{}), make(chan struct{})
	g := &testCLIUpdater{fakeDetAdapter: newFake(catagent.KindClaudeCLI, true), started: make(chan struct{}), finish: make(chan struct{}), fail: true}
	g.set(Status{State: catagent.StateReady, Version: "1.0"}, true)
	s.cfg.Registry.Register(g)
	for _, updater := range []*testCLIUpdater{f, g} {
		t.Cleanup(func() {
			select {
			case <-updater.finish:
			default:
				close(updater.finish)
			}
		})
	}
	start := func(updater *testCLIUpdater) chan error {
		done := make(chan error, 1)
		go func() { _, err := s.UpdateCatCLI(context.Background(), updater.kind); done <- err }()
		select {
		case <-updater.started:
		case err := <-done:
			t.Fatalf("installer did not start: %v", err)
		case <-time.After(3 * time.Second):
			t.Fatal("installer blocked")
		}
		return done
	}
	first, second := start(f), start(g)
	if _, err := s.UpdateCatCLI(context.Background(), f.kind); !apperr.Is(err, apperr.TaskConflict) || f.updates.Load() != 1 {
		t.Fatalf("duplicate installer must be excluded: %v", err)
	}
	if _, err := s.SendCatMessage(context.Background(), SendMessageRequest{ConversationID: c.ID, Content: "hello"}); err != nil {
		t.Fatalf("chat blocked during concurrent updates: %v", err)
	}
	s.Wait()
	if f.runs.Load() != 1 {
		t.Fatal("new turn did not run during installation")
	}
	close(g.finish)
	if err := <-second; !apperr.Is(err, apperr.IOError) {
		t.Fatalf("expected independent failure: %v", err)
	}
	s.cliUpdateMu.Lock()
	firstRunning, secondRunning := s.cliUpdating[f.kind], s.cliUpdating[g.kind]
	s.cliUpdateMu.Unlock()
	if !firstRunning || secondRunning {
		t.Fatal("one failed update cleared another installer")
	}
	close(f.finish)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	// Both success and failure release only their own duplicate protection.
	f.started = nil
	g.started, g.fail = nil, false
	for _, kind := range []string{f.kind, g.kind} {
		if _, err := s.UpdateCatCLI(context.Background(), kind); err != nil {
			t.Fatalf("update could not be retried: %v", err)
		}
	}
	if _, err := s.UpdateCatCLI(context.Background(), "unknown"); err == nil {
		t.Fatal("unknown CLI accepted")
	}
}
