package cat_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

type kindAdapter struct {
	fakeAdapter
	kind string
}

func (a *kindAdapter) Kind() string { return a.kind }

func TestScheduleAgentKindDefaultAndValidation(t *testing.T) {
	ctx := context.Background()
	e := newSchedEnv(t, false, nil)
	s, err := e.svc.CreateCatSchedule(ctx, cat.ScheduleInput{
		Name: "A", Prompt: "p", ProjectID: e.proj.ID, Frequency: cat.FreqOnce, RunAt: e.clock.Now().Add(time.Hour).UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.AgentKind != catagent.KindCatBuild {
		t.Fatalf("default kind=%s", s.AgentKind)
	}

	if _, err := e.svc.SaveCatSettings(ctx, cat.Settings{DefaultAgentKind: catagent.KindClaudeCLI}); err != nil {
		t.Fatal(err)
	}
	s2, err := e.svc.CreateCatSchedule(ctx, cat.ScheduleInput{
		Name: "B", Prompt: "p", ProjectID: e.proj.ID, Frequency: cat.FreqOnce,
		RunAt: e.clock.Now().Add(2 * time.Hour).UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if s2.AgentKind != catagent.KindClaudeCLI {
		t.Fatalf("from settings=%s", s2.AgentKind)
	}

	s3, err := e.svc.CreateCatSchedule(ctx, cat.ScheduleInput{
		Name: "C", Prompt: "p", ProjectID: e.proj.ID, Frequency: cat.FreqOnce,
		RunAt: e.clock.Now().Add(3 * time.Hour).UnixMilli(), AgentKind: catagent.KindCodexCLI,
	})
	if err != nil {
		t.Fatal(err)
	}
	if s3.AgentKind != catagent.KindCodexCLI {
		t.Fatalf("explicit=%s", s3.AgentKind)
	}
	list, _ := e.svc.ListCatSchedules(ctx)
	found := false
	for _, x := range list {
		if x.ID == s3.ID && x.AgentKind == catagent.KindCodexCLI {
			found = true
		}
	}
	if !found {
		t.Fatalf("list missing agentKind")
	}

	if _, err := e.svc.CreateCatSchedule(ctx, cat.ScheduleInput{
		Name: "bad", Prompt: "p", ProjectID: e.proj.ID, Frequency: cat.FreqOnce,
		RunAt: e.clock.Now().Add(4 * time.Hour).UnixMilli(), AgentKind: "nope",
	}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("illegal kind: %v", err)
	}
}

func TestScheduleRunUsesAgentKind(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	clock := &fakeClock{t: time.Date(2026, 10, 14, 8, 59, 0, 0, cst)}
	n := &notes{}
	runFn := func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "ok"}}, nil
	}
	reg := catagent.NewRegistry()
	reg.Register(&kindAdapter{kind: catagent.KindCatBuild, fakeAdapter: fakeAdapter{run: runFn}})
	reg.Register(&kindAdapter{kind: catagent.KindCodexCLI, fakeAdapter: fakeAdapter{run: runFn}})
	svc := cat.New(cat.Config{
		Store: st, Registry: reg, Now: clock.Now, Location: cst, Notify: n.add,
		DataRoot: t.TempDir(), ScheduleTick: time.Hour,
	})
	t.Cleanup(func() { svc.CloseScheduler(); svc.Wait() })

	dir := filepath.Join(t.TempDir(), "proj")
	_ = os.MkdirAll(dir, 0o755)
	pr, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: dir, Name: "演示"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := svc.CreateCatSchedule(ctx, cat.ScheduleInput{
		Name: "C", Prompt: "跑一下", ProjectID: pr.Project.ID, Frequency: cat.FreqOnce,
		RunAt: clock.Now().Add(time.Hour).UnixMilli(), AgentKind: catagent.KindCodexCLI,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := svc.RunNowCatSchedule(ctx, cat.ScheduleIDRequest{ID: s.ID})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rows, _ := st.ListCatScheduleRuns(ctx, s.ID, 10)
		for _, r := range rows {
			if r.ID == run.ID && r.Status == cat.RunSucceeded && r.ConversationID != "" {
				conv, err := st.GetCatConversation(ctx, r.ConversationID)
				if err != nil {
					t.Fatal(err)
				}
				if conv.AgentKind != catagent.KindCodexCLI {
					t.Fatalf("run used %s", conv.AgentKind)
				}
				return
			}
			if r.ID == run.ID && r.Status == cat.RunFailed {
				t.Fatalf("run failed: %+v", r)
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("run did not succeed")
}

func TestMigration0013AgentKind(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	var name string
	err := st.DB().QueryRowContext(ctx, `SELECT name FROM pragma_table_info('cat_schedules') WHERE name='agent_kind'`).Scan(&name)
	if err == sql.ErrNoRows || name != "agent_kind" {
		t.Fatalf("column missing: %v %q", err, name)
	}
	row := store.CatScheduleRow{
		ID: "s1", Name: "n", Prompt: "p", ProjectID: "p1", Frequency: "once",
		ApprovalMode: "plan", Enabled: true, CreatedAt: 1, UpdatedAt: 1, RunAt: 99,
	}
	if err := st.UpsertCatSchedule(ctx, row); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetCatSchedule(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentKind != catagent.KindCatBuild {
		t.Fatalf("default=%q", got.AgentKind)
	}
}
