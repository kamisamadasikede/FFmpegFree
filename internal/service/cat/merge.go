package cat

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/proc"
)

// 契约 v0.50（6.19.25）：主动合并分支 GitCatMerge。文案为产品定稿。
const (
	MsgMergeTurnRunning    = "正在回复，等这一轮结束再合并。"
	MsgMergeNotRepo        = "这个文件夹不是 Git 仓库。"
	MsgMergeInProgress     = "上一次合并还没完成，请先处理完冲突，或放弃这次合并。"
	MsgMergeBranchNotFound = "找不到这个分支，可能已被删除。"
	MsgMergeSameBranch     = "不能把分支合并到它自己。"
	MsgMergeDirty          = "有未提交的改动，请先提交或暂存，再合并。"
	MsgMergeFailed         = "合并失败，请重试。"
	gitMergeTimeout        = 120 * time.Second
	gitMergeDetailMaxBytes = 4 << 10
	MergeStatusMerged      = "merged"
	MergeStatusUpToDate    = "up_to_date"
	MergeStatusConflict    = "conflict"
)

// GitCatMergeRequest 把 Branch 合并到当前分支。
type GitCatMergeRequest struct {
	ConvID string `json:"convId"`
	Branch string `json:"branch"`           // 本地分支名；remote=true 时为 <remote>/<name>
	Remote bool   `json:"remote,omitempty"` // true = 合并远程跟踪分支（不 fetch）
}

// GitCatMergeResult 是合并结果；conflict 不是错误。
type GitCatMergeResult struct {
	Status string   `json:"status"`
	Head   string   `json:"head,omitempty"`
	Files  []string `json:"files,omitempty"`
}

// gitMergeEnv：输出固定英文，便于识别；其余非交互变量 runGit 同款。
var gitMergeEnv = []string{"LC_ALL=C", "LANG=C", "LANGUAGE="}

// gitMergeRun 同 runGit（隐藏窗口、GIT_TERMINAL_PROMPT=0、GIT_MERGE_AUTOEDIT=no、GIT_EDITOR=true），另加 C 语言环境。
func gitMergeRun(ctx context.Context, timeout time.Duration, dir string, args ...string) ([]byte, string, error) {
	exe, err := exec.LookPath("git")
	if err != nil {
		return nil, "", errGitMissing()
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, exe, args...)
	cmd.Dir = dir
	cmd.Env = append(gitMergeCmdEnv(), gitMergeEnv...)
	proc.Configure(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	errText := stderr.String()
	if err != nil && errors.Is(cctx.Err(), context.DeadlineExceeded) && strings.TrimSpace(errText) == "" {
		errText = "git timed out"
	}
	return stdout.Bytes(), errText, err
}

func gitMergeCmdEnv() []string {
	return append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"GIT_MERGE_AUTOEDIT=no",
		"GIT_EDITOR=true",
		"EDITOR=true",
	)
}

func errMergeFailed(raw string) error {
	raw = strings.TrimSpace(raw)
	if len(raw) > gitMergeDetailMaxBytes {
		raw = raw[:gitMergeDetailMaxBytes]
	}
	d := "reason=git_failed"
	if raw != "" {
		d += "\n" + raw
	}
	return apperr.New(apperr.InvalidArgument, MsgMergeFailed).WithDetail(d)
}

func errMergeDirty() error {
	return apperr.New(apperr.TaskConflict, MsgMergeDirty).WithDetail("reason=git_dirty")
}

func errMergeBranchNotFound() error {
	return apperr.New(apperr.NotFound, MsgMergeBranchNotFound).WithDetail("reason=branch_not_found")
}

