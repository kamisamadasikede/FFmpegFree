package cat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

// v0.39 技能 / MCP 管理（契约 6.19.12.9）。

const (
	inspectCatSkill  = "skill"
	inspectCatMCP    = "mcp"
	inspectCatPlugin = "plugin"
	inspectCatHook   = "hook"
)

// SkillsDirRequest 是 Reveal / EnsureCreate 入参。
type SkillsDirRequest struct {
	ProjectID string `json:"projectId"`
}

// SkillsDirResult 是 RevealCatSkillsDir / EnsureCreateCatSkillsDir 返回（契约 CatSkillsDirResult）。
type SkillsDirResult struct {
	SkillsRoot       string `json:"skillsRoot"`
	McpConfigPath    string `json:"mcpConfigPath"`
	SkillsRootExists bool   `json:"skillsRootExists"`
	Opened           bool   `json:"opened"`
}

// SetInspectItemEnabledRequest 是 SetCatInspectItemEnabled 入参。
type SetInspectItemEnabledRequest struct {
	ProjectID string `json:"projectId"`
	AgentKind string `json:"agentKind,omitempty"` // 空 = cat_build；不影响共享目录
	Category  string `json:"category"`            // skill|mcp|plugin|hook
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
}

func errInspectProjectRequired() error {
	return apperr.New(apperr.InvalidArgument, "请先选择一个项目。").WithDetail("reason=project")
}

func errInspectCategory() error {
	return apperr.New(apperr.InvalidArgument, "类型不正确。").WithDetail("reason=category")
}

func errInspectName() error {
	return apperr.New(apperr.InvalidArgument, "请填写名称。").WithDetail("reason=name")
}

func errInspectUnknownName() error {
	return apperr.New(apperr.InvalidArgument, "找不到这一项。").WithDetail("reason=name")
}

func errSkillsDirPermission(err error) error {
	return apperr.Wrap(apperr.IOError, "没有权限创建这个目录。", err).WithDetail("reason=permission")
}

func normalizeInspectCategory(c string) string {
	switch strings.TrimSpace(strings.ToLower(c)) {
	case inspectCatSkill, "skills":
		return inspectCatSkill
	case inspectCatMCP, "mcpservers", "mcp_servers", "mcp-server":
		return inspectCatMCP
	case inspectCatPlugin, "plugins":
		return inspectCatPlugin
	case inspectCatHook, "hooks":
		return inspectCatHook
	default:
		return ""
	}
}

// resolveProjectForSkills 解析项目：空 id → INVALID_ARGUMENT；库中无 → NOT_FOUND；文件夹不见 → CAT_PROJECT_MISSING。
func (s *Service) resolveProjectForSkills(ctx context.Context, projectID string) (store.CatProjectRow, error) {
	if s.cfg.Store == nil {
		return store.CatProjectRow{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return store.CatProjectRow{}, errInspectProjectRequired()
	}
	row, err := s.cfg.Store.GetCatProject(ctx, pid)
	if errors.Is(err, sql.ErrNoRows) {
		return store.CatProjectRow{}, errProjectNotFound()
	}
	if err != nil {
		return store.CatProjectRow{}, apperr.Wrap(apperr.IOError, "读取项目失败", err)
	}
	if s.checkProject(row).Missing {
		return store.CatProjectRow{}, errProjectMissing()
	}
	return row, nil
}

func skillsDirResult(projectAbs string, exists, opened bool) SkillsDirResult {
	root, mcp := catagent.ProjectCatPaths(projectAbs)
	return SkillsDirResult{
		SkillsRoot: root, McpConfigPath: mcp,
		SkillsRootExists: exists, Opened: opened,
	}
}

// pathUnderProject 确认 candidate 落在 projectAbs 之下（拒绝 .. 逃逸）。
func pathUnderProject(projectAbs, candidate string) bool {
	proj := filepath.Clean(projectAbs)
	cand := filepath.Clean(candidate)
	if cand == proj {
		return true
	}
	sep := string(os.PathSeparator)
	prefix := proj + sep
	return strings.HasPrefix(cand, prefix)
}

// RevealCatSkillsDir 打开项目技能目录；目录不存在 → error=nil、Exists=false、Opened=false。
func (s *Service) RevealCatSkillsDir(ctx context.Context, req SkillsDirRequest) (SkillsDirResult, error) {
	row, err := s.resolveProjectForSkills(ctx, req.ProjectID)
	if err != nil {
		return SkillsDirResult{}, err
	}
	root, _ := catagent.ProjectCatPaths(row.Path)
	exists := catagent.SkillsRootExists(root)
	if !exists {
		return skillsDirResult(row.Path, false, false), nil
	}
	if s.cfg.OpenFolder == nil {
		return skillsDirResult(row.Path, true, false), apperr.New(apperr.ProcessFailed, catagent.MsgProjectRevealFail)
	}
	if err := s.cfg.OpenFolder(root); err != nil {
		switch {
		case apperr.Is(err, apperr.NotFound):
			return skillsDirResult(row.Path, false, false), nil
		case apperr.Is(err, apperr.InvalidArgument), apperr.Is(err, apperr.ProcessFailed):
			return skillsDirResult(row.Path, true, false), err
		default:
			return skillsDirResult(row.Path, true, false), apperr.Wrap(apperr.ProcessFailed, catagent.MsgProjectRevealFail, err)
		}
	}
	return skillsDirResult(row.Path, true, true), nil
}

// EnsureCreateCatSkillsDir 只建 `.ffmpegfree/cat/skills`（不写 mcp.json、不改 gitignore），再打开。
func (s *Service) EnsureCreateCatSkillsDir(ctx context.Context, req SkillsDirRequest) (SkillsDirResult, error) {
	row, err := s.resolveProjectForSkills(ctx, req.ProjectID)
	if err != nil {
		return SkillsDirResult{}, err
	}
	root, _ := catagent.ProjectCatPaths(row.Path)
	if !pathUnderProject(row.Path, root) {
		return SkillsDirResult{}, apperr.New(apperr.InvalidArgument, "路径不正确。").WithDetail("reason=skills_dir")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		if isPermissionErr(err) {
			return skillsDirResult(row.Path, false, false), errSkillsDirPermission(err)
		}
		return skillsDirResult(row.Path, false, false), apperr.Wrap(apperr.IOError, "创建目录失败，请重试。", err)
	}
	exists := catagent.SkillsRootExists(root)
	if s.cfg.OpenFolder == nil {
		return skillsDirResult(row.Path, exists, false), apperr.New(apperr.ProcessFailed, catagent.MsgProjectRevealFail)
	}
	if err := s.cfg.OpenFolder(root); err != nil {
		switch {
		case apperr.Is(err, apperr.InvalidArgument), apperr.Is(err, apperr.ProcessFailed):
			return skillsDirResult(row.Path, exists, false), err
		default:
			return skillsDirResult(row.Path, exists, false), apperr.Wrap(apperr.ProcessFailed, catagent.MsgProjectRevealFail, err)
		}
	}
	return skillsDirResult(row.Path, exists, true), nil
}

func isPermissionErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	var pe *os.PathError
	if errors.As(err, &pe) && pe != nil && errors.Is(pe.Err, os.ErrPermission) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied") || strings.Contains(msg, "access is denied")
}

