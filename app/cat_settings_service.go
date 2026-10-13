package app

import "FFmpegFree/internal/service/cat"

// ---------- Cat 设置（契约 v0.35 / v0.35.1，6.19.12） ----------

// GetCatSettings 读 Cat 设置（新对话默认值、自定义指令、完成通知）。
func (s *CatService) GetCatSettings() (cat.Settings, error) {
	c, err := s.svc()
	if err != nil {
		return cat.Settings{}, err
	}
	return c.GetCatSettings(s.rootCtx())
}

// SaveCatSettings 整体保存 Cat 设置，返回保存后的值；失败时库里仍是旧值。
func (s *CatService) SaveCatSettings(req cat.Settings) (cat.Settings, error) {
	c, err := s.svc()
	if err != nil {
		return cat.Settings{}, err
	}
	return c.SaveCatSettings(s.rootCtx(), req)
}

// InspectCatConfig 只读列出技能、插件、钩子、MCP、子代理；失败返回空列表 + reason。
func (s *CatService) InspectCatConfig(req cat.InspectRequest) (cat.InspectResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.InspectResult{}, err
	}
	return c.InspectCatConfig(s.rootCtx(), req)
}

// ClearAllCatConversations 删除所有对话（含定时任务产生的）；不动定时任务、项目和项目文件夹。
func (s *CatService) ClearAllCatConversations() error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.ClearAllCatConversations(s.rootCtx())
}

// ---------- Cat 自定义服务商（契约 v0.38，6.19.12.8） ----------

// ListCatProviderCapabilities 返回三家协议能力（固定顺序，不落库）。
func (s *CatService) ListCatProviderCapabilities() ([]cat.ProviderCapability, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListCatProviderCapabilities(s.rootCtx())
}

// SaveCatProviderKey 保存一家的 API Key；响应无明文，仅 apiKeySet。
func (s *CatService) SaveCatProviderKey(req cat.SaveProviderKeyRequest) (cat.ProviderSettings, error) {
	c, err := s.svc()
	if err != nil {
		return cat.ProviderSettings{}, err
	}
	return c.SaveCatProviderKey(s.rootCtx(), req)
}

// ClearCatProviderKey 清除一家已存 Key；不改 enabledCustom / apiUrl。
func (s *CatService) ClearCatProviderKey(req cat.ClearProviderKeyRequest) (cat.ProviderSettings, error) {
	c, err := s.svc()
	if err != nil {
		return cat.ProviderSettings{}, err
	}
	return c.ClearCatProviderKey(s.rootCtx(), req)
}

// ---------- Cat 技能 / MCP 管理（契约 v0.39，6.19.12.9） ----------

// SetCatInspectItemEnabled 启停项目内技能 / MCP / 插件 / 钩子；不做安装卸载。
func (s *CatService) SetCatInspectItemEnabled(req cat.SetInspectItemEnabledRequest) (cat.InspectItem, error) {
	c, err := s.svc()
	if err != nil {
		return cat.InspectItem{}, err
	}
	return c.SetCatInspectItemEnabled(s.rootCtx(), req)
}

// RevealCatSkillsDir 打开项目技能目录；目录不存在时 Exists=false、Opened=false、error=nil。
func (s *CatService) RevealCatSkillsDir(req cat.SkillsDirRequest) (cat.SkillsDirResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.SkillsDirResult{}, err
	}
	return c.RevealCatSkillsDir(s.rootCtx(), req)
}

// EnsureCreateCatSkillsDir 创建项目技能目录并打开；不写 mcp.json、不改 gitignore。
func (s *CatService) EnsureCreateCatSkillsDir(req cat.SkillsDirRequest) (cat.SkillsDirResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.SkillsDirResult{}, err
	}
	return c.EnsureCreateCatSkillsDir(s.rootCtx(), req)
}
