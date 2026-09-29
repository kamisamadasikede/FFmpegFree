package app

import (
	"context"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/service/convert"
)

func TestConvertServiceNotReadyIsInternal(t *testing.T) {
	s := NewConvertService(func() *convert.Service { return nil }, nil)
	if _, err := s.Submit([]string{"/a.mp4"}, ffmpeg.ConvertOptions{Container: "mp4"}, ""); !apperr.Is(err, apperr.Internal) {
		t.Fatalf("未就绪应 INTERNAL: %v", err)
	}
}

func TestConvertServiceRootCtx(t *testing.T) {
	if NewConvertService(nil, nil).rootCtx() == nil {
		t.Fatal("rootCtx 为 nil 时应退回 Background")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := NewConvertService(nil, func() context.Context { return ctx })
	cancel()
	if s.rootCtx().Err() == nil {
		t.Fatal("应使用应用根 ctx（取消后应已取消）")
	}
	s2 := NewConvertService(nil, func() context.Context { return nil })
	if s2.rootCtx() == nil {
		t.Fatal("根 ctx 未就绪时应退回 Background")
	}
}
