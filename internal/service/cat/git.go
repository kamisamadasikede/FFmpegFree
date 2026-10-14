package cat

import (
	"bytes"
	"context"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/proc"
)

const (
	gitStatusTimeout = 20 * time.Second
	gitDiffTimeout   = 20 * time.Second
	gitAddTimeout    = 45 * time.Second
	gitCommitTimeout = 90 * time.Second
	gitRemoteTimeout = 2 * time.Minute
	maxCommitRunes   = 4000
	maxUntrackedSize = 256 * 1024
	maxGitDiffLines  = 400
)

// GitStatusRequest 是 GitCatStatus 的入参。
type GitStatusRequest struct {
	ConvID string `json:"convId"`
}

// GitChange 是相对对话根的一条改动。Letter 是 M / A / D / R / U。
type GitChange struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Letter   string `json:"letter"`
	Added    int    `json:"added"`
	Deleted  int    `json:"deleted"`
	Staged   bool   `json:"staged"`
	Unstaged bool   `json:"unstaged"`
}

// GitConflictEntry 是未合并路径（契约 6.19.15）。
type GitConflictEntry struct {
	Path   string `json:"path"`
	Ours   bool   `json:"ours"`
	Theirs bool   `json:"theirs"`
	Letter string `json:"letter,omitempty"`
}

// GitStatusResult 是当前对话文件夹的 git 状态。不是仓库时 Repo 为 false，不算错误。
type GitStatusResult struct {
	Repo       bool               `json:"repo"`
	Branch     string             `json:"branch"`
	Upstream   string             `json:"upstream"`
	Ahead      int                `json:"ahead"`
	Behind     int                `json:"behind"`
	Remote     string             `json:"remote"`
	Web        string             `json:"web"`
	Changes    []GitChange        `json:"changes"`
	MergeState string             `json:"mergeState"` // none | merging | rebasing | cherry
	Conflicts  []GitConflictEntry `json:"conflicts"`
	// v0.53：分离 HEAD 时 Detached=true；HeadSha / HeadShortSha 是当前提交（还没有提交时为空）。
	Detached     bool   `json:"detached"`
	HeadSha      string `json:"headSha"`
	HeadShortSha string `json:"headShortSha"`
}

// GitCatConflictResolveRequest 是单文件冲突解决。
type GitCatConflictResolveRequest struct {
	ConvID  string `json:"convId"`
	Path    string `json:"path"`
	Mode    string `json:"mode"` // ours | theirs | both | content
	Content string `json:"content,omitempty"`
}

// GitCatConflictCompleteRequest 是完成合并 / 变基 / 拣选。
type GitCatConflictCompleteRequest struct {
	ConvID  string `json:"convId"`
	Message string `json:"message,omitempty"`
}

// GitCatConflictAbortRequest 是中止合并 / 变基 / 拣选。
type GitCatConflictAbortRequest struct {
	ConvID string `json:"convId"`
}

// GitCatCheckoutRequest 是切已有分支（不新建）。
type GitCatCheckoutRequest struct {
	ConvID string `json:"convId"`
	Name   string `json:"name"`
	Force  bool   `json:"force,omitempty"`
}

// GitPathsRequest 是暂存、取消暂存和提交的入参。Paths 空表示暂存对话根下的全部改动。
type GitPathsRequest struct {
	ConvID  string   `json:"convId"`
	Paths   []string `json:"paths"`
	Message string   `json:"message"`
}

// GitPRResult 是创建 PR 时要在浏览器打开的地址。不推送。
type GitPRResult struct {
	URL string `json:"url"`
}

// GitRemoteRequest 是推送、拉取、获取、同步。Op 只接受已知的几种。
type GitRemoteRequest struct {
	ConvID string `json:"convId"`
	Op     string `json:"op"`
}

// GitBranchRequest 是新建并切换分支。
type GitBranchRequest struct {
	ConvID string `json:"convId"`
	Name   string `json:"name"`
}

// GitDiffRequest 是 GitCatDiff 的入参。RelPath 相对对话根。
type GitDiffRequest struct {
	ConvID  string `json:"convId"`
	RelPath string `json:"relPath"`
}

// GitDiffResult 是这个文件和上一版提交的行对比。Binary 为真时 Text 为空。
type GitDiffResult struct {
	Path   string `json:"path"`
	Text   string `json:"text"`
	Binary bool   `json:"binary"`
}

type gitLayout struct {
	conv string
	top  string
	// prefix 是对话根相对仓库根的正斜杠路径。对话根就是仓库根时为空。
	prefix string
}

type statusEntry struct {
	letter   string
	path     string
	staged   bool
	unstaged bool
}

func errGitMissing() error {
	return apperr.New(apperr.NotFound, "没有找到 git。")
}

func errNotRepo() error {
	return apperr.New(apperr.InvalidArgument, "这里还不是 git 仓库。")
}

// GitCatStatus 读取对话根所在仓库的分支、远程和改动。路径都相对对话根。
func (s *Service) GitCatStatus(ctx context.Context, req GitStatusRequest) (GitStatusResult, error) {
	root, err := s.convRoot(ctx, req.ConvID)
	if err != nil {
		return GitStatusResult{}, err
	}
	return s.statusAt(ctx, root)
}

// GitCatStage 把路径放进暂存。Paths 为空时暂存对话根下的全部改动。
// Windows 上 nul、con 这类设备名没法进索引。点到它们时跳过，其余文件照常暂存。
func (s *Service) GitCatStage(ctx context.Context, req GitPathsRequest) error {
	lay, err := s.gitLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	if len(req.Paths) == 0 {
		return s.stageAll(ctx, lay)
	}
	rels, err := cleanRels(lay.conv, req.Paths)
	if err != nil {
		return err
	}
	keep, blocked := splitReserved(rels)
	if len(keep) > 0 {
		args := []string{"add", "--"}
		for _, rel := range keep {
			args = append(args, toGitPath(lay.prefix, rel))
		}
		if err := s.runGitAdd(ctx, lay, args); err != nil {
			return err
		}
	}
	if len(blocked) > 0 {
		return errReservedDevice(blocked)
	}
	return nil
}