// enabledState 是项目内启停旁路（附录 D7 GUESS：`.ffmpegfree/cat/enabled.json`，三家共用）。
// key = category → name → enabled。缺 name = 沿用列表默认（通常 true）。
type enabledState map[string]map[string]bool

func loadEnabledState(projectAbs string) enabledState {
	path := catagent.EnabledStatePath(projectAbs)
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return enabledState{}
	}
	var top enabledState
	if json.Unmarshal(raw, &top) != nil || top == nil {
		return enabledState{}
	}
	return top
}

func saveEnabledState(projectAbs string, st enabledState) error {
	dir := filepath.Join(filepath.Clean(projectAbs), catagent.ProjectCatDirName, catagent.ProjectCatSubDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := catagent.EnabledStatePath(projectAbs)
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (st enabledState) get(category, name string) (enabled bool, ok bool) {
	m, ok := st[category]
	if !ok || m == nil {
		return false, false
	}
	v, ok := m[name]
	return v, ok
}

func (st enabledState) set(category, name string, enabled bool) {
	if st[category] == nil {
		st[category] = map[string]bool{}
	}
	st[category][name] = enabled
}

func applyEnabledOverlay(items []catagent.InspectItem, st enabledState, category string) []catagent.InspectItem {
	if items == nil {
		items = []catagent.InspectItem{}
	}
	out := make([]catagent.InspectItem, len(items))
	copy(out, items)
	for i := range out {
		if v, ok := st.get(category, out[i].Name); ok {
			out[i].Enabled = v
		}
	}
	return out
}

// SetCatInspectItemEnabled 启停项目内技能 / MCP / 插件 / 钩子（不做安装卸载；三家共用启用态）。
func (s *Service) SetCatInspectItemEnabled(ctx context.Context, req SetInspectItemEnabledRequest) (InspectItem, error) {
	row, err := s.resolveProjectForSkills(ctx, req.ProjectID)
	if err != nil {
		return InspectItem{}, err
	}
	cat := normalizeInspectCategory(req.Category)
	if cat == "" {
		return InspectItem{}, errInspectCategory()
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return InspectItem{}, errInspectName()
	}
	kind := strings.TrimSpace(req.AgentKind)
	if kind == "" {
		kind = catagent.KindCatBuild
	}
	// 先组当前列表，确认 name 已知（契约：未知 name → INVALID_ARGUMENT）。
	res, err := s.inspectForProject(ctx, row, kind)
	if err != nil {
		return InspectItem{}, err
	}
	item, ok := findInspectItem(res, cat, name)
	if !ok {
		return InspectItem{}, errInspectUnknownName()
	}
	st := loadEnabledState(row.Path)
	st.set(cat, name, req.Enabled)
	if err := saveEnabledState(row.Path, st); err != nil {
		if isPermissionErr(err) {
			return InspectItem{}, errSkillsDirPermission(err)
		}
		return InspectItem{}, apperr.Wrap(apperr.IOError, "保存失败，请重试。", err)
	}
	item.Enabled = req.Enabled
	return item, nil
}

func findInspectItem(res InspectResult, category, name string) (InspectItem, bool) {
	var list []InspectItem
	switch category {
	case inspectCatSkill:
		list = res.Skills
	case inspectCatMCP:
		list = res.MCPServers
	case inspectCatPlugin:
		list = res.Plugins
	case inspectCatHook:
		list = res.Hooks
	}
	for _, it := range list {
		if it.Name == name {
			return it, true
		}
	}
	return InspectItem{}, false
}

// inspectForProject 在已解析项目上跑列表（含路径字段、目录补列、启用态覆盖）。
func (s *Service) inspectForProject(ctx context.Context, row store.CatProjectRow, kind string) (InspectResult, error) {
	skillsRoot, mcpPath := catagent.ProjectCatPaths(row.Path)
	exists := catagent.SkillsRootExists(skillsRoot)
	res := s.runAdapterInspect(ctx, kind, row.Path)
	res = mergeProjectDirItems(res, skillsRoot, mcpPath)
	st := loadEnabledState(row.Path)
	res.Skills = applyEnabledOverlay(res.Skills, st, inspectCatSkill)
	res.MCPServers = applyEnabledOverlay(res.MCPServers, st, inspectCatMCP)
	res.Plugins = applyEnabledOverlay(res.Plugins, st, inspectCatPlugin)
	res.Hooks = applyEnabledOverlay(res.Hooks, st, inspectCatHook)
	res.SkillsRoot = skillsRoot
	res.McpConfigPath = mcpPath
	res.SkillsRootExists = exists
	return res, nil
}

func (s *Service) runAdapterInspect(ctx context.Context, kind, cwd string) InspectResult {
	a, err := s.adapter(kind)
	if err != nil {
		if kind == catagent.KindCatBuild {
			return catagent.EmptyInspect(catagent.InspectNotReady)
		}
		return catagent.EmptyInspect(catagent.InspectUnsupported)
	}
	insp, ok := a.(interface {
		Inspect(context.Context, string) catagent.InspectResult
	})
	if !ok {
		if kind == catagent.KindCatBuild {
			return catagent.EmptyInspect(catagent.InspectNotReady)
		}
		return catagent.EmptyInspect(catagent.InspectUnsupported)
	}
	if a.Status().State != catagent.StateReady {
		return catagent.EmptyInspect(catagent.InspectNotReady)
	}
	return insp.Inspect(ctx, cwd)
}

// mergeProjectDirItems 用项目目录补技能 / MCP 名（附录 D5 GUESS）；不覆盖适配器已有同名项的说明。
func mergeProjectDirItems(res InspectResult, skillsRoot, mcpPath string) InspectResult {
	if res.Skills == nil {
		res.Skills = []InspectItem{}
	}
	if res.Plugins == nil {
		res.Plugins = []InspectItem{}
	}
	if res.Hooks == nil {
		res.Hooks = []InspectItem{}
	}
	if res.MCPServers == nil {
		res.MCPServers = []InspectItem{}
	}
	if res.Agents == nil {
		res.Agents = []InspectItem{}
	}
	have := map[string]struct{}{}
	for _, it := range res.Skills {
		have[it.Name] = struct{}{}
	}
	for _, name := range catagent.ListProjectSkillNames(skillsRoot) {
		if _, ok := have[name]; ok {
			continue
		}
		res.Skills = append(res.Skills, InspectItem{Name: name, Source: "project", Enabled: true})
	}
	haveMCP := map[string]struct{}{}
	for _, it := range res.MCPServers {
		haveMCP[it.Name] = struct{}{}
	}
	for _, name := range catagent.ListProjectMCPNames(mcpPath) {
		if _, ok := haveMCP[name]; ok {
			continue
		}
		res.MCPServers = append(res.MCPServers, InspectItem{Name: name, Source: "project", Enabled: true})
	}
	return res
}

// skillsOverlayForProject 有项目路径时组装注入参数；空路径 → 不注入（D6）。
func skillsOverlayForProject(agentKind, projectAbs string) catagent.SkillsOverlay {
	projectAbs = strings.TrimSpace(projectAbs)
	if projectAbs == "" {
		return catagent.SkillsOverlay{}
	}
	root, mcp := catagent.ProjectCatPaths(projectAbs)
	kind := strings.TrimSpace(agentKind)
	if kind == "" {
		kind = catagent.KindCatBuild
	}
	return catagent.SkillsOverlay{AgentKind: kind, SkillsRoot: root, McpConfigPath: mcp}
}
