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
