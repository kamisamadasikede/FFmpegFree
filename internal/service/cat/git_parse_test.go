package cat

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidBranch(t *testing.T) {
	for _, n := range []string{"main", "feature/login", "v2"} {
		if !validBranch(n) {
			t.Fatalf("want ok %q", n)
		}
	}
	for _, n := range []string{"", "-x", "a..b", "a b", "a~b", "HEAD.lock", "@{bad}", "a//b"} {
		if validBranch(n) {
			t.Fatalf("want reject %q", n)
		}
	}
}

func TestReservedDevice(t *testing.T) {
	if runtime.GOOS != "windows" {
		if reservedDevice("nul") || reservedDevice("con") {
			t.Fatal("non-windows should stage these names")
		}
		return
	}
	for _, n := range []string{"nul", "NUL", `dir\nul.txt`, "foo/NUL", "com1", "lpt9", "aux.", "prn "} {
		if !reservedDevice(n) {
			t.Fatalf("want reserved %q", n)
		}
	}
	for _, n := range []string{".env", "nginx.conf", "deploy.bat", "file.nul", "null", "concourse", "readme"} {
		if reservedDevice(n) {
			t.Fatalf("not reserved %q", n)
		}
	}
}

func TestParseStatusZBranchAndRename(t *testing.T) {
	raw := []byte("## v2...origin/v2 [ahead 7, behind 1]\x00 M frontend/vite.config.ts\x00M  wails.json\x00R  newname.ts\x00old.ts\x00?? new.txt\x00")
	branch, upstream, ahead, behind, entries := parseStatusZ(raw)
	if branch != "v2" || upstream != "v2" || ahead != 7 || behind != 1 {
		t.Fatalf("branch: %s %s +%d -%d", branch, upstream, ahead, behind)
	}
	if len(entries) != 4 {
		t.Fatalf("entries: %+v", entries)
	}
	want := []statusEntry{
		{letter: "M", path: "frontend/vite.config.ts", unstaged: true},
		{letter: "M", path: "wails.json", staged: true},
		{letter: "R", path: "newname.ts", staged: true},
		{letter: "U", path: "new.txt", unstaged: true},
	}
	for i, w := range want {
		g := entries[i]
		if g.letter != w.letter || g.path != w.path || g.staged != w.staged || g.unstaged != w.unstaged {
			t.Fatalf("%d: got %+v want %+v", i, g, w)
		}
	}
}

func TestParseBranchForms(t *testing.T) {
	b, up, a, be := parseBranch("## HEAD (no branch)")
	if b != "HEAD" || up != "" || a != 0 || be != 0 {
		t.Fatalf("detached: %s %s %d %d", b, up, a, be)
	}
	b, up, a, be = parseBranch("## No commits yet on main")
	if b != "main" || up != "" {
		t.Fatalf("empty repo: %s %s", b, up)
	}
	b, up, a, be = parseBranch("## feature...origin/feature/foo [ahead 2]")
	if b != "feature" || up != "feature/foo" || a != 2 || be != 0 {
		t.Fatalf("slash upstream: %s %s %d %d", b, up, a, be)
	}
}

func TestParseNumstatZRename(t *testing.T) {
	raw := []byte("1\t0\ta.txt\x000\t0\t\x00old.ts\x00newname.ts\x006\t1\tfrontend/vite.config.ts\x00")
	got := parseNumstatZ(raw)
	if got["a.txt"] != [2]int{1, 0} || got["newname.ts"] != [2]int{0, 0} || got["frontend/vite.config.ts"] != [2]int{6, 1} {
		t.Fatalf("%v", got)
	}
	if _, ok := got["old.ts"]; ok {
		t.Fatalf("old path should not be the key: %v", got)
	}
	if numstatPath("dir/{old => new}.ts") != "dir/new.ts" {
		t.Fatal(numstatPath("dir/{old => new}.ts"))
	}
}

