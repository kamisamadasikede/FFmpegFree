package catagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContextSignalsPathEncodesCwd(t *testing.T) {
	cwd := `C:\Users\Administrator\Videos\Radeon ReLive\VALORANT`
	got := contextSignalsPath(`D:\sessions`, cwd, "01M4GG2WHF4W9HHNMRM6JD6KDW")
	want := filepath.Join(`D:\sessions`, `C%3A%5CUsers%5CAdministrator%5CVideos%5CRadeon%20ReLive%5CVALORANT`, "e38906a4-076d-5e20-98b7-684770b6a8d7", "signals.json")
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
	if contextSignalsPath("", cwd, "c") != "" || contextSignalsPath(`D:\sessions`, "", "c") != "" || contextSignalsPath(`D:\sessions`, cwd, " ") != "" {
		t.Fatal("empty parts should not build a path")
	}
}

func TestReadContextUsage(t *testing.T) {
	root := t.TempDir()
	cwd := `D:\work\demo`
	conv := "conv-1"
	path := contextSignalsPath(root, cwd, conv)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := readContextUsageIn(root, cwd, conv); ok {
		t.Fatal("missing file should not count")
	}
	if err := os.WriteFile(path, []byte(`{"contextWindowUsage":0,"contextTokensUsed":2294,"contextWindowTokens":256000}`), 0o644); err != nil {
		t.Fatal(err)
	}
	used, window, ok := readContextUsageIn(root, cwd, conv)
	if !ok || used != 2294 || window != 256000 {
		t.Fatalf("got %d %d %v", used, window, ok)
	}
	if err := os.WriteFile(path, []byte(`{"contextTokensUsed":0,"contextWindowTokens":256000}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := readContextUsageIn(root, cwd, conv); ok {
		t.Fatal("zero used should stay hidden")
	}
	if err := os.WriteFile(path, []byte(`not-json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := readContextUsageIn(root, cwd, conv); ok {
		t.Fatal("bad json should stay hidden")
	}
}

func TestTurnCwdPrefersProject(t *testing.T) {
	if got := TurnCwd(`D:\data`, `D:\proj`, "c1"); got != `D:\proj` {
		t.Fatalf("project %s", got)
	}
	if got := TurnCwd("", "  ", "c1"); got != "" {
		t.Fatalf("empty root %q", got)
	}
}
