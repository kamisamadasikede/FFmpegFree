package cat

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/store"
)

type ConversationUpdate struct {
	ConvID  string `json:"convId"`
	Action  string `json:"action"`
	Title   string `json:"title"`
	Enabled bool   `json:"enabled"`
}

func (s *Service) UpdateCatConversation(ctx context.Context, req ConversationUpdate) (Conversation, error) {
	c, err := s.requireConv(ctx, req.ConvID)
	if err != nil {
		return c, err
	}
	var field string
	var value any
	switch req.Action {
	case "rename":
		v := strings.TrimSpace(req.Title)
		if v == "" || utf8.RuneCountInString(v) > 120 || strings.ContainsAny(v, "\r\n\x00") {
			return c, apperr.New(apperr.InvalidArgument, "标题需要 1–120 个字符，且不能换行。")
		}
		field, value = "custom_title", v
	case "pin":
		field, value = "pinned", req.Enabled
	case "archive":
		field, value = "archived", req.Enabled
	case "reset_title":
		field, value = "custom_title", ""
	default:
		return c, apperr.New(apperr.InvalidArgument, "未知会话操作。")
	}
	if err = s.cfg.Store.SetCatConversationMetadata(ctx, c.ID, field, value); err != nil {
		return c, err
	}
	c, err = s.requireConv(ctx, c.ID)
	if err == nil {
		s.applyCLITitle(ctx, &c)
	}
	return c, err
}

type ConversationExport struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Export always reads the provider's canonical transcript; it never invents locally cached history.
func (s *Service) PrepareCatConversationExport(ctx context.Context, convID, format string) (ConversationExport, error) {
	if format != "md" && format != "json" {
		return ConversationExport{}, apperr.New(apperr.InvalidArgument, "不支持这个导出格式。")
	}
	d, err := s.GetCatConversation(ctx, convID)
	if err != nil {
		return ConversationExport{}, err
	}
	if d.HistoryStatus != "" || len(d.Messages) == 0 {
		return ConversationExport{}, apperr.New(apperr.NotFound, "暂无可导出的 CLI 会话记录，请在回复结束后重试。")
	}
	name := strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '-'
		}
		return r
	}, d.Title)
	if len([]rune(name)) > 70 {
		name = string([]rune(name)[:70])
	}
	name = "ffc-" + strings.Trim(name, " .") + "." + format
	if format == "json" {
		b, e := json.MarshalIndent(d, "", "  ")
		return ConversationExport{Name: name, Content: string(b) + "\n"}, e
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\nAgent: %s\n\nConversation: %s\n\n", strings.ReplaceAll(d.Title, "\n", " "), d.AgentKind, d.ID)
	for _, m := range d.Messages {
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", m.Role, m.Content)
		for _, f := range m.Media {
			fmt.Fprintf(&b, "Attachment: %s\n\n", strings.ReplaceAll(f.Name, "\n", " "))
		}
	}
	return ConversationExport{Name: name, Content: b.String()}, nil
}

type CatPrompt struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
}
type CatWorkspace struct {
	ProjectID string      `json:"projectId"`
	Rules     string      `json:"rules"`
	Prompts   []CatPrompt `json:"prompts"`
}
type ProjectRulesRequest struct {
	ProjectID string `json:"projectId"`
	Rules     string `json:"rules"`
}

