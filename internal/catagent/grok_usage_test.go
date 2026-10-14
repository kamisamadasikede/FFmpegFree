package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseGrokUsage_PicksLatestAfterStart(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "grok", "usage-1.0.40.json"))
	if err != nil {
		t.Fatal(err)
	}
	start, _ := time.Parse(time.RFC3339, "2026-09-22T14:00:00Z")
	n := parseGrokUsage(raw, start)
	// v0.47：样本 turns[0] inputTokens=471686 outputTokens=17738 reasoningTokens=13913 totalTokens=489424
	if n.Total != 489424 || n.Output != 17738 {
		t.Fatalf("usage=%+v", n)
	}
	// 起点晚于 endedAt → 无匹配
	late, _ := time.Parse(time.RFC3339, "2026-09-22T15:00:00Z")
	if parseGrokUsage(raw, late) != (grokTurnUsage{}) {
		t.Fatal("late start should omit")
	}
	if parseGrokUsage([]byte("not json"), start) != (grokTurnUsage{}) {
		t.Fatal("bad json → 0")
	}
}

func TestGrokUsageArgs_NoTurn(t *testing.T) {
	args := grokUsageArgs("abc")
	if len(args) != 2 || args[0] != "usage" || args[1] != "abc" {
		t.Fatalf("%q", args)
	}
}

func TestGrokTurnTokens_TimeoutOrFailOmits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "grok")
	_ = os.WriteFile(fake, []byte("#!/bin/sh\n[ \"$1\" = usage ] && exit 1\necho unexpected >&2; exit 2\n"), 0o755)
	a := NewBuildAdapter(BuildConfig{
		DataTemp: dir,
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
		Now: time.Now,
	})
	n := a.grokTurnTokens(fake, os.Environ(), dir, "sess", time.Now().Add(-time.Minute))
	if n != (grokTurnUsage{}) {
		t.Fatalf("fail → 0, got %+v", n)
	}
}

func TestGrokTurnTokens_ParsesSample(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "grok")
	sample, _ := os.ReadFile(filepath.Join("testdata", "grok", "usage-1.0.40.json"))
	script := "#!/bin/sh\n[ \"$1\" = usage ] || exit 2\ncat <<'JSON'\n" + string(sample) + "\nJSON\n"
	_ = os.WriteFile(fake, []byte(script), 0o755)
	a := NewBuildAdapter(BuildConfig{
		DataTemp: dir,
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
		Now: time.Now,
	})
	start, _ := time.Parse(time.RFC3339, "2026-09-22T14:00:00Z")
	n := a.grokTurnTokens(fake, os.Environ(), dir, "01a0c98b-895d-7db1-8aa1-f66ea798989a", start)
	if n.Total != 489424 || n.Output != 17738 {
		t.Fatalf("usage=%+v", n)
	}
}

func TestHelpWhitelist_GrokUsage(t *testing.T) {
	assertWhitelisted(t, "grok usage", "grok-usage.txt", grokUsageArgs("sess-id"))
	raw := string(mustRead(t, "testdata/help/grok.txt"))
	if !strings.Contains(raw, "usage") {
		t.Fatal("grok.txt should document usage subcommand")
	}
	help := string(mustRead(t, "testdata/help/grok-usage.txt"))
	if !strings.Contains(help, "<SESSION_ID>") {
		t.Fatal("grok-usage.txt missing SESSION_ID")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// v0.47：turn 里没有 outputTokens（旧版 / 字段缺）→ Output=0（省略），不拿 total 顶替。
func TestParseGrokUsage_NoOutputTokensOmits(t *testing.T) {
	raw := []byte(`{"turns":[{"turnNumber":1,"endedAt":"2026-09-22T14:30:00Z","totalTokens":100}]}`)
	start, _ := time.Parse(time.RFC3339, "2026-09-22T14:00:00Z")
	n := parseGrokUsage(raw, start)
	if n.Total != 100 || n.Output != 0 {
		t.Fatalf("%+v", n)
	}
}
