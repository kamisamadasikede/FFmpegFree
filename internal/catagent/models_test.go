package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// CLI 1.0.40 未登录时 `grok models` 的真实输出。
const grokModelsOut = "You are not authenticated.\n\nDefault model: grok-4.6\n\nAvailable models:\n  * grok-4.6 (default)\n  - grok-4.5\n"

func TestParseGrokModelsList_RealOutput(t *testing.T) {
	got := parseGrokModelsList(grokModelsOut)
	if want := []string{"grok-4.6", "grok-4.5"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestParseGrokModelsList_DefaultFirst(t *testing.T) {
	out := "Default model: grok-b\n\nAvailable models:\n  - grok-a\n  * grok-b (default)\n  - grok-c\n"
	got := parseGrokModelsList(out)
	if want := []string{"grok-b", "grok-a", "grok-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestProbeGrokModels_RealNamesNoMapping(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	fake := filepath.Join(t.TempDir(), "grok")
	script := "#!/bin/sh\nif [ \"$1\" = models ]; then printf '%s' '" + strings.ReplaceAll(grokModelsOut, "'", "") + "'; fi\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	a.detect()
	models, err := a.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	want := []Model{{ID: "grok-4.6", DisplayName: "grok-4.6"}, {ID: "grok-4.5", DisplayName: "grok-4.5"}}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("got %+v want %+v", models, want)
	}
	for _, m := range models {
		if strings.Contains(m.DisplayName, "Cat 助手") {
			t.Fatalf("mapped display name leaked: %+v", m)
		}
	}
}
