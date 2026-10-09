package cat

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

// 项目（契约 v0.31，6.19.10）。

// DefaultProjectStatTimeout 是每个项目路径 stat 的最长等待（6.19.10.4 第 5 条），超时按 missing 算。
const DefaultProjectStatTimeout = 2 * time.Second

// MaxProjectNameRunes 是项目名的最大字符数（按 Unicode 字符计）。
const MaxProjectNameRunes = 60

func errProjectPath() error {
	return apperr.New(apperr.InvalidArgument, catagent.MsgProjectPath).WithDetail("reason=project_path")
}
func errProjectRoot() error {
	return apperr.New(apperr.InvalidArgument, catagent.MsgProjectRoot).WithDetail("reason=project_root")
}
func errProjectName() error {
	return apperr.New(apperr.InvalidArgument, catagent.MsgProjectName).WithDetail("reason=project_name")
}
func errProjectNotFound() error { return apperr.New(apperr.NotFound, catagent.MsgProjectNotFound) }
func errProjectMissing() error {
	return apperr.New(apperr.CatProjectMissing, catagent.MsgProjectMissing)
}

// projectPathKey 是去重比较键（6.19.10.2 第 2 条）：Clean 后去掉末尾分隔符；Windows 转小写，macOS / Linux 原样。
// 不解析符号链接、不展开 8.3 短文件名。
func projectPathKey(p string) string { return projectPathKeyFor(runtime.GOOS, p) }