// stageAll 暂存对话根下的全部改动。有设备名时用排除路径，避免整批失败。
func (s *Service) stageAll(ctx context.Context, lay gitLayout) error {
	st, err := s.statusIn(ctx, lay)
	if err != nil {
		return err
	}
	var blocked []string
	for _, c := range st.Changes {
		if reservedDevice(c.Path) && (!c.Staged || c.Unstaged) {
			blocked = append(blocked, c.Path)
		}
	}
	args := []string{"add", "-A"}
	if lay.prefix != "" || len(blocked) > 0 {
		args = append(args, "--")
		if lay.prefix != "" {
			args = append(args, lay.prefix)
		} else {
			args = append(args, ".")
		}
		for _, rel := range blocked {
			args = append(args, ":(exclude)"+toGitPath(lay.prefix, rel))
		}
	}
	if err := s.runGitAdd(ctx, lay, args); err != nil {
		return err
	}
	if len(blocked) > 0 {
		return errReservedDevice(blocked)
	}
	return nil
}

func (s *Service) runGitAdd(ctx context.Context, lay gitLayout, args []string) error {
	_, stderr, err := runGit(ctx, gitAddTimeout, lay.top, args...)
	if err != nil {
		if gitNotRepo(stderr, err) {
			return errNotRepo()
		}
		return asGitErr(err, stderr, "")
	}
	return nil
}

// reservedDevice 判断 Windows 保留设备名（nul、con、prn、aux、com1–com9、lpt1–lpt9，含扩展名）。
func reservedDevice(rel string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	name := rel
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimRight(name, ". ")
	if dot := strings.IndexByte(name, '.'); dot >= 0 {
		name = name[:dot]
	}
	switch name {
	case "con", "prn", "aux", "nul":
		return true
	}
	if len(name) == 4 {
		head := name[:3]
		if (head == "com" || head == "lpt") && name[3] >= '1' && name[3] <= '9' {
			return true
		}
	}
	return false
}

func splitReserved(rels []string) (keep, blocked []string) {
	for _, rel := range rels {
		if reservedDevice(rel) {
			blocked = append(blocked, rel)
			continue
		}
		keep = append(keep, rel)
	}
	return keep, blocked
}

func errReservedDevice(paths []string) error {
	return apperr.New(apperr.InvalidArgument, strings.Join(paths, "、")+" 在 Windows 上是设备名，没法暂存。")
}

// GitCatUnstage 把已暂存的路径移出暂存。必须点名路径，不会清空全部暂存。
func (s *Service) GitCatUnstage(ctx context.Context, req GitPathsRequest) error {
	lay, err := s.gitLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	rels, err := cleanRels(lay.conv, req.Paths)
	if err != nil {
		return err
	}
	if len(rels) == 0 {
		return apperr.New(apperr.InvalidArgument, "请选择文件。")
	}
	args := []string{"restore", "--staged", "--"}
	for _, rel := range rels {
		args = append(args, toGitPath(lay.prefix, rel))
	}
	_, stderr, err := runGit(ctx, gitAddTimeout, lay.top, args...)
	if err != nil {
		if gitNotRepo(stderr, err) {
			return errNotRepo()
		}
		return asGitErr(err, stderr, "")
	}
	return nil
}

// GitCatCommit 提交说明非空时提交。没有已暂存的改动时，先暂存对话根下的全部改动再提交。
func (s *Service) GitCatCommit(ctx context.Context, req GitPathsRequest) error {
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		return apperr.New(apperr.InvalidArgument, "请填写提交说明。")
	}
	if strings.Contains(msg, "\x00") || utf8.RuneCountInString(msg) > maxCommitRunes {
		return apperr.New(apperr.InvalidArgument, "提交说明太长。")
	}
	lay, err := s.gitLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	st, err := s.statusIn(ctx, lay)
	if err != nil {
		return err
	}
	staged := false
	for _, c := range st.Changes {
		if c.Staged {
			staged = true
			break
		}
	}
	if !staged {
		if err := s.GitCatStage(ctx, GitPathsRequest{ConvID: req.ConvID}); err != nil {
			return err
		}
	}
	args := []string{"commit", "-m", msg}
	if lay.prefix != "" {
		args = append(args, "--", lay.prefix)
	}
	_, stderr, err := runGit(ctx, gitCommitTimeout, lay.top, args...)
	if err != nil {
		if gitNotRepo(stderr, err) {
			return errNotRepo()
		}
		return asGitErr(err, stderr, "")
	}
	return nil
}

// GitCatPullRequest 返回 GitHub 的比较页地址。不推送，也不改远程。
func (s *Service) GitCatPullRequest(ctx context.Context, req GitStatusRequest) (GitPRResult, error) {
	st, err := s.GitCatStatus(ctx, req)
	if err != nil {
		return GitPRResult{}, err
	}
	if !st.Repo {
		return GitPRResult{}, errNotRepo()
	}
	if st.Web == "" {
		return GitPRResult{}, apperr.New(apperr.InvalidArgument, "这个远程地址不是 GitHub，打不开创建页面。")
	}
	if st.Branch == "" || st.Branch == "HEAD" {
		return GitPRResult{URL: st.Web}, nil
	}
	return GitPRResult{URL: st.Web + "/compare/" + url.PathEscape(st.Branch) + "?expand=1"}, nil
}

func errNoUpstream() error {
	return apperr.New(apperr.InvalidArgument, "还没有上游分支。")
}

func errDetached() error {
	return apperr.New(apperr.InvalidArgument, "当前不在分支上。")
}

