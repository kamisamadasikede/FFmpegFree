package cat

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/proc"
)

// 契约 v0.51（6.19.26）：Git 分支树（对标 IDEA Git Branches）。

const (
	// EventBranchTree 在一次 fetch 结束后发出，payload {convId}，前端重拉分支树。
	EventBranchTree = "cat:branchtree"
	// BranchPrefsKey 存收藏 / 最近，按仓库根分开（6.19.26.2）。
	BranchPrefsKey = "cat.branchPrefs"

	branchFetchInterval = 5 * time.Minute
	branchFetchTimeout  = 60 * time.Second
	branchFetchErrMax   = 4096
	branchRecentMax     = 5
	branchCompareMax    = 200
)

// 文案：契约 v0.51.1 6.19.26.5 / 6.19.25 定稿照抄。is_current / git_failed 契约只走前端通用失败 +「查看详情」，message 仅兜底。
const (
	MsgTreeTurnRunning    = "正在回复，等这一轮结束再切换分支。"
	MsgTreeDirty          = "有未提交的改动，请先提交或暂存，再切换分支。"
	MsgTreeNotRepo        = "这个文件夹不是 Git 仓库。"
	MsgTreeBranchNotFound = "找不到这个分支，可能已被删除。"
	MsgTreeBranchExists   = "已经有同名分支了。"
	MsgTreeInvalidName    = "分支名不能包含空格或 ~ ^ : ? * [ \\ 这些字符。"
	MsgTreeNotMerged      = "这个分支有还没合并的提交，删除后可能找不回来。"
	MsgTreeIsCurrent      = "不能删除当前分支"
	MsgTreeGitFailed      = "操作失败，请重试。"
)

// CatBranchCommit 是一个提交的摘要。
type CatBranchCommit struct {
	Sha     string `json:"sha"`
	Subject string `json:"subject"`
	Time    int64  `json:"time"` // unix 秒
}

// CatTreeBranch 是分支树里的一个分支。
type CatTreeBranch struct {
	Name       string          `json:"name"`               // 本地：feature/a；远程：不含 remote 前缀
	Remote     string          `json:"remote,omitempty"`   // 远程分支所属 remote；本地为空
	Upstream   string          `json:"upstream,omitempty"` // 本地分支的上游，如 origin/feature/a
	Ahead      int             `json:"ahead"`
	Behind     int             `json:"behind"`
	Current    bool            `json:"current"`
	Favorite   bool            `json:"favorite"`
	LastCommit CatBranchCommit `json:"lastCommit"`
}

// CatBranchGroup 是本地组或某个 remote 的组。
type CatBranchGroup struct {
	Kind     string          `json:"kind"` // local | remote
	Remote   string          `json:"remote,omitempty"`
	Branches []CatTreeBranch `json:"branches"`
}

// CatBranchTree 是 ListCatBranchTree 等的返回。
type CatBranchTree struct {
	Groups     []CatBranchGroup `json:"groups"`
	Recent     []string         `json:"recent"`
	Detached   bool             `json:"detached"`
	FetchedAt  int64            `json:"fetchedAt"` // 该仓库上次 fetch 成功时间（unix 秒），0=从未
	Fetching   bool             `json:"fetching"`
	FetchError string           `json:"fetchError,omitempty"` // 上次 fetch 失败的原始 stderr（截 4 KiB）
	NoCommits  bool             `json:"noCommits"`            // v0.51.1：仓库还没有任何提交，此时 groups 为空
}

// CatBranchCompare 是当前分支与目标分支的比较。
type CatBranchCompare struct {
	Base          string            `json:"base"` // 当前分支名（分离 HEAD 时为短 sha）
	Target        string            `json:"target"`
	OnlyInTarget  []CatBranchCommit `json:"onlyInTarget"`
	OnlyInCurrent []CatBranchCommit `json:"onlyInCurrent"`
	Truncated     bool              `json:"truncated"`
	Files         []CatCompareFile  `json:"files"`
}

// CatCompareFile 是两分支间改动的文件。Status: A|M|D|R|C|T。
type CatCompareFile struct {
	Path    string `json:"path"`
	OldPath string `json:"oldPath"`
	Status  string `json:"status"`
}

