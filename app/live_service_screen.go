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

// ListCaptureSources 返回屏幕推流可选的采集来源：屏幕（screen:<序号>），Windows 上还有应用窗口（window:<hwnd>）。
// 不能采集屏幕时返回 UNSUPPORTED_PLATFORM。id 原样传给 StartScreenPush 的 captureSourceId；来源失效时 StartScreenPush 返回 LIVE_SOURCE_GONE。
func (s *LiveService) ListCaptureSources() ([]live.CaptureSource, error) {
	l, err := s.svc()
	if err != nil {
		return nil, err
	}
	return l.ListCaptureSources(s.rootCtx())
}
