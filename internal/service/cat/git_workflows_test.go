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

func wbGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
func wbWrite(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
func wbRepo(t *testing.T, initial bool) string {
	t.Helper()
	requireGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.name", "Workbench test")
	gitRun(t, dir, "config", "user.email", "workbench@example.com")
	gitRun(t, dir, "config", "core.autocrlf", "false")
	if initial {
		wbWrite(t, dir, "a.txt", "base\n")
		gitRun(t, dir, "add", "a.txt")
		gitRun(t, dir, "commit", "-m", "base")
	}
	return dir
}
func wbCommit(t *testing.T, dir, file, body, message string) string {
	t.Helper()
	wbWrite(t, dir, file, body)
	gitRun(t, dir, "add", "--", file)
	gitRun(t, dir, "commit", "-m", message)
	return wbGit(t, dir, "rev-parse", "HEAD")
}

func TestGitWorkbenchRangesAndRemote(t *testing.T) {
	ctx := context.Background()
	dir := wbRepo(t, true)
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare", "-b", "main")
	svc, id := convAt(t, dir)
	if err := svc.GitCatAddRemote(ctx, cat.GitAddRemoteRequest{ConvID: id, Name: "cloud", URL: bare}); err != nil {
		t.Fatal(err)
	}
	if err := svc.GitCatRemote(ctx, cat.GitRemoteRequest{ConvID: id, Op: "publish"}); err != nil {
		t.Fatal(err)
	}
	other := wbRepo(t, false)
	gitRun(t, other, "remote", "add", "cloud", bare)
	gitRun(t, other, "fetch", "cloud")
	gitRun(t, other, "reset", "--hard", "cloud/main")
	localFirst := wbCommit(t, dir, "local1.txt", "one\n", "local first")
	localSecond := wbCommit(t, dir, "local2.txt", "two\n", "local second")
	remoteFirst := wbCommit(t, other, "remote.txt", "remote\n", "remote first")
	gitRun(t, other, "push", "cloud", "main")
	if err := svc.GitCatRemote(ctx, cat.GitRemoteRequest{ConvID: id, Op: "fetch"}); err != nil {
		t.Fatal(err)
	}
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	if st.Ahead != 2 || st.Behind != 1 || st.RemoteName != "cloud" || st.Root == "" {
		t.Fatalf("sync: %+v", st)
	}
	page, err := svc.ListCatCommits(ctx, cat.ListCatCommitsRequest{ConvID: id, Scope: "outgoing", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Commits) != 1 || page.Commits[0].Sha != localSecond || page.Total != 2 || page.NextCursor == "" {
		t.Fatalf("outgoing: %+v", page)
	}
	next, err := svc.ListCatCommits(ctx, cat.ListCatCommitsRequest{ConvID: id, Scope: "outgoing", Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(next.Commits) != 1 || next.Commits[0].Sha != localFirst {
		t.Fatalf("next: %+v, %v", next, err)
	}
	incoming, err := svc.ListCatCommits(ctx, cat.ListCatCommitsRequest{ConvID: id, Scope: "incoming"})
	if err != nil || len(incoming.Commits) != 1 || incoming.Commits[0].Sha != remoteFirst {
		t.Fatalf("incoming: %+v, %v", incoming, err)
	}
	searched, err := svc.ListCatCommits(ctx, cat.ListCatCommitsRequest{ConvID: id, Scope: "outgoing", Query: "first"})
	if err != nil || len(searched.Commits) != 1 || searched.Commits[0].Sha != localFirst {
		t.Fatalf("search: %+v, %v", searched, err)
	}
	if _, err := svc.ListCatCommits(ctx, cat.ListCatCommitsRequest{ConvID: id, Scope: "incoming", Cursor: page.NextCursor}); err == nil {
		t.Fatal("cross-scope cursor accepted")
	}
	wbCommit(t, other, "remote2.txt", "remote2\n", "remote second")
	gitRun(t, other, "push", "cloud", "main")
	gitRun(t, dir, "fetch", "cloud")
	if _, err := svc.ListCatCommits(ctx, cat.ListCatCommitsRequest{ConvID: id, Scope: "outgoing", Cursor: page.NextCursor}); err == nil {
		t.Fatal("stale cursor accepted")
	}
}

func TestGitWorkbenchPartialDiffAndDiscard(t *testing.T) {
	ctx := context.Background()
	dir := wbRepo(t, true)
	svc, id := convAt(t, dir)
	wbWrite(t, dir, "a.txt", "staged\n")
	gitRun(t, dir, "add", "a.txt")
	wbWrite(t, dir, "a.txt", "worktree\n")
	staged, err := svc.GitCatDiff(ctx, cat.GitDiffRequest{ConvID: id, RelPath: "a.txt", Scope: "staged"})
	if err != nil || !strings.Contains(staged.Text, "+staged") || strings.Contains(staged.Text, "+worktree") {
		t.Fatalf("staged: %+v, %v", staged, err)
	}
	work, err := svc.GitCatDiff(ctx, cat.GitDiffRequest{ConvID: id, RelPath: "a.txt", Scope: "worktree"})
	if err != nil || !strings.Contains(work.Text, "-staged") || !strings.Contains(work.Text, "+worktree") {
		t.Fatalf("worktree: %+v, %v", work, err)
	}
	wbWrite(t, dir, "[selected].txt", "selected\n")
	wbWrite(t, dir, "s.txt", "keep\n")
	if err := svc.GitCatDiscard(ctx, cat.GitPathsRequest{ConvID: id, Paths: []string{"a.txt", "[selected].txt"}}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(body) != "staged\n" {
		t.Fatalf("staged snapshot lost: %s", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "[selected].txt")); !os.IsNotExist(err) {
		t.Fatalf("selected remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "s.txt")); err != nil {
		t.Fatal("unrelated file removed")
	}
	if err := svc.GitCatDiscard(ctx, cat.GitPathsRequest{ConvID: id, Paths: []string{"s.txt", "../escape"}}); err == nil {
		t.Fatal("escape accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "s.txt")); err != nil {
		t.Fatal("discard mutated before validating all paths")
	}
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := changeBy(st, "a.txt")
	if !c.Staged || c.Unstaged {
		t.Fatalf("staged state: %+v", c)
	}
}

func TestGitWorkbenchUnbornUnstageAndInvalidScope(t *testing.T) {
	ctx := context.Background()
	dir := wbRepo(t, false)
	svc, id := convAt(t, dir)
	if _, err := svc.ListCatCommits(ctx, cat.ListCatCommitsRequest{ConvID: id, Scope: "invalid"}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("invalid scope: %v", err)
	}
	wbWrite(t, dir, "a.txt", "index\n")
	gitRun(t, dir, "add", "a.txt")
	wbWrite(t, dir, "a.txt", "working\n")
	if err := svc.GitCatUnstage(ctx, cat.GitPathsRequest{ConvID: id, Paths: []string{"a.txt"}}); err != nil {
		t.Fatal(err)
	}
	if got := wbGit(t, dir, "ls-files"); got != "" {
		t.Fatalf("still staged: %s", got)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(body) != "working\n" {
		t.Fatal("working file lost")
	}
}

func TestGitWorkbenchInitAndUnpublishedCount(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	requireGit(t)
	svc, id := convAt(t, dir)
	if err := svc.GitCatInit(ctx, cat.GitStatusRequest{ConvID: id}); err != nil {
		t.Fatal(err)
	}
	if err := svc.GitCatInit(ctx, cat.GitStatusRequest{ConvID: id}); err == nil {
		t.Fatal("existing repo reinitialized")
	}
	gitRun(t, dir, "config", "user.name", "Workbench test")
	gitRun(t, dir, "config", "user.email", "workbench@example.com")
	first := wbCommit(t, dir, "a.txt", "first\n", "initial unpublished")
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || st.Ahead != 1 || st.Upstream != "" {
		t.Fatalf("unpublished: %+v, %v", st, err)
	}
	page, err := svc.ListCatCommits(ctx, cat.ListCatCommitsRequest{ConvID: id, Scope: "outgoing"})
	if err != nil || len(page.Commits) != 1 || page.Commits[0].Sha != first {
		t.Fatalf("unpublished list: %+v, %v", page, err)
	}
}

func TestGitWorkbenchSubfolderStashLiteralPath(t *testing.T) {
	ctx := context.Background()
	dir := wbRepo(t, true)
	sub := filepath.Join(dir, "folder[1]")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	wbCommit(t, dir, "folder[1]/child.txt", "base\n", "child")
	svc, id := convAt(t, sub)
	wbWrite(t, sub, "child.txt", "changed\n")
	wbWrite(t, sub, "new.txt", "new\n")
	wbWrite(t, dir, "a.txt", "outside\n")
	if err := svc.GitCatStash(ctx, cat.GitStashRequest{ConvID: id, Op: "save", IncludeUntracked: true}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(sub, "child.txt"))
	if string(body) != "base\n" {
		t.Fatalf("subfolder not restored: %s", body)
	}
	if _, err := os.Stat(filepath.Join(sub, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("untracked left: %v", err)
	}
	body, _ = os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(body) != "outside\n" {
		t.Fatal("outside changes were stashed")
	}
}

func TestGitWorkbenchStashBySHAAndPreserveIndex(t *testing.T) {
	ctx := context.Background()
	dir := wbRepo(t, true)
	svc, id := convAt(t, dir)
	wbWrite(t, dir, "a.txt", "staged\n")
	gitRun(t, dir, "add", "a.txt")
	wbWrite(t, dir, "a.txt", "working\n")
	wbWrite(t, dir, "new.txt", "untracked\n")
	if err := svc.GitCatStash(ctx, cat.GitStashRequest{ConvID: id, Op: "save", Message: "first", IncludeUntracked: true}); err != nil {
		t.Fatal(err)
	}
	first, err := svc.ListCatStashes(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || len(first) != 1 {
		t.Fatalf("list: %+v, %v", first, err)
	}
	diff, err := svc.GetCatStashDiff(ctx, cat.GitStashRequest{ConvID: id, Sha: first[0].Sha})
	if err != nil || !strings.Contains(diff.Text, "new.txt") {
		t.Fatalf("stash diff: %+v, %v", diff, err)
	}
	wbWrite(t, dir, "second.txt", "second\n")
	if err := svc.GitCatStash(ctx, cat.GitStashRequest{ConvID: id, Op: "save", Message: "second", IncludeUntracked: true}); err != nil {
		t.Fatal(err)
	}
	if err := svc.GitCatStash(ctx, cat.GitStashRequest{ConvID: id, Op: "pop", Sha: first[0].Sha}); err != nil {
		t.Fatalf("%v; status=%s", err, wbGit(t, dir, "status", "--porcelain=v1"))
	}
	list, err := svc.ListCatStashes(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || len(list) != 1 || !strings.Contains(list[0].Message, "second") {
		t.Fatalf("wrong stash dropped: %+v, %v", list, err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(body) != "working\n" {
		t.Fatalf("worktree: %s", body)
	}
	if got := wbGit(t, dir, "show", ":a.txt"); got != "staged" {
		t.Fatalf("index: %s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	if err := svc.GitCatStash(ctx, cat.GitStashRequest{ConvID: id, Op: "apply", Sha: list[0].Sha}); err == nil {
		t.Fatal("dirty restoration accepted")
	}
	if err := svc.GitCatStash(ctx, cat.GitStashRequest{ConvID: id, Op: "drop", Sha: first[0].Sha}); err == nil {
		t.Fatal("missing stash accepted")
	}
}

func TestGitWorkbenchStashConflictKeepsOriginal(t *testing.T) {
	ctx := context.Background()
	dir := wbRepo(t, true)
	svc, id := convAt(t, dir)
	wbWrite(t, dir, "a.txt", "stashed\n")
	if err := svc.GitCatStash(ctx, cat.GitStashRequest{ConvID: id, Op: "save"}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ListCatStashes(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v, %v", list, err)
	}
	wbCommit(t, dir, "a.txt", "committed\n", "later")
	if err := svc.GitCatStash(ctx, cat.GitStashRequest{ConvID: id, Op: "pop", Sha: list[0].Sha}); err == nil {
		t.Fatal("expected stash conflict")
	}
	kept, err := svc.ListCatStashes(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || len(kept) != 1 || kept[0].Sha != list[0].Sha {
		t.Fatalf("original lost: %+v, %v", kept, err)
	}
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || len(st.Conflicts) != 1 || st.MergeState != "none" {
		t.Fatalf("stash conflicts: %+v, %v", st, err)
	}
	if err := svc.GitCatConflictResolve(ctx, cat.GitCatConflictResolveRequest{ConvID: id, Path: "a.txt", Mode: "ours"}); err != nil {
		t.Fatal(err)
	}
	st, err = svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || len(st.Conflicts) != 0 {
		t.Fatalf("resolve: %+v, %v", st, err)
	}
}

func TestGitWorkbenchUndoLocalOnly(t *testing.T) {
	ctx := context.Background()
	dir := wbRepo(t, true)
	svc, id := convAt(t, dir)
	base := wbGit(t, dir, "rev-parse", "HEAD")
	if _, err := svc.GitCatUndoCommit(ctx, cat.CatCommitRequest{ConvID: id, Sha: base}); err == nil {
		t.Fatal("root undo accepted")
	}
	head := wbCommit(t, dir, "a.txt", "committed\n", "draft subject")
	wbWrite(t, dir, "a.txt", "new staged\n")
	gitRun(t, dir, "add", "a.txt")
	wbWrite(t, dir, "a.txt", "new working\n")
	if _, err := svc.GitCatUndoCommit(ctx, cat.CatCommitRequest{ConvID: id, Sha: base}); err == nil {
		t.Fatal("stale undo accepted")
	}
	result, err := svc.GitCatUndoCommit(ctx, cat.CatCommitRequest{ConvID: id, Sha: head})
	if err != nil || result.Message != "draft subject" {
		t.Fatalf("undo: %+v, %v", result, err)
	}
	if got := wbGit(t, dir, "rev-parse", "HEAD"); got != base {
		t.Fatal("HEAD did not move to parent")
	}
	if got := wbGit(t, dir, "show", ":a.txt"); got != "new staged" {
		t.Fatal("index lost")
	}
	body, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(body) != "new working\n" {
		t.Fatal("worktree lost")
	}
	gitRun(t, dir, "add", "a.txt")
	gitRun(t, dir, "commit", "-m", "published")
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare", "-b", "main")
	gitRun(t, dir, "remote", "add", "cloud", bare)
	gitRun(t, dir, "push", "-u", "cloud", "main")
	published := wbGit(t, dir, "rev-parse", "HEAD")
	if _, err := svc.GitCatUndoCommit(ctx, cat.CatCommitRequest{ConvID: id, Sha: published}); err == nil {
		t.Fatal("published undo accepted")
	}
}

func TestGitWorkbenchRevertCherryAndConflictAbort(t *testing.T) {
	ctx := context.Background()
	dir := wbRepo(t, true)
	svc, id := convAt(t, dir)
	changed := wbCommit(t, dir, "a.txt", "changed\n", "change")
	if err := svc.GitCatCommitAction(ctx, cat.GitCommitActionRequest{ConvID: id, Sha: changed, HeadSha: changed, Op: "revert"}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(body) != "base\n" {
		t.Fatal("revert did not restore content")
	}
	main := wbGit(t, dir, "rev-parse", "HEAD")
	gitRun(t, dir, "switch", "-c", "feature")
	feature := wbCommit(t, dir, "feature.txt", "feature\n", "feature")
	gitRun(t, dir, "switch", "main")
	if err := svc.GitCatCommitAction(ctx, cat.GitCommitActionRequest{ConvID: id, Sha: feature, HeadSha: main, Op: "cherry-pick"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "feature.txt")); err != nil {
		t.Fatal(err)
	}
	cherryHead := wbGit(t, dir, "rev-parse", "HEAD")
	if err := svc.GitCatCommitAction(ctx, cat.GitCommitActionRequest{ConvID: id, Sha: feature, HeadSha: cherryHead, Op: "cherry-pick"}); err == nil {
		t.Fatal("expected empty cherry-pick")
	}
	if err := svc.GitCatConflictSkip(ctx, cat.GitStatusRequest{ConvID: id}); err != nil {
		t.Fatal(err)
	}
	if got := wbGit(t, dir, "rev-parse", "HEAD"); got != cherryHead {
		t.Fatal("skip changed HEAD")
	}
	change := wbCommit(t, dir, "a.txt", "middle\n", "middle")
	latest := wbCommit(t, dir, "a.txt", "latest\n", "latest")
	if err := svc.GitCatCommitAction(ctx, cat.GitCommitActionRequest{ConvID: id, Sha: change, HeadSha: main, Op: "revert"}); err == nil {
		t.Fatal("stale action accepted")
	}
	if err := svc.GitCatCommitAction(ctx, cat.GitCommitActionRequest{ConvID: id, Sha: change, HeadSha: latest, Op: "revert"}); err == nil {
		t.Fatal("expected revert conflict")
	}
	st, err := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || st.MergeState != "reverting" || len(st.Conflicts) != 1 {
		t.Fatalf("revert state: %+v, %v", st, err)
	}
	if err := svc.GitCatConflictAbort(ctx, cat.GitCatConflictAbortRequest{ConvID: id}); err != nil {
		t.Fatal(err)
	}
	st, err = svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if err != nil || st.MergeState != "none" || len(st.Changes) > 0 || st.HeadSha != latest {
		t.Fatalf("abort: %+v, %v", st, err)
	}
}
