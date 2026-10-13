package cat_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/cat"
)

func initGitUser(t *testing.T, dir string) {
	t.Helper()
	gitRun(t, dir, "config", "user.email", "t@example.com")
	gitRun(t, dir, "config", "user.name", "t")
	gitRun(t, dir, "config", "core.autocrlf", "false")
}

func gitRunErr(t *testing.T, dir string, args ...string) error {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("git %v: %v\n%s", args, err, out)
	}
	return err
}

func TestGitMergeStateAndConflictsOnMerge(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main")
	initGitUser(t, root)
	mustWrite(t, filepath.Join(root, "a.txt"), "base\n")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "base")

	gitRun(t, root, "checkout", "-b", "feature")
	mustWrite(t, filepath.Join(root, "a.txt"), "feature\n")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "feature")

	gitRun(t, root, "checkout", "main")
	mustWrite(t, filepath.Join(root, "a.txt"), "main\n")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "main")

	svc, id := convAt(t, root)
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	if st.MergeState != "none" || len(st.Conflicts) != 0 {
		t.Fatalf("clean: mergeState=%q conflicts=%+v", st.MergeState, st.Conflicts)
	}

	if err := gitRunErr(t, root, "merge", "--no-edit", "feature"); err == nil {
		t.Fatal("expected merge conflict")
	}

	st, err = svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	if st.MergeState != "merging" {
		t.Fatalf("mergeState=%q", st.MergeState)
	}
	if len(st.Conflicts) != 1 || st.Conflicts[0].Path != "a.txt" {
		t.Fatalf("conflicts: %+v", st.Conflicts)
	}
	if !st.Conflicts[0].Ours || !st.Conflicts[0].Theirs {
		t.Fatalf("ours/theirs: %+v", st.Conflicts[0])
	}
}

func TestGitPullConflictReason(t *testing.T) {
	requireGit(t)
	ctx := context.Background()

	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare", "-b", "main")

	a := t.TempDir()
	gitRun(t, a, "init", "-b", "main")
	initGitUser(t, a)
	mustWrite(t, filepath.Join(a, "a.txt"), "base\n")
	gitRun(t, a, "add", "a.txt")
	gitRun(t, a, "commit", "-m", "base")
	gitRun(t, a, "remote", "add", "origin", bare)
	gitRun(t, a, "push", "-u", "origin", "main")

	b := t.TempDir()
	gitRun(t, b, "-c", "core.autocrlf=false", "clone", "-b", "main", bare, ".")
	initGitUser(t, b)
	gitRun(t, b, "config", "pull.rebase", "false")

	mustWrite(t, filepath.Join(a, "a.txt"), "from-a\n")
	gitRun(t, a, "add", "a.txt")
	gitRun(t, a, "commit", "-m", "a")
	gitRun(t, a, "push")

	mustWrite(t, filepath.Join(b, "a.txt"), "from-b\n")
	gitRun(t, b, "add", "a.txt")
	gitRun(t, b, "commit", "-m", "b")

	svc, id := convAt(t, b)
	err := svc.GitCatRemote(ctx, cat.GitRemoteRequest{ConvID: id, Op: "pull"})
	if err == nil {
		t.Fatal("expected pull conflict")
	}
	if !apperr.Is(err, apperr.InvalidArgument) || reasonOf(err) != "reason=git_conflict" {
		t.Fatalf("want git_conflict, got %v detail=%q msg=%q", err, apperr.From(err).Detail, apperr.From(err).Message)
	}
	if apperr.From(err).Message != "有冲突需要解决。" {
		t.Fatalf("message=%q", apperr.From(err).Message)
	}
	st, serr := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if serr != nil {
		t.Fatal(serr)
	}
	if st.MergeState != "merging" || len(st.Conflicts) == 0 {
		t.Fatalf("after pull: %+v", st)
	}
}

