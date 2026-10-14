package cat_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

func btGitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func btCommitFile(t *testing.T, dir, name, body, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", msg)
}

type btClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *btClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *btClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type btEnv struct {
	svc  *cat.Service
	st   *store.Store
	id   string
	root string
	bare string
	clk  *btClock
	evMu sync.Mutex
	evs  []string
}

// newBT：bare 远程 origin（main、feat/remote-only）+ clone，本地另有 dev、solo、topic/x。
func newBT(t *testing.T) *btEnv {
	t.Helper()
	requireGit(t)
	bare := t.TempDir()
	gitRun(t, bare, "init", "-q", "--bare", "-b", "main")
	seed := t.TempDir()
	initRepo(t, seed)
	gitRun(t, seed, "remote", "add", "origin", bare)
	gitRun(t, seed, "push", "-q", "-u", "origin", "main")
	gitRun(t, seed, "switch", "-q", "-c", "feat/remote-only")
	btCommitFile(t, seed, "r.txt", "r\n", "remote work")
	gitRun(t, seed, "push", "-q", "-u", "origin", "feat/remote-only")

	root := t.TempDir()
	gitRun(t, root, "-c", "core.autocrlf=false", "clone", "-q", "-b", "main", bare, ".")
	gitRun(t, root, "config", "user.email", "t@example.com")
	gitRun(t, root, "config", "user.name", "t")
	gitRun(t, root, "branch", "dev")
	gitRun(t, root, "branch", "topic/x")
	e := &btEnv{root: root, bare: bare, clk: &btClock{t: time.Unix(1_800_000_000, 0)}}
	e.st = openStore(t)
	svc, _ := newProjSvc(t, e.st, nil, cat.Config{DataRoot: t.TempDir(), Now: e.clk.now, Emit: func(ev string, p any) {
		e.evMu.Lock()
		e.evs = append(e.evs, ev)
		e.evMu.Unlock()
	}})
	p, err := svc.CreateCatProject(context.Background(), cat.CreateProjectRequest{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.CreateCatConversation(context.Background(), cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	e.svc, e.id = svc, conv.ID
	return e
}

func (e *btEnv) tree(t *testing.T) cat.CatBranchTree {
	t.Helper()
	tr, err := e.svc.ListCatBranchTree(context.Background(), cat.CatBranchTreeRequest{ConvID: e.id})
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func btFindB(tr cat.CatBranchTree, remote, name string) (cat.CatTreeBranch, bool) {
	for _, g := range tr.Groups {
		for _, b := range g.Branches {
			if b.Remote == remote && b.Name == name {
				return b, true
			}
		}
	}
	return cat.CatTreeBranch{}, false
}

func btNames(g cat.CatBranchGroup) string {
	var n []string
	for _, b := range g.Branches {
		n = append(n, b.Name)
	}
	return strings.Join(n, ",")
}

func btWantReason(t *testing.T, err error, code apperr.Code, reason string) {
	t.Helper()
	if err == nil || !apperr.Is(err, code) {
		t.Fatalf("want %s %s, got %v", code, reason, err)
	}
	first := strings.SplitN(apperr.From(err).Detail, "\n", 2)[0]
	if first != "reason="+reason {
		t.Fatalf("want reason=%s, got %q (%v)", reason, first, err)
	}
}

func TestBranchTreeListGroupsAheadBehind(t *testing.T) {
	ctx := context.Background()
	e := newBT(t)
	// main ahead 1 / behind 1 relative to origin/main
	seed := t.TempDir()
	gitRun(t, seed, "clone", "-q", e.bare, ".")
	gitRun(t, seed, "config", "user.email", "t@example.com")
	gitRun(t, seed, "config", "user.name", "t")
	btCommitFile(t, seed, "up.txt", "u\n", "upstream")
	gitRun(t, seed, "push", "-q", "origin", "main")
	gitRun(t, e.root, "fetch", "-q", "origin")
	btCommitFile(t, e.root, "mine.txt", "m\n", "local")

	tr := e.tree(t)
	if len(tr.Groups) != 2 || tr.Groups[0].Kind != "local" || tr.Groups[1].Kind != "remote" || tr.Groups[1].Remote != "origin" {
		t.Fatalf("groups %+v", tr.Groups)
	}
	if btNames(tr.Groups[0]) != "dev,main,topic/x" {
		t.Fatalf("locals %s", btNames(tr.Groups[0]))
	}
	if btNames(tr.Groups[1]) != "feat/remote-only,main" { // 跳过 origin/HEAD
		t.Fatalf("remotes %s", btNames(tr.Groups[1]))
	}
	m, _ := btFindB(tr, "", "main")
	want := btGitOut(t, e.root, "rev-list", "--left-right", "--count", "main...origin/main")
	if !m.Current || m.Upstream != "origin/main" || strconv.Itoa(m.Ahead)+"\t"+strconv.Itoa(m.Behind) != want || m.Ahead != 1 || m.Behind != 1 {
		t.Fatalf("main %+v want %q", m, want)
	}
	if m.LastCommit.Sha != btGitOut(t, e.root, "rev-parse", "main") || m.LastCommit.Subject != "local" || m.LastCommit.Time == 0 {
		t.Fatalf("last commit %+v", m.LastCommit)
	}
	d, _ := btFindB(tr, "", "dev")
	if d.Upstream != "" || d.Ahead != 0 || d.Behind != 0 || d.Current {
		t.Fatalf("dev %+v", d)
	}
	if tr.Detached || tr.NoCommits || tr.FetchedAt != 0 {
		t.Fatalf("flags %+v", tr)
	}
	gitRun(t, e.root, "switch", "-q", "--detach", "HEAD")
	if tr := e.tree(t); !tr.Detached {
		t.Fatal("detached")
	}
	_ = ctx
}

func TestBranchTreeNotRepoAndNoCommits(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	dir := t.TempDir()
	svc, id := convAt(t, dir)
	_, err := svc.ListCatBranchTree(ctx, cat.CatBranchTreeRequest{ConvID: id})
	btWantReason(t, err, apperr.InvalidArgument, "not_git_repo")
	if apperr.From(err).Message != cat.MsgTreeNotRepo {
		t.Fatal(err)
	}
	for _, f := range []func() error{
		func() error { _, err := svc.FetchCatRemotes(ctx, cat.FetchCatRemotesRequest{ConvID: id}); return err },
		func() error {
			return svc.SetCatBranchFavorite(ctx, cat.SetCatBranchFavoriteRequest{ConvID: id, Branch: "x", Favorite: true})
		},
		func() error {
			_, err := svc.GitCatCreateBranch(ctx, cat.GitCatCreateBranchRequest{ConvID: id, From: "main", Name: "x"})
			return err
		},
		func() error {
			_, err := svc.GitCatCompareBranch(ctx, cat.GitCatCompareBranchRequest{ConvID: id, Branch: "x"})
			return err
		},
		func() error {
			_, err := svc.GitCatRenameBranch(ctx, cat.GitCatRenameBranchRequest{ConvID: id, From: "a", To: "b"})
			return err
		},
		func() error {
			_, err := svc.GitCatDeleteBranch(ctx, cat.GitCatDeleteBranchRequest{ConvID: id, Branch: "a"})
			return err
		},
	} {
		btWantReason(t, f(), apperr.InvalidArgument, "not_git_repo")
	}
	gitRun(t, dir, "init", "-q", "-b", "main")
	tr, err := svc.ListCatBranchTree(ctx, cat.CatBranchTreeRequest{ConvID: id})
	if err != nil || !tr.NoCommits || len(tr.Groups) != 0 || tr.Recent == nil {
		t.Fatalf("empty repo %+v %v", tr, err)
	}
}

func TestBranchTreeFavoritesPerRepo(t *testing.T) {
	ctx := context.Background()
	e := newBT(t)
	cfgBefore, _ := os.ReadFile(filepath.Join(e.root, ".git", "config"))
	if err := e.svc.SetCatBranchFavorite(ctx, cat.SetCatBranchFavoriteRequest{ConvID: e.id, Branch: "dev", Favorite: true}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetCatBranchFavorite(ctx, cat.SetCatBranchFavoriteRequest{ConvID: e.id, Branch: "feat/remote-only", Remote: "origin", Favorite: true}); err != nil {
		t.Fatal(err)
	}
	err := e.svc.SetCatBranchFavorite(ctx, cat.SetCatBranchFavoriteRequest{ConvID: e.id, Branch: "nope", Favorite: true})
	btWantReason(t, err, apperr.NotFound, "branch_not_found")
	tr := e.tree(t)
	if b, _ := btFindB(tr, "", "dev"); !b.Favorite {
		t.Fatal("dev fav")
	}
	if b, _ := btFindB(tr, "origin", "feat/remote-only"); !b.Favorite {
		t.Fatal("remote fav")
	}
	if b, _ := btFindB(tr, "origin", "main"); b.Favorite {
		t.Fatal("origin/main fav")
	}
	cfgAfter, _ := os.ReadFile(filepath.Join(e.root, ".git", "config"))
	if string(cfgBefore) != string(cfgAfter) {
		t.Fatal(".git/config changed")
	}
	// 存储形态：按仓库根
	var raw map[string]struct {
		Favorites []struct{ Branch, Remote string } `json:"favorites"`
	}
	if ok, err := e.st.GetSetting(ctx, cat.BranchPrefsKey, &raw); !ok || err != nil {
		t.Fatal(ok, err)
	}
	top := btGitOut(t, e.root, "rev-parse", "--show-toplevel")
	if len(raw) != 1 || len(raw[filepath.Clean(top)].Favorites) != 2 {
		t.Fatalf("raw %+v", raw)
	}
	// 同一服务里另一个仓库互不影响
	other := t.TempDir()
	initRepo(t, other)
	gitRun(t, other, "branch", "dev")
	p, err := e.svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: other})
	if err != nil {
		t.Fatal(err)
	}
	c2, _ := e.svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	tr2, err := e.svc.ListCatBranchTree(ctx, cat.CatBranchTreeRequest{ConvID: c2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := btFindB(tr2, "", "dev"); b.Favorite {
		t.Fatal("leaked across repos")
	}
	if err := e.svc.SetCatBranchFavorite(ctx, cat.SetCatBranchFavoriteRequest{ConvID: e.id, Branch: "dev", Favorite: false}); err != nil {
		t.Fatal(err)
	}
	if b, _ := btFindB(e.tree(t), "", "dev"); b.Favorite {
		t.Fatal("unfav")
	}
}

func TestBranchTreeRecentOnCheckout(t *testing.T) {
	ctx := context.Background()
	e := newBT(t)
	for _, b := range []string{"b1", "b2", "b3", "b4", "b5"} {
		gitRun(t, e.root, "branch", b)
	}
	for _, b := range []string{"dev", "b1", "b2", "dev", "b3", "b4", "b5"} {
		if err := e.svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: e.id, Branch: b}); err != nil {
			t.Fatal(err)
		}
	}
	if r := strings.Join(e.tree(t).Recent, ","); r != "b5,b4,b3,dev,b2" {
		t.Fatalf("recent %s", r)
	}
	if err := e.svc.GitCatCheckout(ctx, cat.GitCatCheckoutRequest{ConvID: e.id, Name: "main"}); err != nil {
		t.Fatal(err)
	}
	if r := e.tree(t).Recent; r[0] != "main" || len(r) != 5 {
		t.Fatalf("recent %v", r)
	}
	// 失败的签出不记
	_ = e.svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: e.id, Branch: "nope"})
	if r := e.tree(t).Recent; r[0] != "main" {
		t.Fatalf("recent %v", r)
	}
}

func TestBranchTreeFetchThrottleAndFailure(t *testing.T) {
	ctx := context.Background()
	e := newBT(t)
	seed := t.TempDir()
	gitRun(t, seed, "clone", "-q", e.bare, ".")
	gitRun(t, seed, "config", "user.email", "t@example.com")
	gitRun(t, seed, "config", "user.name", "t")
	gitRun(t, seed, "switch", "-q", "-c", "new/one")
	btCommitFile(t, seed, "n.txt", "n\n", "n")
	gitRun(t, seed, "push", "-q", "origin", "new/one")

	tr, err := e.svc.FetchCatRemotes(ctx, cat.FetchCatRemotesRequest{ConvID: e.id})
	if err != nil || tr.FetchError != "" || tr.FetchedAt != e.clk.now().Unix() || tr.Fetching {
		t.Fatalf("fetch1 %+v %v", tr, err)
	}
	if _, ok := btFindB(tr, "origin", "new/one"); !ok {
		t.Fatal("fetched branch missing")
	}
	first := tr.FetchedAt
	// 节流：5 分钟内不跑（远程新分支看不到）
	gitRun(t, seed, "push", "-q", "origin", "new/one:new/two")
	e.clk.add(4 * time.Minute)
	tr, _ = e.svc.FetchCatRemotes(ctx, cat.FetchCatRemotesRequest{ConvID: e.id})
	if _, ok := btFindB(tr, "origin", "new/two"); ok || tr.FetchedAt != first {
		t.Fatal("should be throttled")
	}
	// force 绕过
	tr, _ = e.svc.FetchCatRemotes(ctx, cat.FetchCatRemotesRequest{ConvID: e.id, Force: true})
	if _, ok := btFindB(tr, "origin", "new/two"); !ok || tr.FetchedAt != e.clk.now().Unix() {
		t.Fatal("force fetch")
	}
	okAt := tr.FetchedAt
	// 超过 5 分钟再跑；远程坏了 → 不报错、保留 fetchedAt、填 fetchError、数量照本地 refs 算
	btCommitFile(t, e.root, "l.txt", "l\n", "local ahead")
	if err := os.RemoveAll(e.bare); err != nil {
		t.Fatal(err)
	}
	e.clk.add(5*time.Minute + time.Second)
	tr, err = e.svc.FetchCatRemotes(ctx, cat.FetchCatRemotesRequest{ConvID: e.id})
	if err != nil || tr.FetchError == "" || tr.FetchedAt != okAt || len(tr.FetchError) > 4096 {
		t.Fatalf("fail %+v %v", tr, err)
	}
	if m, _ := btFindB(tr, "", "main"); m.Ahead != 1 || m.Behind != 0 {
		t.Fatalf("fallback ahead/behind %+v", m)
	}
	if lt := e.tree(t); lt.FetchError == "" || lt.FetchedAt != okAt {
		t.Fatal("list keeps fetch state")
	}
	e.evMu.Lock()
	n := 0
	for _, ev := range e.evs {
		if ev == cat.EventBranchTree {
			n++
		}
	}
	e.evMu.Unlock()
	if n != 3 {
		t.Fatalf("events %d", n)
	}
}

func TestBranchTreeCreate(t *testing.T) {
	ctx := context.Background()
	e := newBT(t)
	tr, err := e.svc.GitCatCreateBranch(ctx, cat.GitCatCreateBranchRequest{ConvID: e.id, From: "dev", Name: "made/here"})
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := btFindB(tr, "", "made/here"); !ok || b.Current || b.Upstream != "" {
		t.Fatalf("create %+v", b)
	}
	// 远程 from 带跟踪 + 签出
	tr, err = e.svc.GitCatCreateBranch(ctx, cat.GitCatCreateBranchRequest{ConvID: e.id, From: "origin/feat/remote-only", Name: "feat/remote-only", Checkout: true})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := btFindB(tr, "", "feat/remote-only")
	if !b.Current || b.Upstream != "origin/feat/remote-only" || tr.Recent[0] != "feat/remote-only" {
		t.Fatalf("remote checkout %+v %v", b, tr.Recent)
	}
	// 同名本地已存在 → 改为签出本地
	gitRun(t, e.root, "switch", "-q", "main")
	tr, err = e.svc.GitCatCreateBranch(ctx, cat.GitCatCreateBranchRequest{ConvID: e.id, From: "origin/feat/remote-only", Name: "feat/remote-only", Checkout: true})
	if b, _ := btFindB(tr, "", "feat/remote-only"); err != nil || !b.Current {
		t.Fatalf("existing local %v", err)
	}
	// 本地 from + 签出
	tr, err = e.svc.GitCatCreateBranch(ctx, cat.GitCatCreateBranchRequest{ConvID: e.id, From: "main", Name: "fromMain", Checkout: true})
	if b, _ := btFindB(tr, "", "fromMain"); err != nil || !b.Current || tr.Recent[0] != "fromMain" {
		t.Fatalf("checkout %v", err)
	}
	// reasons
	_, err = e.svc.GitCatCreateBranch(ctx, cat.GitCatCreateBranchRequest{ConvID: e.id, From: "nope", Name: "z"})
	btWantReason(t, err, apperr.NotFound, "branch_not_found")
	_, err = e.svc.GitCatCreateBranch(ctx, cat.GitCatCreateBranchRequest{ConvID: e.id, From: "main", Name: "dev"})
	btWantReason(t, err, apperr.TaskConflict, "branch_exists")
	if apperr.From(err).Message != "已经有同名分支了。" {
		t.Fatal(err)
	}
	_, err = e.svc.GitCatCreateBranch(ctx, cat.GitCatCreateBranchRequest{ConvID: e.id, From: "main", Name: "bad name"})
	btWantReason(t, err, apperr.InvalidArgument, "invalid_branch_name")
	if apperr.From(err).Message != `分支名不能包含空格或 ~ ^ : ? * [ \ 这些字符。` {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.root, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.GitCatCreateBranch(ctx, cat.GitCatCreateBranchRequest{ConvID: e.id, From: "main", Name: "dirtyco", Checkout: true})
	btWantReason(t, err, apperr.TaskConflict, "git_dirty")
	if apperr.From(err).Message != "有未提交的改动，请先提交或暂存，再切换分支。" {
		t.Fatal(err)
	}
	// 不签出时工作区脏也可以建
	if _, err := e.svc.GitCatCreateBranch(ctx, cat.GitCatCreateBranchRequest{ConvID: e.id, From: "main", Name: "dirtyok"}); err != nil {
		t.Fatal(err)
	}
}

func TestBranchTreeCompare(t *testing.T) {
	ctx := context.Background()
	e := newBT(t)
	gitRun(t, e.root, "switch", "-q", "dev")
	gitRun(t, e.root, "mv", "a.txt", "b.txt")
	gitRun(t, e.root, "commit", "-q", "-m", "rename")
	btCommitFile(t, e.root, "new.txt", "n\n", "add")
	gitRun(t, e.root, "switch", "-q", "main")
	btCommitFile(t, e.root, "m.txt", "m\n", "main only")
	res, err := e.svc.GitCatCompareBranch(ctx, cat.GitCatCompareBranchRequest{ConvID: e.id, Branch: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Base != "main" || res.Target != "dev" || len(res.OnlyInTarget) != 2 || len(res.OnlyInCurrent) != 1 || res.Truncated {
		t.Fatalf("compare %+v", res)
	}
	if res.OnlyInTarget[0].Subject != "add" || res.OnlyInCurrent[0].Subject != "main only" {
		t.Fatalf("subjects %+v", res)
	}
	st := map[string]string{}
	for _, f := range res.Files {
		st[f.Path] = f.Status + ":" + f.OldPath
	}
	if st["b.txt"] != "R:a.txt" || st["new.txt"] != "A:" || len(res.Files) != 2 {
		t.Fatalf("files %+v", res.Files)
	}
	res, err = e.svc.GitCatCompareBranch(ctx, cat.GitCatCompareBranchRequest{ConvID: e.id, Branch: "origin/feat/remote-only", Remote: true})
	if err != nil || len(res.OnlyInTarget) != 1 || res.Files[0].Path != "r.txt" || res.Files[0].Status != "A" {
		t.Fatalf("remote compare %+v %v", res, err)
	}
	_, err = e.svc.GitCatCompareBranch(ctx, cat.GitCatCompareBranchRequest{ConvID: e.id, Branch: "feat/remote-only"})
	btWantReason(t, err, apperr.NotFound, "branch_not_found")
	// 截断（>200）
	gitRun(t, e.root, "switch", "-q", "-c", "many")
	for i := 0; i < 201; i++ {
		gitRun(t, e.root, "commit", "-q", "--allow-empty", "-m", "c"+strconv.Itoa(i))
	}
	gitRun(t, e.root, "switch", "-q", "main")
	res, err = e.svc.GitCatCompareBranch(ctx, cat.GitCatCompareBranchRequest{ConvID: e.id, Branch: "many"})
	if err != nil || !res.Truncated || len(res.OnlyInTarget) != 200 {
		t.Fatalf("trunc %v %d", err, len(res.OnlyInTarget))
	}
	gitRun(t, e.root, "switch", "-q", "--detach", "HEAD")
	res, _ = e.svc.GitCatCompareBranch(ctx, cat.GitCatCompareBranchRequest{ConvID: e.id, Branch: "dev"})
	if res.Base != btGitOut(t, e.root, "rev-parse", "--short", "HEAD") {
		t.Fatalf("detached base %q", res.Base)
	}
}

func TestBranchTreeRename(t *testing.T) {
	ctx := context.Background()
	e := newBT(t)
	_ = e.svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: e.id, Branch: "dev"})
	_ = e.svc.SetCatBranchFavorite(ctx, cat.SetCatBranchFavoriteRequest{ConvID: e.id, Branch: "dev", Favorite: true})
	tr, err := e.svc.GitCatRenameBranch(ctx, cat.GitCatRenameBranchRequest{ConvID: e.id, From: "dev", To: "dev2"})
	if err != nil {
		t.Fatal(err)
	}
	b, ok := btFindB(tr, "", "dev2")
	if !ok || !b.Current || !b.Favorite || tr.Recent[0] != "dev2" {
		t.Fatalf("rename current %+v %v", b, tr.Recent)
	}
	if _, ok := btFindB(tr, "", "dev"); ok {
		t.Fatal("old remains")
	}
	_, err = e.svc.GitCatRenameBranch(ctx, cat.GitCatRenameBranchRequest{ConvID: e.id, From: "dev2", To: "main"})
	btWantReason(t, err, apperr.TaskConflict, "branch_exists")
	_, err = e.svc.GitCatRenameBranch(ctx, cat.GitCatRenameBranchRequest{ConvID: e.id, From: "dev2", To: "a..b"})
	btWantReason(t, err, apperr.InvalidArgument, "invalid_branch_name")
	_, err = e.svc.GitCatRenameBranch(ctx, cat.GitCatRenameBranchRequest{ConvID: e.id, From: "origin/main", To: "x"})
	btWantReason(t, err, apperr.NotFound, "branch_not_found")
}

func TestBranchTreeDelete(t *testing.T) {
	ctx := context.Background()
	e := newBT(t)
	_ = e.svc.SetCatBranchFavorite(ctx, cat.SetCatBranchFavoriteRequest{ConvID: e.id, Branch: "dev", Favorite: true})
	_ = e.svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: e.id, Branch: "dev"})
	_ = e.svc.SwitchCatBranch(ctx, cat.SwitchCatBranchRequest{ConvID: e.id, Branch: "main"})
	tr, err := e.svc.GitCatDeleteBranch(ctx, cat.GitCatDeleteBranchRequest{ConvID: e.id, Branch: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := btFindB(tr, "", "dev"); ok || strings.Join(tr.Recent, ",") != "main" {
		t.Fatalf("delete %+v", tr.Recent)
	}
	var raw map[string]map[string]any
	_, _ = e.st.GetSetting(ctx, cat.BranchPrefsKey, &raw)
	for _, p := range raw {
		if fs, _ := p["favorites"].([]any); len(fs) != 0 {
			t.Fatalf("fav not removed %+v", raw)
		}
	}
	gitRun(t, e.root, "switch", "-q", "topic/x")
	btCommitFile(t, e.root, "u.txt", "u\n", "unmerged")
	gitRun(t, e.root, "switch", "-q", "main")
	_, err = e.svc.GitCatDeleteBranch(ctx, cat.GitCatDeleteBranchRequest{ConvID: e.id, Branch: "topic/x"})
	btWantReason(t, err, apperr.TaskConflict, "branch_not_merged")
	if apperr.From(err).Message != "这个分支有还没合并的提交，删除后可能找不回来。" {
		t.Fatal(err)
	}
	if _, err := e.svc.GitCatDeleteBranch(ctx, cat.GitCatDeleteBranchRequest{ConvID: e.id, Branch: "topic/x", Force: true}); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.GitCatDeleteBranch(ctx, cat.GitCatDeleteBranchRequest{ConvID: e.id, Branch: "main"})
	btWantReason(t, err, apperr.InvalidArgument, "is_current")
	_, err = e.svc.GitCatDeleteBranch(ctx, cat.GitCatDeleteBranchRequest{ConvID: e.id, Branch: "origin/main"})
	btWantReason(t, err, apperr.NotFound, "branch_not_found")
	// git_failed：被另一个 worktree 签出的分支
	gitRun(t, e.root, "branch", "wt")
	gitRun(t, e.root, "worktree", "add", "-q", filepath.Join(t.TempDir(), "w"), "wt")
	_, err = e.svc.GitCatDeleteBranch(ctx, cat.GitCatDeleteBranchRequest{ConvID: e.id, Branch: "wt", Force: true})
	btWantReason(t, err, apperr.InvalidArgument, "git_failed")
	if !strings.Contains(apperr.From(err).Detail, "\n") {
		t.Fatal("git_failed needs stderr line")
	}
}
