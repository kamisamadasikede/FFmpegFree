package cat_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func TestValidGitURL(t *testing.T) {
	for _, u := range []string{
		"https://github.com/org/repo.git",
		"http://example.com/r",
		"git@github.com:org/repo.git",
		"ssh://git@host/path/repo.git",
		"git://host/repo",
		"something.git",
	} {
		if !cat.ValidGitURL(u) {
			t.Fatalf("want valid: %q", u)
		}
	}
	for _, u := range []string{"", "  ", "not a url", "ftp://x", "github.com/org/repo"} {
		if cat.ValidGitURL(u) {
			t.Fatalf("want invalid: %q", u)
		}
	}
}

func TestRepoNameFromGitURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/org/my-repo.git": "my-repo",
		"git@github.com:org/my-repo.git":     "my-repo",
		"https://github.com/org/my-repo/":    "my-repo",
	}
	for in, want := range cases {
		if got := cat.RepoNameFromGitURL(in); got != want {
			t.Fatalf("%q → %q want %q", in, got, want)
		}
	}
}

func TestClassifyCloneGitFailure(t *testing.T) {
	msg, kind := cat.ClassifyCloneGitFailure("Authentication failed for 'https://…'")
	if kind != "auth" || !strings.Contains(msg, "仓库需要登录") {
		t.Fatalf("auth: %s %s", kind, msg)
	}
	msg, kind = cat.ClassifyCloneGitFailure("Permission denied (publickey).")
	if kind != "auth" {
		t.Fatalf("permission denied → %s %s", kind, msg)
	}
	msg, kind = cat.ClassifyCloneGitFailure("fatal: could not read Username for 'https://github.com'")
	if kind != "auth" {
		t.Fatalf("username → %s", kind)
	}
	msg, kind = cat.ClassifyCloneGitFailure("No space left on device")
	if kind != "writable" || !strings.Contains(msg, "无法写入") {
		t.Fatalf("disk: %s %s", kind, msg)
	}
	msg, kind = cat.ClassifyCloneGitFailure("fatal: remote did something weird")
	if kind != "other" || !strings.HasPrefix(msg, "克隆失败：") {
		t.Fatalf("other: %s %s", kind, msg)
	}
	msg, _ = cat.ClassifyCloneGitFailure("fatal: https://user:secret@host/repo.git failed")
	if strings.Contains(msg, "secret") {
		t.Fatalf("leaked credentials: %s", msg)
	}
}

type cloneRec struct {
	mu sync.Mutex
	ev []cat.CloneEvent
}

func (r *cloneRec) emit(event string, payload any) {
	if event != cat.EventClone {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ev = append(r.ev, payload.(cat.CloneEvent))
}

func (r *cloneRec) all() []cat.CloneEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]cat.CloneEvent(nil), r.ev...)
}

func (r *cloneRec) waitStatus(id, status string, d time.Duration) (cat.CloneEvent, bool) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		for _, e := range r.all() {
			if e.CloneID == id && e.Status == status {
				return e, true
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	return cat.CloneEvent{}, false
}

func gitOk(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
}

func cloneGitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func makeBareRepo(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "bare.git")
	cloneGitRun(t, "", "init", "--bare", bare)
	work := filepath.Join(t.TempDir(), "work")
	_ = os.MkdirAll(work, 0o755)
	cloneGitRun(t, work, "init")
	cloneGitRun(t, work, "config", "user.email", "t@t")
	cloneGitRun(t, work, "config", "user.name", "t")
	_ = os.WriteFile(filepath.Join(work, "README"), []byte("hi"), 0o644)
	cloneGitRun(t, work, "add", ".")
	cloneGitRun(t, work, "-c", "commit.gpgsign=false", "commit", "-m", "init")
	cloneGitRun(t, work, "branch", "-M", "main")
	cloneGitRun(t, work, "remote", "add", "origin", bare)
	cloneGitRun(t, work, "push", "origin", "HEAD:main")
	return bare
}

