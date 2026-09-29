package system

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"FFmpegFree/internal/apperr"
)

func TestRevealCommand(t *testing.T) {
	cases := []struct {
		goos, path string
		dir        bool
		name       string
		args       []string
	}{
		{"windows", `C:\a b\x.mp4`, false, "explorer", []string{`/select,C:\a b\x.mp4`}},
		{"windows", `C:\a b`, true, "explorer", []string{`C:\a b`}},
		{"darwin", "/Users/a/x.mp4", false, "open", []string{"-R", "/Users/a/x.mp4"}},
		{"darwin", "/Users/a", true, "open", []string{"/Users/a"}},
		{"linux", "/home/a/x.mp4", false, "xdg-open", []string{"/home/a"}},
		{"linux", "/home/a", true, "xdg-open", []string{"/home/a"}},
	}
	for _, c := range cases {
		n, a := revealCommand(c.goos, c.path, c.dir)
		if n != c.name || !reflect.DeepEqual(a, c.args) {
			t.Errorf("%v: got %s %q", c, n, a)
		}
	}
}

func TestRevealInFolderValidation(t *testing.T) {
	called := false
	start := func(string, ...string) error { called = true; return nil }
	code := func(err error) apperr.Code {
		var ae *apperr.AppError
		if errors.As(err, &ae) {
			return ae.Code
		}
		return ""
	}
	for _, p := range []string{"", "rel/x.mp4", "x.mp4"} {
		if c := code(revealIn("linux", start, p)); c != apperr.InvalidArgument {
			t.Errorf("%q: %v", p, c)
		}
	}
	missing := filepath.Join(t.TempDir(), "nope.mp4")
	if c := code(revealIn("linux", start, missing)); c != apperr.NotFound {
		t.Errorf("missing: %v", c)
	}
	if called {
		t.Fatal("校验失败不应启动命令")
	}
	f := filepath.Join(t.TempDir(), "中 文 a.mp4")
	os.WriteFile(f, []byte("x"), 0o644)
	var gotName string
	var gotArgs []string
	err := revealIn("linux", func(n string, a ...string) error { gotName, gotArgs = n, a; return nil }, f)
	if err != nil || gotName != "xdg-open" || gotArgs[0] != filepath.Dir(f) {
		t.Fatalf("%v %s %v", err, gotName, gotArgs)
	}
	// 启动失败
	err = revealIn("linux", func(string, ...string) error { return errors.New("boom") }, f)
	if code(err) != apperr.ProcessFailed {
		t.Fatalf("%v", err)
	}
}

func TestAppContextNilBeforeStart(t *testing.T) {
	if NewManager().AppContext() != nil {
		t.Fatal("Start 之前应为 nil")
	}
}
