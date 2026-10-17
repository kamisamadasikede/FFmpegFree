package cat

import (
	"os"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/store"
)

// 契约 6.21.17 ④：status = running（有正在跑的一轮）/ queued（没在跑但队列 > 0）/ idle。
func TestDecorateGroupMemberStatus(t *testing.T) {
	const id = "unit-member-status"
	t.Cleanup(func() {
		groupQueueMu.Lock()
		delete(groupQueues, id)
		groupQueueMu.Unlock()
	})
	m := store.CatGroupMember{ID: id}
	decorateGroupMember(&m)
	if m.Status != "idle" || m.QueueLength != 0 || m.Queue == nil {
		t.Fatalf("idle %+v", m)
	}
	groupQueueMu.Lock()
	st := queueStateLocked(id)
	st.active = true
	st.items = []groupQueueItem{{chainID: "a", hop: 2, fromSeq: 7}, {chainID: "b", hop: 0, fromSeq: 9}}
	groupQueueMu.Unlock()
	decorateGroupMember(&m)
	if m.Status != "queued" || m.QueueLength != 2 {
		t.Fatalf("queued %+v", m)
	}
	want := []store.CatGroupQueuedTask{{ChainID: "a", Hop: 2, FromSeq: 7, QueuePosition: 0}, {ChainID: "b", Hop: 0, FromSeq: 9, QueuePosition: 1}}
	for i := range want {
		if m.Queue[i] != want[i] {
			t.Fatalf("queue %+v", m.Queue)
		}
	}
	groupQueueMu.Lock()
	st.running = &GroupRunning{TurnID: "t", StartedAt: 1}
	groupQueueMu.Unlock()
	decorateGroupMember(&m)
	if m.Status != "running" || m.QueueLength != 2 {
		t.Fatalf("running %+v", m)
	}
	if r := groupRunningSnapshot(id); r == nil || r.TurnID != "t" || r.ElapsedMs <= 0 {
		t.Fatalf("snapshot %+v", r)
	}
}

func TestRouteDurationMs(t *testing.T) {
	cases := []struct {
		s, f, want int64
	}{{0, 0, 0}, {100, 0, 0}, {0, 100, 0}, {100, 50, 0}, {100, 350, 250}}
	for _, c := range cases {
		if got := routeDurationMs(store.CatGroupRouteDetail{StartedAt: c.s, FinishedAt: c.f}); got != c.want {
			t.Fatalf("%+v got %d", c, got)
		}
	}
}

func TestGroupFileExists(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "a"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "a", "x.go"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(filepath.Dir(root), "outside.txt"), []byte("x"), 0o644)
	for p, want := range map[string]bool{
		"a/x.go": true, "a/y.go": false, "a": false, "": false,
		"../outside.txt": false, filepath.Join(root, "a", "x.go"): false,
	} {
		if got := groupFileExists(root, p); got != want {
			t.Fatalf("%q = %v", p, got)
		}
	}
	if groupFileExists("", "a/x.go") {
		t.Fatal("no root")
	}
}
