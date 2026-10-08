package system

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/apperr"
)

func TestClassifyOpenExit(t *testing.T) {
	cases := []struct {
		goos   string
		code   int
		stderr string
		noApp  bool
		ok     bool
	}{
		{"linux", 0, "", false, true},
		{"linux", 3, "", true, false},
		{"linux", 4, "", true, false},
		{"linux", 1, "xdg-open: bad\nsyntax", false, false},
		{"darwin", 1, "No application knows how to open URL file:///a.xyz", true, false},
		{"darwin", 1, "The file /a does not exist.", false, false},
		{"darwin", 0, "", false, true},
	}
	for _, c := range cases {
		err := classifyOpenExit(c.goos, c.code, c.stderr)
		if c.ok != (err == nil) || c.noApp != errors.Is(err, ErrNoApp) {
			t.Errorf("%+v: %v", c, err)
		}
	}
}

func TestOpenWithDefaultApp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mp4")
	os.WriteFile(p, []byte("x"), 0o644)
	var got []string
	m := &Manager{open: func(path string) error { got = append(got, path); return nil }}
	if err := m.OpenWithDefaultApp(p); err != nil || len(got) != 1 || got[0] != p {
		t.Fatalf("%v %v", err, got)
	}
	code := func(err error) (apperr.Code, string) {
		var ae *apperr.AppError
		if errors.As(err, &ae) {
			return ae.Code, ae.Detail
		}
		return "", ""
	}
	if c, _ := code(m.OpenWithDefaultApp("rel.mp4")); c != apperr.InvalidArgument {
		t.Fatal(c)
	}
	for _, bad := range []string{filepath.Join(dir, "nope.mp4"), dir} {
		if c, d := code(m.OpenWithDefaultApp(bad)); c != apperr.NotFound || d != "reason=file" {
			t.Fatalf("%s: %s %s", bad, c, d)
		}
	}
	m.open = func(string) error { return ErrNoApp }
	if c, d := code(m.OpenWithDefaultApp(p)); c != apperr.NotFound || d != "reason=no_app" {
		t.Fatalf("%s %s", c, d)
	}
	m.open = func(string) error { return errors.New("exec: not found") }
	if c, _ := code(m.OpenWithDefaultApp(p)); c != apperr.ProcessFailed {
		t.Fatal(c)
	}
}

// RevealRegisteredPath（转换页 RevealSource / RevealRecord 用）：路径来自表，不走 RevealInFolder 的范围白名单。
func TestRevealRegisteredPathSkipsScopeCheck(t *testing.T) {
	f := newRevealFx(t)
	p := f.file(t, filepath.Join(f.outside, "x.mp4"))
	if c := revealCode(f.mgr.RevealInFolder(p)); c != apperr.InvalidArgument {
		t.Fatalf("RevealInFolder 对未登记路径应拒绝: %v", c)
	}
	f.launched = nil
	if err := f.mgr.RevealRegisteredPath(p); err != nil || len(f.launched) != 1 {
		t.Fatalf("%v %v", err, f.launched)
	}
	if c := revealCode(f.mgr.RevealRegisteredPath(filepath.Join(f.outside, "nope.mp4"))); c != apperr.NotFound {
		t.Fatalf("%v", c)
	}
}
