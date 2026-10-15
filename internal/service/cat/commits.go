package cat

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/proc"
)

// 契约 v0.53（6.19.28）：Git 面板「提交历史」。

const (
	MsgCommitNotFound      = "找不到这个提交，可能已被删除。"
	MsgHistoryNotGitRepo   = "这个文件夹不是 Git 仓库。"
	MsgHistoryMergeRunning = "上一次合并还没完成，请先处理完冲突，或放弃这次合并。"
	MsgHistoryChanged      = "提交历史已更新。"
	MsgHistoryFailed       = "读取提交历史失败，请重试。"
	MsgCheckoutCommitFail  = "签出失败，请重试。"

	commitPageDefault   = 200
	commitPageMax       = 500
	commitSearchScanMax = 50000
	commitDetailMaxFile = 1000
	commitLogTimeout    = 20 * time.Second
	commitSearchTimeout = 30 * time.Second
	commitDetailTimeout = 15 * time.Second
)

type CatCommitRef struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // head | local | remote | tag
}

type CatCommit struct {
	Sha         string         `json:"sha"`
	ShortSha    string         `json:"shortSha"`
	Parents     []string       `json:"parents"`
	Subject     string         `json:"subject"`
	AuthorName  string         `json:"authorName"`
	AuthorEmail string         `json:"authorEmail"`
	AuthorTime  int64          `json:"authorTime"`
	Refs        []CatCommitRef `json:"refs"`
	IsHead      bool           `json:"isHead"`
}

type CatCommitPage struct {
	Commits    []CatCommit `json:"commits"`
	NextCursor string      `json:"nextCursor,omitempty"`
	Total      int         `json:"total,omitempty"`
	NoCommits  bool        `json:"noCommits"`
}

type ListCatCommitsRequest struct {
	ConvID string `json:"convId"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Query  string `json:"query,omitempty"`
	Branch string `json:"branch,omitempty"`
	Scope  string `json:"scope,omitempty"` // all | current | outgoing | incoming
}

type CatCommitFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"oldPath,omitempty"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary"`
}

type CatCommitDetail struct {
	Sha            string          `json:"sha"`
	Parents        []string        `json:"parents"`
	AuthorName     string          `json:"authorName"`
	AuthorEmail    string          `json:"authorEmail"`
	AuthorTime     int64           `json:"authorTime"`
	CommitterName  string          `json:"committerName"`
	CommitterEmail string          `json:"committerEmail"`
	CommitTime     int64           `json:"commitTime"`
	Message        string          `json:"message"`
	Files          []CatCommitFile `json:"files"`
	FilesTruncated bool            `json:"filesTruncated"`
	DiffBase       string          `json:"diffBase"`
}

type CatCommitRequest struct {
	ConvID string `json:"convId"`
	Sha    string `json:"sha"`
}

type CatCommitFileDiffRequest struct {
	ConvID string `json:"convId"`
	Sha    string `json:"sha"`
	Path   string `json:"path"`
}

func errCommitNotFound() error {
	return apperr.New(apperr.NotFound, MsgCommitNotFound).WithDetail("reason=commit_not_found")
}

func errHistoryChanged() error {
	return apperr.New(apperr.InvalidArgument, MsgHistoryChanged).WithDetail("reason=history_changed")
}

func errHistoryNotRepo() error {
	return apperr.New(apperr.InvalidArgument, MsgHistoryNotGitRepo).WithDetail("reason=not_git_repo")
}

func errHistoryGit(msg, detail string) error {
	d := "reason=git_failed"
	if s := clipText(detail, 2000); s != "" {
		d += "\n" + s
	}
	return apperr.New(apperr.InvalidArgument, msg).WithDetail(d)
}

// historyLayout 和 gitLayout 一样，但「不是仓库」带 reason=not_git_repo。
func (s *Service) historyLayout(ctx context.Context, convID string) (gitLayout, error) {
	root, err := s.convRoot(ctx, convID)
	if err != nil {
		return gitLayout{}, err
	}
	lay, ok, err := findGitLayout(ctx, root)
	if err != nil {
		return gitLayout{}, err
	}
	if !ok {
		return gitLayout{}, errHistoryNotRepo()
	}
	return lay, nil
}