// GitCatRemote 对当前分支做远程操作。不改提交说明，也不打开网页。
// force 用 --force-with-lease，避免盖过别人刚推上去、本地还没看到的提交。
func (s *Service) GitCatRemote(ctx context.Context, req GitRemoteRequest) error {
	lay, err := s.gitLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	st, err := s.statusIn(ctx, lay)
	if err != nil {
		return err
	}
	op := strings.TrimSpace(req.Op)
	if op != "fetch" && (st.Branch == "" || st.Branch == "HEAD") {
		return errDetached()
	}
	up := st.Upstream != ""
	needUp := op == "push" || op == "force" || op == "pull" || op == "ff" || op == "rebase" || op == "sync"
	if needUp && !up {
		return errNoUpstream()
	}
	switch op {
	case "fetch":
		return s.runGitOp(ctx, lay, "fetch")
	case "publish":
		return s.runGitOp(ctx, lay, "push", "-u", "origin", "HEAD")
	case "push":
		return s.runGitOp(ctx, lay, "push")
	case "force":
		return s.runGitOp(ctx, lay, "push", "--force-with-lease")
	case "pull":
		return s.runGitOpConflict(ctx, lay, "pull")
	case "ff":
		return s.runGitOp(ctx, lay, "pull", "--ff-only")
	case "rebase":
		return s.runGitOpConflict(ctx, lay, "pull", "--rebase")
	case "sync":
		if err := s.runGitOpConflict(ctx, lay, "pull"); err != nil {
			return err
		}
		return s.runGitOp(ctx, lay, "push")
	default:
		return apperr.New(apperr.InvalidArgument, "不认识这个操作。")
	}
}

// GitCatBranch 从当前提交新建分支并切过去。
func (s *Service) GitCatBranch(ctx context.Context, req GitBranchRequest) error {
	name := strings.TrimSpace(req.Name)
	if !validBranch(name) {
		return apperr.New(apperr.InvalidArgument, "分支名称不能用。")
	}
	lay, err := s.gitLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	return s.runGitOp(ctx, lay, "checkout", "-b", name)
}

func errGitConflict(msg string) error {
	return apperr.New(apperr.InvalidArgument, msg).WithDetail("reason=git_conflict")
}

func errGitDirty() error {
	return apperr.New(apperr.TaskConflict, "").WithDetail("reason=git_dirty")
}

// GitCatConflictResolve 对单个冲突文件采用当前 / 传入 / 双方或手改后暂存。
func (s *Service) GitCatConflictResolve(ctx context.Context, req GitCatConflictResolveRequest) error {
	lay, err := s.gitLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	if detectMergeState(ctx, lay.top) == "none" {
		return errGitConflict("当前没有进行中的合并。")
	}
	rel, err := cleanRel(lay.conv, req.Path)
	if err != nil {
		return err
	}
	conflicts, err := listConflicts(ctx, lay)
	if err != nil {
		return err
	}
	var hit *GitConflictEntry
	for i := range conflicts {
		if conflicts[i].Path == rel {
			hit = &conflicts[i]
			break
		}
	}
	if hit == nil {
		return apperr.New(apperr.InvalidArgument, "这个文件不在冲突列表里。").WithDetail("reason=path")
	}
	gitPath := toGitPath(lay.prefix, rel)
	mode := strings.TrimSpace(req.Mode)
	switch mode {
	case "ours", "theirs":
		flag := "--ours"
		if mode == "theirs" {
			flag = "--theirs"
		}
		if _, stderr, err := runGit(ctx, gitAddTimeout, lay.top, "checkout", flag, "--", gitPath); err != nil {
			if gitNotRepo(stderr, err) {
				return errNotRepo()
			}
			return asGitErr(err, stderr, "")
		}
	case "both":
		body, err := combineBothSides(ctx, lay.top, gitPath)
		if err != nil {
			return err
		}
		if err := writeConvFile(lay.conv, rel, body); err != nil {
			return err
		}
	case "content":
		if conflictLooksBinary(ctx, lay, gitPath, rel) {
			return apperr.New(apperr.InvalidArgument, "这个文件是二进制，请选用当前或传入。")
		}
		if strings.Contains(req.Content, "\x00") {
			return apperr.New(apperr.InvalidArgument, "这个文件是二进制，请选用当前或传入。")
		}
		if err := writeConvFile(lay.conv, rel, []byte(req.Content)); err != nil {
			return err
		}
	default:
		return apperr.New(apperr.InvalidArgument, "不认识这个解决方式。")
	}
	if _, stderr, err := runGit(ctx, gitAddTimeout, lay.top, "add", "--", gitPath); err != nil {
		if gitNotRepo(stderr, err) {
			return errNotRepo()
		}
		return asGitErr(err, stderr, "")
	}
	return nil
}

// GitCatConflictComplete 在无未解决冲突时完成合并 / 变基 / 拣选。
func (s *Service) GitCatConflictComplete(ctx context.Context, req GitCatConflictCompleteRequest) error {
	lay, err := s.gitLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	state := detectMergeState(ctx, lay.top)
	if state == "none" {
		return errGitConflict("当前没有进行中的合并。")
	}
	conflicts, err := listConflicts(ctx, lay)
	if err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return errGitConflict("还有未解决的冲突。")
	}
	switch state {
	case "merging":
		msg := strings.TrimSpace(req.Message)
		var args []string
		if msg == "" {
			args = []string{"commit", "--no-edit"}
		} else {
			if strings.Contains(msg, "\x00") || utf8.RuneCountInString(msg) > maxCommitRunes {
				return apperr.New(apperr.InvalidArgument, "提交说明太长。")
			}
			args = []string{"commit", "-m", msg}
		}
		_, stderr, err := runGit(ctx, gitCommitTimeout, lay.top, args...)
		if err != nil {
			if gitNotRepo(stderr, err) {
				return errNotRepo()
			}
			return asGitErr(err, stderr, "")
		}
		return nil
	case "rebasing":
		_, stderr, err := runGit(ctx, gitCommitTimeout, lay.top, "rebase", "--continue")
		if err != nil {
			if gitNotRepo(stderr, err) {
				return errNotRepo()
			}
			if detectMergeState(ctx, lay.top) != "none" {
				return errGitConflict("有冲突需要解决。")
			}
			return asGitErr(err, stderr, "")
		}
		return nil
	case "cherry":
		_, stderr, err := runGit(ctx, gitCommitTimeout, lay.top, "cherry-pick", "--continue")
		if err != nil {
			if gitNotRepo(stderr, err) {
				return errNotRepo()
			}
			if detectMergeState(ctx, lay.top) != "none" {
				return errGitConflict("有冲突需要解决。")
			}
			return asGitErr(err, stderr, "")
		}
		return nil
	default:
		return errGitConflict("当前没有进行中的合并。")
	}
}

