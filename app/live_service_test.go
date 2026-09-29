package app

import (
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/live"
)

func TestLiveServiceNotReady(t *testing.T) {
	s := NewLiveService(func() *live.Service { return nil }, nil)
	if _, err := s.StartFilePush(live.FilePushRequest{}); apperr.From(err).Code != apperr.Internal {
		t.Fatalf("%v", err)
	}
	if _, err := s.CheckPushURL("rtmp://a/b"); apperr.From(err).Code != apperr.Internal {
		t.Fatalf("%v", err)
	}
	if NewLiveService(nil, nil).rootCtx() == nil {
		t.Fatal("rootCtx 不能为 nil")
	}
}