// ---- refs 快照 ----

type refSnapshot struct {
	fp      string
	head    string // HEAD sha，没有提交时为空
	headRef string // 当前分支短名；分离 HEAD 时为空
	locals  map[string]bool
	remotes map[string]bool
	empty   bool // 没有任何 ref
}

func readRefSnapshot(ctx context.Context, top string) (refSnapshot, error) {
	out, stderr, err := runGitEnv(ctx, commitLogTimeout, top, gitCLocale,
		"for-each-ref", "--format=%(refname)%00%(objectname)", "refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		if gitNotRepo(stderr, err) {
			return refSnapshot{}, errHistoryNotRepo()
		}
		return refSnapshot{}, errHistoryGit(MsgHistoryFailed, stderr)
	}
	snap := refSnapshot{locals: map[string]bool{}, remotes: map[string]bool{}}
	h := sha256.New()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	n := 0
	for _, ln := range lines {
		if ln == "" {
			continue
		}
		n++
		h.Write([]byte(ln))
		h.Write([]byte{'\n'})
		name, _, _ := strings.Cut(ln, "\x00")
		switch {
		case strings.HasPrefix(name, "refs/heads/"):
			snap.locals[strings.TrimPrefix(name, "refs/heads/")] = true
		case strings.HasPrefix(name, "refs/remotes/"):
			snap.remotes[strings.TrimPrefix(name, "refs/remotes/")] = true
		}
	}
	if b, _, err := runGit(ctx, commitLogTimeout, top, "rev-parse", "--verify", "-q", "HEAD"); err == nil {
		snap.head = strings.TrimSpace(string(b))
	}
	if b, _, err := runGit(ctx, commitLogTimeout, top, "symbolic-ref", "--short", "-q", "HEAD"); err == nil {
		snap.headRef = strings.TrimSpace(string(b))
	}
	h.Write([]byte("HEAD\x00" + snap.head + "\x00" + snap.headRef))
	snap.fp = hex.EncodeToString(h.Sum(nil))[:16]
	snap.empty = n == 0 && snap.head == ""
	return snap, nil
}

// ---- cursor ----

type commitCursor struct {
	V       int    `json:"v"`
	Skip    int    `json:"skip"`    // 无 query：已返回的提交数；有 query：已返回的命中数
	Scanned int    `json:"scanned"` // 有 query：下一次从 git log 的第几条开始扫
	FP      string `json:"fp"`
}

func encodeCursor(c commitCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (commitCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return commitCursor{}, err
	}
	var c commitCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return commitCursor{}, err
	}
	if c.V != 1 || c.Skip < 0 || c.Scanned < 0 || c.FP == "" {
		return commitCursor{}, errors.New("bad cursor")
	}
	return c, nil
}

// ---- ListCatCommits ----

const commitLogFormat = "--format=%H%x1f%h%x1f%P%x1f%an%x1f%ae%x1f%at%x1f%D%x1f%s"

