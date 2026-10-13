package cat

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/proc"
)

// 事件与状态（契约 6.19.10.11）。
const (
	EventClone = "cat:clone"

	CloneStarting  = "starting"
	CloneCloning   = "cloning"
	CloneCreating  = "creating"
	CloneSucceeded = "succeeded"
	CloneFailed    = "failed"
	CloneCanceled  = "canceled"

	cloneTimeout       = 30 * time.Minute
	cloneEmitMinGap    = 250 * time.Millisecond // ≤4/s
	cloneParentKey     = "cat.cloneParentDir"
	maxCloneCauseRunes = 120
)

// CloneCatProjectRequest 是 CloneCatProject 入参。
type CloneCatProjectRequest struct {
	URL       string `json:"url"`
	ParentDir string `json:"parentDir,omitempty"`
	Name      string `json:"name,omitempty"`
}

// CloneCatProjectResult 是同步返回；进行中也先带 cloneId。
type CloneCatProjectResult struct {
	CloneID string   `json:"cloneId"`
	Status  string   `json:"status"`
	Project *Project `json:"project,omitempty"`
}

// CancelCloneCatProjectRequest 是取消入参。
type CancelCloneCatProjectRequest struct {
	CloneID string `json:"cloneId"`
}

// CloneEvent 是 cat:clone 载荷。
type CloneEvent struct {
	CloneID   string           `json:"cloneId"`
	Status    string           `json:"status"`
	URL       string           `json:"url,omitempty"`
	ParentDir string           `json:"parentDir,omitempty"`
	Name      string           `json:"name,omitempty"`
	Progress  *float64         `json:"progress,omitempty"`
	Project   *Project         `json:"project,omitempty"`
	Error     *apperr.AppError `json:"error,omitempty"`
	Canceled  bool             `json:"canceled,omitempty"`
}

type cloneJob struct {
	id     string
	cancel context.CancelFunc
	target string
	cmd    *exec.Cmd
	mu     sync.Mutex
}

// ValidGitURL 校验 HTTPS / SSH / git@ / *.git 形态（契约 6.19.10.11-1）。
func ValidGitURL(raw string) bool {
	u := strings.TrimSpace(raw)
	if u == "" {
		return false
	}
	lower := strings.ToLower(u)
	switch {
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return true
	case strings.HasPrefix(lower, "ssh://"), strings.HasPrefix(lower, "git://"):
		return true
	case strings.HasPrefix(u, "git@"):
		return true
	case strings.HasSuffix(lower, ".git"):
		return true
	}
	return false
}

// RepoNameFromGitURL 取仓库名（URL 最后一段去 .git）。
func RepoNameFromGitURL(raw string) string {
	u := strings.TrimSpace(raw)
	u = strings.TrimRight(u, "/")
	u = strings.TrimSuffix(u, ".git")
	u = strings.TrimSuffix(u, ".GIT")
	if i := strings.LastIndexAny(u, "/:"); i >= 0 {
		u = u[i+1:]
	}
	u = strings.TrimSpace(u)
	if u == "" {
		return "repo"
	}
	return u
}

// ClassifyCloneGitFailure 把 git stderr 归到产品文案；第二个返回值是 reason 标签（仅测试用）。
func ClassifyCloneGitFailure(stderr string) (message, kind string) {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "no space left"),
		strings.Contains(s, "disk quota exceeded"),
		strings.Contains(s, "read-only file system"),
		strings.Contains(s, "unable to write to"),
		strings.Contains(s, "error: unable to create"):
		return "克隆失败：无法写入目标文件夹。", "writable"
	case strings.Contains(s, "authentication failed"),
		strings.Contains(s, "permission denied"),
		strings.Contains(s, "could not read username"),
		strings.Contains(s, "host key verification failed"),
		strings.Contains(s, "invalid username or password"),
		strings.Contains(s, "fatal: could not read from remote repository"):
		return "克隆失败：仓库需要登录。请先在本机配置 SSH 或凭据后再试。", "auth"
	}
	cause := sanitizeCloneCause(stderr)
	if cause == "" {
		cause = "未知错误"
	}
	return "克隆失败：" + cause, "other"
}

func sanitizeCloneCause(stderr string) string {
	s := strings.TrimSpace(stderr)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	// 去掉可能带凭据的 URL 片段：scheme://user:pass@host
	lines := strings.Split(s, "\n")
	var keep []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = redactCredentialURLs(line)
		keep = append(keep, line)
		if len(keep) >= 3 {
			break
		}
	}
	s = strings.Join(keep, " ")
	r := []rune(s)
	if len(r) > maxCloneCauseRunes {
		return string(r[:maxCloneCauseRunes]) + "…"
	}
	return s
}