// GitCatConflictAbort 中止进行中的合并 / 变基 / 拣选。
func (s *Service) GitCatConflictAbort(ctx context.Context, req GitCatConflictAbortRequest) error {
	lay, err := s.gitLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	state := detectMergeState(ctx, lay.top)
	var args []string
	switch state {
	case "merging":
		args = []string{"merge", "--abort"}
	case "rebasing":
		args = []string{"rebase", "--abort"}
	case "cherry":
		args = []string{"cherry-pick", "--abort"}
	default:
		return errGitConflict("当前没有进行中的合并。")
	}
	_, stderr, err := runGit(ctx, gitCommitTimeout, lay.top, args...)
	if err != nil {
		if gitNotRepo(stderr, err) {
			return errNotRepo()
		}
		return asGitErr(err, stderr, "")
	}
	return nil
}

// GitCatCheckout 切到已有本地分支。脏工作区须 Force 确认；不新建分支。
func (s *Service) GitCatCheckout(ctx context.Context, req GitCatCheckoutRequest) error {
	name := strings.TrimSpace(req.Name)
	if name == "" || strings.Contains(name, "\x00") {
		return apperr.New(apperr.NotFound, "找不到这个分支。")
	}
	lay, err := s.gitLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	if !localBranchExists(ctx, lay.top, name) {
		return apperr.New(apperr.NotFound, "找不到这个分支。")
	}
	if !req.Force {
		dirty, err := worktreeDirty(ctx, lay)
		if err != nil {
			return err
		}
		if dirty {
			return errGitDirty()
		}
	}
	_, stderr, err := runGit(ctx, gitAddTimeout, lay.top, "checkout", name)
	if err != nil {
		if gitNotRepo(stderr, err) {
			return errNotRepo()
		}
		return asGitErr(err, stderr, "")
	}
	return nil
}

func (s *Service) runGitOp(ctx context.Context, lay gitLayout, args ...string) error {
	_, stderr, err := runGit(ctx, gitRemoteTimeout, lay.top, args...)
	if err != nil {
		if gitNotRepo(stderr, err) {
			return errNotRepo()
		}
		return asGitErr(err, stderr, "")
	}
	return nil
}

// runGitOpConflict 用于可能产生冲突的远程操作；失败且仓库进入冲突态时标 git_conflict。
func (s *Service) runGitOpConflict(ctx context.Context, lay gitLayout, args ...string) error {
	_, stderr, err := runGit(ctx, gitRemoteTimeout, lay.top, args...)
	if err != nil {
		if gitNotRepo(stderr, err) {
			return errNotRepo()
		}
		if detectMergeState(ctx, lay.top) != "none" {
			return errGitConflict("有冲突需要解决。")
		}
		return asGitErr(err, stderr, "")
	}
	return nil
}

func validBranch(name string) bool {
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return false
	}
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".") || strings.HasSuffix(name, ".lock") {
		return false
	}
	if strings.Contains(name, "..") || strings.Contains(name, "@{") || strings.Contains(name, "//") {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(" ~^:?*[\\", r) {
			return false
		}
	}
	return true
}

// GitCatDiff 比较对话根下的一个文件和上一版提交。未跟踪的新文件按整文件新增显示。
func (s *Service) GitCatDiff(ctx context.Context, req GitDiffRequest) (GitDiffResult, error) {
	lay, err := s.gitLayout(ctx, req.ConvID)
	if err != nil {
		return GitDiffResult{}, err
	}
	rel, err := cleanRel(lay.conv, req.RelPath)
	if err != nil {
		return GitDiffResult{}, err
	}
	gitPath := toGitPath(lay.prefix, rel)
	out, stderr, err := runGit(ctx, gitDiffTimeout, lay.top,
		"--literal-pathspecs", "--no-pager",
		"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--unified=3", "-M", "HEAD", "--", gitPath,
	)
	if err != nil {
		if gitNotRepo(stderr, err) {
			return GitDiffResult{}, errNotRepo()
		}
		// 还没有任何提交时，HEAD 不存在。工作区里的文件就是相对空版本的新增。
		if gitNoHead(stderr) {
			return readNewDiff(lay.conv, rel)
		}
		return GitDiffResult{}, asGitErr(err, stderr, string(out))
	}
	text, binary := formatUnifiedDiff(string(out))
	if binary {
		return GitDiffResult{Path: rel, Binary: true}, nil
	}
	if strings.TrimSpace(text) == "" {
		// git diff 不列出未跟踪文件。上一版里没有它时，按磁盘内容显示成新增。
		has, herr := headHasBlob(ctx, lay.top, gitPath)
		if herr != nil {
			return GitDiffResult{}, herr
		}
		if !has {
			return readNewDiff(lay.conv, rel)
		}
	}
	return GitDiffResult{Path: rel, Text: text}, nil
}

func (s *Service) convRoot(ctx context.Context, convID string) (string, error) {
	c, err := s.requireConv(ctx, convID)
	if err != nil {
		return "", err
	}
	return s.conversationRoot(ctx, c)
}

func (s *Service) gitLayout(ctx context.Context, convID string) (gitLayout, error) {
	root, err := s.convRoot(ctx, convID)
	if err != nil {
		return gitLayout{}, err
	}
	lay, ok, err := findGitLayout(ctx, root)
	if err != nil {
		return gitLayout{}, err
	}
	if !ok {
		return gitLayout{}, errNotRepo()
	}
	return lay, nil
}