// GitCatMerge 执行 git merge --no-edit <ref>（允许快进）。校验顺序见 6.19.25。
func (s *Service) GitCatMerge(ctx context.Context, req GitCatMergeRequest) (GitCatMergeResult, error) {
	c, err := s.requireConv(ctx, req.ConvID)
	if err != nil {
		return GitCatMergeResult{}, err
	}
	ids := []string{c.ID}
	if c.ProjectID != "" && s.cfg.Store != nil {
		if more, err := s.cfg.Store.CatConversationIDsByProject(ctx, c.ProjectID); err == nil {
			ids = append(ids, more...)
		}
	}
	if s.projectHasRunningTurn(ids) {
		return GitCatMergeResult{}, apperr.New(apperr.TaskConflict, MsgMergeTurnRunning).WithDetail("reason=turn_running")
	}
	root, err := s.convRoot(ctx, req.ConvID)
	if err != nil {
		return GitCatMergeResult{}, err
	}
	lay, ok, err := findGitLayout(ctx, root)
	if err != nil {
		return GitCatMergeResult{}, err
	}
	if !ok {
		return GitCatMergeResult{}, apperr.New(apperr.InvalidArgument, MsgMergeNotRepo).WithDetail("reason=not_git_repo")
	}
	if detectMergeState(ctx, lay.top) != "none" {
		return GitCatMergeResult{}, apperr.New(apperr.TaskConflict, MsgMergeInProgress).WithDetail("reason=merge_in_progress")
	}
	name := strings.TrimSpace(req.Branch)
	ref := "refs/heads/" + name
	if req.Remote {
		ref = "refs/remotes/" + name
	}
	if !validBranch(name) || (req.Remote && !strings.Contains(name, "/")) {
		return GitCatMergeResult{}, errMergeBranchNotFound()
	}
	if _, _, err := gitMergeRun(ctx, gitStatusTimeout, lay.top, "check-ref-format", "--branch", name); err != nil {
		return GitCatMergeResult{}, errMergeBranchNotFound()
	}
	out, _, err := gitMergeRun(ctx, gitStatusTimeout, lay.top, "rev-parse", "--verify", "-q", ref+"^{commit}")
	if err != nil {
		return GitCatMergeResult{}, errMergeBranchNotFound()
	}
	target := strings.TrimSpace(string(out))
	if !req.Remote {
		if cur, _, err := gitMergeRun(ctx, gitStatusTimeout, lay.top, "symbolic-ref", "--short", "-q", "HEAD"); err == nil && strings.TrimSpace(string(cur)) == name {
			// v0.50.1（6.19.25.1）：同一分支拒绝，第 5 步后、祖先判断前。
			return GitCatMergeResult{}, apperr.New(apperr.InvalidArgument, MsgMergeSameBranch).WithDetail("reason=same_branch")
		}
	}
	head, _, herr := gitMergeRun(ctx, gitStatusTimeout, lay.top, "rev-parse", "--verify", "-q", "HEAD")
	if herr == nil {
		h := strings.TrimSpace(string(head))
		if _, _, err := gitMergeRun(ctx, gitStatusTimeout, lay.top, "merge-base", "--is-ancestor", target, h); err == nil {
			return GitCatMergeResult{Status: MergeStatusUpToDate, Head: h}, nil
		}
	}
	dirty, err := worktreeDirty(ctx, lay)
	if err != nil {
		return GitCatMergeResult{}, err
	}
	if dirty {
		return GitCatMergeResult{}, errMergeDirty()
	}
	mout, merr, err := gitMergeRun(ctx, gitMergeTimeout, lay.top, "merge", "--no-edit", ref)
	if err == nil {
		h, stderr, herr := gitMergeRun(ctx, gitStatusTimeout, lay.top, "rev-parse", "HEAD")
		if herr != nil {
			return GitCatMergeResult{}, errMergeFailed(stderr)
		}
		st := MergeStatusMerged
		if strings.Contains(strings.ToLower(string(mout)), "already up to date") {
			st = MergeStatusUpToDate
		}
		return GitCatMergeResult{Status: st, Head: strings.TrimSpace(string(h))}, nil
	}
	combined := merr + "\n" + string(mout)
	low := strings.ToLower(combined)
	if strings.Contains(low, "would be overwritten") || strings.Contains(low, "untracked working tree files would be") {
		return GitCatMergeResult{}, errMergeDirty()
	}
	if gitPathExists(ctx, lay.top, "MERGE_HEAD") {
		files, ferr := mergeUnmergedFiles(ctx, lay)
		if ferr == nil && len(files) > 0 {
			return GitCatMergeResult{Status: MergeStatusConflict, Files: files}, nil
		}
	}
	return GitCatMergeResult{}, errMergeFailed(firstNonEmpty(strings.TrimSpace(merr), strings.TrimSpace(string(mout))))
}

// mergeUnmergedFiles 列 ls-files -u 的路径（相对对话根，排序去重）。
func mergeUnmergedFiles(ctx context.Context, lay gitLayout) ([]string, error) {
	out, stderr, err := gitMergeRun(ctx, gitStatusTimeout, lay.top, "ls-files", "-u", "-z")
	if err != nil {
		return nil, errMergeFailed(stderr)
	}
	seen := map[string]bool{}
	var files []string
	for _, rec := range bytes.Split(out, []byte{0}) {
		line := string(rec)
		i := strings.IndexByte(line, '\t')
		if i < 0 {
			continue
		}
		gp := filepath.ToSlash(line[i+1:])
		if seen[gp] {
			continue
		}
		seen[gp] = true
		if rel, ok := fromGitPath(lay.prefix, gp); ok && rel != "" {
			files = append(files, rel)
		}
	}
	sort.Strings(files)
	return files, nil
}
