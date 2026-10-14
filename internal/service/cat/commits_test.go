//go:build !windows

package cat_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/cat"
)

var hClock = 1700000000

func hgit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	hClock += 60
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	d := fmt.Sprintf("%d +0800", hClock)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+d, "GIT_COMMITTER_DATE="+d)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func hcommit(t *testing.T, dir, file, body, msg string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	hgit(t, dir, "add", "-A")
	hgit(t, dir, "commit", "-q", "-m", msg)
	return hgit(t, dir, "rev-parse", "HEAD")
}

func histReason(err error) string {
	return strings.SplitN(apperr.From(err).Detail, "\n", 2)[0]
}

// 分叉 + 合并 + tag + 远程。
func histRepo(t *testing.T) (string, map[string]string) {
	requireGit(t)
	root := t.TempDir()
	initRepo(t, root)
	m := map[string]string{}
	m["c1"] = hcommit(t, root, "a.txt", "1\n", "c1 中文 | tab\there")
	hgit(t, root, "switch", "-q", "-c", "feat")
	m["f1"] = hcommit(t, root, "f.txt", "f\n", "feature work")
	hgit(t, root, "switch", "-q", "main")
	m["c2"] = hcommit(t, root, "a.txt", "2\n", "main two")
	hgit(t, root, "-c", "user.name=t", "-c", "user.email=t@example.com", "merge", "-q", "--no-ff", "-m", "Merge feat", "feat")
	m["merge"] = hgit(t, root, "rev-parse", "HEAD")
	hgit(t, root, "tag", "v1.0")
	bare := t.TempDir()
	hgit(t, bare, "init", "-q", "--bare")
	hgit(t, root, "remote", "add", "origin", bare)
	hgit(t, root, "push", "-q", "origin", "main")
	hgit(t, root, "fetch", "-q", "origin")
	hgit(t, root, "remote", "set-head", "origin", "main")
	return root, m
}

func allPages(t *testing.T, svc *cat.Service, id string, req cat.ListCatCommitsRequest) []cat.CatCommit {
	t.Helper()
	var all []cat.CatCommit
	req.ConvID = id
	for i := 0; i < 1000; i++ {
		p, err := svc.ListCatCommits(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, p.Commits...)
		if p.NextCursor == "" {
			return all
		}
		req.Cursor = p.NextCursor
	}
	t.Fatal("too many pages")
	return nil
}

func TestListCatCommitsOrderParentsRefs(t *testing.T) {
	root, m := histRepo(t)
	svc, id := convAt(t, root)
	hgit(t, root, "stash", "list") // no stash
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("stashed\n"), 0o644)
	hgit(t, root, "stash", "-q")
	stash := hgit(t, root, "rev-parse", "refs/stash")

	p, err := svc.ListCatCommits(context.Background(), cat.ListCatCommitsRequest{ConvID: id})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Fields(hgit(t, root, "log", "--topo-order", "--date-order", "--format=%H", "HEAD", "--branches", "--remotes", "--tags"))
	if len(p.Commits) != len(want) || p.Total != len(want) || p.NextCursor != "" || p.NoCommits {
		t.Fatalf("page %+v want %d", p, len(want))
	}
	for i, c := range p.Commits {
		if c.Sha != want[i] {
			t.Fatalf("order %d", i)
		}
		if c.Sha == stash {
			t.Fatal("stash listed")
		}
	}
	top := p.Commits[0]
	if top.Sha != m["merge"] || len(top.Parents) != 2 || top.Parents[0] != m["c2"] || top.Parents[1] != m["f1"] || !top.IsHead {
		t.Fatalf("merge commit %+v", top)
	}
	kinds := map[string]string{}
	for _, r := range top.Refs {
		kinds[r.Name] = r.Kind
	}
	if kinds["main"] != "head" || kinds["origin/main"] != "remote" || kinds["v1.0"] != "tag" || len(kinds) != 3 {
		t.Fatalf("refs %+v", top.Refs)
	}
	for _, c := range p.Commits {
		if c.Sha == m["f1"] && (len(c.Refs) != 1 || c.Refs[0] != (cat.CatCommitRef{Name: "feat", Kind: "local"})) {
			t.Fatalf("feat refs %+v", c.Refs)
		}
		if c.Sha == m["c1"] && c.Subject != "c1 中文 | tab\there" {
			t.Fatalf("subject %q", c.Subject)
		}
		if c.Sha == m["c1"] && (len(c.Parents) != 1 || c.ShortSha == "" || c.AuthorName != "t" || c.AuthorTime == 0) {
			t.Fatalf("fields %+v", c)
		}
	}
	// branch filters
	fp, err := svc.ListCatCommits(context.Background(), cat.ListCatCommitsRequest{ConvID: id, Branch: "feat"})
	if err != nil || len(fp.Commits) != 3 {
		t.Fatalf("feat filter %v %d", err, len(fp.Commits))
	}
	if _, err := svc.ListCatCommits(context.Background(), cat.ListCatCommitsRequest{ConvID: id, Branch: "origin/main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListCatCommits(context.Background(), cat.ListCatCommitsRequest{ConvID: id, Branch: "nope"}); histReason(err) != "reason=branch_not_found" {
		t.Fatalf("want branch_not_found %v", err)
	}
	// detached HEAD
	hgit(t, root, "switch", "-q", "--detach", m["c1"])
	dp, _ := svc.ListCatCommits(context.Background(), cat.ListCatCommitsRequest{ConvID: id, Branch: "HEAD"})
	if len(dp.Commits) != 2 || !dp.Commits[0].IsHead || dp.Commits[0].Refs[0] != (cat.CatCommitRef{Name: "HEAD", Kind: "head"}) {
		t.Fatalf("detached %+v", dp.Commits)
	}
}

