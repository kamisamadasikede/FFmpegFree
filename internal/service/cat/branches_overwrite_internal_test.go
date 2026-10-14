package cat

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 以下 stderr 是 git 2.47.3（LC_ALL=C）真实输出。
const (
	stderrLocalCheckout = "error: Your local changes to the following files would be overwritten by checkout:\n\ta\nPlease commit your changes or stash them before you switch branches.\nAborting\n"
	stderrLocalMerge    = "error: Your local changes to the following files would be overwritten by merge:\n\ta\nPlease commit your changes or stash them before you merge.\nAborting\n"
	stderrUntracked     = "error: The following untracked working tree files would be overwritten by checkout:\n\tu\nPlease move or remove them before you switch branches.\nAborting\n"
)

func TestGitWouldOverwrite(t *testing.T) {
	for _, s := range []string{stderrLocalCheckout, stderrLocalMerge, stderrUntracked, "ERROR: ... WOULD BE OVERWRITTEN BY CHECKOUT"} {
		if !gitWouldOverwrite(s) {
			t.Fatalf("should match: %q", s)
		}
	}
	for _, s := range []string{"", "fatal: invalid reference: nope", "error: pathspec 'x' did not match any file(s) known to git", "fatal: not a git repository"} {
		if gitWouldOverwrite(s) {
			t.Fatalf("should not match: %q", s)
		}
	}
}

func gitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// 在真仓库里绕过预检直接 switch，复现三种 stderr，确认 LC_ALL=C 下都能识别。
func TestGitWouldOverwriteRealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	ctx := context.Background()
	dir := t.TempDir()
	w := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, dir, "init", "-q", "-b", "main")
	gitT(t, dir, "config", "user.email", "t@example.com")
	gitT(t, dir, "config", "user.name", "t")
	gitT(t, dir, "config", "core.autocrlf", "false")
	w("a.txt", "one\n")
	gitT(t, dir, "add", "a.txt")
	gitT(t, dir, "commit", "-q", "-m", "init")
	gitT(t, dir, "switch", "-q", "-c", "dev")
	w("a.txt", "two\n")
	w("u.txt", "tracked on dev\n")
	gitT(t, dir, "add", "a.txt", "u.txt")
	gitT(t, dir, "commit", "-q", "-m", "dev")
	gitT(t, dir, "switch", "-q", "main")

	// 本地改动会被覆盖
	w("a.txt", "local\n")
	_, stderr, err := runGitEnv(ctx, gitAddTimeout, dir, gitCLocale, "switch", "dev")
	if err == nil || !gitWouldOverwrite(stderr) || !strings.Contains(stderr, "Your local changes") {
		t.Fatalf("local: err=%v stderr=%q", err, stderr)
	}
	gitT(t, dir, "checkout", "--", "a.txt")

	// 未跟踪文件会被覆盖
	w("u.txt", "untracked\n")
	_, stderr, err = runGitEnv(ctx, gitAddTimeout, dir, gitCLocale, "switch", "dev")
	if err == nil || !gitWouldOverwrite(stderr) || !strings.Contains(stderr, "untracked working tree files") {
		t.Fatalf("untracked: err=%v stderr=%q", err, stderr)
	}
}

// gitCLocale 确实压过外部的 LANG / LC_ALL（用 ! 别名打印子进程环境）。
func TestRunGitEnvForcesCLocale(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LANG", "zh_CN.UTF-8")
	out, stderr, err := runGitEnv(context.Background(), gitStatusTimeout, t.TempDir(), gitCLocale, "-c", "alias.pe=!echo \"$LC_ALL|$LANG\"", "pe")
	if err != nil {
		t.Fatalf("%v %s", err, stderr)
	}
	if got := strings.TrimSpace(string(out)); got != "C|C" {
		t.Fatalf("env: %q", got)
	}
}
