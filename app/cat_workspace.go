package app

import (
	"os"
	"path/filepath"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/cat"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (s *CatService) UpdateCatConversation(req cat.ConversationUpdate) (cat.Conversation, error) {
	c, e := s.svc()
	if e != nil {
		return cat.Conversation{}, e
	}
	return c.UpdateCatConversation(s.rootCtx(), req)
}
func (s *CatService) GetCatWorkspace(projectID string) (cat.CatWorkspace, error) {
	c, e := s.svc()
	if e != nil {
		return cat.CatWorkspace{}, e
	}
	return c.GetCatWorkspace(s.rootCtx(), projectID)
}
func (s *CatService) SaveCatProjectRules(req cat.ProjectRulesRequest) error {
	c, e := s.svc()
	if e != nil {
		return e
	}
	return c.SaveCatProjectRules(s.rootCtx(), req)
}
func (s *CatService) SaveCatPrompt(req cat.CatPrompt) (cat.CatPrompt, error) {
	c, e := s.svc()
	if e != nil {
		return cat.CatPrompt{}, e
	}
	return c.SaveCatPrompt(s.rootCtx(), req)
}
func (s *CatService) DeleteCatPrompt(promptID string) error {
	c, e := s.svc()
	if e != nil {
		return e
	}
	return c.DeleteCatPrompt(s.rootCtx(), promptID)
}
func (s *CatService) SearchCatContextFiles(req cat.SearchContextRequest) (cat.ContextFiles, error) {
	c, e := s.svc()
	if e != nil {
		return cat.ContextFiles{}, e
	}
	return c.SearchCatContextFiles(s.rootCtx(), req)
}
func (s *CatService) ExportCatConversation(convID, format string) (string, error) {
	c, e := s.svc()
	if e != nil {
		return "", e
	}
	export, e := c.PrepareCatConversationExport(s.rootCtx(), convID, format)
	if e != nil {
		return "", e
	}
	path, e := runtime.SaveFileDialog(s.rootCtx(), runtime.SaveDialogOptions{DefaultFilename: export.Name, CanCreateDirectories: true, Filters: []runtime.FileFilter{{DisplayName: format, Pattern: "*." + format}}})
	if e != nil || path == "" {
		return "", e
	}
	if e = writeCatExport(path, export.Content); e != nil {
		return "", apperr.Wrap(apperr.IOError, "导出会话失败。", e)
	}
	return path, nil
}

// Commit the completed snapshot only after the temporary file has been flushed.
func writeCatExport(path, content string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".ffc-export-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.WriteString(content); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