func (s *Service) statusAt(ctx context.Context, convRoot string) (GitStatusResult, error) {
	lay, ok, err := findGitLayout(ctx, convRoot)
	if err != nil {
		return GitStatusResult{}, err
	}
	if !ok {
		return GitStatusResult{Changes: []GitChange{}, MergeState: "none", Conflicts: []GitConflictEntry{}}, nil
	}
	return s.statusIn(ctx, lay)
}

func (s *Service) statusIn(ctx context.Context, lay gitLayout) (GitStatusResult, error) {
	out, stderr, err := runGit(ctx, gitStatusTimeout, lay.top, "status", "--porcelain=v1", "-b", "-z", "-uall", "--ahead-behind")
	if err != nil {
		if gitNotRepo(stderr, err) {
			return GitStatusResult{Changes: []GitChange{}, MergeState: "none", Conflicts: []GitConflictEntry{}}, nil
		}
		return GitStatusResult{}, asGitErr(err, stderr, string(out))
	}
	branch, upstream, ahead, behind, entries := parseStatusZ(out)
	unstaged, _, _ := runGit(ctx, gitStatusTimeout, lay.top, "diff", "--numstat", "-z")
	staged, _, _ := runGit(ctx, gitStatusTimeout, lay.top, "diff", "--cached", "--numstat", "-z")
	counts := parseNumstatZ(unstaged)
	for path, n := range parseNumstatZ(staged) {
		cur := counts[path]
		cur[0] += n[0]
		cur[1] += n[1]
		counts[path] = cur
	}
	remoteOut, _, remoteErr := runGit(ctx, gitStatusTimeout, lay.top, "remote", "get-url", "origin")
	remote := ""
	if remoteErr == nil {
		remote = strings.TrimSpace(string(remoteOut))
	}
	web := githubWeb(remote)
	changes := make([]GitChange, 0, len(entries))
	counted := 0
	for _, e := range entries {
		rel, ok := fromGitPath(lay.prefix, e.path)
		if !ok || rel == "" {
			continue
		}
		name, dir := splitRel(rel)
		item := GitChange{
			Path: rel, Name: name, Dir: dir, Letter: e.letter,
			Staged: e.staged, Unstaged: e.unstaged,
		}
		if n, ok := counts[filepath.ToSlash(e.path)]; ok {
			item.Added, item.Deleted = n[0], n[1]
		} else if e.letter == "U" && counted < 200 && !reservedDevice(rel) {
			item.Added = untrackedAdded(lay.conv, rel)
			counted++
		}
		changes = append(changes, item)
	}
	sort.SliceStable(changes, func(i, j int) bool {
		return strings.ToLower(changes[i].Path) < strings.ToLower(changes[j].Path)
	})
	mergeState := detectMergeState(ctx, lay.top)
	conflicts := []GitConflictEntry{}
	if mergeState != "none" {
		var cerr error
		conflicts, cerr = listConflicts(ctx, lay)
		if cerr != nil {
			return GitStatusResult{}, cerr
		}
	}
	if conflicts == nil {
		conflicts = []GitConflictEntry{}
	}
	res := GitStatusResult{
		Repo: true, Branch: branch, Upstream: upstream, Ahead: ahead, Behind: behind,
		Remote: remote, Web: web, Changes: changes,
		MergeState: mergeState, Conflicts: conflicts,
	}
	fillHeadInfo(ctx, lay.top, &res)
	return res, nil
}

func findGitLayout(ctx context.Context, convRoot string) (gitLayout, bool, error) {
	out, stderr, err := runGit(ctx, gitStatusTimeout, convRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		if gitNotRepo(stderr, err) {
			return gitLayout{}, false, nil
		}
		return gitLayout{}, false, asGitErr(err, stderr, string(out))
	}
	top := filepath.Clean(strings.TrimSpace(string(out)))
	prefix, ok := relUnder(top, convRoot)
	if !ok {
		return gitLayout{}, false, nil
	}
	return gitLayout{conv: filepath.Clean(convRoot), top: top, prefix: prefix}, true, nil
}

func runGit(ctx context.Context, timeout time.Duration, dir string, args ...string) ([]byte, string, error) {
	return runGitEnv(ctx, timeout, dir, nil, args...)
}

