package app

import (
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/service/doc"
)

func (s *CatService) StatCatFile(req cat.ReadFileRequest) (cat.FileRevision, error) {
	c, e := s.svc()
	if e != nil {
		return cat.FileRevision{}, e
	}
	return c.StatCatFile(s.rootCtx(), req)
}
func (s *CatService) PreviewCatDocument(req cat.ReadFileRequest) (doc.DocPreview, error) {
	c, e := s.svc()
	if e != nil {
		return doc.DocPreview{}, e
	}
	return c.PreviewCatDocument(s.rootCtx(), req)
}
func (s *CatService) ReleaseCatDocument(previewID string) error {
	c, e := s.svc()
	if e != nil {
		return e
	}
	return c.ReleaseCatDocument(previewID)
}
func (s *CatService) OpenCatWorkspaceFile(req cat.ReadFileRequest) error {
	c, e := s.svc()
	if e != nil {
		return e
	}
	return c.OpenCatWorkspaceFile(s.rootCtx(), req)
}
func (s *CatService) GetCatTurnChanges(convID string) ([]cat.TurnChanges, error) {
	c, e := s.svc()
	if e != nil {
		return nil, e
	}
	return c.GetCatTurnChanges(s.rootCtx(), convID)
}

func (s *CatService) GetCatTurnChangeDetail(convID, turnID string) (cat.TurnChanges, error) {
	c, e := s.svc()
	if e != nil {
		return cat.TurnChanges{}, e
	}
	return c.GetCatTurnChangeDetail(s.rootCtx(), convID, turnID)
}
func (s *CatService) GetCatMessageQueue(convID string) (cat.MessageQueue, error) {
	c, e := s.svc()
	if e != nil {
		return cat.MessageQueue{}, e
	}
	return c.GetCatMessageQueue(s.rootCtx(), convID)
}
func (s *CatService) QueueCatMessage(req cat.SendMessageRequest) (cat.MessageQueue, error) {
	c, e := s.svc()
	if e != nil {
		return cat.MessageQueue{}, e
	}
	return c.QueueCatMessage(s.rootCtx(), req)
}
func (s *CatService) UpdateCatMessageQueue(req cat.QueueAction) (cat.MessageQueue, error) {
	c, e := s.svc()
	if e != nil {
		return cat.MessageQueue{}, e
	}
	return c.UpdateCatMessageQueue(s.rootCtx(), req)
}
