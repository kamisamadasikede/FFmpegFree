package cat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
)

// SettingsKey 是 settings 表里 Cat 设置的键（契约 6.19.12.1 第 1 条）。
const SettingsKey = "cat.settings"

// MaxCustomInstruction 是全局自定义指令的字数上限。
const MaxCustomInstruction = 4000

// MsgSaveFailed 是设置保存失败的提示（6.19.12.7 第 1 条）。
const MsgSaveFailed = "保存失败，请重试。"

// CatSettings 是 Cat 设置（契约 6.19.12.1）。NotifyOnFinish 为 nil 视为 true；后端返回时总是填上。
type CatSettings struct {
	DefaultModel        string `json:"defaultModel"`
	DefaultEffort       string `json:"defaultEffort"`
	DefaultApprovalMode string `json:"defaultApprovalMode"`
	CustomInstruction   string `json:"customInstruction"`
	NotifyOnFinish      *bool  `json:"notifyOnFinish"`
}

// Notify 返回是否发完成通知（nil = 开）。
func (c CatSettings) Notify() bool { return c.NotifyOnFinish == nil || *c.NotifyOnFinish }

func normalizeSettings(c CatSettings) CatSettings {
	c.DefaultModel = strings.TrimSpace(c.DefaultModel)
	c.DefaultEffort = strings.TrimSpace(c.DefaultEffort)
	c.DefaultApprovalMode = strings.TrimSpace(c.DefaultApprovalMode)
	on := c.Notify()
	c.NotifyOnFinish = &on
	return c
}

func validEffort(e string) bool {
	switch e {
	case "", "low", "medium", "high", "xhigh":
		return true
	}
	return false
}

func validApproval(m string) bool {
	switch m {
	case "", "normal", "plan", "auto", "approve":
		return true
	}
	return false
}

// GetCatSettings 读取设置；没有记录 = 全部空值（通知开）。读坏了也按空值返回，不让设置页打不开。
func (s *Service) GetCatSettings(ctx context.Context) (CatSettings, error) {
	return s.loadSettings(ctx), nil
}

func (s *Service) loadSettings(ctx context.Context) CatSettings {
	var c CatSettings
	if s.cfg.Store != nil {
		if _, err := s.cfg.Store.GetSetting(ctx, SettingsKey, &c); err != nil {
			s.cfg.Logf("cat settings read: %v", err)
			c = CatSettings{}
		}
	}
	return normalizeSettings(c)
}

// SaveCatSettings 整体保存并返回保存后的值。校验失败 INVALID_ARGUMENT；写库失败 IO_ERROR「保存失败，请重试。」，库里仍是旧值。
func (s *Service) SaveCatSettings(ctx context.Context, req CatSettings) (CatSettings, error) {
	c := normalizeSettings(req)
	if !validEffort(c.DefaultEffort) {
		return CatSettings{}, apperr.New(apperr.InvalidArgument, "思考强度不正确")
	}
	if !validApproval(c.DefaultApprovalMode) {
		return CatSettings{}, apperr.New(apperr.InvalidArgument, "批准方式不正确")
	}
	if utf8.RuneCountInString(c.CustomInstruction) > MaxCustomInstruction {
		return CatSettings{}, apperr.New(apperr.InvalidArgument, "最多 4000 字。")
	}
	if s.cfg.Store == nil {
		return CatSettings{}, apperr.New(apperr.IOError, MsgSaveFailed)
	}
	if err := s.cfg.Store.SetSetting(ctx, SettingsKey, c); err != nil {
		return CatSettings{}, apperr.Wrap(apperr.IOError, MsgSaveFailed, err)
	}
	return c, nil
}

// turnRules 是这一轮要带的全局自定义指令（每轮现读，保存后下一轮生效）。
func (s *Service) turnRules(ctx context.Context) string {
	return strings.TrimSpace(s.loadSettings(ctx).CustomInstruction)
}

// InspectRequest 是 InspectCatConfig 入参。
type InspectRequest struct {
	ProjectID string `json:"projectId"`
}

// CatInspectResult / CatInspectItem 见 catagent。
type CatInspectResult = catagent.InspectResult
type CatInspectItem = catagent.InspectItem

// InspectCatConfig 只读列出本机助手发现的技能 / 插件 / 钩子 / MCP / 子代理（6.19.12.1 第 5 条）。
// 目录：项目文件夹；空或项目不在 → <数据目录>/cat。失败返回空列表 + reason，error 恒为 nil。
func (s *Service) InspectCatConfig(ctx context.Context, req InspectRequest) (CatInspectResult, error) {
	a, err := s.adapter(catagent.KindCatBuild)
	if err != nil {
		return catagent.EmptyInspect(catagent.InspectNotReady), nil
	}
	ins, ok := a.(interface {
		Inspect(context.Context, string) catagent.InspectResult
	})
	if !ok {
		return catagent.EmptyInspect(catagent.InspectNotReady), nil
	}
	dir := ""
	if pid := strings.TrimSpace(req.ProjectID); pid != "" && s.cfg.Store != nil {
		if row, err := s.cfg.Store.GetCatProject(ctx, pid); err == nil && !s.checkProject(row).Missing {
			dir = row.Path
		}
	}
	if dir == "" && s.cfg.DataRoot != "" {
		dir = filepath.Join(s.cfg.DataRoot, "cat")
		_ = mkdirAll(dir)
	}
	return ins.Inspect(ctx, dir), nil
}

func mkdirAll(dir string) error { return os.MkdirAll(dir, 0o755) }