func (s *Service) GetCatWorkspace(ctx context.Context, projectID string) (CatWorkspace, error) {
	out := CatWorkspace{ProjectID: projectID, Prompts: []CatPrompt{}}
	if s.cfg.Store == nil {
		return out, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	if projectID != "" {
		if _, err := s.cfg.Store.GetCatProject(ctx, projectID); err != nil {
			return out, errProjectNotFound()
		}
		if _, err := s.cfg.Store.GetSetting(ctx, "cat.projectRules."+projectID, &out.Rules); err != nil {
			return out, err
		}
	}
	_, err := s.cfg.Store.GetSetting(ctx, "cat.prompts", &out.Prompts)
	if out.Prompts == nil {
		out.Prompts = []CatPrompt{}
	}
	return out, err
}

func (s *Service) SaveCatProjectRules(ctx context.Context, req ProjectRulesRequest) error {
	if s.cfg.Store == nil {
		return apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	if req.ProjectID == "" {
		return apperr.New(apperr.InvalidArgument, "请先选择项目。")
	}
	if _, err := s.cfg.Store.GetCatProject(ctx, req.ProjectID); err != nil {
		return errProjectNotFound()
	}
	if utf8.RuneCountInString(req.Rules) > 8000 || strings.ContainsRune(req.Rules, 0) {
		return apperr.New(apperr.InvalidArgument, "项目规则最多 8000 个字符。")
	}
	return s.cfg.Store.SetSetting(ctx, "cat.projectRules."+req.ProjectID, strings.TrimSpace(req.Rules))
}

func (s *Service) SaveCatPrompt(ctx context.Context, p CatPrompt) (CatPrompt, error) {
	p.Name, p.Content = strings.TrimSpace(p.Name), strings.TrimSpace(p.Content)
	if p.Name == "" || p.Content == "" || utf8.RuneCountInString(p.Name) > 60 || utf8.RuneCountInString(p.Content) > 16000 || strings.ContainsAny(p.Name, "\r\n\x00") || strings.ContainsRune(p.Content, 0) {
		return p, apperr.New(apperr.InvalidArgument, "请填写名称（最多 60 字）和指令（最多 16000 字）。")
	}
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()
	w, err := s.GetCatWorkspace(ctx, "")
	if err != nil {
		return p, err
	}
	found := false
	for i := range w.Prompts {
		if w.Prompts[i].ID == p.ID && p.ID != "" {
			w.Prompts[i] = p
			found = true
			break
		}
	}
	if !found {
		if p.ID != "" {
			return p, apperr.New(apperr.NotFound, "这条指令已被删除。")
		}
		if len(w.Prompts) >= 100 {
			return p, apperr.New(apperr.InvalidArgument, "常用指令最多 100 条。")
		}
		p.ID = id.New()
		w.Prompts = append(w.Prompts, p)
	}
	return p, s.cfg.Store.SetSetting(ctx, "cat.prompts", w.Prompts)
}

func (s *Service) DeleteCatPrompt(ctx context.Context, promptID string) error {
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()
	w, err := s.GetCatWorkspace(ctx, "")
	if err != nil {
		return err
	}
	out := make([]CatPrompt, 0, len(w.Prompts))
	for _, p := range w.Prompts {
		if p.ID != promptID {
			out = append(out, p)
		}
	}
	return s.cfg.Store.SetSetting(ctx, "cat.prompts", out)
}

func (s *Service) workspaceRulesForTurn(ctx context.Context, projectID string) string {
	global := s.rulesForTurn(ctx)
	if projectID == "" || s.cfg.Store == nil {
		return global
	}
	var project string
	if _, err := s.cfg.Store.GetSetting(ctx, "cat.projectRules."+projectID, &project); err != nil {
		s.cfg.Logf("cat project rules read: %v", err)
		return global
	}
	if strings.TrimSpace(project) == "" {
		return global
	}
	if global == "" {
		return "Project instructions:\n" + project
	}
	return "Global instructions:\n" + global + "\n\nProject instructions:\n" + project
}

type SearchContextRequest struct {
	ConvID    string `json:"convId"`
	ProjectID string `json:"projectId"`
	Query     string `json:"query"`
}
type ContextFile struct {
	Name    string `json:"name"`
	RelPath string `json:"relPath"`
	Path    string `json:"path"`
}
type ContextFiles struct {
	Files     []ContextFile `json:"files"`
	Truncated bool          `json:"truncated"`
}

func (s *Service) SearchCatContextFiles(ctx context.Context, req SearchContextRequest) (ContextFiles, error) {
	var root string
	var err error
	if req.ConvID != "" {
		var c store.CatConversation
		c, err = s.requireConv(ctx, req.ConvID)
		if err == nil {
			root, err = s.conversationRoot(ctx, c)
		}
	} else if req.ProjectID != "" {
		p, e := s.projectForConversation(ctx, req.ProjectID)
		err = e
		root = p.Path
	}
	if err != nil {
		return ContextFiles{}, err
	}
	if root == "" {
		return ContextFiles{Files: []ContextFile{}}, nil
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return ContextFiles{}, errFileMissing()
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return ContextFiles{}, err
	}
	q := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(req.Query), "\\", "/"))
	if utf8.RuneCountInString(q) > 256 {
		return ContextFiles{}, apperr.New(apperr.InvalidArgument, "搜索文字过长。")
	}
	return searchContextFiles(ctx, root, q)
}

func searchContextFiles(ctx context.Context, root, q string) (ContextFiles, error) {
	out := ContextFiles{Files: []ContextFile{}}
	scanned := 0
	deadline := time.Now().Add(2 * time.Second)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if scanned >= 20000 || time.Now().After(deadline) || len(out.Files) >= 50 {
			out.Truncated = true
			return fs.SkipAll
		}
		scanned++
		if walkErr != nil {
			if path == root {
				return walkErr
			}
			return nil
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || !lexicalInside(root, path) {
			return fs.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			switch strings.ToLower(d.Name()) {
			case ".git", "node_modules", ".gocache", ".cache", "dist", "target", "vendor", ".venv", "__pycache__":
				return fs.SkipDir
			}
			if strings.Count(filepath.ToSlash(rel), "/") >= 12 {
				out.Truncated = true
				return fs.SkipDir
			}
			resolved, e := filepath.EvalSymlinks(path)
			if e != nil || !paths.SamePath(resolved, path) {
				return fs.SkipDir
			}
			return nil
		}
		// Files must be regular and not reparse-point aliases outside the workspace.
		info, e := d.Info()
		if e != nil || !info.Mode().IsRegular() {
			return nil
		}
		resolved, e := filepath.EvalSymlinks(path)
		if e != nil || !paths.SamePath(resolved, path) {
			return nil
		}
		n := strings.ToLower(d.Name())
		if n == ".env" || strings.HasPrefix(n, ".env.") || strings.HasSuffix(n, ".pem") || strings.HasSuffix(n, ".key") {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if q == "" || strings.Contains(strings.ToLower(rel), q) {
			out.Files = append(out.Files, ContextFile{Name: d.Name(), RelPath: rel, Path: path})
		}
		return nil
	})
	return out, err
}