func TestListCatCommitsPaginationAndHistoryChanged(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepo(t, root)
	for i := 0; i < 24; i++ {
		hcommit(t, root, "a.txt", fmt.Sprint(i), fmt.Sprintf("n%d", i))
		if i == 10 {
			hgit(t, root, "branch", "side")
		}
	}
	hgit(t, root, "switch", "-q", "side")
	for i := 0; i < 6; i++ {
		hcommit(t, root, "s.txt", fmt.Sprint(i), fmt.Sprintf("s%d", i))
	}
	hgit(t, root, "switch", "-q", "main")
	svc, id := convAt(t, root)
	want := strings.Fields(hgit(t, root, "log", "--topo-order", "--date-order", "--format=%H", "HEAD", "--branches", "--remotes", "--tags"))
	got := allPages(t, svc, id, cat.ListCatCommitsRequest{Limit: 7})
	if len(got) != len(want) {
		t.Fatalf("%d vs %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Sha != want[i] {
			t.Fatalf("page order %d", i)
		}
	}
	p1, _ := svc.ListCatCommits(context.Background(), cat.ListCatCommitsRequest{ConvID: id, Limit: 7})
	hcommit(t, root, "a.txt", "new", "new commit")
	if _, err := svc.ListCatCommits(context.Background(), cat.ListCatCommitsRequest{ConvID: id, Limit: 7, Cursor: p1.NextCursor}); histReason(err) != "reason=history_changed" || !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("want history_changed %v", err)
	}
	if _, err := svc.ListCatCommits(context.Background(), cat.ListCatCommitsRequest{ConvID: id, Cursor: "!!bad"}); histReason(err) != "reason=git_failed" {
		t.Fatalf("bad cursor %v", err)
	}
	big, _ := svc.ListCatCommits(context.Background(), cat.ListCatCommitsRequest{ConvID: id, Limit: 9999})
	if len(big.Commits) != len(want)+1 || big.NextCursor != "" {
		t.Fatal("limit cap")
	}
}

func TestListCatCommitsEmptyRepoAndNotRepo(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	hgit(t, root, "init", "-q")
	svc, id := convAt(t, root)
	p, err := svc.ListCatCommits(context.Background(), cat.ListCatCommitsRequest{ConvID: id})
	if err != nil || !p.NoCommits || p.Commits == nil || len(p.Commits) != 0 {
		t.Fatalf("%+v %v", p, err)
	}
	plain := t.TempDir()
	svc2, id2 := convAt(t, plain)
	if _, err := svc2.ListCatCommits(context.Background(), cat.ListCatCommitsRequest{ConvID: id2}); histReason(err) != "reason=not_git_repo" {
		t.Fatalf("%v", err)
	}
}

