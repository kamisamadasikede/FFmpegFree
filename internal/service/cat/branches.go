package cat

import (
	"context"
	"os/exec"
	"sort"
	"strings"

	"FFmpegFree/internal/apperr"
)

// 契约 v0.49（6.19.24）：输入框底栏的文件夹 / 远程分支 / 本地分支。

const (
	MsgBranchDirty       = "有未提交的改动，先提交或暂存再切换分支。"
	MsgBranchTurnRunning = "正在回复，等这一轮结束再切换分支。"
	MsgBranchNotFound    = "找不到这个分支。"
)

// CatBranchesRequest 是 ListCatBranches / SwitchCatBranch 的对话定位。
type CatBranchesRequest struct {
	ConvID string `json:"convId"`
}

// CatBranchesResult 是底栏要的全部信息。Git=false（没装 git）或 Repo=false 时分支字段都为空，Root 照样给。
type CatBranchesResult struct {
	Root     string   `json:"root"`
	Git      bool     `json:"git"`
	Repo     bool     `json:"repo"`
	Branch   string   `json:"branch"`   // 当前本地分支；分离 HEAD 时为空
	Detached bool     `json:"detached"` // 分离 HEAD
	Upstream string   `json:"upstream"` // 当前分支的上游，如 origin/main；没有为空
	Locals   []string `json:"locals"`   // 全部本地分支，按名字排序
	Remotes  []string `json:"remotes"`  // 只在远程、本地还没有同名分支的，如 origin/feat；不含 */HEAD
}

// SwitchCatBranchRequest 切分支。Remote=true 时 Branch 是 origin/feat 形态，本地没有同名分支就新建跟踪分支。
type SwitchCatBranchRequest struct {
	ConvID string `json:"convId"`
	Branch string `json:"branch"`
	Remote bool   `json:"remote,omitempty"`
}

func gitInstalled() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// ListCatBranches 读对话根（项目文件夹或普通对话的文件夹）和它所在仓库的分支。不是仓库、没装 git 都不算错。
func (s *Service) ListCatBranches(ctx context.Context, req CatBranchesRequest) (CatBranchesResult, error) {
	root, err := s.convRoot(ctx, req.ConvID)
	if err != nil {
		return CatBranchesResult{}, err
	}
	res := CatBranchesResult{Root: root, Locals: []string{}, Remotes: []string{}}
	if !gitInstalled() {
		return res, nil
	}
	res.Git = true
	lay, ok, err := findGitLayout(ctx, root)
	if err != nil {
		if apperr.Is(err, apperr.NotFound) {
			res.Git = false
			return res, nil
		}
		return res, err
	}
	if !ok {
		return res, nil
	}
	res.Repo = true
	return fillBranches(ctx, lay.top, res)
}

func fillBranches(ctx context.Context, top string, res CatBranchesResult) (CatBranchesResult, error) {
	if out, _, err := runGit(ctx, gitStatusTimeout, top, "symbolic-ref", "--short", "-q", "HEAD"); err == nil {
		res.Branch = strings.TrimSpace(string(out))
	}
	res.Detached = res.Branch == ""
	if !res.Detached {
		if out, _, err := runGit(ctx, gitStatusTimeout, top, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
			res.Upstream = strings.TrimSpace(string(out))
		}
	}
	out, stderr, err := runGit(ctx, gitStatusTimeout, top, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/remotes")
	if err != nil {
		return res, asGitErr(err, stderr, string(out))
	}
	locals, remotes := splitRefs(string(out))
	res.Locals, res.Remotes = locals, remotes
	return res, nil
}

// splitRefs 把 for-each-ref 的全名分成本地分支和「只在远程」的分支。
// 远程分支 <remote>/<name> 在本地已有 <name> 时不列（选本地那条即可）。
func splitRefs(out string) (locals, remotes []string) {
	locals, remotes = []string{}, []string{}
	have := map[string]bool{}
	var rs []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "refs/heads/"):
			n := strings.TrimPrefix(line, "refs/heads/")
			locals = append(locals, n)
			have[n] = true
		case strings.HasPrefix(line, "refs/remotes/"):
			n := strings.TrimPrefix(line, "refs/remotes/")
			if n == "" || strings.HasSuffix(n, "/HEAD") || !strings.Contains(n, "/") {
				continue
			}
			rs = append(rs, n)
		}
	}
	for _, r := range rs {
		if !have[remoteBranchLocalName(r)] {
			remotes = append(remotes, r)
		}
	}
	sort.Strings(locals)
	sort.Strings(remotes)
	return locals, remotes
}

func remoteBranchLocalName(r string) string {
	if i := strings.Index(r, "/"); i >= 0 {
		return r[i+1:]
	}
	return r
}

func errBranchNotFound() error {
	return apperr.New(apperr.NotFound, MsgBranchNotFound).WithDetail("reason=branch_not_found")
}

// SwitchCatBranch 切到本地分支，或从远程分支建跟踪分支并切过去。
// 顺序：对话 → 有进行中的一轮 turn_running → 仓库 → 分支存在 → 工作区不干净 git_dirty → git switch。不自动暂存、不强切。
func (s *Service) SwitchCatBranch(ctx context.Context, req SwitchCatBranchRequest) (err error) {
	defer s.recordCheckoutOnSuccess(ctx, req.ConvID, &err) // v0.51：签出成功记进「最近」
	c, err := s.requireConv(ctx, req.ConvID)
	if err != nil {
		return err
	}
	ids := []string{c.ID}
	if c.ProjectID != "" && s.cfg.Store != nil {
		if more, err := s.cfg.Store.CatConversationIDsByProject(ctx, c.ProjectID); err == nil {
			ids = append(ids, more...)
		}
	}
	if s.projectHasRunningTurn(ids) {
		return apperr.New(apperr.TaskConflict, MsgBranchTurnRunning).WithDetail("reason=turn_running")
	}
	name := strings.TrimSpace(req.Branch)
	if !validBranch(name) {
		return errBranchNotFound()
	}
	lay, err := s.gitLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	local := name
	if req.Remote {
		if _, _, err := runGit(ctx, gitStatusTimeout, lay.top, "show-ref", "--verify", "--quiet", "refs/remotes/"+name); err != nil || !strings.Contains(name, "/") {
			return errBranchNotFound()
		}
		local = remoteBranchLocalName(name)
		if !validBranch(local) {
			return errBranchNotFound()
		}
	} else if !localBranchExists(ctx, lay.top, name) {
		return errBranchNotFound()
	}
	if cur, _, err := runGit(ctx, gitStatusTimeout, lay.top, "symbolic-ref", "--short", "-q", "HEAD"); err == nil && strings.TrimSpace(string(cur)) == local && localBranchExists(ctx, lay.top, local) {
		return nil
	}
	dirty, err := worktreeDirty(ctx, lay)
	if err != nil {
		return err
	}
	if dirty {
		return apperr.New(apperr.TaskConflict, MsgBranchDirty).WithDetail("reason=git_dirty")
	}
	args := []string{"switch", local}
	if req.Remote && !localBranchExists(ctx, lay.top, local) {
		args = []string{"switch", "--track", name}
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