type CatBranchTreeRequest struct {
	ConvID string `json:"convId"`
}

type FetchCatRemotesRequest struct {
	ConvID string `json:"convId"`
	Force  bool   `json:"force"`
}

type SetCatBranchFavoriteRequest struct {
	ConvID   string `json:"convId"`
	Branch   string `json:"branch"`
	Remote   string `json:"remote"`
	Favorite bool   `json:"favorite"`
}

type GitCatCreateBranchRequest struct {
	ConvID   string `json:"convId"`
	From     string `json:"from"`
	Name     string `json:"name"`
	Checkout bool   `json:"checkout"`
}

// GitCatCompareBranchRequest：Remote=true 时 Branch 是 <remote>/<b>。
type GitCatCompareBranchRequest struct {
	ConvID string `json:"convId"`
	Branch string `json:"branch"`
	Remote bool   `json:"remote"`
}

type GitCatRenameBranchRequest struct {
	ConvID string `json:"convId"`
	From   string `json:"from"`
	To     string `json:"to"`
}

type GitCatDeleteBranchRequest struct {
	ConvID string `json:"convId"`
	Branch string `json:"branch"`
	Force  bool   `json:"force"`
}

// ---- 状态 ----

type branchFetchState struct {
	attemptAt time.Time
	fetchedAt time.Time
	errText   string
	running   bool
}

type branchTreeState struct {
	mu      sync.Mutex
	fetch   map[string]*branchFetchState // 仓库根 key → fetch 状态（只在内存）
	prefsMu sync.Mutex
}

type branchFav struct {
	Branch string `json:"branch"`
	Remote string `json:"remote"`
}

type repoBranchPrefs struct {
	Favorites []branchFav `json:"favorites"`
	Recent    []string    `json:"recent"`
}

// ---- 错误 ----

func treeErr(code apperr.Code, msg, reason string) error {
	return apperr.New(code, msg).WithDetail("reason=" + reason)
}

func errTreeGitFailed(stderr string) error {
	d := "reason=git_failed"
	if s := strings.TrimSpace(stderr); s != "" {
		d += "\n" + clipText(s, branchFetchErrMax)
	}
	return apperr.New(apperr.InvalidArgument, MsgTreeGitFailed).WithDetail(d)
}

func errTreeNotFound() error {
	return treeErr(apperr.NotFound, MsgTreeBranchNotFound, "branch_not_found")
}

// ---- git ----

// branchTreeGitEnv：非交互 + 输出固定英文（便于识别 stderr）。
func branchTreeGitEnv() []string {
	return append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"GIT_MERGE_AUTOEDIT=no",
		"GIT_EDITOR=true",
		"EDITOR=true",
		"LC_ALL=C", "LANG=C", "LANGUAGE=",
	)
}

func branchTreeCmd(ctx context.Context, exe, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	cmd.Env = branchTreeGitEnv()
	proc.Configure(cmd) // Windows 隐藏控制台窗口
	return cmd
}

