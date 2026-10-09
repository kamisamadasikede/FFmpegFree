package app

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/langasr"
	"FFmpegFree/internal/service/lang"
	"FFmpegFree/internal/store"
)

// LangService 是语音工具（转字幕一期）的 Wails 绑定（契约 6.18）。
type LangService struct {
	get func() *lang.Service
	ctx func() context.Context
}

// NewLangService 创建绑定。get 在 OnStartup 完成前可能返回 nil。
func NewLangService(get func() *lang.Service, rootCtx func() context.Context) *LangService {
	return &LangService{get: get, ctx: rootCtx}
}

func (s *LangService) rootCtx() context.Context {
	if s.ctx != nil {
		if c := s.ctx(); c != nil {
			return c
		}
	}
	return context.Background()
}

func (s *LangService) svc() (*lang.Service, error) {
	if s.get != nil {
		if c := s.get(); c != nil {
			return c, nil
		}
	}
	return nil, apperr.New(apperr.Internal, "语音识别服务尚未初始化")
}

func (s *LangService) GetLangAsrStatus() (langasr.Status, error) {
	c, err := s.svc()
	if err != nil {
		return langasr.Status{}, err
	}
	return c.GetLangAsrStatus(s.rootCtx())
}

func (s *LangService) InstallLangAsr(tier string) (langasr.Status, error) {
	c, err := s.svc()
	if err != nil {
		return langasr.Status{}, err
	}
	return c.InstallLangAsr(s.rootCtx(), tier)
}

func (s *LangService) CancelLangAsrInstall() error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.CancelLangAsrInstall(s.rootCtx())
}

func (s *LangService) RecheckLangAsr() (langasr.Status, error) {
	c, err := s.svc()
	if err != nil {
		return langasr.Status{}, err
	}
	return c.RecheckLangAsr(s.rootCtx())
}

func (s *LangService) SubmitSpeechToSubtitle(req lang.SpeechToSubtitleRequest) ([]store.Task, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.SubmitSpeechToSubtitle(s.rootCtx(), req)
}

func (s *LangService) ExportSubtitleCues(req lang.ExportSubtitleRequest) (lang.ExportSubtitleResult, error) {
	c, err := s.svc()
	if err != nil {
		return lang.ExportSubtitleResult{}, err
	}
	return c.ExportSubtitleCues(s.rootCtx(), req)
}
