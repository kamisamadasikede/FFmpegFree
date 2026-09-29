package app

import (
	"FFmpegFree/internal/service/live"
	"FFmpegFree/internal/store"
)

// StartScreenPush 开始屏幕推流。错误码同 StartFilePush，另有 UNSUPPORTED_PLATFORM、SCREEN_PERMISSION_DENIED。
func (s *LiveService) StartScreenPush(req live.ScreenPushRequest) (store.Task, error) {
	l, err := s.svc()
	if err != nil {
		return store.Task{}, err
	}
	return l.StartScreenPush(s.rootCtx(), req)
}

// GetCaptureCapabilities 返回屏幕采集能不能用、为什么不能用。
func (s *LiveService) GetCaptureCapabilities() (live.CaptureCapabilities, error) {
	l, err := s.svc()
	if err != nil {
		return live.CaptureCapabilities{}, err
	}
	return l.GetCaptureCapabilities()
}

// ListScreens 返回可采集的显示器；不能采集时返回 UNSUPPORTED_PLATFORM。
func (s *LiveService) ListScreens() ([]live.ScreenInfo, error) {
	l, err := s.svc()
	if err != nil {
		return nil, err
	}
	return l.ListScreens(s.rootCtx())
}