func redactCredentialURLs(s string) string {
	// https://user:pass@host → https://***@host
	for _, scheme := range []string{"https://", "http://", "ssh://"} {
		for {
			i := strings.Index(strings.ToLower(s), scheme)
			if i < 0 {
				break
			}
			rest := s[i+len(scheme):]
			at := strings.Index(rest, "@")
			slash := strings.IndexAny(rest, "/ \t")
			if at > 0 && (slash < 0 || at < slash) && strings.Contains(rest[:at], ":") {
				s = s[:i+len(scheme)] + "***@" + rest[at+1:]
				continue
			}
			break
		}
	}
	return s
}

// CloneCatProject 从 Git 地址克隆并 CreateCatProject（契约 6.19.10.11）。
func (s *Service) CloneCatProject(ctx context.Context, req CloneCatProjectRequest) (CloneCatProjectResult, error) {
	url := strings.TrimSpace(req.URL)
	if !ValidGitURL(url) {
		return CloneCatProjectResult{}, apperr.New(apperr.InvalidArgument, "请输入有效的 Git 地址。").WithDetail("reason=git_url")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return CloneCatProjectResult{}, errGitMissing()
	}

	parent, err := s.resolveCloneParent(ctx, req.ParentDir)
	if err != nil {
		return CloneCatProjectResult{}, err
	}
	folder := RepoNameFromGitURL(url)
	target := filepath.Join(parent, folder)
	if st, serr := os.Stat(target); serr == nil {
		if !st.IsDir() {
			return CloneCatProjectResult{}, apperr.New(apperr.InvalidArgument, "克隆失败：目标文件夹已存在。").WithDetail("reason=clone_exists")
		}
		if nonemptyDir(target) {
			return CloneCatProjectResult{}, apperr.New(apperr.InvalidArgument, "克隆失败：目标文件夹已存在。").WithDetail("reason=clone_exists")
		}
	} else if !os.IsNotExist(serr) {
		return CloneCatProjectResult{}, apperr.Wrap(apperr.IOError, "克隆失败：无法写入目标文件夹。", serr)
	}
	if !parentWritable(parent) {
		return CloneCatProjectResult{}, apperr.New(apperr.IOError, "克隆失败：无法写入目标文件夹。")
	}

	display := strings.TrimSpace(req.Name)
	if display == "" {
		display = folder
	}
	display = s.uniqueProjectDisplayName(ctx, display)

	cloneID := id.New()
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cloneTimeout)

	s.cloneMu.Lock()
	if s.clone != nil {
		s.cloneMu.Unlock()
		cancel()
		return CloneCatProjectResult{}, apperr.New(apperr.TaskConflict, "已有克隆在进行。").WithDetail("reason=clone_busy")
	}
	job := &cloneJob{id: cloneID, cancel: cancel, target: target}
	s.clone = job
	s.cloneMu.Unlock()

	s.emitClone(CloneEvent{CloneID: cloneID, Status: CloneStarting, URL: url, ParentDir: parent, Name: display}, true)

	go s.runClone(jobCtx, job, url, parent, display)

	return CloneCatProjectResult{CloneID: cloneID, Status: CloneStarting}, nil
}

// CancelCloneCatProject 取消进行中的克隆；未知 / 已结束幂等 nil。
func (s *Service) CancelCloneCatProject(ctx context.Context, req CancelCloneCatProjectRequest) error {
	_ = ctx
	cid := strings.TrimSpace(req.CloneID)
	if cid == "" {
		return nil
	}
	s.cloneMu.Lock()
	job := s.clone
	if job == nil || job.id != cid {
		s.cloneMu.Unlock()
		return nil
	}
	cancel := job.cancel
	cmd := job.cmd
	target := job.target
	s.cloneMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cmd != nil {
		_ = proc.Kill(cmd)
	}
	// 尽力删半拉目录；终态事件由 runClone 发 canceled
	_ = os.RemoveAll(target)
	return nil
}

func (s *Service) runClone(ctx context.Context, job *cloneJob, url, parent, display string) {
	defer func() {
		s.cloneMu.Lock()
		if s.clone == job {
			s.clone = nil
		}
		s.cloneMu.Unlock()
		job.cancel()
	}()

	s.emitClone(CloneEvent{CloneID: job.id, Status: CloneCloning, URL: url, ParentDir: parent, Name: display}, true)

	stderr, err := s.execGitClone(ctx, job, url, job.target)
	if err != nil {
		canceled := ctx.Err() != nil
		_ = os.RemoveAll(job.target)
		if canceled {
			s.emitClone(CloneEvent{CloneID: job.id, Status: CloneCanceled, URL: url, ParentDir: parent, Name: display, Canceled: true}, true)
			return
		}
		msg, _ := ClassifyCloneGitFailure(stderr)
		if writeFailLooksLike(stderr, err) {
			msg = "克隆失败：无法写入目标文件夹。"
		}
		s.emitClone(CloneEvent{
			CloneID: job.id, Status: CloneFailed, URL: url, ParentDir: parent, Name: display,
			Error: apperr.New(apperr.ProcessFailed, msg),
		}, true)
		return
	}

	s.emitClone(CloneEvent{CloneID: job.id, Status: CloneCreating, URL: url, ParentDir: parent, Name: display}, true)

	res, cerr := s.CreateCatProject(context.Background(), CreateProjectRequest{Path: job.target, Name: display})
	if cerr != nil {
		s.emitClone(CloneEvent{
			CloneID: job.id, Status: CloneFailed, URL: url, ParentDir: parent, Name: display,
			Error: apperr.From(cerr),
		}, true)
		return
	}
	_ = s.saveCloneParent(context.Background(), parent)
	proj := res.Project
	s.emitClone(CloneEvent{
		CloneID: job.id, Status: CloneSucceeded, URL: url, ParentDir: parent, Name: display, Project: &proj,
	}, true)
}