func projectPathKeyFor(goos, p string) string {
	k := filepath.Clean(p)
	for len(k) > 1 && (strings.HasSuffix(k, "/") || strings.HasSuffix(k, `\`)) && filepath.Dir(k) != k {
		k = k[:len(k)-1]
	}
	if goos == "windows" {
		k = strings.ToLower(k)
	}
	return k
}

// validProjectName 去首尾空白后 1~60 个字符、不含换行 / 控制字符。
func validProjectName(name string) (string, bool) {
	n := strings.TrimSpace(name)
	if n == "" || utf8.RuneCountInString(n) > MaxProjectNameRunes {
		return "", false
	}
	for _, r := range n {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return n, true
}

// defaultProjectName 取文件夹名，超过 60 个字符截到前 60 个；控制字符换成空格。
func defaultProjectName(clean string) string {
	base := filepath.Base(clean)
	base = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, base)
	base = strings.TrimSpace(base)
	if r := []rune(base); len(r) > MaxProjectNameRunes {
		base = strings.TrimSpace(string(r[:MaxProjectNameRunes]))
	}
	if base == "" {
		base = clean
	}
	return base
}

// dirMissing 用 stat 判断文件夹是否“不见了”（6.19.10.4）：stat 失败或不是文件夹即 missing；
// 最多等 statTimeout，超时按 missing 算，后台 stat 结束后结果丢弃。
func (s *Service) dirMissing(path string) bool {
	ch := make(chan bool, 1)
	go func() {
		fi, err := s.stat(path)
		ch <- err != nil || fi == nil || !fi.IsDir()
	}()
	t := time.NewTimer(s.statTimeout)
	defer t.Stop()
	select {
	case m := <-ch:
		return m
	case <-t.C:
		return true
	}
}

func (s *Service) stat(path string) (os.FileInfo, error) {
	if s.cfg.Stat != nil {
		return s.cfg.Stat(path)
	}
	return os.Stat(path)
}

// observeMissing 记录项目这次的 missing；与上一次不同时发 cat:project（6.19.10.3）。
// 进程启动后第一次计算只记录、不发事件。
func (s *Service) observeMissing(projectID string, missing bool) {
	s.projMu.Lock()
	prev, known := s.projMissing[projectID]
	s.projMissing[projectID] = missing
	s.projMu.Unlock()
	if known && prev != missing {
		s.emit(catagent.EventProject, catagent.ProjectEvent{ID: projectID, Missing: missing})
	}
}

func (s *Service) forgetMissing(projectID string) {
	s.projMu.Lock()
	delete(s.projMissing, projectID)
	s.projMu.Unlock()
}

// checkProject 实时计算该项目的 missing 并记录 / 发事件。
func (s *Service) checkProject(row store.CatProjectRow) Project {
	m := s.dirMissing(row.Path)
	s.observeMissing(row.ID, m)
	return toProject(row, m)
}

func toProject(r store.CatProjectRow, missing bool) Project {
	return Project{ID: r.ID, Name: r.Name, Path: r.Path, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Missing: missing}
}

// ListCatProjects 返回全部项目（6.19.10.2 第 1 条），各项目 missing 并行实时计算，整体不超过约 2 秒。
func (s *Service) ListCatProjects(ctx context.Context) ([]Project, error) {
	if s.cfg.Store == nil {
		return nil, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	rows, err := s.cfg.Store.ListCatProjects(ctx)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "读取项目失败", err)
	}
	out := make([]Project, len(rows))
	var wg sync.WaitGroup
	for i := range rows {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out[i] = s.checkProject(rows[i])
		}(i)
	}
	wg.Wait()
	return out, nil
}

// CreateCatProject 建项目（6.19.10.2 第 2 条）；同一 path_key 已有项目 → 返回已有项目 existed=true（忽略本次 name）。
func (s *Service) CreateCatProject(ctx context.Context, req CreateProjectRequest) (CreateProjectResult, error) {
	if s.cfg.Store == nil {
		return CreateProjectResult{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	raw := strings.TrimSpace(req.Path)
	if raw == "" || !filepath.IsAbs(raw) {
		return CreateProjectResult{}, errProjectPath()
	}
	clean := filepath.Clean(raw)
	if filepath.Dir(clean) == clean {
		return CreateProjectResult{}, errProjectRoot()
	}
	if s.dirMissing(clean) {
		return CreateProjectResult{}, errProjectPath()
	}
	key := projectPathKey(clean)
	if row, err := s.cfg.Store.GetCatProjectByKey(ctx, key); err == nil {
		return CreateProjectResult{Project: s.checkProject(row), Existed: true}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return CreateProjectResult{}, apperr.Wrap(apperr.IOError, "读取项目失败", err)
	}
	name := defaultProjectName(clean)
	if strings.TrimSpace(req.Name) != "" {
		n, ok := validProjectName(req.Name)
		if !ok {
			return CreateProjectResult{}, errProjectName()
		}
		name = n
	}
	row, existed, err := s.cfg.Store.InsertOrGetCatProject(ctx, store.CatProjectRow{Name: name, Path: clean, PathKey: key})
	if err != nil {
		return CreateProjectResult{}, apperr.Wrap(apperr.IOError, "创建项目失败", err)
	}
	if existed { // 并发同时创建同一路径：另一边先插入了
		return CreateProjectResult{Project: s.checkProject(row), Existed: true}, nil
	}
	s.observeMissing(row.ID, false)
	return CreateProjectResult{Project: toProject(row, false)}, nil
}

// RenameCatProject 只改名字和 updatedAt（6.19.10.2 第 3 条）；missing 的项目也能改名。
func (s *Service) RenameCatProject(ctx context.Context, req RenameProjectRequest) (Project, error) {
	if s.cfg.Store == nil {
		return Project{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	pid := strings.TrimSpace(req.ID)
	if pid == "" {
		return Project{}, apperr.New(apperr.InvalidArgument, "项目编号不能为空")
	}
	name, ok := validProjectName(req.Name)
	if !ok {
		return Project{}, errProjectName()
	}
	row, err := s.cfg.Store.RenameCatProject(ctx, pid, name)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, errProjectNotFound()
	}
	if err != nil {
		return Project{}, apperr.Wrap(apperr.IOError, "项目改名失败", err)
	}
	return s.checkProject(row), nil
}

// DeleteCatProject 删项目及其下对话（6.19.10.2 第 4 条）：未知 id 幂等返回 nil；先取消其下进行中的一轮并等它收尾，
// 再在一个显式事务里删消息 → 对话 → 项目。绝不访问用户文件夹（不 stat、不删、不改、不建）。不发事件。
func (s *Service) DeleteCatProject(ctx context.Context, req DeleteProjectRequest) error {
	if s.cfg.Store == nil {
		return apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	pid := strings.TrimSpace(req.ID)
	if pid == "" {
		return apperr.New(apperr.InvalidArgument, "项目编号不能为空")
	}
	convIDs, err := s.cfg.Store.CatConversationIDsByProject(ctx, pid)
	if err != nil {
		return apperr.Wrap(apperr.IOError, catagent.MsgProjectDelete, err)
	}
	for _, cid := range convIDs {
		s.cancelTurnAndWait(cid)
	}
	if _, err := s.cfg.Store.DeleteCatProject(ctx, pid); err != nil {
		return apperr.Wrap(apperr.IOError, catagent.MsgProjectDelete, err)
	}
	// 取消与删除之间若有新一轮刚启动，删除后再停一次（其对话已不存在，落库会失败并被丢弃）。
	for _, cid := range convIDs {
		s.cancelTurn(cid, "")
	}
	s.forgetMissing(pid)
	return nil
}

// projectForConversation 取会话所属项目并实时检查 missing；项目行已不在按 missing 处理。
func (s *Service) projectForConversation(ctx context.Context, projectID string) (Project, error) {
	row, err := s.cfg.Store.GetCatProject(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, errProjectMissing()
	}
	if err != nil {
		return Project{}, apperr.Wrap(apperr.IOError, "读取项目失败", err)
	}
	p := s.checkProject(row)
	if p.Missing {
		return p, errProjectMissing()
	}
	return p, nil
}
