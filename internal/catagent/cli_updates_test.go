package catagent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type cliUpdateRoundTrip func(*http.Request) (*http.Response, error)

func (f cliUpdateRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCLIUpdateVersionComparison(t *testing.T) {
	for _, c := range []struct {
		current, latest string
		want            bool
	}{
		{"codex-cli 0.162.0", "0.162.1", true},
		{"codex-cli 0.162.1", "0.162.1", false},
		{"0.163.0-alpha.1", "0.162.1", false},
		{"codex-cli 0.162.1-alpha.2 (Codex)", "0.162.1", true},
		{"1.0.50", "1.0.52", true},
		{"path", "1.2.3", false},
		{"", "1.2.3", false},
	} {
		if got := newerCLI(c.current, c.latest); got != c.want {
			t.Errorf("%q -> %q = %v", c.current, c.latest, got)
		}
	}
}

func TestCLIUpdateCheckAndNpmOwnership(t *testing.T) {
	old := cliUpdateHTTP
	t.Cleanup(func() { cliUpdateHTTP = old })
	cliUpdateHTTP = &http.Client{Transport: cliUpdateRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://registry.npmjs.org/@openai/codex/latest" {
			t.Errorf("unexpected registry %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"version":"0.162.1"}`))}, nil
	})}
	prefix := t.TempDir()
	pkgDir := filepath.Join(prefix, "node_modules", "@openai", "codex")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	a := NewCodexAdapter(CLIAgentConfig{})
	a.exe = filepath.Join(prefix, "codex.cmd")
	a.st = Status{State: StateReady, Version: "0.162.0"}
	r, err := a.CheckCLIUpdate(context.Background())
	if err != nil || !r.Available || !r.CanUpdate || r.LatestVersion != "0.162.1" {
		t.Fatalf("%+v %v", r, err)
	}
	a.exe = filepath.Join(t.TempDir(), "desktop", "codex.exe")
	r, err = a.CheckCLIUpdate(context.Background())
	if err != nil || r.CanUpdate || r.UpdateURL == "" {
		t.Fatalf("bundled executable must use its installer: %+v %v", r, err)
	}
	if err := a.UpdateCLI(context.Background()); err == nil {
		t.Fatal("must not update unrelated global npm package")
	}
}

func TestCLIUpdateCommandTargetsDetectedPrefix(t *testing.T) {
	prefix := t.TempDir()
	pkgDir := filepath.Join(prefix, "node_modules", "@openai", "codex")
	_ = os.MkdirAll(pkgDir, 0755)
	_ = os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{}`), 0644)
	var got []string
	a := NewCodexAdapter(CLIAgentConfig{
		LookPath: func(string) (string, error) { return "npm-test.exe", nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			got = append([]string{name}, args...)
			return cliUpdateHelper(ctx, "ok")
		},
	})
	a.exe = filepath.Join(prefix, "codex.cmd")
	if err := a.UpdateCLI(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"npm-test.exe", "install", "--global", "--prefix", prefix, "@openai/codex@latest", "--no-audit", "--no-fund"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v != %v", got, want)
	}
}

func TestCLIUpdateGrokNativeCheck(t *testing.T) {
	a := NewBuildAdapter(BuildConfig{Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if strings.Join(args, " ") != "update --check --json" {
			t.Fatalf("mutating check args: %v", args)
		}
		return cliUpdateHelper(ctx, "grok")
	}})
	a.exe = "grok-test.exe"
	a.st = Status{State: StateReady, Version: "1.0.50"}
	r, err := a.CheckCLIUpdate(context.Background())
	if err != nil || !r.Available || r.LatestVersion != "1.0.52" {
		t.Fatalf("%+v %v", r, err)
	}
}

func cliUpdateHelper(ctx context.Context, mode string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCLIUpdateHelper$")
	cmd.Env = append(os.Environ(), "FFMPEGFREE_CLI_UPDATE_TEST="+mode)
	return cmd
}

func TestCLIUpdateHelper(t *testing.T) {
	mode := os.Getenv("FFMPEGFREE_CLI_UPDATE_TEST")
	if mode == "" {
		return
	}
	if mode == "grok" {
		fmt.Print(`{"currentVersion":"1.0.50","latestVersion":"1.0.52","updateAvailable":true,"installer":"internal","channel":"stable","error":null}`)
	}
	if mode == "fail" {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestCLIUpdateOldGrokFallback(t *testing.T) {
	old := cliUpdateHTTP
	t.Cleanup(func() { cliUpdateHTTP = old })
	cliUpdateHTTP = &http.Client{Transport: cliUpdateRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://x.ai/cli/stable" {
			t.Fatalf("unexpected release source: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("1.0.52\n"))}, nil
	})}
	a := NewBuildAdapter(BuildConfig{Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd { return cliUpdateHelper(ctx, "fail") }})
	a.exe = "grok-test.exe"
	a.st = Status{State: StateReady, Version: "1.0.10"}
	r, err := a.CheckCLIUpdate(context.Background())
	if err != nil || !r.Available || r.LatestVersion != "1.0.52" {
		t.Fatalf("%+v %v", r, err)
	}
}
