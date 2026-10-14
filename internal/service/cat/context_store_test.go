package cat

import "testing"

func TestContextUsageStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	if _, _, ok := loadContextUsage(root, "c1"); ok {
		t.Fatal("empty must be not ok")
	}
	saveContextUsage(root, "c1", 158000, 260000)
	u, w, ok := loadContextUsage(root, "c1")
	if !ok || u != 158000 || w != 260000 {
		t.Fatalf("got %d/%d %v", u, w, ok)
	}
	saveContextUsage(root, "c2", 0, 260000) // 未知不落盘
	if _, _, ok := loadContextUsage(root, "c2"); ok {
		t.Fatal("zero used must not persist")
	}
}
