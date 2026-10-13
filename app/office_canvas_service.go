package app

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/officecanvas"
	"FFmpegFree/internal/service/office"
)

// OfficeCanvasService 是办公画布组件的 Wails 绑定（契约 6.19.16）。
type OfficeCanvasService struct {
	get func() *office.Service
	ctx func() context.Context
}

// NewOfficeCanvasService 创建绑定。get 在 OnStartup 完成前可能返回 nil。
func NewOfficeCanvasService(get func() *office.Service, rootCtx func() context.Context) *OfficeCanvasService {
	return &OfficeCanvasService{get: get, ctx: rootCtx}
}

func (s *OfficeCanvasService) rootCtx() context.Context {
	if s.ctx != nil {
		if c := s.ctx(); c != nil {
			return c
		}
	}
	return context.Background()
}

func (s *OfficeCanvasService) svc() (*office.Service, error) {
	if s.get != nil {
		if c := s.get(); c != nil {
			return c, nil
		}
	}
	return nil, apperr.New(apperr.Internal, "办公画布服务尚未初始化")
}

func (s *OfficeCanvasService) GetOfficeCanvasStatus() (officecanvas.Status, error) {
	c, err := s.svc()
	if err != nil {
		return officecanvas.Status{}, err
	}
	return c.GetOfficeCanvasStatus(s.rootCtx())
}

func (s *OfficeCanvasService) InstallOfficeCanvas() error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.InstallOfficeCanvas(s.rootCtx())
}

func (s *OfficeCanvasService) CancelOfficeCanvasInstall() error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.CancelOfficeCanvasInstall(s.rootCtx())
}

func (s *OfficeCanvasService) RecheckOfficeCanvas() (officecanvas.Status, error) {
	c, err := s.svc()
	if err != nil {
		return officecanvas.Status{}, err
	}
	return c.RecheckOfficeCanvas(s.rootCtx())
}

func (s *OfficeCanvasService) ExportOfficeCanvasPptx(req office.ExportOfficeCanvasPptxRequest) (office.ExportOfficeCanvasPptxResult, error) {
	c, err := s.svc()
	if err != nil {
		return office.ExportOfficeCanvasPptxResult{}, err
	}
	return c.ExportOfficeCanvasPptx(s.rootCtx(), req)
}