func TestGitConflictResolveOursTheirsContentAndCompleteAbort(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main")
	initGitUser(t, root)
	mustWrite(t, filepath.Join(root, "a.txt"), "base\n")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "base")

	gitRun(t, root, "checkout", "-b", "feature")
	mustWrite(t, filepath.Join(root, "a.txt"), "feature\n")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "feature")

	gitRun(t, root, "checkout", "main")
	mustWrite(t, filepath.Join(root, "a.txt"), "main\n")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "main")

	svc, id := convAt(t, root)

	// resolve with no merge
	err := svc.GitCatConflictResolve(ctx, cat.GitCatConflictResolveRequest{ConvID: id, Path: "a.txt", Mode: "ours"})
	if !apperr.Is(err, apperr.InvalidArgument) || reasonOf(err) != "reason=git_conflict" {
		t.Fatalf("no merge resolve: %v", err)
	}
	if apperr.From(err).Message != "当前没有进行中的合并。" {
		t.Fatalf("msg=%q", apperr.From(err).Message)
	}

	if err := gitRunErr(t, root, "merge", "--no-edit", "feature"); err == nil {
		t.Fatal("expected conflict")
	}

	// path not in conflicts
	if err := svc.GitCatConflictResolve(ctx, cat.GitCatConflictResolveRequest{ConvID: id, Path: "missing.txt", Mode: "ours"}); reasonOf(err) != "reason=path" {
		t.Fatalf("path: %v", err)
	}

	// content hand-edit
	if err := svc.GitCatConflictResolve(ctx, cat.GitCatConflictResolveRequest{
		ConvID: id, Path: "a.txt", Mode: "content", Content: "resolved\n",
	}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil || string(body) != "resolved\n" {
		t.Fatalf("content write: %v %q", err, body)
	}
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	if st.MergeState != "merging" || len(st.Conflicts) != 0 {
		t.Fatalf("after content resolve: %+v", st)
	}
	if err := svc.GitCatConflictComplete(ctx, cat.GitCatConflictCompleteRequest{ConvID: id, Message: "Merge branch 'feature'"}); err != nil {
		t.Fatal(err)
	}
	st, err = svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || st.MergeState != "none" {
		t.Fatalf("completed: %v %+v", err, st)
	}

	// ours/theirs on a fresh conflict
	gitRun(t, root, "checkout", "-b", "other")
	mustWrite(t, filepath.Join(root, "a.txt"), "other\n")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "other")
	gitRun(t, root, "checkout", "main")
	mustWrite(t, filepath.Join(root, "a.txt"), "main2\n")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "main2")
	if err := gitRunErr(t, root, "merge", "--no-edit", "other"); err == nil {
		t.Fatal("expected conflict2")
	}
	if err := svc.GitCatConflictResolve(ctx, cat.GitCatConflictResolveRequest{ConvID: id, Path: "a.txt", Mode: "ours"}); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(filepath.Join(root, "a.txt"))
	if string(body) != "main2\n" {
		t.Fatalf("ours: %q", body)
	}
	if err := svc.GitCatConflictAbort(ctx, cat.GitCatConflictAbortRequest{ConvID: id}); err != nil {
		t.Fatal(err)
	}
	st, err = svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || st.MergeState != "none" {
		t.Fatalf("aborted: %v %+v", err, st)
	}

	// both mode
	if err := gitRunErr(t, root, "merge", "--no-edit", "other"); err == nil {
		t.Fatal("expected conflict3")
	}
	if err := svc.GitCatConflictResolve(ctx, cat.GitCatConflictResolveRequest{ConvID: id, Path: "a.txt", Mode: "theirs"}); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(filepath.Join(root, "a.txt"))
	if string(body) != "other\n" {
		t.Fatalf("theirs: %q", body)
	}
	if err := svc.GitCatConflictAbort(ctx, cat.GitCatConflictAbortRequest{ConvID: id}); err != nil {
		t.Fatal(err)
	}
	if err := gitRunErr(t, root, "merge", "--no-edit", "other"); err == nil {
		t.Fatal("expected conflict4")
	}
	if err := svc.GitCatConflictResolve(ctx, cat.GitCatConflictResolveRequest{ConvID: id, Path: "a.txt", Mode: "both"}); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(filepath.Join(root, "a.txt"))
	if !strings.Contains(string(body), "main2") || !strings.Contains(string(body), "other") {
		t.Fatalf("both: %q", body)
	}
	if strings.Contains(string(body), "<<<<<<<") {
		t.Fatalf("both still has markers: %q", body)
	}
}

