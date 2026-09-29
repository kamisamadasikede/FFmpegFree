package app

import (
	"context"

	"FFmpegFree/internal/service/system"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SystemService 是系统相关能力的 Wails 绑定（契约第 4、9 节）。
// 目前包含 ffmpeg 检测与设置里和 ffmpeg 相关的两项；PickFiles、GetEnv 等后续 PR 补充。
//
// 检测状态机在 system.Manager 里，由 App.startup 调用 Manager.Start 在后台启动，
// 所以这里的方法都只是转发。
type SystemService struct {
	mgr *system.Manager
}

func NewSystemService(mgr *system.Manager) *SystemService { return &SystemService{mgr: mgr} }

// GetFFmpegStatus 返回当前 ffmpeg 状态。启动检测未完成时为 checking。
func (s *SystemService) GetFFmpegStatus() (system.FFmpegStatus, error) {
	return s.mgr.Status(), nil
}

// RecheckFFmpeg 重新检测，会推送 ffmpeg:status（先 checking，后结果），返回最终状态。
func (s *SystemService) RecheckFFmpeg() (system.FFmpegStatus, error) {
	return s.mgr.Recheck(context.Background())
}

// SetFFmpegPath 手动指定 ffmpeg 所在目录（也接受直接指向 ffmpeg 可执行文件）。
// 校验失败返回 INVALID_ARGUMENT；成功后写入设置并推送 ffmpeg:status。传空字符串清除手动指定。
func (s *SystemService) SetFFmpegPath(dir string) (system.FFmpegStatus, error) {
	return s.mgr.SetPath(context.Background(), dir)
}

// InstallFFmpeg 下载并安装 ffmpeg 到 <数据目录>/bin（契约 9.3），立即返回任务信息，安装在后台进行。
// mirror 只接受 ""（默认源）和 "cn"，其他值返回 INVALID_ARGUMENT；"cn" 在清单没有对应镜像的平台会退回默认源。
// 幂等：已有进行中的安装时返回同一个任务。进度走 task:progress，状态走 task:status 和 ffmpeg:status。
func (s *SystemService) InstallFFmpeg(mirror string) (system.InstallTask, error) {
	return s.mgr.Install(context.Background(), mirror)
}

// CancelFFmpegInstall 取消进行中的安装，已下载的部分保留以便下次续传。没有安装在进行时什么也不做。
func (s *SystemService) CancelFFmpegInstall() error {
	return s.mgr.CancelInstall()
}

// GetSettings 返回设置。目前只有 ffmpegPath 和 ffmpegPromptDismissed，其余字段后续补充。
func (s *SystemService) GetSettings() (system.Settings, error) {
	return s.mgr.GetSettings(context.Background())
}

// UpdateSettings 保存设置。ffmpegPath 变化时会先校验，失败返回 INVALID_ARGUMENT 且整体不生效。
func (s *SystemService) UpdateSettings(st system.Settings) error {
	return s.mgr.UpdateSettings(context.Background(), st)
}

// wailsEmitter 用应用 ctx 把事件发给前端。ctx 必须是 OnStartup 收到的那个。
type wailsEmitter struct{ ctx context.Context }

// NewWailsEmitter 返回基于 runtime.EventsEmit 的 system.Emitter。
func NewWailsEmitter(ctx context.Context) system.Emitter { return wailsEmitter{ctx: ctx} }

func (e wailsEmitter) Emit(event string, payload any) {
	runtime.EventsEmit(e.ctx, event, payload)
}
