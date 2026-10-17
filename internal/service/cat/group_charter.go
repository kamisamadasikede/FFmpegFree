package cat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

func (s *Service) RegenerateCatCharter(ctx context.Context, memberID string) (GroupMember, error) {
	m, err := s.requireMember(ctx, memberID)
	if err != nil {
		return GroupMember{}, err
	}
	if m.Charter.Status == "generating" {
		return GroupMember{}, apperr.New(apperr.InvalidArgument, "职责规范正在生成").WithDetail("reason=charter_generating")
	}
	m.Charter.Status = "generating"
	_ = s.cfg.Store.UpdateCatGroupMemberCharter(ctx, m.ID, m.Charter)
	s.emitGroupMemberSnapshot(m)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.generateCharter(context.Background(), m.ID, true)
	}()
	return m, nil
}

func (s *Service) UpdateCatCharter(ctx context.Context, req UpdateCharterRequest) (GroupMember, error) {
	m, err := s.requireMember(ctx, req.MemberID)
	if err != nil {
		return GroupMember{}, err
	}
	c := m.Charter
	edited := map[string]bool{}
	for _, e := range c.Edited {
		edited[e] = true
	}
	if req.Owns != nil {
		c.Owns = strings.TrimSpace(*req.Owns)
		edited["owns"] = true
	}
	if req.NotTouch != nil {
		c.NotTouch = strings.TrimSpace(*req.NotTouch)
		edited["notTouch"] = true
	}
	if req.Dirs != nil {
		c.Dirs = append([]string{}, req.Dirs...)
		edited["dirs"] = true
	}
	c.Edited = nil
	for k := range edited {
		c.Edited = append(c.Edited, k)
	}
	c.Status = "ok"
	c.UpdatedAt = time.Now().UnixMilli()
	if err := s.cfg.Store.UpdateCatGroupMemberCharter(ctx, m.ID, c); err != nil {
		return GroupMember{}, apperr.Wrap(apperr.IOError, "更新职责规范失败", err)
	}
	m.Charter = c
	s.emitGroupMemberSnapshot(m)
	return m, nil
}