func (s *Service) execGitClone(ctx context.Context, job *cloneJob, url, target string) (stderr string, err error) {
	exe, err := exec.LookPath("git")
	if err != nil {
		return "", errGitMissing()
	}
	cmd := exec.CommandContext(ctx, exe, "clone", "--progress", "--", url, target)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_MERGE_AUTOEDIT=no")
	proc.Configure(cmd)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	job.mu.Lock()
	job.cmd = cmd
	job.mu.Unlock()

	s.cloneMu.Lock()
	if s.clone == job {
		job.cmd = cmd
	}
	s.cloneMu.Unlock()

	runErr := proc.Run(cmd) // Start+Wait，Windows 进 Job
	stderr = errBuf.String()
	if runErr != nil {
		return stderr, runErr
	}
	return stderr, nil
}

func writeFailLooksLike(stderr string, err error) bool {
	s := strings.ToLower(stderr + " " + errString(err))
	return strings.Contains(s, "no space left") ||
		strings.Contains(s, "read-only file system") ||
		strings.Contains(s, "disk quota") ||
		(strings.Contains(s, "permission denied") && (strings.Contains(s, "mkdir") || strings.Contains(s, "create") || strings.Contains(s, "write")))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Service) resolveCloneParent(ctx context.Context, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if !filepath.IsAbs(raw) {
			return "", apperr.New(apperr.InvalidArgument, "请选择有效的保存位置。").WithDetail("reason=project_path")
		}
		clean := filepath.Clean(raw)
		st, err := os.Stat(clean)
		if err != nil || !st.IsDir() {
			return "", apperr.New(apperr.InvalidArgument, "请选择有效的保存位置。").WithDetail("reason=project_path")
		}
		return clean, nil
	}
	if s.cfg.Store != nil {
		var last string
		if ok, _ := s.cfg.Store.GetSetting(ctx, cloneParentKey, &last); ok {
			last = strings.TrimSpace(last)
			if last != "" && filepath.IsAbs(last) {
				if st, err := os.Stat(last); err == nil && st.IsDir() {
					return filepath.Clean(last), nil
				}
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", apperr.New(apperr.IOError, "克隆失败：无法写入目标文件夹。")
	}
	return home, nil
}

func (s *Service) saveCloneParent(ctx context.Context, parent string) error {
	if s.cfg.Store == nil || parent == "" {
		return nil
	}
	return s.cfg.Store.SetSetting(ctx, cloneParentKey, parent)
}

func (s *Service) uniqueProjectDisplayName(ctx context.Context, base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "项目"
	}
	// 对齐项目名长度上限（与 validProjectName 一致，最多 60 字）
	if utf8.RuneCountInString(base) > 60 {
		r := []rune(base)
		base = string(r[:60])
	}
	names := map[string]bool{}
	if list, err := s.ListCatProjects(ctx); err == nil {
		for _, p := range list {
			names[p.Name] = true
		}
	}
	if !names[base] {
		return base
	}
	for i := 2; ; i++ {
		cand := base + "-" + strconv.Itoa(i)
		if utf8.RuneCountInString(cand) > 60 {
			// 截短基名再拼
			keep := 60 - utf8.RuneCountInString("-"+strconv.Itoa(i))
			if keep < 1 {
				keep = 1
			}
			r := []rune(base)
			if len(r) > keep {
				r = r[:keep]
			}
			cand = string(r) + "-" + strconv.Itoa(i)
		}
		if !names[cand] {
			return cand
		}
	}
}

func nonemptyDir(path string) bool {
	ents, err := os.ReadDir(path)
	return err == nil && len(ents) > 0
}

func parentWritable(dir string) bool {
	probe := filepath.Join(dir, ".ffmpegfree-clone-write-probe-"+id.New())
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return true
}

func (s *Service) emitClone(ev CloneEvent, force bool) {
	s.cloneEmitMu.Lock()
	defer s.cloneEmitMu.Unlock()
	now := time.Now()
	if !force && !s.cloneEmitAt.IsZero() && now.Sub(s.cloneEmitAt) < cloneEmitMinGap {
		return
	}
	s.cloneEmitAt = now
	s.emit(EventClone, ev)
}
