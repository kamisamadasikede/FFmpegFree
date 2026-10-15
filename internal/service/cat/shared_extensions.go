package cat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"gopkg.in/yaml.v3"
)

type SharedItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}
type SharedExtensions struct {
	SkillsRoot    string                       `json:"skillsRoot"`
	MCPConfigPath string                       `json:"mcpConfigPath"`
	Skills        []SharedItem                 `json:"skills"`
	MCPServers    []SharedItem                 `json:"mcpServers"`
	Targets       []catagent.NativeShareResult `json:"targets"`
	Warnings      []string                     `json:"warnings"`
}
type SharedImportRequest struct {
	ProjectID  string `json:"projectId"`
	SourcePath string `json:"sourcePath"`
}
type SharedMCPRequest struct {
	ProjectID string `json:"projectId"`
	JSON      string `json:"json"`
}
type SharedDirectoryRequest struct {
	ProjectID string `json:"projectId"`
	Kind      string `json:"kind"`
}

var shareName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var skillName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func shareError(message string) error { return apperr.New(apperr.InvalidArgument, message) }

func (s *Service) shareBase(ctx context.Context, projectID string) (string, string, error) {
	if strings.TrimSpace(projectID) != "" {
		p, e := s.resolveProjectForSkills(ctx, projectID)
		if e != nil {
			return "", "", e
		}
		root, _ := catagent.ProjectCatPaths(p.Path)
		return filepath.Dir(root), p.Path, nil
	}
	if s.cfg.DataRoot == "" {
		return "", "", shareError("应用数据目录尚未就绪")
	}
	return filepath.Join(s.cfg.DataRoot, "cat", "shared"), "", nil
}
func readSharedFile(path string, limit int) ([]byte, error) {
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, int64(limit+1)))
	if e != nil {
		return nil, e
	}
	if len(b) > limit {
		return nil, shareError("共享配置文件过大")
	}
	return b, nil
}
func readShareEnabled(base string) (enabledState, error) {
	b, e := readSharedFile(filepath.Join(base, "enabled.json"), 1<<20)
	if e != nil {
		return nil, e
	}
	st := enabledState{}
	if len(b) > 0 && (json.Unmarshal(b, &st) != nil || st == nil) {
		return nil, shareError("启用状态无法解析，请修复 enabled.json 后重试")
	}
	return st, nil
}
func readSkillInfo(path string) (SharedItem, error) {
	b, e := readSharedFile(filepath.Join(path, "SKILL.md"), 1<<20)
	if e != nil {
		return SharedItem{}, e
	}
	text := strings.ReplaceAll(strings.TrimPrefix(string(b), "\ufeff"), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return SharedItem{}, shareError("技能需要包含带 name 和 description 的 SKILL.md")
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return SharedItem{}, shareError("SKILL.md 元数据不完整")
	}
	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if yaml.Unmarshal([]byte(text[4:4+end]), &meta) != nil || !skillName.MatchString(meta.Name) || len(meta.Name) > 64 || strings.TrimSpace(meta.Description) == "" {
		return SharedItem{}, shareError("SKILL.md 的名称或说明不符合技能格式")
	}
	return SharedItem{Name: meta.Name, Description: meta.Description, Enabled: true}, nil
}
func parseSharedMCP(raw []byte) (map[string]catagent.SharedMCPServer, error) {
	if len(raw) == 0 {
		return map[string]catagent.SharedMCPServer{}, nil
	}
	if len(raw) > 1<<20 {
		return nil, shareError("MCP 配置不能超过 1 MB")
	}
	var top struct {
		Servers map[string]catagent.SharedMCPServer `json:"mcpServers"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&top) != nil || top.Servers == nil {
		return nil, shareError("MCP 格式不正确，需要 mcpServers 对象；支持 command、args、env、url、type、headers、disabled")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, shareError("MCP JSON 包含多余内容")
	}
	for name, m := range top.Servers {
		if !shareName.MatchString(name) {
			return nil, shareError("MCP 名称只支持字母、数字、横线和下划线，最多 64 字符")
		}
		if (strings.TrimSpace(m.Command) == "") == (strings.TrimSpace(m.URL) == "") {
			return nil, shareError("每个 MCP 需要填写 command 或 url，不能同时填写")
		}
		if m.URL != "" {
			u, e := url.Parse(m.URL)
			if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
				return nil, shareError("MCP 地址需要有效的 HTTP 或 HTTPS URL")
			}
			if m.Type != "" && m.Type != "http" && m.Type != "sse" {
				return nil, shareError("远程 MCP 类型支持 http 或 sse")
			}
			if len(m.Env) > 0 || len(m.Args) > 0 {
				return nil, shareError("远程 MCP 不能填写 args 或 env")
			}
		} else if (m.Type != "" && m.Type != "stdio") || len(m.Headers) > 0 {
			return nil, shareError("本地 MCP 使用 stdio，不能填写 HTTP 请求头")
		}
		for k, v := range m.Headers {
			if strings.ContainsAny(k+v, "\r\n") || strings.TrimSpace(k) == "" {
				return nil, shareError("MCP 请求头格式不正确")
			}
		}
		for k, v := range m.Env {
			if strings.ContainsAny(k, "=\x00") || k == "" || strings.ContainsRune(v, 0) {
				return nil, shareError("MCP 环境变量格式不正确")
			}
		}
	}
	return top.Servers, nil
}

// Called with sharingMu held. Inspection never mutates native configuration.
func (s *Service) sharedAt(base, project string, write bool) (SharedExtensions, error) {
	r := SharedExtensions{SkillsRoot: filepath.Join(base, "skills"), MCPConfigPath: filepath.Join(base, "mcp.json"), Skills: []SharedItem{}, MCPServers: []SharedItem{}, Targets: []catagent.NativeShareResult{}, Warnings: []string{}}
	st, e := readShareEnabled(base)
	if e != nil {
		return r, e
	}
	raw, e := readSharedFile(r.MCPConfigPath, 1<<20)
	if e != nil {
		return r, e
	}
	all, e := parseSharedMCP(raw)
	mcpInvalid := e != nil
	if mcpInvalid {
		r.Warnings = append(r.Warnings, "MCP 配置无法解析，请在编辑器中修复；原生 MCP 保留上次同步结果。")
	}
	skills := map[string]string{}
	servers := map[string]catagent.SharedMCPServer{}
	ents, e := os.ReadDir(r.SkillsRoot)
	if e != nil && !os.IsNotExist(e) {
		return r, e
	}
	invalid := map[string]bool{}
	for _, ent := range ents {
		if strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		p := filepath.Join(r.SkillsRoot, ent.Name())
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			continue
		}
		info, err := readSkillInfo(p)
		if err != nil || info.Name != ent.Name() {
			invalid[ent.Name()] = true
			r.Warnings = append(r.Warnings, "技能目录无效，请检查 SKILL.md 和目录名称："+ent.Name())
			continue
		}
		if value, ok := st.get("skill", info.Name); ok {
			info.Enabled = value
		}
		r.Skills = append(r.Skills, info)
		if info.Enabled {
			skills[info.Name] = p
		}
	}
	for name, m := range all {
		enabled := !m.Disabled
		if value, ok := st.get("mcp", name); ok {
			enabled = value
		}
		description := "stdio"
		if m.URL != "" {
			description = m.Type
			if description == "" {
				description = "http"
			}
		}
		r.MCPServers = append(r.MCPServers, SharedItem{Name: name, Description: description, Enabled: enabled})
		if enabled {
			m.Disabled = false
			servers[catagent.ShareNativeName(name)] = m
		}
	}
	sort.Slice(r.Skills, func(i, j int) bool { return r.Skills[i].Name < r.Skills[j].Name })
	// Stable application-owned entries; runtime endpoint/token are inherited by
	// CLI children, never persisted in a native MCP configuration.
	if s.cfg.FileBridge && s.cfg.DataRoot != "" {
		if builtin, err := s.ensurePptSkill(); err == nil {
			userSkill := invalid["ffc-ppt-live"]
			for _, item := range r.Skills {
				userSkill = userSkill || item.Name == "ffc-ppt-live"
			}
			if !userSkill {
				skills["ffc-ppt-live"] = filepath.Dir(builtin)
			}
			name := catagent.ShareNativeName("ffc-office")
			if _, declared := all["ffc-office"]; !declared {
				command := "node"
				if tool := s.GetCatPptTool(false); tool.State == "ready" && tool.Path != "" {
					command = tool.Path
				}
				servers[name] = catagent.SharedMCPServer{Command: command, Args: []string{filepath.Join(filepath.Dir(builtin), "scripts", "office.cjs"), "--mcp"}}
			}
		}
	}
	sort.Slice(r.MCPServers, func(i, j int) bool { return r.MCPServers[i].Name < r.MCPServers[j].Name })
	manifest, e := s.loadShareManifest(base)
	if e != nil {
		return r, e
	}
	home := s.cfg.CLIHome
	useEnv := home == ""
	if home == "" {
		home, e = os.UserHomeDir()
		if e != nil {
			return r, e
		}
	}
	for _, target := range catagent.NativeShareTargets(home, project, useEnv) {
		// Keep invalid sources out of this sync without blocking other extensions.
		previous := manifest[target.AgentKind]
		effective := previous
		effective.Links = map[string]string{}
		for name, path := range previous.Links {
			if !invalid[name] {
				effective.Links[name] = path
			}
		}
		if mcpInvalid {
			effective.Servers = nil
		}
		result, owned := catagent.SyncNativeShare(target, skills, servers, effective, write)
		for name, path := range previous.Links {
			if invalid[name] {
				owned.Links[name] = path
			}
		}
		if mcpInvalid {
			owned.Servers = previous.Servers
		}
		if len(r.Warnings) > 0 {
			result.State = "error"
		}
		manifest[target.AgentKind] = owned
		r.Targets = append(r.Targets, result)
	}
	if write && (len(r.Skills)+len(r.MCPServers)+len(manifest) > 0) {
		if e := catagent.SaveNativeShareManifest(base, manifest); e != nil {
			return r, apperr.Wrap(apperr.IOError, "保存共享同步记录失败，请重试", e)
		}
	}
	return r, nil
}
func (s *Service) GetCatSharedExtensions(ctx context.Context, req SkillsDirRequest) (SharedExtensions, error) {
	base, p, e := s.shareBase(ctx, req.ProjectID)
	if e != nil {
		return SharedExtensions{}, e
	}
	s.sharingMu.Lock()
	defer s.sharingMu.Unlock()
	return s.sharedAt(base, p, false)
}
func (s *Service) SyncCatSharedExtensions(ctx context.Context, req SkillsDirRequest) (SharedExtensions, error) {
	base, p, e := s.shareBase(ctx, req.ProjectID)
	if e != nil {
		return SharedExtensions{}, e
	}
	s.sharingMu.Lock()
	defer s.sharingMu.Unlock()
	return s.sharedAt(base, p, true)
}
func (s *Service) SetCatSharedItemEnabled(ctx context.Context, req SetInspectItemEnabledRequest) (SharedExtensions, error) {
	base, p, e := s.shareBase(ctx, req.ProjectID)
	if e != nil {
		return SharedExtensions{}, e
	}
	if req.Category != "skill" && req.Category != "mcp" {
		return SharedExtensions{}, errInspectCategory()
	}
	s.sharingMu.Lock()
	defer s.sharingMu.Unlock()
	r, e := s.sharedAt(base, p, false)
	if e != nil {
		return r, e
	}
	list := r.Skills
	if req.Category == "mcp" {
		list = r.MCPServers
	}
	found := false
	for _, it := range list {
		if it.Name == req.Name {
			found = true
			break
		}
	}
	if !found {
		return r, errInspectUnknownName()
	}
	st, e := readShareEnabled(base)
	if e != nil {
		return r, e
	}
	st.set(req.Category, req.Name, req.Enabled)
	raw, e := json.MarshalIndent(st, "", "  ")
	if e != nil {
		return r, e
	}
	if e = catagent.WriteShareFile(filepath.Join(base, "enabled.json"), raw); e != nil {
		return r, e
	}
	return s.sharedAt(base, p, true)
}
func (s *Service) ReadCatSharedMCP(ctx context.Context, req SkillsDirRequest) (string, error) {
	base, _, e := s.shareBase(ctx, req.ProjectID)
	if e != nil {
		return "", e
	}
	s.sharingMu.Lock()
	defer s.sharingMu.Unlock()
	raw, e := readSharedFile(filepath.Join(base, "mcp.json"), 1<<20)
	if e != nil {
		return "", e
	}
	if len(raw) == 0 {
		return "{\n  \"mcpServers\": {}\n}", nil
	}
	// The editor reflects row toggles too. Leave malformed JSON intact for repair.
	if all, err := parseSharedMCP(raw); err == nil {
		st, err := readShareEnabled(base)
		if err != nil {
			return "", err
		}
		for name, server := range all {
			if enabled, ok := st.get("mcp", name); ok {
				server.Disabled = !enabled
				all[name] = server
			}
		}
		formatted, err := json.MarshalIndent(map[string]any{"mcpServers": all}, "", "  ")
		if err != nil {
			return "", err
		}
		return string(formatted), nil
	}
	return string(raw), nil
}
func (s *Service) SaveCatSharedMCP(ctx context.Context, req SharedMCPRequest) (SharedExtensions, error) {
	base, p, e := s.shareBase(ctx, req.ProjectID)
	if e != nil {
		return SharedExtensions{}, e
	}
	all, e := parseSharedMCP([]byte(req.JSON))
	if e != nil {
		return SharedExtensions{}, e
	}
	raw, e := json.MarshalIndent(map[string]any{"mcpServers": all}, "", "  ")
	if e != nil {
		return SharedExtensions{}, e
	}
	s.sharingMu.Lock()
	defer s.sharingMu.Unlock()
	// Confirm ownership/state are readable before replacing the canonical source.
	if _, e = s.loadShareManifest(base); e != nil {
		return SharedExtensions{}, e
	}
	st, e := readShareEnabled(base)
	if e != nil {
		return SharedExtensions{}, e
	}
	previousState, _ := json.MarshalIndent(st, "", "  ")
	// Persist the editor's explicit disabled value over any previous row toggle.
	for name, m := range all {
		st.set("mcp", name, !m.Disabled)
	}
	stateRaw, _ := json.MarshalIndent(st, "", "  ")
	if e = catagent.WriteShareFile(filepath.Join(base, "enabled.json"), stateRaw); e != nil {
		return SharedExtensions{}, e
	}
	if e = catagent.WriteShareFile(filepath.Join(base, "mcp.json"), raw); e != nil {
		if rollback := catagent.WriteShareFile(filepath.Join(base, "enabled.json"), previousState); rollback != nil {
			return SharedExtensions{}, apperr.Wrap(apperr.IOError, "保存失败，启用状态未恢复，请检查 enabled.json", rollback)
		}
		return SharedExtensions{}, e
	}
	return s.sharedAt(base, p, true)
}
func (s *Service) RevealCatSharedDirectory(ctx context.Context, req SharedDirectoryRequest) error {
	base, _, e := s.shareBase(ctx, req.ProjectID)
	if e != nil {
		return e
	}
	if req.Kind == "skills" {
		base = filepath.Join(base, "skills")
	} else if req.Kind != "mcp" {
		return errInspectCategory()
	}
	if e = os.MkdirAll(base, 0700); e != nil {
		return e
	}
	if s.cfg.OpenFolder == nil {
		return apperr.New(apperr.ProcessFailed, "无法打开共享目录")
	}
	return s.cfg.OpenFolder(base)
}
func copySkill(source, dest string) error {
	count := 0
	var total int64
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		count++
		if count > 10000 {
			return shareError("技能文件数量超过 10000")
		}
		st, e := os.Lstat(path)
		if e != nil {
			return e
		}
		if st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return shareError("技能包含链接，请改用实际文件后导入")
		}
		rel, e := filepath.Rel(source, path)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return shareError("技能路径不正确")
		}
		to := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0700)
		}
		if !st.Mode().IsRegular() {
			return shareError("技能包含不支持的文件类型")
		}
		total += st.Size()
		if total > 64<<20 {
			return shareError("技能大小超过 64 MB")
		}
		in, e := os.Open(path)
		if e != nil {
			return e
		}
		defer in.Close()
		mode := fs.FileMode(0600)
		if st.Mode()&0111 != 0 {
			mode = 0700
		}
		out, e := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if e != nil {
			return e
		}
		_, e = io.Copy(out, io.LimitReader(in, st.Size()+1))
		closeErr := out.Close()
		if e != nil {
			return e
		}
		return closeErr
	})
}
func (s *Service) ImportCatSharedSkill(ctx context.Context, req SharedImportRequest) (SharedExtensions, error) {
	base, p, e := s.shareBase(ctx, req.ProjectID)
	if e != nil {
		return SharedExtensions{}, e
	}
	source := filepath.Clean(req.SourcePath)
	if !filepath.IsAbs(source) {
		return SharedExtensions{}, shareError("请选择技能文件夹")
	}
	info, e := readSkillInfo(source)
	if e != nil {
		return SharedExtensions{}, e
	}
	s.sharingMu.Lock()
	defer s.sharingMu.Unlock()
	root := filepath.Join(base, "skills")
	dest := filepath.Join(root, info.Name)
	if _, e = os.Lstat(dest); !os.IsNotExist(e) {
		return SharedExtensions{}, shareError("共享库已有同名技能，请先在目录中重命名或移除")
	}
	if rel, e := filepath.Rel(source, root); e == nil && (rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..")) {
		return SharedExtensions{}, shareError("技能来源不能包含共享库")
	}
	if e = os.MkdirAll(root, 0700); e != nil {
		return SharedExtensions{}, e
	}
	temp, e := os.MkdirTemp(root, ".import-")
	if e != nil {
		return SharedExtensions{}, e
	}
	defer os.RemoveAll(temp) // generated child of the verified canonical skills root
	if e = copySkill(source, temp); e != nil {
		return SharedExtensions{}, e
	}
	copied, e := readSkillInfo(temp)
	if e != nil || copied.Name != info.Name {
		return SharedExtensions{}, shareError("导入期间技能发生变化，请重试")
	}
	if e = os.Rename(temp, dest); e != nil {
		return SharedExtensions{}, e
	}
	return s.sharedAt(base, p, true)
}
func (s *Service) syncSharingBeforeTurn(ctx context.Context, project string) {
	s.sharingMu.Lock()
	defer s.sharingMu.Unlock()
	bases := [][2]string{}
	if s.cfg.DataRoot != "" {
		bases = append(bases, [2]string{filepath.Join(s.cfg.DataRoot, "cat", "shared"), ""})
	}
	if project != "" {
		root, _ := catagent.ProjectCatPaths(project)
		bases = append(bases, [2]string{filepath.Dir(root), project})
	}
	for _, entry := range bases {
		if ctx.Err() != nil {
			return
		}
		// An untouched shared library has no effects on the native CLI home.
		if _, e := os.Stat(entry[0]); errors.Is(e, os.ErrNotExist) {
			continue
		}
		r, e := s.sharedAt(entry[0], entry[1], true)
		failures := len(r.Warnings)
		for _, t := range r.Targets {
			failures += len(t.Warnings)
		}
		if (e != nil || failures > 0) && s.cfg.Logf != nil {
			s.cfg.Logf("agent shared extensions: sync incomplete (%d conflicts, error=%v)", failures, e)
		}
	}
}

func (s *Service) loadShareManifest(base string) (catagent.NativeShareManifest, error) {
	var builtin map[string]string
	if s.cfg.DataRoot != "" {
		builtin = map[string]string{"ffc-ppt-live": filepath.Join(s.cfg.DataRoot, "cat", "builtin-skills", "ffc-ppt-live")}
	}
	return catagent.LoadNativeShareManifestWithSkills(base, builtin)
}
