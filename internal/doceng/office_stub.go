//go:build !windows

package doceng

import (
	"context"

	"FFmpegFree/internal/apperr"
)

// OfficeConverter 非 Windows 桩：永远不可用。
type OfficeConverter struct{}

func NewOfficeConverter() *OfficeConverter { return &OfficeConverter{} }

func (c *OfficeConverter) Convert(ctx context.Context, d Detected, req ConvertRequest) error {
	return apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。")
}

func (c *OfficeConverter) PreviewPDF(ctx context.Context, d Detected, req PreviewPDFRequest) error {
	return apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。")
}
