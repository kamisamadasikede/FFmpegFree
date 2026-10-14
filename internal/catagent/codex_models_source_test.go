package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// Codex 模型列表来自 CLI：先取刷新后的目录（与 /model 一致），stderr 警告不影响解析；拿不到才用 --bundled。
func TestCodexModels_RefreshedCatalogFirst_StderrIgnored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "codex")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "codex-cli 0.162.1"; exit 0; fi
if [ "$1" = "debug" ] && [ "$3" = "--bundled" ]; then echo '{"models":[{"slug":"bundled-only","visibility":"list"}]}'; exit 0; fi
if [ "$1" = "debug" ]; then echo "WARN refresh via relay" >&2; echo '{"models":[{"slug":"relay-live","visibility":"list"}]}'; exit 0; fi
exit 1
`
	_ = os.WriteFile(fake, []byte(script), 0o755)
	a := NewCodexAdapter(CLIAgentConfig{
		DataTemp: t.TempDir(),
		LookPath: func(file string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake, args...)
		},
	})
	got := a.probeCodexModels(fake)
	if len(got) != 1 || got[0].ID != "relay-live" {
		t.Fatalf("want refreshed catalog, got %+v", got)
	}
	// 刷新失败 → 内置目录
	script2 := `#!/bin/sh
if [ "$3" = "--bundled" ]; then echo '{"models":[{"slug":"bundled-only","visibility":"list"}]}'; exit 0; fi
exit 1
`
	_ = os.WriteFile(fake, []byte(script2), 0o755)
	got = a.probeCodexModels(fake)
	if len(got) != 1 || got[0].ID != "bundled-only" {
		t.Fatalf("want bundled fallback, got %+v", got)
	}
}
