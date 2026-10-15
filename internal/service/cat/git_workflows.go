package cat

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
)

type GitStash struct {
	Sha     string `json:"sha"`
	Ref     string `json:"ref"`
	Message string `json:"message"`
	Time    int64  `json:"time"`
}

type GitStashRequest struct {
	ConvID           string `json:"convId"`
	Sha              string `json:"sha,omitempty"`
	Message          string `json:"message,omitempty"`
	IncludeUntracked bool   `json:"includeUntracked"`
	Op               string `json:"op,omitempty"` // save | apply | pop | drop
}

type GitUndoResult struct {
	Message string `json:"message"`
}

type GitAddRemoteRequest struct {
	ConvID string `json:"convId"`
	Name   string `json:"name"`
	URL    string `json:"url"`
}

type GitCommitActionRequest struct {
	ConvID  string `json:"convId"`
	Sha     string `json:"sha"`
	HeadSha string `json:"headSha"`
	Op      string `json:"op"` // cherry-pick | revert
}

func (s *Service) GitCatAddRemote(ctx context.Context, req GitAddRemoteRequest) error {
	name, url := strings.TrimSpace(req.Name), strings.TrimSpace(req.URL)
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`).MatchString(name) || len(name) > 100 || url == "" || strings.HasPrefix(url, "-") || strings.ContainsAny(url, "\x00\r\n") {
		return apperr.New(apperr.InvalidArgument, "请填写有效的远程名称和地址。")
	}
	lay, unlock, err := s.workflowLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	defer unlock()
	return s.runGitOp(ctx, lay, "remote", "add", "--", name, url)
}

// 创建新的提交，不重写已有历史。拒绝脏工作区及需要选择主线的合并提交。
func (s *Service) GitCatCommitAction(ctx context.Context, req GitCommitActionRequest) error {
	if req.Op != "cherry-pick" && req.Op != "revert" {
		return apperr.New(apperr.InvalidArgument, "不认识这个提交操作。")
	}
	lay, unlock, err := s.workflowLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	defer unlock()
	if _, _, err := runGit(ctx, gitStatusTimeout, lay.top, "symbolic-ref", "-q", "HEAD"); err != nil {
		return errDetached()
	}
	head, err := resolveCommit(ctx, lay.top, req.HeadSha)
	if err != nil {
		return errHistoryChanged()
	}
	out, stderr, err := runGit(ctx, gitStatusTimeout, lay.top, "rev-parse", "HEAD")
	if err != nil {
		return asGitErr(err, stderr, "")
	}
	if strings.TrimSpace(string(out)) != head {
		return errHistoryChanged()
	}
	sha, err := resolveCommit(ctx, lay.top, req.Sha)
	if err != nil {
		return err
	}
	parents, stderr, err := runGit(ctx, gitStatusTimeout, lay.top, "rev-list", "--parents", "-n", "1", sha)
	if err != nil {
		return asGitErr(err, stderr, "")
	}
	if len(strings.Fields(string(parents))) > 2 {
		return apperr.New(apperr.InvalidArgument, "合并提交需要选择主线，请在终端处理。")
	}
	dirty, err := worktreeDirty(ctx, gitLayout{conv: lay.top, top: lay.top})
	if err != nil {
		return err
	}
	if dirty {
		return apperr.New(apperr.TaskConflict, "请先提交或储藏当前改动。")
	}
	return s.runGitOpConflict(ctx, lay, req.Op, "--no-edit", sha)
}

var gitWorkflowLocks sync.Map

func (s *Service) GitCatInit(ctx context.Context, req GitStatusRequest) error {
	if err := s.treeRequireIdle(ctx, req.ConvID); err != nil {
		return err
	}
	root, err := s.convRoot(ctx, req.ConvID)
	if err != nil {
		return err
	}
	if _, ok, err := findGitLayout(ctx, root); err != nil {
		return err
	} else if ok {
		return apperr.New(apperr.InvalidArgument, "这个目录已经在 Git 仓库中。")
	}
	_, stderr, err := runGit(ctx, gitStatusTimeout, root, "init", "-b", "main")
	return asGitErr(err, stderr, "")
}

func (s *Service) GitCatConflictSkip(ctx context.Context, req GitStatusRequest) error {
	if err := s.treeRequireIdle(ctx, req.ConvID); err != nil {
		return err
	}
	lay, err := s.gitLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	op := ""
	switch detectMergeState(ctx, lay.top) {
	case "rebasing":
		op = "rebase"
	case "cherry":
		op = "cherry-pick"
	case "reverting":
		op = "revert"
	default:
		return apperr.New(apperr.InvalidArgument, "当前没有可以跳过的提交操作。")
	}
	return s.runGitOpConflict(ctx, lay, op, "--skip")
}

// 储藏和撤销按仓库串行；使用提交号定位，避免陈旧的 stash@{0} 指向另一条储藏。
func (s *Service) workflowLayout(ctx context.Context, convID string) (gitLayout, func(), error) {
	if err := s.treeRequireIdle(ctx, convID); err != nil {
		return gitLayout{}, nil, err
	}
	lay, err := s.historyLayout(ctx, convID)
	if err != nil {
		return gitLayout{}, nil, err
	}
	v, _ := gitWorkflowLocks.LoadOrStore(branchRepoKey(lay.top), &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	if detectMergeState(ctx, lay.top) != "none" {
		mu.Unlock()
		return gitLayout{}, nil, treeErr(apperr.TaskConflict, MsgHistoryMergeRunning, "merge_in_progress")
	}
	unmerged, stderr, err := runGit(ctx, gitStatusTimeout, lay.top, "ls-files", "--unmerged", "-z")
	if err != nil {
		mu.Unlock()
		return gitLayout{}, nil, asGitErr(err, stderr, "")
	}
	if len(unmerged) > 0 {
		mu.Unlock()
		return gitLayout{}, nil, errGitConflict("还有未解决的冲突，请先解决并暂存。")
	}
	return lay, mu.Unlock, nil
}

func readGitStashes(ctx context.Context, top string) ([]GitStash, error) {
	out, stderr, err := runGit(ctx, gitStatusTimeout, top, "stash", "list", "-z", "--format=%H%x1f%gd%x1f%at%x1f%gs")
	if err != nil {
		return nil, asGitErr(err, stderr, "")
	}
	list := []GitStash{}
	for _, rec := range splitNul(out) {
		f := strings.SplitN(string(rec), "\x1f", 4)
		if len(f) != 4 {
			return nil, errHistoryGit("读取储藏失败，请重试。", "unexpected stash output")
		}
		at, _ := strconv.ParseInt(f[2], 10, 64)
		list = append(list, GitStash{Sha: f[0], Ref: f[1], Time: at, Message: f[3]})
	}
	return list, nil
}

func findGitStash(ctx context.Context, top, sha string) (GitStash, error) {
	list, err := readGitStashes(ctx, top)
	if err != nil {
		return GitStash{}, err
	}
	for _, st := range list {
		if st.Sha == sha {
			return st, nil
		}
	}
	return GitStash{}, apperr.New(apperr.NotFound, "这条储藏已不存在，请刷新列表。")
}

func (s *Service) ListCatStashes(ctx context.Context, req GitStatusRequest) ([]GitStash, error) {
	lay, err := s.historyLayout(ctx, req.ConvID)
	if err != nil {
		return nil, err
	}
	return readGitStashes(ctx, lay.top)
}

func (s *Service) GetCatStashDiff(ctx context.Context, req GitStashRequest) (GitDiffResult, error) {
	lay, err := s.historyLayout(ctx, req.ConvID)
	if err != nil {
		return GitDiffResult{}, err
	}
	stash, err := findGitStash(ctx, lay.top, req.Sha)
	if err != nil {
		return GitDiffResult{}, err
	}
	out, stderr, err := runGit(ctx, gitDiffTimeout, lay.top, "--no-pager", "stash", "show", "--include-untracked", "--patch", "--no-color", "--no-ext-diff", "--no-textconv", stash.Sha)
	if err != nil {
		return GitDiffResult{}, asGitErr(err, stderr, "")
	}
	// 保留各文件的 diff 标头，也能看见二进制文件提示。
	return GitDiffResult{Text: clipText(string(out), 512*1024)}, nil
}

func (s *Service) GitCatStash(ctx context.Context, req GitStashRequest) error {
	lay, unlock, err := s.workflowLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	defer unlock()
	switch req.Op {
	case "save":
		message := strings.TrimSpace(req.Message)
		if strings.ContainsRune(message, 0) || utf8.RuneCountInString(message) > maxCommitRunes {
			return apperr.New(apperr.InvalidArgument, "储藏说明太长。")
		}
		st, err := s.statusIn(ctx, lay)
		if err != nil {
			return err
		}
		eligible := false
		for _, c := range st.Changes {
			eligible = eligible || c.Letter != "U" || req.IncludeUntracked
		}
		if !eligible {
			return apperr.New(apperr.InvalidArgument, "没有可以储藏的改动。")
		}
		// 不设全局 literal：它会传播给 stash 内部的 clean 子进程，令未跟踪文件残留。
		args := []string{"stash", "push"}
		if req.IncludeUntracked {
			args = append(args, "--include-untracked")
		}
		if message != "" {
			args = append(args, "-m", message)
		}
		if lay.prefix != "" {
			args = append(args, "--", ":(literal)"+lay.prefix)
		}
		return s.runGitOp(ctx, lay, args...)
	case "apply", "pop", "drop":
		stash, err := findGitStash(ctx, lay.top, req.Sha)
		if err != nil {
			return err
		}
		if req.Op != "drop" {
			dirty, err := worktreeDirty(ctx, gitLayout{conv: lay.top, top: lay.top})
			if err != nil {
				return err
			}
			if dirty {
				return apperr.New(apperr.TaskConflict, "请先提交或储藏当前改动，再恢复储藏。")
			}
			if err := s.runGitOpConflict(ctx, lay, "stash", "apply", "--index", stash.Sha); err != nil {
				// 恢复失败时保留原储藏，包括 pop。
				return err
			}
		}
		if req.Op != "apply" {
			// apply 期间其它工具可能增加储藏，再按 SHA 找一次当前序号。
			stash, err = findGitStash(ctx, lay.top, req.Sha)
			if err != nil {
				return err
			}
			return s.runGitOp(ctx, lay, "stash", "drop", stash.Ref)
		}
		return nil
	default:
		return apperr.New(apperr.InvalidArgument, "不认识这个储藏操作。")
	}
}

// 撤销最近的本地提交，保留暂存区和工作区；远程记录中已发布的提交应使用反向提交。
func (s *Service) GitCatUndoCommit(ctx context.Context, req CatCommitRequest) (GitUndoResult, error) {
	lay, unlock, err := s.workflowLayout(ctx, req.ConvID)
	if err != nil {
		return GitUndoResult{}, err
	}
	defer unlock()
	if _, _, err := runGit(ctx, gitStatusTimeout, lay.top, "symbolic-ref", "-q", "HEAD"); err != nil {
		return GitUndoResult{}, errDetached()
	}
	out, stderr, err := runGit(ctx, gitStatusTimeout, lay.top, "rev-list", "--parents", "-n", "1", "HEAD")
	if err != nil {
		return GitUndoResult{}, asGitErr(err, stderr, "")
	}
	parts := strings.Fields(string(out))
	if len(parts) != 2 {
		return GitUndoResult{}, apperr.New(apperr.InvalidArgument, "首个提交或合并提交不能在这里撤销。")
	}
	if req.Sha != parts[0] {
		return GitUndoResult{}, errHistoryChanged()
	}
	remoteRefs, stderr, err := runGit(ctx, gitStatusTimeout, lay.top, "for-each-ref", "--contains="+parts[0], "--format=%(refname)", "refs/remotes")
	if err != nil {
		return GitUndoResult{}, asGitErr(err, stderr, "")
	}
	if strings.TrimSpace(string(remoteRefs)) != "" {
		return GitUndoResult{}, apperr.New(apperr.InvalidArgument, "这个提交已在远程分支中，请使用反向提交。")
	}
	body, stderr, err := runGit(ctx, gitStatusTimeout, lay.top, "show", "-s", "--format=%B", parts[0])
	if err != nil {
		return GitUndoResult{}, asGitErr(err, stderr, "")
	}
	if err := s.runGitOp(ctx, lay, "reset", "--soft", parts[1]); err != nil {
		return GitUndoResult{}, err
	}
	return GitUndoResult{Message: strings.TrimSpace(string(body))}, nil
}

// 只还原选中的未暂存改动；已经暂存的快照保留。
func (s *Service) GitCatDiscard(ctx context.Context, req GitPathsRequest) error {
	lay, unlock, err := s.workflowLayout(ctx, req.ConvID)
	if err != nil {
		return err
	}
	defer unlock()
	rels, err := cleanRels(lay.conv, req.Paths)
	if err != nil {
		return err
	}
	if len(rels) == 0 {
		return apperr.New(apperr.InvalidArgument, "请选择文件。")
	}
	st, err := s.statusIn(ctx, lay)
	if err != nil {
		return err
	}
	changes := map[string]GitChange{}
	for _, c := range st.Changes {
		changes[c.Path] = c
	}
	// 先校验整个选择，再开始写入。
	for _, rel := range rels {
		c, ok := changes[rel]
		if !ok || c.Letter != "U" && !c.Unstaged {
			return apperr.New(apperr.InvalidArgument, "文件状态已变化，请刷新后重试。")
		}
	}
	for _, rel := range rels {
		path := toGitPath(lay.prefix, rel)
		if changes[rel].Letter == "U" {
			if err := s.runGitOp(ctx, lay, "--literal-pathspecs", "clean", "-f", "--", path); err != nil {
				return err
			}
		} else if err := s.runGitOp(ctx, lay, "--literal-pathspecs", "restore", "--worktree", "--", path); err != nil {
			return err
		}
	}
	return nil
}
