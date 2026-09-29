package app

import (
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/live"
)

func TestLiveServiceScreenNotReady(t *testing.T) {
	s := NewLiveService(func() *live.Service { return nil }, nil)
	if _, err := s.StartScreenPush(live.ScreenPushRequest{}); apperr.From(err).Code != apperr.Internal {
		t.Fatalf("%v", err)
	}
	if _, err := s.GetCaptureCapabilities(); apperr.From(err).Code != apperr.Internal {
		t.Fatalf("%v", err)
	}
	if _, err := s.ListScreens(); apperr.From(err).Code != apperr.Internal {
		t.Fatalf("%v", err)
	}
}