// runGitEnv 同 runGit，额外追加环境变量（后者覆盖前者）。
func runGitEnv(ctx context.Context, timeout time.Duration, dir string, extraEnv []string, args ...string) ([]byte, string, error) {
	exe, err := exec.LookPath("git")
	if err != nil {
		return nil, "", errGitMissing()
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, exe, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"GIT_MERGE_AUTOEDIT=no",
		"GIT_EDITOR=true",
		"EDITOR=true",
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	proc.Configure(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	errText := stderr.String()
	if err != nil && cctx.Err() == context.DeadlineExceeded && strings.TrimSpace(errText) == "" {
		errText = "git 超时。"
	}
	return stdout.Bytes(), errText, err
}

func gitNotRepo(stderr string, err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(stderr)
	return strings.Contains(s, "not a git repository") || strings.Contains(s, "不是 git 仓库")
}

func asGitErr(err error, stderr, stdout string) error {
	if err == nil || apperr.Is(err, apperr.NotFound) {
		return err
	}
	return gitFail(err, stderr, stdout)
}

func gitFail(runErr error, stderr, stdout string) error {
	msg := clipText(firstNonEmpty(strings.TrimSpace(stderr), strings.TrimSpace(stdout)), 500)
	if msg == "" {
		msg = "git 没有完成。"
	}
	return apperr.Wrap(apperr.ProcessFailed, msg, runErr)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func clipText(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func parseStatusZ(b []byte) (branch, upstream string, ahead, behind int, entries []statusEntry) {
	parts := bytes.Split(b, []byte{0})
	i := 0
	if len(parts) > 0 && bytes.HasPrefix(parts[0], []byte("## ")) {
		branch, upstream, ahead, behind = parseBranch(string(parts[0]))
		i = 1
	}
	for i < len(parts) {
		line := parts[i]
		i++
		if len(line) < 4 {
			continue
		}
		x, y := line[0], line[1]
		path := string(line[3:])
		if path == "" {
			continue
		}
		if x == 'R' || y == 'R' || x == 'C' || y == 'C' {
			// -z 里改名是「新路径 \0 旧路径」。显示新路径。
			if i < len(parts) {
				i++
			}
		}
		letter, staged, unstaged := statusLetter(x, y)
		entries = append(entries, statusEntry{letter: letter, path: filepath.ToSlash(path), staged: staged, unstaged: unstaged})
	}
	return branch, upstream, ahead, behind, entries
}

func parseBranch(line string) (branch, upstream string, ahead, behind int) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "## "))
	if line == "" {
		return "", "", 0, 0
	}
	if strings.HasPrefix(line, "HEAD (no branch)") {
		return "HEAD", "", 0, 0
	}
	if rest, ok := strings.CutPrefix(line, "No commits yet on "); ok {
		return strings.TrimSpace(rest), "", 0, 0
	}
	if i := strings.LastIndex(line, " ["); i >= 0 && strings.HasSuffix(line, "]") {
		meta := line[i+2 : len(line)-1]
		line = strings.TrimSpace(line[:i])
		ahead = leadingInt(meta, "ahead ")
		behind = leadingInt(meta, "behind ")
	}
	if i := strings.Index(line, "..."); i >= 0 {
		return strings.TrimSpace(line[:i]), shortUpstream(strings.TrimSpace(line[i+3:])), ahead, behind
	}
	return line, "", ahead, behind
}

func leadingInt(s, key string) int {
	i := strings.Index(s, key)
	if i < 0 {
		return 0
	}
	rest := s[i+len(key):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return n
}

func shortUpstream(full string) string {
	if i := strings.Index(full, "/"); i >= 0 && i < len(full)-1 {
		return full[i+1:]
	}
	return full
}

func statusLetter(x, y byte) (letter string, staged, unstaged bool) {
	if x == '?' && y == '?' {
		return "U", false, true
	}
	staged = x != ' ' && x != '?'
	unstaged = y != ' ' && y != '?'
	has := func(c byte) bool { return x == c || y == c }
	switch {
	case has('D'):
		letter = "D"
	case has('U'), has('M'), has('T'):
		letter = "M"
	case has('R'):
		letter = "R"
	case has('A'), has('C'):
		letter = "A"
	default:
		letter = "M"
	}
	return letter, staged, unstaged
}

func parseNumstatZ(b []byte) map[string][2]int {
	out := map[string][2]int{}
	i := 0
	for i < len(b) {
		j := bytes.IndexByte(b[i:], 0)
		if j < 0 {
			break
		}
		rec := string(b[i : i+j])
		i += j + 1
		fields := strings.SplitN(rec, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		path := fields[2]
		if path == "" {
			if i >= len(b) {
				break
			}
			j = bytes.IndexByte(b[i:], 0)
			if j < 0 {
				break
			}
			i += j + 1 // 旧路径
			if i >= len(b) {
				break
			}
			j = bytes.IndexByte(b[i:], 0)
			if j < 0 {
				break
			}
			path = string(b[i : i+j])
			i += j + 1
		}
		path = numstatPath(filepath.ToSlash(path))
		if path == "" {
			continue
		}
		cur := out[path]
		cur[0] += atoiCount(fields[0])
		cur[1] += atoiCount(fields[1])
		out[path] = cur
	}
	return out
}

func numstatPath(p string) string {
	if !strings.Contains(p, " => ") {
		return p
	}
	if l := strings.Index(p, "{"); l >= 0 {
		if r := strings.Index(p, "}"); r > l {
			mid := strings.SplitN(p[l+1:r], " => ", 2)
			if len(mid) == 2 {
				return p[:l] + mid[1] + p[r+1:]
			}
		}
	}
	parts := strings.SplitN(p, " => ", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return p
}

func atoiCount(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func untrackedAdded(convRoot, rel string) int {
	p := filepath.Join(convRoot, filepath.FromSlash(rel))
	if !lexicalInside(convRoot, p) {
		return 0
	}
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 || fi.Size() > maxUntrackedSize {
		return 0
	}
	b, err := os.ReadFile(p)
	if err != nil || bytes.IndexByte(b, 0) >= 0 {
		return 0
	}
	n := bytes.Count(b, []byte("\n"))
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

func cleanRels(root string, paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, raw := range paths {
		rel, err := cleanRel(root, raw)
		if err != nil {
			return nil, err
		}
		key := rel
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			key = strings.ToLower(rel)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, rel)
	}
	return out, nil
}

func cleanRel(root, rel string) (string, error) {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, `\`, "/"))
	rel = strings.TrimPrefix(rel, "./")
	if rel == "" || rel == "." || strings.Contains(rel, ":") || strings.HasPrefix(rel, "/") {
		return "", errFilePath()
	}
	var parts []string
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.EqualFold(seg, ".git") {
			return "", errFilePath()
		}
		parts = append(parts, seg)
	}
	rel = strings.Join(parts, "/")
	target := filepath.Join(root, filepath.FromSlash(rel))
	if !lexicalInside(root, target) {
		return "", errFilePath()
	}
	return rel, nil
}

func toGitPath(prefix, rel string) string {
	if prefix == "" {
		return rel
	}
	return prefix + "/" + rel
}

func fromGitPath(prefix, gitPath string) (string, bool) {
	gitPath = filepath.ToSlash(strings.TrimSpace(gitPath))
	if gitPath == "" {
		return "", false
	}
	if prefix == "" {
		return gitPath, true
	}
	got := stripPrefix(gitPath, prefix)
	if got == "" && !hasRelPrefix(gitPath, prefix) {
		return "", false
	}
	if got == "" {
		return "", false
	}
	return got, true
}

func hasRelPrefix(path, prefix string) bool {
	if prefix == "" {
		return true
	}
	a, b := path, prefix
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		a, b = strings.ToLower(a), strings.ToLower(b)
	}
	return a == b || strings.HasPrefix(a, b+"/")
}

func stripPrefix(path, prefix string) string {
	if prefix == "" {
		return path
	}
	parts := strings.Split(path, "/")
	pre := strings.Split(prefix, "/")
	if len(parts) <= len(pre) {
		return ""
	}
	for i, seg := range pre {
		a, b := seg, parts[i]
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			a, b = strings.ToLower(a), strings.ToLower(b)
		}
		if a != b {
			return ""
		}
	}
	return strings.Join(parts[len(pre):], "/")
}

func relUnder(top, conv string) (string, bool) {
	top, conv = filepath.Clean(top), filepath.Clean(conv)
	// 本机临时目录常是 8.3 短路径，git 却打出长路径。同一文件夹按文件身份认，不当成仓库外面。
	if eqPath(top, conv) {
		return "", true
	}
	if !lexicalInside(top, conv) {
		return "", false
	}
	rel, err := filepath.Rel(top, conv)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return "", true
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

func eqPath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		if strings.EqualFold(a, b) {
			return true
		}
	} else if a == b {
		return true
	}
	ia, ea := os.Stat(a)
	ib, eb := os.Stat(b)
	return ea == nil && eb == nil && os.SameFile(ia, ib)
}

func splitRel(rel string) (name, dir string) {
	rel = filepath.ToSlash(rel)
	i := strings.LastIndex(rel, "/")
	if i < 0 {
		return rel, ""
	}
	return rel[i+1:], rel[:i]
}

func githubWeb(remote string) string {
	owner, repo, ok := parseGitHubRemote(remote)
	if !ok {
		return ""
	}
	return "https://github.com/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
}

func parseGitHubRemote(raw string) (owner, repo string, ok bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "/")
	if strings.HasPrefix(s, "git@github.com:") {
		return splitOwnerRepo(strings.TrimSuffix(strings.TrimPrefix(s, "git@github.com:"), ".git"))
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", "", false
	}
	host := strings.ToLower(u.Hostname())
	if h, _, err := net.SplitHostPort(u.Host); err == nil {
		host = strings.ToLower(h)
	}
	if host != "github.com" && host != "www.github.com" {
		return "", "", false
	}
	return splitOwnerRepo(strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"))
}

func splitOwnerRepo(p string) (string, string, bool) {
	parts := strings.Split(p, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func gitNoHead(stderr string) bool {
	s := strings.ToLower(stderr)
	if !strings.Contains(s, "head") {
		return false
	}
	return strings.Contains(s, "bad revision") || strings.Contains(s, "ambiguous argument") ||
		strings.Contains(s, "unknown revision") || strings.Contains(s, "坏的版本") ||
		strings.Contains(s, "有歧义") || strings.Contains(s, "未知的版本") || strings.Contains(s, "未知的修订")
}

func headHasBlob(ctx context.Context, top, gitPath string) (bool, error) {
	_, stderr, err := runGit(ctx, gitDiffTimeout, top, "cat-file", "-e", "HEAD:"+gitPath)
	if err == nil {
		return true, nil
	}
	if gitNotRepo(stderr, err) {
		return false, errNotRepo()
	}
	if apperr.Is(err, apperr.NotFound) {
		return false, err
	}
	return false, nil
}

// readNewDiff 把磁盘上的新文件收成「+」行。没有文件时返回空对比。
func readNewDiff(convRoot, rel string) (GitDiffResult, error) {
	p := filepath.Join(convRoot, filepath.FromSlash(rel))
	if !lexicalInside(convRoot, p) {
		return GitDiffResult{}, errFilePath()
	}
	fi, err := os.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return GitDiffResult{Path: rel}, nil
		}
		return GitDiffResult{}, apperr.Wrap(apperr.ProcessFailed, "读不到这个文件。", err)
	}
	if !fi.Mode().IsRegular() || fi.Size() > maxUntrackedSize {
		return GitDiffResult{Path: rel, Binary: true}, nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return GitDiffResult{}, apperr.Wrap(apperr.ProcessFailed, "读不到这个文件。", err)
	}
	if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
		return GitDiffResult{Path: rel, Binary: true}, nil
	}
	return GitDiffResult{Path: rel, Text: capDiffText(prefixPlus(string(b)))}, nil
}

func prefixPlus(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = "+" + strings.TrimSuffix(line, "\r")
	}
	return strings.Join(lines, "\n")
}

func capDiffText(s string) string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxGitDiffLines {
		return s
	}
	return strings.Join(lines[:maxGitDiffLines], "\n") + "\n…其余已省略"
}

// formatUnifiedDiff 留下 @@ 和正文行，丢掉文件头。二进制对比不返回正文。
func formatUnifiedDiff(raw string) (string, bool) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if binaryDiff(raw) {
		return "", true
	}
	var kept []string
	inHunk := false
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "diff --git ") {
			inHunk = false
			continue
		}
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			kept = append(kept, line)
			if len(kept) > maxGitDiffLines {
				break
			}
			continue
		}
		if !inHunk || line == "" || strings.HasPrefix(line, `\`) {
			continue
		}
		switch line[0] {
		case '+', '-', ' ':
			kept = append(kept, line)
		default:
			continue
		}
		if len(kept) > maxGitDiffLines {
			break
		}
	}
	if len(kept) > maxGitDiffLines {
		kept = append(kept[:maxGitDiffLines], "…其余已省略")
	}
	return strings.Join(kept, "\n"), false
}

func binaryDiff(raw string) bool {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "Binary files ") && strings.HasSuffix(strings.TrimSpace(line), "differ") {
			return true
		}
		if strings.HasPrefix(line, "GIT binary patch") {
			return true
		}
	}
	return false
}

func detectMergeState(ctx context.Context, top string) string {
	if gitPathExists(ctx, top, "MERGE_HEAD") {
		return "merging"
	}
	if gitPathExists(ctx, top, "rebase-merge") || gitPathExists(ctx, top, "rebase-apply") {
		return "rebasing"
	}
	if gitPathExists(ctx, top, "CHERRY_PICK_HEAD") {
		return "cherry"
	}
	return "none"
}

func gitPathExists(ctx context.Context, top, name string) bool {
	out, _, err := runGit(ctx, gitStatusTimeout, top, "rev-parse", "--git-path", name)
	if err != nil {
		return false
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return false
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(top, p)
	}
	_, err = os.Stat(p)
	return err == nil
}

func listConflicts(ctx context.Context, lay gitLayout) ([]GitConflictEntry, error) {
	out, stderr, err := runGit(ctx, gitStatusTimeout, lay.top, "diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		if gitNotRepo(stderr, err) {
			return nil, errNotRepo()
		}
		return nil, asGitErr(err, stderr, string(out))
	}
	stages, letters := unmergedMeta(ctx, lay.top)
	seen := map[string]bool{}
	var list []GitConflictEntry
	for _, raw := range bytes.Split(out, []byte{0}) {
		gp := filepath.ToSlash(strings.TrimSpace(string(raw)))
		if gp == "" || seen[gp] {
			continue
		}
		seen[gp] = true
		rel, ok := fromGitPath(lay.prefix, gp)
		if !ok || rel == "" {
			continue
		}
		meta := stages[gp]
		entry := GitConflictEntry{
			Path:   rel,
			Ours:   meta[0],
			Theirs: meta[1],
			Letter: letters[gp],
		}
		list = append(list, entry)
	}
	sort.SliceStable(list, func(i, j int) bool {
		return strings.ToLower(list[i].Path) < strings.ToLower(list[j].Path)
	})
	if list == nil {
		list = []GitConflictEntry{}
	}
	return list, nil
}

// unmergedMeta 读 ls-files -u 与 porcelain，得到 path→(ours,theirs) 与 XY 字母。
func unmergedMeta(ctx context.Context, top string) (stages map[string][2]bool, letters map[string]string) {
	stages = map[string][2]bool{}
	letters = map[string]string{}
	out, _, err := runGit(ctx, gitStatusTimeout, top, "ls-files", "-u", "-z")
	if err == nil {
		for _, rec := range bytes.Split(out, []byte{0}) {
			line := string(rec)
			if line == "" {
				continue
			}
			tab := strings.IndexByte(line, '\t')
			if tab < 0 {
				continue
			}
			meta, path := line[:tab], filepath.ToSlash(line[tab+1:])
			fields := strings.Fields(meta)
			if len(fields) < 3 || path == "" {
				continue
			}
			cur := stages[path]
			switch fields[2] {
			case "2":
				cur[0] = true
			case "3":
				cur[1] = true
			}
			stages[path] = cur
		}
	}
	pout, _, perr := runGit(ctx, gitStatusTimeout, top, "status", "--porcelain=v1", "-z", "--untracked-files=no")
	if perr == nil {
		parts := bytes.Split(pout, []byte{0})
		i := 0
		for i < len(parts) {
			line := parts[i]
			i++
			if len(line) < 4 {
				continue
			}
			x, y := line[0], line[1]
			path := filepath.ToSlash(string(line[3:]))
			if path == "" {
				continue
			}
			if x == 'R' || y == 'R' || x == 'C' || y == 'C' {
				if i < len(parts) {
					i++
				}
			}
			if isUnmergedXY(x, y) {
				letters[path] = string([]byte{x, y})
			}
		}
	}
	return stages, letters
}

func isUnmergedXY(x, y byte) bool {
	pair := string([]byte{x, y})
	switch pair {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	}
	return x == 'U' || y == 'U'
}

func combineBothSides(ctx context.Context, top, gitPath string) ([]byte, error) {
	ours, oerr := showStage(ctx, top, "2", gitPath)
	theirs, terr := showStage(ctx, top, "3", gitPath)
	if oerr != nil && terr != nil {
		return nil, apperr.New(apperr.InvalidArgument, "读不到冲突双方的内容。")
	}
	var b []byte
	if oerr == nil {
		b = append(b, ours...)
	}
	if terr == nil {
		if len(b) > 0 && b[len(b)-1] != '\n' {
			b = append(b, '\n')
		}
		b = append(b, theirs...)
	}
	return b, nil
}

func showStage(ctx context.Context, top, stage, gitPath string) ([]byte, error) {
	out, stderr, err := runGit(ctx, gitDiffTimeout, top, "show", ":"+stage+":"+gitPath)
	if err != nil {
		if gitNotRepo(stderr, err) {
			return nil, errNotRepo()
		}
		return nil, err
	}
	return out, nil
}

func writeConvFile(convRoot, rel string, body []byte) error {
	abs := filepath.Join(convRoot, filepath.FromSlash(rel))
	if !lexicalInside(convRoot, abs) {
		return errFilePath()
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return apperr.Wrap(apperr.ProcessFailed, "写不了这个文件。", err)
	}
	if err := os.WriteFile(abs, body, 0o644); err != nil {
		return apperr.Wrap(apperr.ProcessFailed, "写不了这个文件。", err)
	}
	return nil
}

func conflictLooksBinary(ctx context.Context, lay gitLayout, gitPath, rel string) bool {
	for _, stage := range []string{"2", "3"} {
		b, err := showStage(ctx, lay.top, stage, gitPath)
		if err == nil && bytesLookBinary(b) {
			return true
		}
	}
	abs := filepath.Join(lay.conv, filepath.FromSlash(rel))
	if lexicalInside(lay.conv, abs) {
		if b, err := os.ReadFile(abs); err == nil && bytesLookBinary(b) {
			return true
		}
	}
	return false
}

func bytesLookBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	return bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b)
}

func localBranchExists(ctx context.Context, top, name string) bool {
	_, _, err := runGit(ctx, gitStatusTimeout, top, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

func worktreeDirty(ctx context.Context, lay gitLayout) (bool, error) {
	out, stderr, err := runGit(ctx, gitStatusTimeout, lay.top, "status", "--porcelain=v1", "-z", "-uall")
	if err != nil {
		if gitNotRepo(stderr, err) {
			return false, errNotRepo()
		}
		return false, asGitErr(err, stderr, string(out))
	}
	for _, part := range bytes.Split(out, []byte{0}) {
		if len(bytes.TrimSpace(part)) > 0 {
			return true, nil
		}
	}
	return false, nil
}