func TestListCatCommitsSearch(t *testing.T) {
	root, m := histRepo(t)
	hgit(t, root, "-c", "user.name=Alice Zhang", "-c", "user.email=alice@x.io", "commit", "-q", "--allow-empty", "-m", "unrelated", "-m", "body has NEEDLE inside")
	svc, id := convAt(t, root)
	q := func(s string) []cat.CatCommit { return allPages(t, svc, id, cat.ListCatCommitsRequest{Query: s, Limit: 1}) }
	if r := q("needle"); len(r) != 1 || r[0].Subject != "unrelated" {
		t.Fatalf("body %+v", r)
	}
	if r := q("ALICE"); len(r) != 1 {
		t.Fatalf("author %+v", r)
	}
	if r := q("alice@x"); len(r) != 1 {
		t.Fatalf("email %+v", r)
	}
	if r := q(m["f1"][:7]); len(r) != 1 || r[0].Sha != m["f1"] {
		t.Fatalf("sha %+v", r)
	}
	// OR + 分页：「main」命中 subject「main two」与 merge 信息里没有 → 只算 subject 的；作者 t 不含 main
	r := q("e") // 多数提交命中，逐页 1 条
	full := allPages(t, svc, id, cat.ListCatCommitsRequest{Query: "e", Limit: 500})
	if len(r) != len(full) || len(r) < 3 {
		t.Fatalf("search paging %d vs %d", len(r), len(full))
	}
	for i := range r {
		if r[i].Sha != full[i].Sha {
			t.Fatal("search page order")
		}
	}
	if r := q("zzzz-none"); len(r) != 0 {
		t.Fatal("none")
	}
}

func TestGetCatCommitDetailAndFileDiff(t *testing.T) {
	root, m := histRepo(t)
	svc, id := convAt(t, root)
	ctx := context.Background()
	d, err := svc.GetCatCommitDetail(ctx, cat.CatCommitRequest{ConvID: id, Sha: m["merge"][:8]})
	if err != nil {
		t.Fatal(err)
	}
	if d.Sha != m["merge"] || d.DiffBase != m["c2"] || len(d.Parents) != 2 || d.Message != "Merge feat" {
		t.Fatalf("%+v", d)
	}
	if len(d.Files) != 1 || d.Files[0].Path != "f.txt" || d.Files[0].Status != "A" || d.Files[0].Additions != 1 {
		t.Fatalf("merge files vs first parent %+v", d.Files)
	}
	root0 := strings.Fields(hgit(t, root, "rev-list", "--max-parents=0", "HEAD"))[0]
	rd, err := svc.GetCatCommitDetail(ctx, cat.CatCommitRequest{ConvID: id, Sha: root0})
	if err != nil || rd.DiffBase != "" || len(rd.Files) != 1 || rd.Files[0].Status != "A" {
		t.Fatalf("root %+v %v", rd, err)
	}
	// rename + delete + binary + modify
	hgit(t, root, "mv", "f.txt", "g.txt")
	os.WriteFile(filepath.Join(root, "bin.dat"), []byte{0, 1, 2, 0}, 0o644)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("2\n3\n"), 0o644)
	sha := hcommit(t, root, "z 文件.txt", "z\n", "multi")
	md, err := svc.GetCatCommitDetail(ctx, cat.CatCommitRequest{ConvID: id, Sha: sha})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]cat.CatCommitFile{}
	for _, f := range md.Files {
		got[f.Path] = f
	}
	if got["g.txt"].Status != "R" || got["g.txt"].OldPath != "f.txt" || !got["bin.dat"].Binary || got["a.txt"].Additions != 1 || got["z 文件.txt"].Status != "A" {
		t.Fatalf("%+v", md.Files)
	}
	fd, err := svc.GetCatCommitFileDiff(ctx, cat.CatCommitFileDiffRequest{ConvID: id, Sha: sha, Path: "a.txt"})
	if err != nil || !strings.Contains(fd.Text, "+3") || fd.Binary {
		t.Fatalf("%+v %v", fd, err)
	}
	if bd, _ := svc.GetCatCommitFileDiff(ctx, cat.CatCommitFileDiffRequest{ConvID: id, Sha: sha, Path: "bin.dat"}); !bd.Binary {
		t.Fatal("binary")
	}
	if rf, err := svc.GetCatCommitFileDiff(ctx, cat.CatCommitFileDiffRequest{ConvID: id, Sha: root0, Path: "a.txt"}); err != nil || !strings.Contains(rf.Text, "+one") {
		t.Fatalf("root diff %+v %v", rf, err)
	}
	if _, err := svc.GetCatCommitFileDiff(ctx, cat.CatCommitFileDiffRequest{ConvID: id, Sha: sha, Path: "nope"}); histReason(err) != "reason=git_failed" {
		t.Fatal(err)
	}
	for _, bad := range []string{"deadbeefdeadbeef", "zz", ""} {
		_, err := svc.GetCatCommitDetail(ctx, cat.CatCommitRequest{ConvID: id, Sha: bad})
		e := apperr.From(err)
		if !apperr.Is(err, apperr.NotFound) || e.Detail != "reason=commit_not_found" || e.Message != "找不到这个提交，可能已被删除。" {
			t.Fatalf("%q: %v", bad, err)
		}
	}
}

