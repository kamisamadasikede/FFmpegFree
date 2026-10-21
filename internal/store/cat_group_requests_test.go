package store

import (
	"context"
	"path/filepath"
	"testing"
)

func requestTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "request.db"))
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := s.InsertOrGetCatProject(context.Background(), CatProjectRow{Path: t.TempDir(), PathKey: "test", Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.InsertCatGroup(context.Background(), CatGroup{ID: "g", ProjectID: p.ID, Name: "g"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func receiveTestRequest(t *testing.T, s *Store, id, client string) CatGroupRequest {
	t.Helper()
	r, created, err := s.ReceiveCatGroupRequest(context.Background(), CatGroupRequest{ID: id, GroupID: "g", ClientID: client, Message: CatGroupMessage{GroupID: "g", Kind: "user", Text: "固定输入", ChainID: id}})
	if err != nil || !created {
		t.Fatalf("receive: %v %v", created, err)
	}
	return r
}
func assignTestRequest(t *testing.T, s *Store, r CatGroupRequest, ids ...string) {
	t.Helper()
	ok, err := s.ClaimCatGroupRequest(context.Background(), r.ID)
	if err != nil || !ok {
		t.Fatalf("claim %v %v", ok, err)
	}
	var d CatGroupDecision
	for _, id := range ids {
		d.Assignments = append(d.Assignments, CatGroupAssignment{ID: id, RequestID: r.ID, MemberID: id, Capability: "read_only"})
	}
	if err = s.CommitCatGroupDecision(context.Background(), r.ID, d, nil); err != nil {
		t.Fatal(err)
	}
}
func TestGroupRequestReceiptIdempotentAndDecisionAtomic(t *testing.T) {
	ctx := context.Background()
	s := requestTestStore(t)
	r := receiveTestRequest(t, s, "r1", "client-1")
	duplicate, created, err := s.ReceiveCatGroupRequest(ctx, CatGroupRequest{ID: "other", GroupID: "g", ClientID: "client-1"})
	if err != nil || created || duplicate.ID != r.ID || duplicate.Message.Seq != r.Message.Seq {
		t.Fatalf("duplicate: %+v %v %v", duplicate, created, err)
	}
	messages, _ := s.ListCatGroupMessages(ctx, "g", 0, 100)
	if len(messages) != 1 {
		t.Fatalf("duplicate messages: %d", len(messages))
	}
	ok, _ := s.ClaimCatGroupRequest(ctx, r.ID)
	if !ok {
		t.Fatal("claim")
	}
	// A duplicate assignment id causes the entire decision transaction to roll back.
	d := CatGroupDecision{Assignments: []CatGroupAssignment{{ID: "same", MemberID: "m1"}, {ID: "same", MemberID: "m2"}}}
	w := CatGroupWorkflow{ChainID: r.ID, GroupID: "g", Status: "running"}
	if err = s.CommitCatGroupDecision(ctx, r.ID, d, &w); err == nil {
		t.Fatal("bad decision accepted")
	}
	current, _ := s.GetCatGroupRequest(ctx, r.ID)
	as, _ := s.ListCatGroupAssignments(ctx, r.ID)
	if current.Status != "routing" || len(as) != 0 {
		t.Fatalf("partial commit: %s %d", current.Status, len(as))
	}
	if _, err = s.GetCatGroupWorkflow(ctx, r.ID); err == nil {
		t.Fatal("partial workflow")
	}
	d.Assignments = d.Assignments[:1]
	if err = s.CommitCatGroupDecision(ctx, r.ID, d, &w); err != nil {
		t.Fatal(err)
	}
	if err = s.CommitCatGroupDecision(ctx, r.ID, d, &w); err == nil {
		t.Fatal("duplicate decision committed")
	}
}
func TestGroupAssignmentRecoveryNeverReplaysRunningWork(t *testing.T) {
	ctx := context.Background()
	s := requestTestStore(t)
	received := receiveTestRequest(t, s, "received", "c1")
	routing := receiveTestRequest(t, s, "routing", "c2")
	s.ClaimCatGroupRequest(ctx, routing.ID)
	pending := receiveTestRequest(t, s, "pending", "c3")
	assignTestRequest(t, s, pending, "enqueued")
	running := receiveTestRequest(t, s, "running", "c4")
	assignTestRequest(t, s, running, "started")
	for _, id := range []string{"enqueued", "started"} {
		ok, err := s.ClaimCatGroupAssignment(ctx, id, "pending", "enqueued", "")
		if err != nil || !ok {
			t.Fatal("claim enqueue", err)
		}
	}
	ok, _ := s.ClaimCatGroupAssignment(ctx, "started", "enqueued", "running", "old-turn")
	if !ok {
		t.Fatal("claim run")
	}
	if again, _ := s.ClaimCatGroupAssignment(ctx, "started", "enqueued", "running", "duplicate"); again {
		t.Fatal("double run")
	}
	rows, err := s.RecoverCatGroupRequests(ctx)
	if err != nil || len(rows) != 4 {
		t.Fatalf("recover %v %v", rows, err)
	}
	for _, r := range []CatGroupRequest{received, routing} {
		got, _ := s.GetCatGroupRequest(ctx, r.ID)
		if got.Status != "received" {
			t.Fatal(got.Status)
		}
	}
	a, _ := s.GetCatGroupAssignment(ctx, "enqueued")
	b, _ := s.GetCatGroupAssignment(ctx, "started")
	r, _ := s.GetCatGroupRequest(ctx, running.ID)
	if a.Status != "pending" || b.Status != "interrupted" || r.Status != "interrupted" {
		t.Fatalf("unsafe recovery: %+v %+v %+v", a, b, r)
	}
}
func TestGroupAssignmentStopFencesLateReplyAndNewRequestSurvives(t *testing.T) {
	ctx := context.Background()
	s := requestTestStore(t)
	r := receiveTestRequest(t, s, "old", "c1")
	assignTestRequest(t, s, r, "a")
	s.ClaimCatGroupAssignment(ctx, "a", "pending", "enqueued", "")
	s.ClaimCatGroupAssignment(ctx, "a", "enqueued", "running", "t")
	if err := s.StopCatGroupRequests(ctx, "g", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteCatGroupAssignment(ctx, "a", "t", CatGroupMessage{GroupID: "g", Kind: "member", Text: "late", ChainID: r.ID, TurnID: "t"}); err == nil {
		t.Fatal("late result accepted")
	}
	fresh := receiveTestRequest(t, s, "fresh", "c2")
	assignTestRequest(t, s, fresh, "new-a")
	s.ClaimCatGroupAssignment(ctx, "new-a", "pending", "enqueued", "")
	s.ClaimCatGroupAssignment(ctx, "new-a", "enqueued", "running", "new-turn")
	before, _ := s.GetCatGroupRequest(ctx, fresh.ID)
	if before.FirstReplyAt != 0 {
		t.Fatal("ack counted as reply")
	}
	if _, err := s.CompleteCatGroupAssignment(ctx, "new-a", "new-turn", CatGroupMessage{GroupID: "g", Kind: "member", Text: "有效回复", ChainID: fresh.ID, TurnID: "new-turn"}); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetCatGroupRequest(ctx, fresh.ID)
	if after.Status != "answered" || after.FirstReplyAt == 0 {
		t.Fatalf("not answered: %+v", after)
	}
	messages, _ := s.ListCatGroupMessages(ctx, "g", 0, 100)
	for _, m := range messages {
		if m.Text == "late" {
			t.Fatal("stale delivery persisted")
		}
	}
}

func TestGroupAssignmentAnsweredRecoveryAndHeldHandoff(t *testing.T) {
	ctx := context.Background()
	s := requestTestStore(t)
	r := receiveTestRequest(t, s, "multi", "client")
	assignTestRequest(t, s, r, "a", "b")
	s.ClaimCatGroupAssignment(ctx, "a", "pending", "enqueued", "")
	s.ClaimCatGroupAssignment(ctx, "a", "enqueued", "running", "ta")
	if _, err := s.CompleteCatGroupAssignment(ctx, "a", "ta", CatGroupMessage{GroupID: "g", Kind: "member", MemberID: "a", ChainID: r.ID, TurnID: "ta", Text: "第一位答复"}); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.RecoverCatGroupRequests(ctx)
	if err != nil || len(recovered) != 1 || recovered[0].Status != "answered" || recovered[0].FirstReplyAt == 0 {
		t.Fatalf("remaining pending work lost: %+v %v", recovered, err)
	}
	next := CatGroupAssignment{ID: "c", RequestID: r.ID, MemberID: "c", Capability: "read_only", Instruction: "继续原始任务"}
	if err = s.MoveCatGroupAssignment(ctx, "b", next, nil); err != nil {
		t.Fatal(err)
	}
	b, _ := s.GetCatGroupAssignment(ctx, "b")
	c, _ := s.GetCatGroupAssignment(ctx, "c")
	if b.Status != "superseded" || c.Status != "held" {
		t.Fatalf("handoff not persisted: %+v %+v", b, c)
	}
	recovered, err = s.RecoverCatGroupRequests(ctx)
	if err != nil || len(recovered) != 1 || recovered[0].Status != "interrupted" || recovered[0].FirstReplyAt == 0 {
		t.Fatalf("unknown handoff replayed: %+v %v", recovered, err)
	}
	if ok, _ := s.ClaimCatGroupAssignment(ctx, "c", "held", "pending", ""); ok {
		t.Fatal("interrupted handoff restarted automatically")
	}
}

func TestGroupRequestsMigrationUpgradeReopenAndCascade(t *testing.T) {
	ctx := context.Background()
	s := requestTestStore(t)
	if _, err := s.DB().Exec(`DROP TABLE cat_group_assignments; DROP TABLE cat_group_requests; DELETE FROM schema_migrations WHERE version>=22`); err != nil {
		t.Fatal(err)
	}
	if err := s.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	r := receiveTestRequest(t, s, "persistent", "client")
	assignTestRequest(t, s, r, "a")
	var filename string
	var seq int
	var dbname string
	if err := s.DB().QueryRow(`PRAGMA database_list`).Scan(&seq, &dbname, &filename); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	back, err := reopened.GetCatGroupRequest(ctx, r.ID)
	if err != nil || back.Message.Text != r.Message.Text || back.Status != "assigned" {
		t.Fatalf("request lost: %+v %v", back, err)
	}
	if _, err = reopened.DB().Exec(`DELETE FROM cat_groups WHERE id='g'`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"cat_group_requests", "cat_group_assignments"} {
		var n int
		if err = reopened.DB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("cascade %s: %d %v", table, n, err)
		}
	}
}

func TestGroupAssignmentWorkflowHandoffAndReleaseAreAtomic(t *testing.T) {
	ctx := context.Background()
	s := requestTestStore(t)
	r := receiveTestRequest(t, s, "handoff", "handoff-client")
	if ok, err := s.ClaimCatGroupRequest(ctx, r.ID); err != nil || !ok {
		t.Fatal("claim", err)
	}
	w := CatGroupWorkflow{ChainID: r.ID, GroupID: "g", Status: "running", Tasks: []CatGroupWorkTask{{ID: "a", MemberID: "m1", Status: "queued"}}}
	old := CatGroupAssignment{ID: "old", RequestID: r.ID, MemberID: "m1", TaskID: "a", Instruction: "固定分工", Capability: "read_only"}
	if err := s.CommitCatGroupDecision(ctx, r.ID, CatGroupDecision{Assignments: []CatGroupAssignment{old}}, &w); err != nil {
		t.Fatal(err)
	}
	next := old
	next.ID, next.MemberID = "new", "m2"
	w.Tasks[0].MemberID = "m2"
	stale := w
	stale.Revision++
	if err := s.MoveCatGroupAssignment(ctx, old.ID, next, &stale); err == nil {
		t.Fatal("stale workflow accepted")
	}
	a, _ := s.GetCatGroupAssignment(ctx, old.ID)
	if a.Status != "pending" {
		t.Fatal("old assignment fenced despite rollback", a)
	}
	if _, err := s.GetCatGroupAssignment(ctx, next.ID); err == nil {
		t.Fatal("partial handoff inserted")
	}
	if err := s.MoveCatGroupAssignment(ctx, old.ID, next, &w); err != nil {
		t.Fatal(err)
	}
	durable, err := s.GetCatGroupWorkflow(ctx, r.ID)
	if err != nil || durable.Tasks[0].MemberID != "m2" || durable.Revision != w.Revision {
		t.Fatal("workflow not committed", durable, err)
	}
	w.Tasks[0].Status = "pending"
	stale = w
	stale.Revision++
	if _, err := s.ReleaseCatGroupAssignment(ctx, next.ID, &stale); err == nil {
		t.Fatal("stale release accepted")
	}
	a, _ = s.GetCatGroupAssignment(ctx, next.ID)
	if a.Status != "held" {
		t.Fatal("release escaped rollback", a)
	}
	if ok, err := s.ReleaseCatGroupAssignment(ctx, next.ID, &w); err != nil || !ok {
		t.Fatal("release", ok, err)
	}
	if ok, err := s.ReleaseCatGroupAssignment(ctx, next.ID, &w); err != nil || ok {
		t.Fatal("duplicate release", ok, err)
	}
	durable, _ = s.GetCatGroupWorkflow(ctx, r.ID)
	a, _ = s.GetCatGroupAssignment(ctx, next.ID)
	if durable.Tasks[0].Status != "pending" || a.Status != "pending" {
		t.Fatal("release states disagree", durable, a)
	}
}