func (s *Service) ListCatCommits(ctx context.Context, req ListCatCommitsRequest) (CatCommitPage, error) {
	lay, err := s.historyLayout(ctx, req.ConvID)
	if err != nil {
		return CatCommitPage{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = commitPageDefault
	}
	if limit > commitPageMax {
		limit = commitPageMax
	}
	snap, err := readRefSnapshot(ctx, lay.top)
	if err != nil {
		return CatCommitPage{}, err
	}
	refs, err := commitScopeArgs(ctx, lay.top, snap, req)
	if err != nil {
		return CatCommitPage{}, err
	}
	// 游标同时绑定筛选条件和上游，避免切换范围后混入另一份历史。
	fingerprint := sha256.Sum256([]byte(snap.fp + "\x00" + req.Scope + "\x00" + req.Query + "\x00" + strings.Join(refs, "\x00")))
	snap.fp = hex.EncodeToString(fingerprint[:])[:16]
	cur := commitCursor{V: 1, FP: snap.fp}
	first := strings.TrimSpace(req.Cursor) == ""
	if !first {
		c, derr := decodeCursor(strings.TrimSpace(req.Cursor))
		if derr != nil {
			return CatCommitPage{}, errHistoryGit(MsgHistoryFailed, "cursor")
		}
		if c.FP != snap.fp {
			return CatCommitPage{}, errHistoryChanged()
		}
		cur = c
	}
	if snap.empty {
		return CatCommitPage{Commits: []CatCommit{}, NoCommits: true}, nil
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		args := []string{"-c", "core.quotePath=false", "log", "--no-color", "--topo-order", "--date-order", "-z", commitLogFormat,
			"--skip=" + strconv.Itoa(cur.Skip), "-n", strconv.Itoa(limit + 1)}
		args = append(args, refs...)
		args = append(args, "--")
		out, stderr, err := runGitEnv(ctx, commitLogTimeout, lay.top, gitCLocale, args...)
		if err != nil {
			return CatCommitPage{}, errHistoryGit(MsgHistoryFailed, firstNonEmpty(stderr, err.Error()))
		}
		commits := []CatCommit{}
		for _, rec := range splitNul(out) {
			c, ok := parseCommitRecord(rec, snap, false)
			if !ok {
				return CatCommitPage{}, errHistoryGit(MsgHistoryFailed, "unexpected log output")
			}
			commits = append(commits, c.CatCommit)
		}
		page := CatCommitPage{Commits: commits}
		if len(commits) > limit {
			page.Commits = commits[:limit]
			page.NextCursor = encodeCursor(commitCursor{V: 1, Skip: cur.Skip + limit, FP: snap.fp})
		}
		if first {
			cargs := append([]string{"rev-list", "--count"}, refs...)
			cargs = append(cargs, "--")
			if b, _, err := runGitEnv(ctx, commitLogTimeout, lay.top, gitCLocale, cargs...); err == nil {
				page.Total, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			}
		}
		return page, nil
	}
	return s.searchCommits(ctx, lay, snap, refs, query, limit, cur)
}

func commitScopeArgs(ctx context.Context, top string, snap refSnapshot, req ListCatCommitsRequest) ([]string, error) {
	if req.Scope != "" && req.Scope != "all" && req.Scope != "current" && req.Scope != "outgoing" && req.Scope != "incoming" {
		return nil, apperr.New(apperr.InvalidArgument, "不认识这个提交范围。")
	}
	if snap.empty {
		return nil, nil
	}
	switch req.Scope {
	case "", "all":
		return commitRefArgs(ctx, top, snap, strings.TrimSpace(req.Branch))
	case "current":
		return commitRefArgs(ctx, top, snap, "HEAD")
	case "outgoing", "incoming":
		if snap.head == "" || snap.headRef == "" {
			return nil, errDetached()
		}
		out, _, err := runGit(ctx, commitLogTimeout, top, "rev-parse", "--verify", "-q", "@{upstream}^{commit}")
		if err != nil {
			if req.Scope == "outgoing" {
				return []string{snap.head, "--not", "--remotes"}, nil
			}
			return nil, errNoUpstream()
		}
		upstream := strings.TrimSpace(string(out))
		if req.Scope == "outgoing" {
			return []string{upstream + ".." + snap.head}, nil
		}
		return []string{snap.head + ".." + upstream}, nil
	default:
		return nil, apperr.New(apperr.InvalidArgument, "不认识这个提交范围。")
	}
}

func commitRefArgs(ctx context.Context, top string, snap refSnapshot, branch string) ([]string, error) {
	switch branch {
	case "":
		args := []string{"--branches", "--remotes", "--tags"}
		if snap.head != "" {
			args = append([]string{"HEAD"}, args...)
		}
		return args, nil
	case "HEAD":
		if snap.head == "" {
			return nil, errBranchNotFound()
		}
		return []string{"HEAD"}, nil
	}
	if strings.HasPrefix(branch, "-") {
		return nil, errBranchNotFound()
	}
	for _, full := range []string{"refs/heads/" + branch, "refs/remotes/" + branch} {
		if _, _, err := runGit(ctx, commitLogTimeout, top, "rev-parse", "--verify", "-q", full); err == nil {
			return []string{full}, nil
		}
	}
	return nil, errBranchNotFound()
}

type parsedCommit struct {
	CatCommit
	body string
}

func splitNul(b []byte) [][]byte {
	parts := bytes.Split(b, []byte{0})
	out := parts[:0]
	for _, p := range parts {
		p = bytes.TrimLeft(p, "\n")
		if len(p) > 0 {
			out = append(out, p)
		}
	}
	return out
}

// parseCommitRecord 解析一条记录。withBody 时末尾多一段 %B。
func parseCommitRecord(rec []byte, snap refSnapshot, withBody bool) (parsedCommit, bool) {
	want := 8
	if withBody {
		want = 9
	}
	f := strings.SplitN(string(rec), "\x1f", want)
	if len(f) != want {
		return parsedCommit{}, false
	}
	at, _ := strconv.ParseInt(f[5], 10, 64)
	c := parsedCommit{CatCommit: CatCommit{
		Sha: f[0], ShortSha: f[1], Parents: []string{},
		AuthorName: f[3], AuthorEmail: f[4], AuthorTime: at,
		Refs: parseDecorations(f[6], snap), Subject: f[7],
	}}
	if strings.ContainsRune(c.Subject, '\x1f') && !withBody {
		return parsedCommit{}, false
	}
	if p := strings.TrimSpace(f[2]); p != "" {
		c.Parents = strings.Fields(p)
	}
	c.IsHead = snap.head != "" && c.Sha == snap.head
	if withBody {
		c.body = f[8]
	}
	return c, true
}

// parseDecorations 解析 %D：「HEAD -> main, origin/main, origin/HEAD, tag: v1, feat」。
func parseDecorations(d string, snap refSnapshot) []CatCommitRef {
	refs := []CatCommitRef{}
	d = strings.TrimSpace(d)
	if d == "" {
		return refs
	}
	for _, part := range strings.Split(d, ", ") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case strings.HasPrefix(part, "HEAD -> "):
			refs = append(refs, CatCommitRef{Name: strings.TrimPrefix(part, "HEAD -> "), Kind: "head"})
		case part == "HEAD":
			refs = append(refs, CatCommitRef{Name: "HEAD", Kind: "head"})
		case strings.HasPrefix(part, "tag: "):
			refs = append(refs, CatCommitRef{Name: strings.TrimPrefix(part, "tag: "), Kind: "tag"})
		case snap.locals[part]:
			refs = append(refs, CatCommitRef{Name: part, Kind: "local"})
		case snap.remotes[part]:
			if strings.HasSuffix(part, "/HEAD") {
				continue
			}
			refs = append(refs, CatCommitRef{Name: part, Kind: "remote"})
		}
	}
	return refs
}