func (s *Service) generateCharter(ctx context.Context, memberID string, full bool) {
	m, err := s.cfg.Store.GetCatGroupMember(ctx, memberID)
	if err != nil {
		return
	}
	g, err := s.cfg.Store.GetCatGroup(ctx, m.GroupID)
	if err != nil {
		return
	}
	members, _ := s.cfg.Store.ListCatGroupMembers(ctx, m.GroupID)
	projectPath := ""
	if row, err := s.cfg.Store.GetCatProject(ctx, g.ProjectID); err == nil {
		projectPath = row.Path
	}
	prompt := buildCharterPrompt(m, members, projectPath)
	a, err := s.adapter(m.AgentKind)
	if err != nil {
		s.failCharter(ctx, m, full)
		return
	}
	turnCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	s.applyProviderOverlay(ctx, a, m.AgentKind)
	opts := catagent.TurnOptions{
		Ctx: turnCtx, ConversationID: "group-charter-" + m.ID,
		ModelID: m.ModelID, ThinkLevelID: s.effortForTurn(ctx, m.AgentKind, m.ModelID, m.ThinkLevel),
		ProjectPath: projectPath, Ephemeral: true, NoTools: true,
		JSONSchema: catagent.CharterSchema,
		Messages:   []catagent.WireMessage{{Role: "user", Content: prompt}},
		Provider:   s.providerOverlayFor(ctx, m.AgentKind),
		Timeout:    60 * time.Second,
	}
	resp, runErr := a.RunTurn(opts)
	body := strings.TrimSpace(resp.Message.Content)
	if runErr != nil || body == "" {
		s.failCharter(ctx, m, full)
		return
	}
	var out struct {
		Owns     string   `json:"owns"`
		NotTouch string   `json:"notTouch"`
		Handoff  string   `json:"handoff"`
		Dirs     []string `json:"dirs"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(body)), &out); err != nil {
		s.failCharter(ctx, m, full)
		return
	}
	c := m.Charter
	if full {
		c.Owns = strings.TrimSpace(out.Owns)
		c.NotTouch = strings.TrimSpace(out.NotTouch)
		c.Dirs = out.Dirs
		if c.Dirs == nil {
			c.Dirs = []string{}
		}
		c.Edited = nil
	}
	c.Handoff = strings.TrimSpace(out.Handoff)
	if c.Handoff == "" {
		c.Handoff = defaultHandoff(m, members)
	}
	c.Status = "ok"
	c.UpdatedAt = time.Now().UnixMilli()
	_ = s.cfg.Store.UpdateCatGroupMemberCharter(ctx, m.ID, c)
	m.Charter = c
	s.emitGroupMemberSnapshot(m)
}

func (s *Service) failCharter(ctx context.Context, m store.CatGroupMember, full bool) {
	c := m.Charter
	c.Status = "failed"
	c.UpdatedAt = time.Now().UnixMilli()
	_ = s.cfg.Store.UpdateCatGroupMemberCharter(ctx, m.ID, c)
	m.Charter = c
	s.emitGroupMemberSnapshot(m)
}

func (s *Service) refreshHandoffs(ctx context.Context, groupID string) {
	members, err := s.cfg.Store.ListCatGroupMembers(ctx, groupID)
	if err != nil || len(members) == 0 {
		return
	}
	for _, m := range members {
		c := m.Charter
		c.Handoff = defaultHandoff(m, members)
		c.UpdatedAt = time.Now().UnixMilli()
		_ = s.cfg.Store.UpdateCatGroupMemberCharter(ctx, m.ID, c)
		m.Charter = c
		s.emitGroupMemberSnapshot(m)
	}
}

// defaultHandoff：v0.56 / 6.21.19 — 默认多数时候只向老板汇报、不必交接；
// 禁止「做完交给全员 / @ 所有其他人」。加人/减人/改名重算时也用这段。
func defaultHandoff(self store.CatGroupMember, members []store.CatGroupMember) string {
	_ = self
	_ = members
	return "多数时候只向老板汇报，不必交接。只有确实需要别人接着、或要并行专家时，才在「交给谁」里 @（可一次 @ 多人）。不要为了填栏目硬 @，禁止做完交给全员。"
}

func buildCharterPrompt(self store.CatGroupMember, members []store.CatGroupMember, projectPath string) string {
	var b strings.Builder
	b.WriteString("为项目群聊成员生成职责规范 JSON。\n")
	b.WriteString("职位：" + self.Title + "\n名称：" + self.Name + "\n")
	b.WriteString("其他成员：\n")
	for _, m := range members {
		if m.ID == self.ID {
			continue
		}
		b.WriteString("- " + m.Name + "（" + m.Title + "）\n")
	}
	b.WriteString("\n项目概况：\n")
	b.WriteString(projectOverview(projectPath))
	b.WriteString("\n只输出 JSON，字段 owns / notTouch / handoff / dirs。dirs 为相对项目根的目录前缀数组，可空。\n")
	b.WriteString("handoff 字段写交接习惯说明即可：默认「多数时候只向老板汇报，不必交接」；不要列出 @所有其他成员，不要写「做完交给全员」。\n")
	return b.String()
}

func projectOverview(root string) string {
	if root == "" {
		return "（无项目路径）"
	}
	var b strings.Builder
	readme := readHead(filepath.Join(root, "README.md"), 200)
	if readme == "" {
		readme = readHead(filepath.Join(root, "readme.md"), 200)
	}
	if readme != "" {
		b.WriteString("README：\n")
		b.WriteString(readme)
		b.WriteString("\n")
	}
	b.WriteString("顶层目录：\n")
	entries, err := os.ReadDir(root)
	if err != nil {
		return b.String()
	}
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		b.WriteString("- " + e.Name())
		if e.IsDir() {
			b.WriteString("/")
			sub, _ := os.ReadDir(filepath.Join(root, e.Name()))
			var kids []string
			for _, s := range sub {
				if strings.HasPrefix(s.Name(), ".") {
					continue
				}
				name := s.Name()
				if s.IsDir() {
					name += "/"
				}
				kids = append(kids, name)
				if len(kids) >= 8 {
					break
				}
			}
			if len(kids) > 0 {
				b.WriteString(" → " + strings.Join(kids, ", "))
			}
		}
		b.WriteByte('\n')
		n++
		if n >= 40 {
			break
		}
	}
	// light stack hints
	for _, f := range []string{"go.mod", "package.json", "pyproject.toml", "Cargo.toml", "pom.xml"} {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			b.WriteString("检测到：" + f + "\n")
		}
	}
	return b.String()
}

func readHead(path string, maxLines int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := string(b)
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	out := strings.Join(lines, "\n")
	if utf8.RuneCountInString(out) > 8000 {
		r := []rune(out)
		out = string(r[:8000])
	}
	return out
}

func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '{'); i >= 0 {
		if j := strings.LastIndexByte(s, '}'); j > i {
			return s[i : j+1]
		}
	}
	return s
}