func TestParseGitHubRemote(t *testing.T) {
	cases := []struct {
		raw, owner, repo string
		ok               bool
	}{
		{"https://github.com/octocat/Hello-World.git", "octocat", "Hello-World", true},
		{"git@github.com:octocat/Hello-World.git", "octocat", "Hello-World", true},
		{"ssh://git@github.com/octocat/Hello-World.git", "octocat", "Hello-World", true},
		{"https://www.github.com/octocat/Hello-World", "octocat", "Hello-World", true},
		{"https://gitlab.com/octocat/Hello-World.git", "", "", false},
		{"https://github.com.evil/octocat/Hello-World", "", "", false},
		{"not a url", "", "", false},
	}
	for _, c := range cases {
		owner, repo, ok := parseGitHubRemote(c.raw)
		if owner != c.owner || repo != c.repo || ok != c.ok {
			t.Fatalf("%s: %s %s %v", c.raw, owner, repo, ok)
		}
	}
	if githubWeb("git@github.com:octocat/Hello-World.git") != "https://github.com/octocat/Hello-World" {
		t.Fatal(githubWeb("git@github.com:octocat/Hello-World.git"))
	}
}

func TestFromGitPathAndCleanRel(t *testing.T) {
	got, ok := fromGitPath("sub", "sub/a.txt")
	if !ok || got != "a.txt" {
		t.Fatalf("strip: %s %v", got, ok)
	}
	if _, ok := fromGitPath("sub", "other/a.txt"); ok {
		t.Fatal("outside path accepted")
	}
	if _, ok := fromGitPath("", "a.txt"); !ok {
		t.Fatal("root path rejected")
	}
	root := t.TempDir()
	if _, err := cleanRel(root, `..\outside`); err == nil {
		t.Fatal("dotdot")
	}
	if _, err := cleanRel(root, `.git/config`); err == nil {
		t.Fatal("git dir")
	}
	if _, err := cleanRel(root, `D:/abs.txt`); err == nil {
		t.Fatal("abs")
	}
	rel, err := cleanRel(root, `src/main.go`)
	if err != nil || rel != "src/main.go" {
		t.Fatalf("rel: %s %v", rel, err)
	}
}

func TestFormatUnifiedDiff(t *testing.T) {
	raw := "diff --git a/a.txt b/a.txt\nindex 111..222 100644\n--- a/a.txt\n+++ b/a.txt\n@@ -1,2 +1,3 @@\n one\n-old\n+two\n context\n\\ No newline at end of file\n"
	text, bin := formatUnifiedDiff(raw)
	if bin {
		t.Fatal("binary")
	}
	want := "@@ -1,2 +1,3 @@\n one\n-old\n+two\n context"
	if text != want {
		t.Fatalf("got %q", text)
	}

	text, bin = formatUnifiedDiff("diff --git a/a.bin b/a.bin\nindex 1..2 100644\nBinary files a/a.bin and b/a.bin differ\n")
	if !bin || text != "" {
		t.Fatalf("bin %v %q", bin, text)
	}

	text, bin = formatUnifiedDiff("diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -0,0 +1,2 @@\n+++ keep\n+ok\n")
	if bin || !strings.Contains(text, "+++ keep") || strings.Contains(text, "+++ b/a.txt") {
		t.Fatalf("header strip: %q", text)
	}

	text, _ = formatUnifiedDiff("@@ -1 +1 @@\r\n-a\r\n+b\r\n")
	if text != "@@ -1 +1 @@\n-a\n+b" {
		t.Fatalf("crlf: %q", text)
	}

	var b strings.Builder
	b.WriteString("diff --git a/a.txt b/a.txt\n@@ -0,0 +1,401 @@\n")
	for i := 0; i < 401; i++ {
		b.WriteString("+x\n")
	}
	text, bin = formatUnifiedDiff(b.String())
	if bin {
		t.Fatal("cap binary")
	}
	lines := strings.Split(text, "\n")
	if len(lines) != 401 || lines[0] != "@@ -0,0 +1,401 @@" || lines[399] != "+x" || lines[400] != "…其余已省略" {
		t.Fatalf("cap %d last %q", len(lines), lines[len(lines)-1])
	}
}

func TestDetectMergeStateFixtures(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("没有 git")
	}
	ctx := context.Background()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-m", "base")
	if got := detectMergeState(ctx, root); got != "none" {
		t.Fatalf("none: %s", got)
	}

	run("checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-m", "feature")
	run("checkout", "main")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-m", "main")

	cmd := exec.Command("git", "merge", "--no-edit", "feature")
	cmd.Dir = root
	_ = cmd.Run()
	if got := detectMergeState(ctx, root); got != "merging" {
		t.Fatalf("merging: %s", got)
	}
	run("merge", "--abort")
	if got := detectMergeState(ctx, root); got != "none" {
		t.Fatalf("after abort: %s", got)
	}
}
