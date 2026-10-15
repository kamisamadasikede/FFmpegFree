package app

import "FFmpegFree/internal/service/cat"

func (s *CatService) GetCatSharedExtensions(req cat.SkillsDirRequest) (cat.SharedExtensions, error) {
	c, e := s.svc()
	if e != nil {
		return cat.SharedExtensions{}, e
	}
	return c.GetCatSharedExtensions(s.rootCtx(), req)
}
func (s *CatService) SyncCatSharedExtensions(req cat.SkillsDirRequest) (cat.SharedExtensions, error) {
	c, e := s.svc()
	if e != nil {
		return cat.SharedExtensions{}, e
	}
	return c.SyncCatSharedExtensions(s.rootCtx(), req)
}
func (s *CatService) ImportCatSharedSkill(req cat.SharedImportRequest) (cat.SharedExtensions, error) {
	c, e := s.svc()
	if e != nil {
		return cat.SharedExtensions{}, e
	}
	return c.ImportCatSharedSkill(s.rootCtx(), req)
}
func (s *CatService) SetCatSharedItemEnabled(req cat.SetInspectItemEnabledRequest) (cat.SharedExtensions, error) {
	c, e := s.svc()
	if e != nil {
		return cat.SharedExtensions{}, e
	}
	return c.SetCatSharedItemEnabled(s.rootCtx(), req)
}
func (s *CatService) ReadCatSharedMCP(req cat.SkillsDirRequest) (string, error) {
	c, e := s.svc()
	if e != nil {
		return "", e
	}
	return c.ReadCatSharedMCP(s.rootCtx(), req)
}
func (s *CatService) SaveCatSharedMCP(req cat.SharedMCPRequest) (cat.SharedExtensions, error) {
	c, e := s.svc()
	if e != nil {
		return cat.SharedExtensions{}, e
	}
	return c.SaveCatSharedMCP(s.rootCtx(), req)
}
func (s *CatService) RevealCatSharedDirectory(req cat.SharedDirectoryRequest) error {
	c, e := s.svc()
	if e != nil {
		return e
	}
	return c.RevealCatSharedDirectory(s.rootCtx(), req)
}
