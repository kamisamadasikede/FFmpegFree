package cat

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
)

// MediaPreview 是页面用来加载本机文件的地址。
type MediaPreview struct {
	URL  string `json:"url"`
	Mime string `json:"mime"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// PreviewCatMedia 把用户选中或消息里的本机文件登记成可加载地址。只接受能预览的普通文件。
func (s *Service) PreviewCatMedia(_ context.Context, path string) (MediaPreview, error) {
	kind, mime, name, err := catagent.ClassifyPreviewPath(path)
	if err != nil {
		return MediaPreview{}, err
	}
	if s.cfg.Preview == nil {
		return MediaPreview{}, apperr.New(apperr.Internal, "这里还不能预览这个文件")
	}
	entry, err := s.cfg.Preview.Register(path)
	if err != nil {
		return MediaPreview{}, apperr.New(apperr.NotFound, "找不到这个文件")
	}
	if entry.Mime != "" && entry.Mime != "application/octet-stream" {
		mime = entry.Mime
	}
	return MediaPreview{URL: entry.URL, Mime: mime, Kind: kind, Name: name}, nil
}