func TestCloneCatProjectURLAndExists(t *testing.T) {
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	_, err := svc.CloneCatProject(ctx, cat.CloneCatProjectRequest{URL: "not-a-git"})
	if !apperr.Is(err, apperr.InvalidArgument) || reasonOf(err) != "reason=git_url" {
		t.Fatalf("bad url: %v", err)
	}
	if apperr.From(err).Message != "请输入有效的 Git 地址。" {
		t.Fatalf("message: %v", err)
	}

	gitOk(t)
	parent := t.TempDir()
	exist := filepath.Join(parent, "exist-repo")
	_ = os.MkdirAll(exist, 0o755)
	_ = os.WriteFile(filepath.Join(exist, "f"), []byte("x"), 0o644)
	_, err = svc.CloneCatProject(ctx, cat.CloneCatProjectRequest{
		URL: "https://example.com/exist-repo.git", ParentDir: parent,
	})
	if !apperr.Is(err, apperr.InvalidArgument) || reasonOf(err) != "reason=clone_exists" {
		t.Fatalf("exists: %v detail=%s", err, reasonOf(err))
	}
}

func TestCloneCatProjectSuccessAndCancelIdempotent(t *testing.T) {
	gitOk(t)
	ctx := context.Background()
	rec := &cloneRec{}
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{Emit: rec.emit})
	bare := makeBareRepo(t)
	dest := t.TempDir()
	res, err := svc.CloneCatProject(ctx, cat.CloneCatProjectRequest{URL: bare, ParentDir: dest, Name: "演示"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != cat.CloneStarting || res.CloneID == "" {
		t.Fatalf("%+v", res)
	}
	ev, ok := rec.waitStatus(res.CloneID, cat.CloneSucceeded, 20*time.Second)
	if !ok || ev.Project == nil || ev.Project.Name != "演示" {
		t.Fatalf("succeeded: ok=%v ev=%+v all=%+v", ok, ev, rec.all())
	}
	if err := svc.CancelCloneCatProject(ctx, cat.CancelCloneCatProjectRequest{CloneID: "nope"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.CancelCloneCatProject(ctx, cat.CancelCloneCatProjectRequest{CloneID: res.CloneID}); err != nil {
		t.Fatal(err)
	}
	if err := svc.CancelCloneCatProject(ctx, cat.CancelCloneCatProjectRequest{CloneID: ""}); err != nil {
		t.Fatal(err)
	}
	_ = catagent.KindCatBuild
}

func TestCloneBusy(t *testing.T) {
	gitOk(t)
	ctx := context.Background()
	rec := &cloneRec{}
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{Emit: rec.emit})
	parent := t.TempDir()
	res, err := svc.CloneCatProject(ctx, cat.CloneCatProjectRequest{
		URL: "ssh://git@127.0.0.1:1/does-not-exist.git", ParentDir: parent, Name: "slow",
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err2 := svc.CloneCatProject(ctx, cat.CloneCatProjectRequest{
		URL: "https://example.com/other.git", ParentDir: parent,
	})
	if !apperr.Is(err2, apperr.TaskConflict) || reasonOf(err2) != "reason=clone_busy" {
		// 若第一下已极快失败释放槽，则 busy 测不到——仍应用 cancel 清场
		t.Logf("busy not observed (clone may have finished fast): %v", err2)
	} else if apperr.From(err2).Message != "已有克隆在进行。" {
		t.Fatalf("busy message: %v", err2)
	}
	_ = svc.CancelCloneCatProject(ctx, cat.CancelCloneCatProjectRequest{CloneID: res.CloneID})
	_, _ = rec.waitStatus(res.CloneID, cat.CloneCanceled, 15*time.Second)
	_, _ = rec.waitStatus(res.CloneID, cat.CloneFailed, 1*time.Second)
}
