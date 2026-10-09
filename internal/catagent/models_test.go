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
	"time"
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

func TestParseGrokModelsList_FailuresGiveEmpty(t *testing.T) {
	for _, out := range []string{
		"",
		"Error: Not signed in. To authenticate without a browser, run:\n  grok login --device-code\n",
		"Not signed in\nsomething went wrong\n",
		"You are not authenticated.\n\nDefault model: grok-4.6\n",
	} {
		if got := parseGrokModelsList(out); len(got) != 0 {
			t.Fatalf("%q → %v, want empty", out, got)
		}
	}
}

func fakeModelsAdapter(t *testing.T, body string) *BuildAdapter {
	t.Helper()
	fake := filepath.Join(t.TempDir(), "grok")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
}

func TestProbeGrokModels_FailureNoFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	cases := map[string]string{
		"error_exit":   "echo 'Error: boom' >&2; exit 1\n",
		"unrecognized": "echo 'hello there'\n",
	}
	for name, body := range cases {
		a := fakeModelsAdapter(t, body)
		a.detect()
		models, err := a.ListModels()
		if err != nil || models == nil || len(models) != 0 {
			t.Fatalf("%s: models %+v %v", name, models, err)
		}
	}
}

func TestProbeGrokModels_TimeoutEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	old := modelsProbeTimeout
	modelsProbeTimeout = 200 * time.Millisecond
	defer func() { modelsProbeTimeout = old }()
	// 先吐出可解析的列表再卡住：超时按失败处理，不用半截输出。
	a := fakeModelsAdapter(t, "if [ \"$1\" = models ]; then printf 'Available models:\\n  * grok-x\\n'; exec sleep 5; fi\n")
	a.detect()
	models, err := a.ListModels()
	if err != nil || len(models) != 0 {
		t.Fatalf("models %+v %v", models, err)
	}
}