func TestGitCheckoutDirtyAndForce(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main")
	initGitUser(t, root)
	mustWrite(t, filepath.Join(root, "a.txt"), "base\n")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "base")
	gitRun(t, root, "checkout", "-b", "feature")
	mustWrite(t, filepath.Join(root, "a.txt"), "feature\n")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-m", "feature")
	gitRun(t, root, "checkout", "main")

	svc, id := convAt(t, root)

	err := svc.GitCatCheckout(ctx, cat.GitCatCheckoutRequest{ConvID: id, Name: "no-such"})
	if !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("missing: %v", err)
	}
	if apperr.From(err).Message != "找不到这个分支。" {
		t.Fatalf("msg=%q", apperr.From(err).Message)
	}

	// clean checkout
	if err := svc.GitCatCheckout(ctx, cat.GitCatCheckoutRequest{ConvID: id, Name: "feature"}); err != nil {
		t.Fatal(err)
	}
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || st.Branch != "feature" {
		t.Fatalf("checkout feature: %v %+v", err, st)
	}
	if err := svc.GitCatCheckout(ctx, cat.GitCatCheckoutRequest{ConvID: id, Name: "main"}); err != nil {
		t.Fatal(err)
	}

	// dirty without force
	mustWrite(t, filepath.Join(root, "a.txt"), "dirty\n")
	err = svc.GitCatCheckout(ctx, cat.GitCatCheckoutRequest{ConvID: id, Name: "feature"})
	if !apperr.Is(err, apperr.TaskConflict) || reasonOf(err) != "reason=git_dirty" {
		t.Fatalf("dirty: %v detail=%q", err, apperr.From(err).Detail)
	}

	// force succeeds when change does not block checkout of same file differently —
	// feature has different a.txt content; plain checkout may refuse. Use a dirty *unrelated* file.
	mustWrite(t, filepath.Join(root, "a.txt"), "base\n") // restore to match main tip? main tip is base
	// Actually main tip is still "base\n" from first commit — we dirtied then restored.
	// Make an untracked/new file dirty instead.
	mustWrite(t, filepath.Join(root, "extra.txt"), "x\n")
	err = svc.GitCatCheckout(ctx, cat.GitCatCheckoutRequest{ConvID: id, Name: "feature"})
	if !apperr.Is(err, apperr.TaskConflict) || reasonOf(err) != "reason=git_dirty" {
		t.Fatalf("untracked dirty: %v", err)
	}
	if err := svc.GitCatCheckout(ctx, cat.GitCatCheckoutRequest{ConvID: id, Name: "feature", Force: true}); err != nil {
		t.Fatal(err)
	}
	st, err = svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || st.Branch != "feature" {
		t.Fatalf("force checkout: %v %+v", err, st)
	}
}

func TestGitConflictResolveContentRejectsBinary(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main")
	initGitUser(t, root)
	mustWrite(t, filepath.Join(root, "a.bin"), "base\n")
	gitRun(t, root, "add", "a.bin")
	gitRun(t, root, "commit", "-m", "base")

	gitRun(t, root, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(root, "a.bin"), []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "a.bin")
	gitRun(t, root, "commit", "-m", "feature")

	gitRun(t, root, "checkout", "main")
	if err := os.WriteFile(filepath.Join(root, "a.bin"), []byte{0x00, 0x03, 0x04}, 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "a.bin")
	gitRun(t, root, "commit", "-m", "main")

	svc, id := convAt(t, root)
	if err := gitRunErr(t, root, "merge", "--no-edit", "feature"); err == nil {
		t.Fatal("expected binary conflict")
	}
	err := svc.GitCatConflictResolve(ctx, cat.GitCatConflictResolveRequest{
		ConvID: id, Path: "a.bin", Mode: "content", Content: "nope",
	})
	if err == nil || apperr.From(err).Message != "这个文件是二进制，请选用当前或传入。" {
		t.Fatalf("binary content: %v", err)
	}
	// ours still ok
	if err := svc.GitCatConflictResolve(ctx, cat.GitCatConflictResolveRequest{ConvID: id, Path: "a.bin", Mode: "ours"}); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
