package officecli

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSafeHTMLRemovesActiveContent(t *testing.T) {
	raw := `<html><head><script>alert(1)</script><link href="http://evil"></head><body><div class="slide" data-path="/slide[1]" style="color:red"><img src="data:image/png;base64,AAAA" onerror="alert(1)"><img src="http://evil"><a href="javascript:alert(1)">Label</a><iframe srcdoc="x"></iframe><svg><foreignObject><script>bad()</script><div onclick="bad()">Safe</div></foreignObject></svg></div></body></html>`
	s, err := SafeHTML(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script", "<link", "<iframe", "srcdoc=", "onerror=", "onclick=", "http://evil", "javascript:"} {
		if strings.Contains(s, bad) {
			t.Fatalf("active content survived: %s", bad)
		}
	}
	for _, good := range []string{`class="slide"`, `data-path="/slide[1]"`, `color:red`, `data:image/png`, "Safe"} {
		if !strings.Contains(s, good) {
			t.Fatalf("lost native layout %q", good)
		}
	}
}

func TestPreviewLeasesInstallAndBounds(t *testing.T) {
	var installs atomic.Int32
	m := New(Config{Install: func(ctx context.Context) (string, error) { installs.Add(1); <-ctx.Done(); return "", ctx.Err() }})
	for i := 0; i < 10; i++ {
		m.Tool(false)
	}
	deadline := time.Now().Add(time.Second)
	for installs.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if installs.Load() != 1 {
		t.Fatalf("duplicate installers %d", installs.Load())
	}
	root := t.TempDir()
	a, _ := m.Start(filepath.Join(root, "a.pptx"), func() error { return nil })
	b, _ := m.Start(filepath.Join(root, "a.pptx"), func() error { return nil })
	if len(m.sessions) != 1 || a.ID == b.ID {
		t.Fatal("leases must share a worker but use distinct IDs")
	}
	m.Release(a.ID)
	if _, err := m.Read(b.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Read(a.ID, 0); err == nil {
		t.Fatal("released lease readable")
	}
	for i := 0; i < 7; i++ {
		if _, err := m.Start(filepath.Join(root, fmt.Sprintf("%d.pptx", i)), func() error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Start(filepath.Join(root, "overflow.pptx"), func() error { return nil }); err == nil {
		t.Fatal("unbounded preview processes")
	}
	m.Close()
	if _, err := m.Start(filepath.Join(root, "closed.pptx"), func() error { return nil }); err == nil {
		t.Fatal("start after close")
	}
}

func TestPreviewZIPBoundsAndPartialSaves(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.pptx")
	if _, err := stableRevision(p); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	os.WriteFile(p, []byte("PK partial"), 0600)
	if _, err := stableRevision(p); err == nil {
		t.Fatal("accepted incomplete ZIP")
	}
	f, _ := os.Create(p)
	z := zip.NewWriter(f)
	w, _ := z.Create("ppt/presentation.xml")
	w.Write([]byte("<presentation/>"))
	z.Close()
	f.Close()
	if rev, err := stableRevision(p); err != nil || rev == "" {
		t.Fatalf("%q %v", rev, err)
	}
}

// Opt in with OFFICECLI_TEST_BINARY pointing to an installed real executable.
// This tests actual CLI -> Watch/SSE -> manager snapshots, and an external ZIP
// replacement for the fallback path. No model credentials or paid API are used.
func TestOfficeCLIIntegration(t *testing.T) {
	exe := os.Getenv("OFFICECLI_TEST_BINARY")
	if exe == "" {
		t.Skip("set OFFICECLI_TEST_BINARY for native integration")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "live.pptx")
	m := New(Config{Executable: exe})
	defer m.Close()
	p, err := m.Start(file, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	read := func(want string, version uint64) Preview {
		t.Helper()
		deadline := time.Now().Add(25 * time.Second)
		var p Preview
		for time.Now().Before(deadline) {
			p, err = m.Read(pID(m), version)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(p.HTML, want) && p.Version > version {
				return p
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %q: %+v", want, p)
		return p
	}
	run := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for attempt := 0; attempt < 4; attempt++ {
			out, e := Command(ctx, exe, args...).CombinedOutput()
			if e == nil {
				return
			}
			if attempt < 3 && strings.Contains(string(out), "being used by another process") {
				time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
				continue
			}
			t.Fatalf("%v: %v %s", args, e, out)
		}
	}
	run("create", file)
	run("add", file, "/", "--type", "slide", "--prop", "title=First visible slide")
	p = read("First visible slide", 0)
	if p.Mode != "watch" {
		t.Fatalf("Watch did not start: %+v", p)
	}
	started := time.Now()
	run("add", file, "/", "--type", "slide", "--prop", "title=Second incremental slide")
	p = read("Second incremental slide", p.Version)
	t.Logf("CLI write to preview snapshot: %s", time.Since(started))
	run("set", file, "/slide[1]/shape[1]", "--prop", "size=52")
	p = read("font-size:52pt", p.Version)
	run("add", file, "/", "--type", "slide", "--prop", "title=Summary")
	p = read("Summary", p.Version)
	run("move", file, "/slide[3]", "--to", "/", "--index", "1")
	run("remove", file, "/slide[2]")
	run("save", file)
	z, e := zip.OpenReader(file)
	if e != nil {
		t.Fatal(e)
	}
	editable := false
	for _, f := range z.File {
		if f.Name == "ppt/slides/slide1.xml" {
			r, _ := f.Open()
			var b strings.Builder
			buf := make([]byte, 64<<10)
			n, _ := r.Read(buf)
			b.Write(buf[:n])
			r.Close()
			editable = strings.Contains(b.String(), "<p:sp") && strings.Contains(b.String(), "<a:t>")
		}
	}
	z.Close()
	if !editable {
		t.Fatal("output lacks native editable shapes/text")
	}
	other := filepath.Join(dir, "external.pptx")
	run("create", other)
	run("add", other, "/", "--type", "slide", "--prop", "title=External replacement")
	raw, _ := os.ReadFile(other)
	if err = os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	p = read("External replacement", p.Version)
	if p.Mode != "static" {
		t.Fatalf("external write did not use disk renderer: %s", p.Mode)
	}
	// A real Vue/iframe check can join the same native manager via a test-only HTTP bridge.
	if os.Getenv("FFC_PPT_BROWSER_QA") == "1" {
		root, _ := filepath.Abs(filepath.Join("..", ".."))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:4182")
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/start":
				p, e := m.Start(file, func() error { return nil })
				if e != nil {
					http.Error(w, e.Error(), 500)
					return
				}
				json.NewEncoder(w).Encode(p)
			case "/read":
				v, _ := strconv.ParseUint(r.URL.Query().Get("version"), 10, 64)
				p, e := m.Read(r.URL.Query().Get("id"), v)
				if e != nil {
					http.Error(w, e.Error(), 404)
					return
				}
				json.NewEncoder(w).Encode(p)
			case "/release":
				m.Release(r.URL.Query().Get("id"))
				json.NewEncoder(w).Encode(true)
			case "/edit":
				step := r.URL.Query().Get("step")
				var args []string
				switch step {
				case "add":
					args = []string{"add", file, "/", "--type", "slide", "--prop", "title=Browser live update"}
				case "set":
					args = []string{"set", file, "/slide[2]/shape[1]", "--prop", "size=60"}
				default:
					http.Error(w, "unknown edit", 400)
					return
				}
				if output, e := Command(r.Context(), exe, args...).CombinedOutput(); e != nil {
					http.Error(w, string(output), 500)
					return
				}
				json.NewEncoder(w).Encode(true)
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()
		c := exec.Command("node", filepath.Join(root, "frontend", "scripts", "check-ppt-live.cjs"))
		c.Dir = root
		c.Env = append(os.Environ(), "FFC_PPT_QA_URL="+server.URL, "FFC_PPT_QA_FILE="+file)
		if output, e := c.CombinedOutput(); e != nil {
			t.Fatalf("browser: %v\n%s", e, output)
		} else {
			t.Log(string(output))
		}
	}
	m.Release(p.ID)
}
func pID(m *Manager) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.leases {
		return id
	}
	return ""
}
