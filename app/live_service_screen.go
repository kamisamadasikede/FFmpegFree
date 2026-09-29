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

// GetPreview 返回直播会话最新一帧预览（契约 v0.14）：sessionId 是推流任务 id（StartFilePush / StartScreenPush 返回的 Task.id）
// 或 StartPullPreview 返回的拉流预览会话 id。没有画面（会话不存在或已结束、preview=false、ffmpeg 还没出第一帧、读到半帧）
// 返回空 data、ts=0，不是错误。前端约每 500 毫秒轮询一次，active=false 或页面不可见时停止。
func (s *LiveService) GetPreview(sessionID string) (live.Preview, error) {
	l, err := s.svc()
	if err != nil {
		return live.Preview{}, err
	}
	return l.GetPreview(sessionID)
}

// StartPullPreview 开始拉流预览会话：后端 ffmpeg 读远端流，只输出预览画面（播放仍由前端播放器直接拉地址）。
// 支持 rtmp / rtmps / srt / http / https（ws / wss 返回 LIVE_URL_INVALID reason=scheme_unsupported）；同一地址重复调用返回同一会话；
// 纯音频的流没有预览（会话很快结束，GetPreview 的 active 变为 false）；preview=false 不启动 ffmpeg。
func (s *LiveService) StartPullPreview(req live.PullPreviewRequest) (live.PullSession, error) {
	l, err := s.svc()
	if err != nil {
		return live.PullSession{}, err
	}
	return l.StartPullPreview(s.rootCtx(), req)
}

// StopPullPreview 停止拉流预览会话并清理预览文件；会话不存在（已结束）时什么也不做。
func (s *LiveService) StopPullPreview(sessionID string) error {
	l, err := s.svc()
	if err != nil {
		return err
	}
	return l.StopPullPreview(sessionID)
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