var hexQuery = regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`)

func commitMatches(c parsedCommit, q, lq string, isHex bool) bool {
	if isHex && strings.HasPrefix(c.Sha, strings.ToLower(q)) {
		return true
	}
	if strings.Contains(strings.ToLower(c.body), lq) || strings.Contains(strings.ToLower(c.Subject), lq) {
		return true
	}
	return strings.Contains(strings.ToLower(c.AuthorName), lq) || strings.Contains(strings.ToLower(c.AuthorEmail), lq)
}

// searchCommits 流式读取 git log，在 Go 里按 OR 过滤（git 的 --grep 与 --author 是 AND）。
// cursor.Scanned 是 git log 输出中的位置，下一页用 --skip 从那里继续，命中顺序与一次性扫描一致。
func (s *Service) searchCommits(ctx context.Context, lay gitLayout, snap refSnapshot, refs []string, q string, limit int, cur commitCursor) (CatCommitPage, error) {
	args := []string{"-c", "core.quotePath=false", "log", "--no-color", "--topo-order", "--date-order", "-z",
		commitLogFormat + "%x1f%B", "--skip=" + strconv.Itoa(cur.Scanned)}
	args = append(args, refs...)
	args = append(args, "--")
	cctx, cancel := context.WithTimeout(ctx, commitSearchTimeout)
	defer cancel()
	cmd, err := gitStreamCmd(cctx, lay.top, args...)
	if err != nil {
		return CatCommitPage{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return CatCommitPage{}, errHistoryGit(MsgHistoryFailed, err.Error())
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return CatCommitPage{}, errHistoryGit(MsgHistoryFailed, err.Error())
	}
	lq := strings.ToLower(q)
	isHex := hexQuery.MatchString(q)
	br := bufio.NewReaderSize(stdout, 64*1024)
	commits := []CatCommit{}
	scanned := cur.Scanned
	more := false
	stoppedEarly := false
	for {
		if scanned-cur.Scanned >= commitSearchScanMax {
			stoppedEarly = true
			more = true
			break
		}
		rec, rerr := br.ReadBytes(0)
		rec = bytes.TrimSuffix(rec, []byte{0})
		rec = bytes.TrimLeft(rec, "\n")
		if len(rec) > 0 {
			c, ok := parseCommitRecord(rec, snap, true)
			if !ok {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return CatCommitPage{}, errHistoryGit(MsgHistoryFailed, "unexpected log output")
			}
			if commitMatches(c, q, lq, isHex) {
				if len(commits) == limit {
					more = true // 第 limit+1 个命中：不计入 scanned，下一页从它开始
					stoppedEarly = true
					break
				}
				commits = append(commits, c.CatCommit)
			}
			scanned++
		}
		if rerr != nil {
			if rerr != io.EOF {
				stoppedEarly = true
			}
			break
		}
	}
	if stoppedEarly {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if cctx.Err() != nil {
			return CatCommitPage{}, errHistoryGit(MsgHistoryFailed, "timeout")
		}
	} else if werr := cmd.Wait(); werr != nil {
		if cctx.Err() != nil {
			return CatCommitPage{}, errHistoryGit(MsgHistoryFailed, "timeout")
		}
		return CatCommitPage{}, errHistoryGit(MsgHistoryFailed, firstNonEmpty(stderr.String(), werr.Error()))
	}
	page := CatCommitPage{Commits: commits}
	if more {
		page.NextCursor = encodeCursor(commitCursor{V: 1, Skip: cur.Skip + len(commits), Scanned: scanned, FP: snap.fp})
	}
	return page, nil
}

// gitStreamCmd 构造与 runGitEnv 同样环境（隐藏窗口、不提示、LC_ALL=C）的命令，供流式读取。
func gitStreamCmd(ctx context.Context, dir string, args ...string) (*exec.Cmd, error) {
	exe, err := exec.LookPath("git")
	if err != nil {
		return nil, errGitMissing()
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"GIT_MERGE_AUTOEDIT=no",
		"GIT_EDITOR=true",
		"EDITOR=true",
	)
	cmd.Env = append(cmd.Env, gitCLocale...)
	proc.Configure(cmd)
	return cmd, nil
}

// ---- 详情 / 文件 diff ----

// resolveCommit 把（可短的）sha 解析成完整 sha；不存在、歧义、不是提交 → commit_not_found。
func resolveCommit(ctx context.Context, top, sha string) (string, error) {
	sha = strings.TrimSpace(sha)
	if sha == "" || strings.HasPrefix(sha, "-") || !hexQuery.MatchString(sha) {
		return "", errCommitNotFound()
	}
	out, _, err := runGitEnv(ctx, commitDetailTimeout, top, gitCLocale, "rev-parse", "--verify", "-q", "--end-of-options", sha+"^{commit}")
	if err != nil {
		return "", errCommitNotFound()
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *Service) GetCatCommitDetail(ctx context.Context, req CatCommitRequest) (CatCommitDetail, error) {
	lay, err := s.historyLayout(ctx, req.ConvID)
	if err != nil {
		return CatCommitDetail{}, err
	}
	full, err := resolveCommit(ctx, lay.top, req.Sha)
	if err != nil {
		return CatCommitDetail{}, err
	}
	out, stderr, err := runGitEnv(ctx, commitDetailTimeout, lay.top, gitCLocale,
		"show", "-s", "-z", "--no-color", "--format=%H%x1f%P%x1f%an%x1f%ae%x1f%at%x1f%cn%x1f%ce%x1f%ct%x1f%B", full, "--")
	if err != nil {
		return CatCommitDetail{}, errHistoryGit(MsgHistoryFailed, stderr)
	}
	f := strings.SplitN(strings.TrimRight(string(out), "\x00"), "\x1f", 9)
	if len(f) != 9 {
		return CatCommitDetail{}, errHistoryGit(MsgHistoryFailed, "unexpected show output")
	}
	at, _ := strconv.ParseInt(f[4], 10, 64)
	ct, _ := strconv.ParseInt(f[7], 10, 64)
	d := CatCommitDetail{
		Sha: f[0], Parents: []string{}, AuthorName: f[2], AuthorEmail: f[3], AuthorTime: at,
		CommitterName: f[5], CommitterEmail: f[6], CommitTime: ct,
		Message: strings.TrimRight(f[8], "\r\n"),
	}
	if p := strings.TrimSpace(f[1]); p != "" {
		d.Parents = strings.Fields(p)
	}
	if len(d.Parents) > 0 {
		d.DiffBase = d.Parents[0]
	}
	files, err := commitFiles(ctx, lay.top, d.DiffBase, full)
	if err != nil {
		return CatCommitDetail{}, err
	}
	if len(files) > commitDetailMaxFile {
		files = files[:commitDetailMaxFile]
		d.FilesTruncated = true
	}
	d.Files = files
	return d, nil
}

func diffTreeArgs(base, sha string, mode string) []string {
	args := []string{"-c", "core.quotePath=false", "diff-tree", "-r", "-z", "-M", "-C", "--no-commit-id", mode}
	if base == "" {
		return append(args, "--root", sha)
	}
	return append(args, base, sha)
}

// commitFiles 合并 --name-status 与 --numstat（同参数、同顺序），按 path 排序。
func commitFiles(ctx context.Context, top, base, sha string) ([]CatCommitFile, error) {
	ns, stderr, err := runGitEnv(ctx, commitDetailTimeout, top, gitCLocale, diffTreeArgs(base, sha, "--name-status")...)
	if err != nil {
		return nil, errHistoryGit(MsgHistoryFailed, stderr)
	}
	num, stderr, err := runGitEnv(ctx, commitDetailTimeout, top, gitCLocale, diffTreeArgs(base, sha, "--numstat")...)
	if err != nil {
		return nil, errHistoryGit(MsgHistoryFailed, stderr)
	}
	files := parseCommitNameStatusZ(ns)
	stats := parseNumstatListZ(num)
	for i := range files {
		if i < len(stats) {
			files[i].Additions, files[i].Deletions, files[i].Binary = stats[i].add, stats[i].del, stats[i].bin
		}
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func parseCommitNameStatusZ(b []byte) []CatCommitFile {
	parts := strings.Split(string(b), "\x00")
	files := []CatCommitFile{}
	for i := 0; i < len(parts); i++ {
		st := strings.TrimSpace(parts[i])
		if st == "" {
			continue
		}
		letter := st[:1]
		if (letter == "R" || letter == "C") && i+2 < len(parts) {
			files = append(files, CatCommitFile{Status: letter, OldPath: parts[i+1], Path: parts[i+2]})
			i += 2
			continue
		}
		if i+1 < len(parts) {
			files = append(files, CatCommitFile{Status: letter, Path: parts[i+1]})
			i++
		}
	}
	return files
}

type numstatItem struct {
	add, del int
	bin      bool
}

func parseNumstatListZ(b []byte) []numstatItem {
	parts := strings.Split(string(b), "\x00")
	items := []numstatItem{}
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if strings.TrimSpace(p) == "" {
			continue
		}
		f := strings.SplitN(p, "\t", 3)
		if len(f) != 3 {
			continue
		}
		it := numstatItem{}
		if f[0] == "-" && f[1] == "-" {
			it.bin = true
		} else {
			it.add, _ = strconv.Atoi(f[0])
			it.del, _ = strconv.Atoi(f[1])
		}
		if f[2] == "" { // 重命名 / 复制：后面跟 old\0new\0
			i += 2
		}
		items = append(items, it)
	}
	return items
}

func (s *Service) GetCatCommitFileDiff(ctx context.Context, req CatCommitFileDiffRequest) (GitDiffResult, error) {
	lay, err := s.historyLayout(ctx, req.ConvID)
	if err != nil {
		return GitDiffResult{}, err
	}
	full, err := resolveCommit(ctx, lay.top, req.Sha)
	if err != nil {
		return GitDiffResult{}, err
	}
	pb, _, err := runGit(ctx, commitDetailTimeout, lay.top, "rev-parse", full+"^@")
	if err != nil {
		return GitDiffResult{}, errHistoryGit(MsgHistoryFailed, "parents")
	}
	base := ""
	if ps := strings.Fields(string(pb)); len(ps) > 0 {
		base = ps[0]
	}
	files, err := commitFiles(ctx, lay.top, base, full)
	if err != nil {
		return GitDiffResult{}, err
	}
	path := strings.TrimSpace(req.Path)
	var hit *CatCommitFile
	for i := range files {
		if files[i].Path == path {
			hit = &files[i]
			break
		}
	}
	if hit == nil || path == "" {
		return GitDiffResult{}, errHistoryGit(MsgHistoryFailed, "path not in commit")
	}
	if base == "" {
		eb, _, eerr := runGitEnv(ctx, commitDetailTimeout, lay.top, gitCLocale, "hash-object", "-t", "tree", "--stdin")
		if eerr != nil {
			return GitDiffResult{}, errHistoryGit(MsgHistoryFailed, "empty tree")
		}
		base = strings.TrimSpace(string(eb))
	}
	args := []string{"--literal-pathspecs", "--no-pager", "-c", "core.quotePath=false", "diff", "--no-color", "--no-ext-diff",
		"--no-textconv", "--unified=3", "-M", "-C", base, full, "--"}
	if hit.OldPath != "" {
		args = append(args, hit.OldPath)
	}
	args = append(args, hit.Path)
	out, stderr, err := runGitEnv(ctx, commitDetailTimeout, lay.top, gitCLocale, args...)
	if err != nil {
		return GitDiffResult{}, errHistoryGit(MsgHistoryFailed, stderr)
	}
	text, binary := formatUnifiedDiff(string(out))
	if binary || hit.Binary {
		return GitDiffResult{Path: hit.Path, Binary: true}, nil
	}
	return GitDiffResult{Path: hit.Path, Text: text}, nil
}

// ---- 签出提交 ----

// GitCatCheckoutCommit：not_git_repo → turn_running → merge_in_progress → commit_not_found → git_dirty → git switch --detach。
// 成功返回最新 CatBranchTree（detached=true），不更新 recent。
func (s *Service) GitCatCheckoutCommit(ctx context.Context, req CatCommitRequest) (CatBranchTree, error) {
	if err := s.treeRequireIdle(ctx, req.ConvID); err != nil {
		return CatBranchTree{}, err
	}
	tc, err := s.treeOpen(ctx, req.ConvID)
	if err != nil {
		return CatBranchTree{}, err
	}
	if detectMergeState(ctx, tc.lay.top) != "none" {
		return CatBranchTree{}, apperr.New(apperr.TaskConflict, MsgHistoryMergeRunning).WithDetail("reason=merge_in_progress")
	}
	full, err := resolveCommit(ctx, tc.lay.top, req.Sha)
	if err != nil {
		return CatBranchTree{}, err
	}
	dirty, err := worktreeDirty(ctx, tc.lay)
	if err != nil {
		return CatBranchTree{}, err
	}
	if dirty {
		return CatBranchTree{}, errBranchDirty()
	}
	_, stderr, err := runGitEnv(ctx, gitAddTimeout, tc.lay.top, gitCLocale, "switch", "--detach", full)
	if err != nil {
		if gitWouldOverwrite(stderr) {
			return CatBranchTree{}, errBranchDirty()
		}
		return CatBranchTree{}, errHistoryGit(MsgCheckoutCommitFail, stderr)
	}
	return s.buildTree(ctx, tc)
}

// ---- GitCatStatus 补充：分离 HEAD 与当前提交（产品 10-14：底栏「未在分支上 · abc1234」） ----

func fillHeadInfo(ctx context.Context, top string, r *GitStatusResult) {
	b, _, err := runGit(ctx, gitStatusTimeout, top, "rev-parse", "--verify", "-q", "HEAD")
	if err != nil {
		return
	}
	r.HeadSha = strings.TrimSpace(string(b))
	if sb, _, err := runGit(ctx, gitStatusTimeout, top, "rev-parse", "--short", r.HeadSha); err == nil {
		r.HeadShortSha = strings.TrimSpace(string(sb))
	}
	if _, _, err := runGit(ctx, gitStatusTimeout, top, "symbolic-ref", "-q", "HEAD"); err != nil {
		r.Detached = true
	}
}