func TestGitCatCheckoutCommitAndStatus(t *testing.T) {
	root, m := histRepo(t)
	svc, id := convAt(t, root)
	ctx := context.Background()
	st, _ := svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if st.Detached || st.HeadSha != m["merge"] || !strings.HasPrefix(m["merge"], st.HeadShortSha) || st.HeadShortSha == "" {
		t.Fatalf("status %+v", st)
	}
	// dirty: tracked, then untracked
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("dirty\n"), 0o644)
	if _, err := svc.GitCatCheckoutCommit(ctx, cat.CatCommitRequest{ConvID: id, Sha: m["c1"]}); histReason(err) != "reason=git_dirty" || apperr.From(err).Message != cat.MsgBranchDirty {
		t.Fatalf("dirty %v", err)
	}
	hgit(t, root, "checkout", "--", "a.txt")
	os.WriteFile(filepath.Join(root, "u.txt"), []byte("u"), 0o644)
	if _, err := svc.GitCatCheckoutCommit(ctx, cat.CatCommitRequest{ConvID: id, Sha: m["c1"]}); histReason(err) != "reason=git_dirty" {
		t.Fatalf("untracked %v", err)
	}
	if hgit(t, root, "rev-parse", "HEAD") != m["merge"] {
		t.Fatal("repo changed")
	}
	os.Remove(filepath.Join(root, "u.txt"))
	if _, err := svc.GitCatCheckoutCommit(ctx, cat.CatCommitRequest{ConvID: id, Sha: "abcdef12"}); histReason(err) != "reason=commit_not_found" {
		t.Fatal(err)
	}
	r, err := svc.GitCatCheckoutCommit(ctx, cat.CatCommitRequest{ConvID: id, Sha: m["c1"][:7]})
	if err != nil || !r.Detached || r.Sha != m["c1"] {
		t.Fatalf("%+v %v", r, err)
	}
	st, _ = svc.GitCatStatus(ctx, cat.GitStatusRequest{ConvID: id})
	if !st.Detached || st.HeadSha != m["c1"] {
		t.Fatalf("detached status %+v", st)
	}
	// merge in progress
	os.WriteFile(filepath.Join(root, ".git", "MERGE_HEAD"), []byte(m["f1"]+"\n"), 0o644)
	if _, err := svc.GitCatCheckoutCommit(ctx, cat.CatCommitRequest{ConvID: id, Sha: m["c2"]}); histReason(err) != "reason=merge_in_progress" {
		t.Fatalf("merge %v", err)
	}
}

func TestGitCatCheckoutCommitOverwrittenMapsToDirty(t *testing.T) {
	root, m := histRepo(t)
	real, _ := exec.LookPath("git")
	svc, id := convAt(t, root)
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = switch ]; then\n  if [ \"$LC_ALL\" != C ]; then echo 'no C locale' >&2; exit 3; fi\n  echo 'error: The following untracked working tree files would be overwritten by checkout:' >&2; exit 1\nfi\nexec '" + real + "' \"$@\"\n"
	os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := svc.GitCatCheckoutCommit(context.Background(), cat.CatCommitRequest{ConvID: id, Sha: m["c1"]}); histReason(err) != "reason=git_dirty" {
		t.Fatalf("%v", err)
	}
}