func treeGit(ctx context.Context, timeout time.Duration, dir string, args ...string) ([]byte, string, error) {
	exe, err := exec.LookPath("git")
	if err != nil {
		return nil, "", errGitMissing()
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := branchTreeCmd(cctx, exe, dir, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	errText := stderr.String()
	if err != nil && errors.Is(cctx.Err(), context.DeadlineExceeded) && strings.TrimSpace(errText) == "" {
		errText = "git timed out"
	}
	return stdout.Bytes(), errText, err
}

func treeRefExists(ctx context.Context, top, ref string) bool {
	_, _, err := treeGit(ctx, gitStatusTimeout, top, "rev-parse", "--verify", "-q", ref+"^{commit}")
	return err == nil
}

func treeValidName(ctx context.Context, top, name string) bool {
	if name == "" || strings.ContainsAny(name, "\x00\n") || strings.HasPrefix(name, "-") || name == "HEAD" {
		return false
	}
	_, _, err := treeGit(ctx, gitStatusTimeout, top, "check-ref-format", "--branch", name)
	return err == nil
}

func treeCurrent(ctx context.Context, top string) string {
	out, _, err := treeGit(ctx, gitStatusTimeout, top, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func treeRemotes(ctx context.Context, top string) ([]string, error) {
	out, stderr, err := treeGit(ctx, gitStatusTimeout, top, "remote")
	if err != nil {
		return nil, errTreeGitFailed(stderr)
	}
	var rs []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			rs = append(rs, l)
		}
	}
	sort.Strings(rs)
	return rs, nil
}

// ---- 定位 ----

type treeCtx struct {
	lay gitLayout
	key string
}

// branchRepoKey：仓库根绝对路径；Windows 统一小写盘符 + 反斜杠。
func branchRepoKey(top string) string {
	return branchRepoKeyFor(runtime.GOOS, top)
}

func branchRepoKeyFor(goos, top string) string {
	if goos != "windows" {
		return filepath.Clean(top)
	}
	k := strings.ReplaceAll(top, "/", `\`)
	if len(k) >= 2 && k[1] == ':' {
		k = strings.ToLower(k[:1]) + k[1:]
	}
	return strings.TrimSuffix(k, `\`)
}

func (s *Service) treeOpen(ctx context.Context, convID string) (treeCtx, error) {
	root, err := s.convRoot(ctx, convID)
	if err != nil {
		return treeCtx{}, err
	}
	lay, ok, err := findGitLayout(ctx, root)
	if err != nil {
		return treeCtx{}, err
	}
	if !ok {
		return treeCtx{}, treeErr(apperr.InvalidArgument, MsgTreeNotRepo, "not_git_repo")
	}
	return treeCtx{lay: lay, key: branchRepoKey(lay.top)}, nil
}

// treeTurnRunning：本对话或同项目任一对话有进行中的一轮。
func (s *Service) treeTurnRunning(ctx context.Context, convID string) (bool, error) {
	c, err := s.requireConv(ctx, convID)
	if err != nil {
		return false, err
	}
	ids := []string{c.ID}
	if c.ProjectID != "" && s.cfg.Store != nil {
		if more, err := s.cfg.Store.CatConversationIDsByProject(ctx, c.ProjectID); err == nil {
			ids = append(ids, more...)
		}
	}
	return s.projectHasRunningTurn(ids), nil
}

func (s *Service) treeRequireIdle(ctx context.Context, convID string) error {
	busy, err := s.treeTurnRunning(ctx, convID)
	if err != nil {
		return err
	}
	if busy {
		return treeErr(apperr.TaskConflict, MsgTreeTurnRunning, "turn_running")
	}
	return nil
}

// ---- 偏好 ----

func (s *Service) loadBranchPrefs(ctx context.Context) (map[string]repoBranchPrefs, error) {
	m := map[string]repoBranchPrefs{}
	if s.cfg.Store == nil {
		return m, nil
	}
	if _, err := s.cfg.Store.GetSetting(ctx, BranchPrefsKey, &m); err != nil {
		return nil, apperr.Wrap(apperr.IOError, "读取设置失败。", err)
	}
	if m == nil {
		m = map[string]repoBranchPrefs{}
	}
	return m, nil
}

func (s *Service) repoPrefs(ctx context.Context, key string) repoBranchPrefs {
	s.btree.prefsMu.Lock()
	defer s.btree.prefsMu.Unlock()
	m, err := s.loadBranchPrefs(ctx)
	if err != nil {
		return repoBranchPrefs{}
	}
	return m[key]
}

func (s *Service) updateRepoPrefs(ctx context.Context, key string, fn func(p *repoBranchPrefs)) error {
	if s.cfg.Store == nil {
		return nil
	}
	s.btree.prefsMu.Lock()
	defer s.btree.prefsMu.Unlock()
	m, err := s.loadBranchPrefs(ctx)
	if err != nil {
		return err
	}
	p := m[key]
	fn(&p)
	if len(p.Favorites) == 0 && len(p.Recent) == 0 {
		delete(m, key)
	} else {
		m[key] = p
	}
	if err := s.cfg.Store.SetSetting(ctx, BranchPrefsKey, m); err != nil {
		return apperr.Wrap(apperr.IOError, "保存失败，请重试。", err)
	}
	return nil
}

func pushRecent(list []string, name string) []string {
	out := []string{name}
	for _, r := range list {
		if r != name && len(out) < branchRecentMax {
			out = append(out, r)
		}
	}
	return out
}

// RecordCatBranchCheckout 在应用内签出成功后调用，把当前分支记进「最近」（6.19.26.1）。失败只忽略。
func (s *Service) RecordCatBranchCheckout(ctx context.Context, convID string) {
	tc, err := s.treeOpen(ctx, convID)
	if err != nil {
		return
	}
	s.recordRecent(ctx, tc, treeCurrent(ctx, tc.lay.top))
}

func (s *Service) recordRecent(ctx context.Context, tc treeCtx, name string) {
	if name == "" {
		return
	}
	_ = s.updateRepoPrefs(ctx, tc.key, func(p *repoBranchPrefs) { p.Recent = pushRecent(p.Recent, name) })
}

// ---- 列表 ----

// ListCatBranchTree 只读本地 refs，不触发 fetch。
func (s *Service) ListCatBranchTree(ctx context.Context, req CatBranchTreeRequest) (CatBranchTree, error) {
	tc, err := s.treeOpen(ctx, req.ConvID)
	if err != nil {
		return CatBranchTree{}, err
	}
	return s.buildTree(ctx, tc)
}

func (s *Service) fetchState(key string) *branchFetchState {
	if s.btree.fetch == nil {
		s.btree.fetch = map[string]*branchFetchState{}
	}
	st := s.btree.fetch[key]
	if st == nil {
		st = &branchFetchState{}
		s.btree.fetch[key] = st
	}
	return st
}

func (s *Service) buildTree(ctx context.Context, tc treeCtx) (CatBranchTree, error) {
	top := tc.lay.top
	tree := CatBranchTree{Groups: []CatBranchGroup{}, Recent: []string{}}
	s.btree.mu.Lock()
	st := s.fetchState(tc.key)
	if !st.fetchedAt.IsZero() {
		tree.FetchedAt = st.fetchedAt.Unix()
	}
	tree.Fetching, tree.FetchError = st.running, st.errText
	s.btree.mu.Unlock()

	if !treeRefExists(ctx, top, "HEAD") {
		tree.NoCommits = true
		return tree, nil
	}
	cur := treeCurrent(ctx, top)
	tree.Detached = cur == ""
	remotes, err := treeRemotes(ctx, top)
	if err != nil {
		return tree, err
	}
	out, stderr, err := treeGit(ctx, gitStatusTimeout, top, "for-each-ref",
		"--format=%(refname)|%(upstream:short)|%(objectname)|%(committerdate:unix)|%(symref)|%(subject)",
		"refs/heads", "refs/remotes")
	if err != nil {
		if gitNotRepo(stderr, err) {
			return tree, treeErr(apperr.InvalidArgument, MsgTreeNotRepo, "not_git_repo")
		}
		return tree, errTreeGitFailed(stderr)
	}
	prefs := s.repoPrefs(ctx, tc.key)
	fav := map[branchFav]bool{}
	for _, f := range prefs.Favorites {
		fav[f] = true
	}
	local := CatBranchGroup{Kind: "local", Branches: []CatTreeBranch{}}
	byRemote := map[string]*CatBranchGroup{}
	for _, r := range remotes {
		byRemote[r] = &CatBranchGroup{Kind: "remote", Remote: r, Branches: []CatTreeBranch{}}
	}
	existing := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(line, "|", 6)
		if len(parts) < 6 {
			continue
		}
		ref, up, sha, ts, symref, subj := parts[0], parts[1], parts[2], parts[3], parts[4], parts[5]
		t, _ := strconv.ParseInt(ts, 10, 64)
		lc := CatBranchCommit{Sha: sha, Subject: subj, Time: t}
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			n := strings.TrimPrefix(ref, "refs/heads/")
			b := CatTreeBranch{Name: n, Upstream: up, Current: n == cur, Favorite: fav[branchFav{Branch: n}], LastCommit: lc}
			if up != "" {
				b.Ahead, b.Behind = treeAheadBehind(ctx, top, ref, up)
			}
			existing[n] = true
			local.Branches = append(local.Branches, b)
		case strings.HasPrefix(ref, "refs/remotes/"):
			rest := strings.TrimPrefix(ref, "refs/remotes/")
			r, n := splitRemoteRef(rest, remotes)
			if r == "" || n == "" || n == "HEAD" || symref != "" {
				continue
			}
			g := byRemote[r]
			if g == nil {
				g = &CatBranchGroup{Kind: "remote", Remote: r, Branches: []CatTreeBranch{}}
				byRemote[r] = g
				remotes = append(remotes, r)
				sort.Strings(remotes)
			}
			g.Branches = append(g.Branches, CatTreeBranch{Name: n, Remote: r, Favorite: fav[branchFav{Branch: n, Remote: r}], LastCommit: lc})
		}
	}
	sortTreeBranches(local.Branches)
	tree.Groups = append(tree.Groups, local)
	for _, r := range remotes {
		g := byRemote[r]
		sortTreeBranches(g.Branches)
		tree.Groups = append(tree.Groups, *g)
	}
	for _, r := range prefs.Recent {
		if existing[r] {
			tree.Recent = append(tree.Recent, r)
		}
	}
	return tree, nil
}

func sortTreeBranches(bs []CatTreeBranch) {
	sort.Slice(bs, func(i, j int) bool { return bs[i].Name < bs[j].Name })
}

// splitRemoteRef 按已配置的 remote 名（取最长前缀）拆 <remote>/<branch>。
func splitRemoteRef(rest string, remotes []string) (string, string) {
	best := ""
	for _, r := range remotes {
		if strings.HasPrefix(rest, r+"/") && len(r) > len(best) {
			best = r
		}
	}
	if best == "" {
		i := strings.Index(rest, "/")
		if i <= 0 {
			return "", ""
		}
		best = rest[:i]
	}
	return best, strings.TrimPrefix(rest, best+"/")
}

// treeAheadBehind：本地 refs 上算，上游 ref 不存在时 0/0。
func treeAheadBehind(ctx context.Context, top, ref, up string) (int, int) {
	out, _, err := treeGit(ctx, gitStatusTimeout, top, "rev-list", "--left-right", "--count", ref+"...refs/remotes/"+up)
	if err != nil {
		out, _, err = treeGit(ctx, gitStatusTimeout, top, "rev-list", "--left-right", "--count", ref+"..."+up)
		if err != nil {
			return 0, 0
		}
	}
	f := strings.Fields(string(out))
	if len(f) != 2 {
		return 0, 0
	}
	a, _ := strconv.Atoi(f[0])
	b, _ := strconv.Atoi(f[1])
	return a, b
}

// ---- fetch ----

// FetchCatRemotes：force=false 且距上次尝试不足 5 分钟直接返回当前树；同仓库同时只跑一个。
// 失败不返回错误：写 fetchError、保留旧 fetchedAt。结束后发 cat:branchtree。
func (s *Service) FetchCatRemotes(ctx context.Context, req FetchCatRemotesRequest) (CatBranchTree, error) {
	tc, err := s.treeOpen(ctx, req.ConvID)
	if err != nil {
		return CatBranchTree{}, err
	}
	now := s.now()
	s.btree.mu.Lock()
	st := s.fetchState(tc.key)
	if st.running || (!req.Force && !st.attemptAt.IsZero() && now.Sub(st.attemptAt) < branchFetchInterval) {
		s.btree.mu.Unlock()
		return s.buildTree(ctx, tc)
	}
	st.running, st.attemptAt = true, now
	s.btree.mu.Unlock()

	errText := s.fetchAll(ctx, tc.lay.top)

	s.btree.mu.Lock()
	st.running = false
	st.errText = errText
	if errText == "" {
		st.fetchedAt = s.now()
	}
	s.btree.mu.Unlock()
	s.emit(EventBranchTree, map[string]string{"convId": req.ConvID})
	return s.buildTree(ctx, tc)
}

func (s *Service) fetchAll(ctx context.Context, top string) string {
	remotes, err := treeRemotes(ctx, top)
	if err != nil {
		return clipBytes(apperr.From(err).Detail, branchFetchErrMax)
	}
	var errs []string
	for _, r := range remotes {
		_, stderr, err := treeGit(ctx, branchFetchTimeout, top, "fetch", "--prune", r)
		if err != nil {
			msg := strings.TrimSpace(stderr)
			if msg == "" {
				msg = err.Error()
			}
			errs = append(errs, msg)
		}
	}
	return clipBytes(strings.Join(errs, "\n"), branchFetchErrMax)
}

// clipBytes 按字节截断，不切断 UTF-8 字符。
func clipBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}

// ---- 收藏 ----

func (s *Service) SetCatBranchFavorite(ctx context.Context, req SetCatBranchFavoriteRequest) error {
	tc, err := s.treeOpen(ctx, req.ConvID)
	if err != nil {
		return err
	}
	b, r := strings.TrimSpace(req.Branch), strings.TrimSpace(req.Remote)
	ref := "refs/heads/" + b
	if r != "" {
		ref = "refs/remotes/" + r + "/" + b
	}
	if b == "" || (req.Favorite && !treeRefExists(ctx, tc.lay.top, ref)) {
		return errTreeNotFound()
	}
	f := branchFav{Branch: b, Remote: r}
	return s.updateRepoPrefs(ctx, tc.key, func(p *repoBranchPrefs) {
		out := p.Favorites[:0:0]
		for _, x := range p.Favorites {
			if x != f {
				out = append(out, x)
			}
		}
		if req.Favorite {
			out = append(out, f)
		}
		p.Favorites = out
	})
}

// ---- 新建 / 签出远程 ----

func (s *Service) GitCatCreateBranch(ctx context.Context, req GitCatCreateBranchRequest) (CatBranchTree, error) {
	if req.Checkout {
		if err := s.treeRequireIdle(ctx, req.ConvID); err != nil {
			return CatBranchTree{}, err
		}
	}
	tc, err := s.treeOpen(ctx, req.ConvID)
	if err != nil {
		return CatBranchTree{}, err
	}
	top := tc.lay.top
	from, name := strings.TrimSpace(req.From), strings.TrimSpace(req.Name)
	remote := false
	fromCommit := false
	switch {
	case from != "" && treeRefExists(ctx, top, "refs/heads/"+from):
	case from != "" && treeRefExists(ctx, top, "refs/remotes/"+from):
		remote = true
	case from != "" && treeRefExists(ctx, top, from):
		// 完整 / 短 sha（6.19.28：从这里新建分支）
		fromCommit = true
	default:
		if from != "" && hexQuery.MatchString(from) {
			return CatBranchTree{}, errCommitNotFound()
		}
		return CatBranchTree{}, errTreeNotFound()
	}
	_ = fromCommit
	if !treeValidName(ctx, top, name) {
		return CatBranchTree{}, treeErr(apperr.InvalidArgument, MsgTreeInvalidName, "invalid_branch_name")
	}
	exists := treeRefExists(ctx, top, "refs/heads/"+name)
	// 远程分支「签出」：同名本地已存在则改为签出本地。
	checkoutLocal := exists && remote && req.Checkout && remoteBranchLocalName(from) == name
	if exists && !checkoutLocal {
		return CatBranchTree{}, treeErr(apperr.TaskConflict, MsgTreeBranchExists, "branch_exists")
	}
	var args []string
	if req.Checkout {
		if checkoutLocal && treeCurrent(ctx, top) == name {
			s.recordRecent(ctx, tc, name)
			return s.buildTree(ctx, tc)
		}
		dirty, err := worktreeDirty(ctx, tc.lay)
		if err != nil {
			return CatBranchTree{}, err
		}
		if dirty {
			return CatBranchTree{}, treeErr(apperr.TaskConflict, MsgTreeDirty, "git_dirty")
		}
		switch {
		case checkoutLocal:
			args = []string{"switch", name}
		case remote:
			args = []string{"switch", "--track", "-c", name, from}
		default:
			args = []string{"switch", "-c", name, from}
		}
	} else if remote {
		args = []string{"branch", "--track", name, from}
	} else {
		args = []string{"branch", name, from}
	}
	if _, stderr, err := treeGit(ctx, gitAddTimeout, top, args...); err != nil {
		low := strings.ToLower(stderr)
		switch {
		case strings.Contains(low, "would be overwritten"):
			return CatBranchTree{}, treeErr(apperr.TaskConflict, MsgTreeDirty, "git_dirty")
		case strings.Contains(low, "already exists"):
			return CatBranchTree{}, treeErr(apperr.TaskConflict, MsgTreeBranchExists, "branch_exists")
		}
		return CatBranchTree{}, errTreeGitFailed(stderr)
	}
	if req.Checkout {
		s.recordRecent(ctx, tc, name)
	}
	return s.buildTree(ctx, tc)
}

// ---- 比较 ----

func (s *Service) GitCatCompareBranch(ctx context.Context, req GitCatCompareBranchRequest) (CatBranchCompare, error) {
	tc, err := s.treeOpen(ctx, req.ConvID)
	if err != nil {
		return CatBranchCompare{}, err
	}
	top := tc.lay.top
	b := strings.TrimSpace(req.Branch)
	ref := "refs/heads/" + b
	target := b
	if req.Remote {
		ref = "refs/remotes/" + b
	} else if b != "" && !treeRefExists(ctx, top, ref) && treeRefExists(ctx, top, b) {
		// 允许传 sha（6.19.28）；Target 回显短 sha
		ref = b
		if out, _, err := treeGit(ctx, gitStatusTimeout, top, "rev-parse", "--short", b); err == nil {
			target = strings.TrimSpace(string(out))
		}
	} else if b != "" && !treeRefExists(ctx, top, ref) && hexQuery.MatchString(b) {
		return CatBranchCompare{}, errCommitNotFound()
	}
	if b == "" || !treeRefExists(ctx, top, ref) {
		return CatBranchCompare{}, errTreeNotFound()
	}
	res := CatBranchCompare{Target: target, OnlyInTarget: []CatBranchCommit{}, OnlyInCurrent: []CatBranchCommit{}, Files: []CatCompareFile{}}
	res.Base = treeCurrent(ctx, top)
	if res.Base == "" {
		out, stderr, err := treeGit(ctx, gitStatusTimeout, top, "rev-parse", "--short", "HEAD")
		if err != nil {
			return res, errTreeGitFailed(stderr)
		}
		res.Base = strings.TrimSpace(string(out))
	}
	var tr1, tr2 bool
	if res.OnlyInTarget, tr1, err = treeLog(ctx, top, "HEAD.."+ref); err != nil {
		return res, err
	}
	if res.OnlyInCurrent, tr2, err = treeLog(ctx, top, ref+"..HEAD"); err != nil {
		return res, err
	}
	res.Truncated = tr1 || tr2
	out, stderr, err := treeGit(ctx, gitDiffTimeout, top, "diff", "--name-status", "-M", "-z", "HEAD..."+ref, "--")
	if err != nil {
		return res, errTreeGitFailed(stderr)
	}
	res.Files = parseNameStatusZ(string(out))
	return res, nil
}

func treeLog(ctx context.Context, top, rng string) ([]CatBranchCommit, bool, error) {
	out, stderr, err := treeGit(ctx, gitDiffTimeout, top, "log", "-n", strconv.Itoa(branchCompareMax+1), "--format=%H%x00%ct%x00%s", rng, "--")
	if err != nil {
		return nil, false, errTreeGitFailed(stderr)
	}
	cs := []CatBranchCommit{}
	for _, l := range strings.Split(string(out), "\n") {
		p := strings.SplitN(l, "\x00", 3)
		if len(p) != 3 {
			continue
		}
		t, _ := strconv.ParseInt(p[1], 10, 64)
		cs = append(cs, CatBranchCommit{Sha: p[0], Time: t, Subject: p[2]})
	}
	if len(cs) > branchCompareMax {
		return cs[:branchCompareMax], true, nil
	}
	return cs, false, nil
}

func parseNameStatusZ(out string) []CatCompareFile {
	files := []CatCompareFile{}
	parts := strings.Split(out, "\x00")
	for i := 0; i < len(parts); i++ {
		st := parts[i]
		if st == "" {
			continue
		}
		letter := st[:1]
		if (letter == "R" || letter == "C") && i+2 < len(parts) {
			files = append(files, CatCompareFile{OldPath: parts[i+1], Path: parts[i+2], Status: letter})
			i += 2
			continue
		}
		if i+1 < len(parts) {
			files = append(files, CatCompareFile{Path: parts[i+1], Status: letter})
			i++
		}
	}
	return files
}

// ---- 重命名 / 删除（只限本地） ----

func (s *Service) GitCatRenameBranch(ctx context.Context, req GitCatRenameBranchRequest) (CatBranchTree, error) {
	tc, err := s.treeOpen(ctx, req.ConvID)
	if err != nil {
		return CatBranchTree{}, err
	}
	top := tc.lay.top
	from, to := strings.TrimSpace(req.From), strings.TrimSpace(req.To)
	if from == "" || !treeRefExists(ctx, top, "refs/heads/"+from) {
		return CatBranchTree{}, errTreeNotFound()
	}
	if from == treeCurrent(ctx, top) {
		if err := s.treeRequireIdle(ctx, req.ConvID); err != nil {
			return CatBranchTree{}, err
		}
	}
	if !treeValidName(ctx, top, to) {
		return CatBranchTree{}, treeErr(apperr.InvalidArgument, MsgTreeInvalidName, "invalid_branch_name")
	}
	if treeRefExists(ctx, top, "refs/heads/"+to) {
		return CatBranchTree{}, treeErr(apperr.TaskConflict, MsgTreeBranchExists, "branch_exists")
	}
	if _, stderr, err := treeGit(ctx, gitAddTimeout, top, "branch", "-m", from, to); err != nil {
		if strings.Contains(strings.ToLower(stderr), "already exists") {
			return CatBranchTree{}, treeErr(apperr.TaskConflict, MsgTreeBranchExists, "branch_exists")
		}
		return CatBranchTree{}, errTreeGitFailed(stderr)
	}
	_ = s.updateRepoPrefs(ctx, tc.key, func(p *repoBranchPrefs) {
		for i, f := range p.Favorites {
			if f.Remote == "" && f.Branch == from {
				p.Favorites[i].Branch = to
			}
		}
		for i, r := range p.Recent {
			if r == from {
				p.Recent[i] = to
			}
		}
	})
	return s.buildTree(ctx, tc)
}

func (s *Service) GitCatDeleteBranch(ctx context.Context, req GitCatDeleteBranchRequest) (CatBranchTree, error) {
	if err := s.treeRequireIdle(ctx, req.ConvID); err != nil {
		return CatBranchTree{}, err
	}
	tc, err := s.treeOpen(ctx, req.ConvID)
	if err != nil {
		return CatBranchTree{}, err
	}
	top := tc.lay.top
	b := strings.TrimSpace(req.Branch)
	if b == "" || !treeRefExists(ctx, top, "refs/heads/"+b) {
		return CatBranchTree{}, errTreeNotFound()
	}
	if b == treeCurrent(ctx, top) {
		return CatBranchTree{}, treeErr(apperr.InvalidArgument, MsgTreeIsCurrent, "is_current")
	}
	flag := "-d"
	if req.Force {
		flag = "-D"
	}
	if _, stderr, err := treeGit(ctx, gitAddTimeout, top, "branch", flag, b); err != nil {
		if strings.Contains(strings.ToLower(stderr), "not fully merged") {
			return CatBranchTree{}, treeErr(apperr.TaskConflict, MsgTreeNotMerged, "branch_not_merged")
		}
		return CatBranchTree{}, errTreeGitFailed(stderr)
	}
	_ = s.updateRepoPrefs(ctx, tc.key, func(p *repoBranchPrefs) {
		fs := p.Favorites[:0:0]
		for _, f := range p.Favorites {
			if !(f.Remote == "" && f.Branch == b) {
				fs = append(fs, f)
			}
		}
		rs := []string{}
		for _, r := range p.Recent {
			if r != b {
				rs = append(rs, r)
			}
		}
		p.Favorites, p.Recent = fs, rs
	})
	return s.buildTree(ctx, tc)
}

func (s *Service) recordCheckoutOnSuccess(ctx context.Context, convID string, err *error) {
	if *err == nil {
		s.RecordCatBranchCheckout(ctx, convID)
	}
}
