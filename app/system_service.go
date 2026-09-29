package app

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/system"
	"FFmpegFree/internal/store"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SystemService 是系统相关能力的 Wails 绑定（契约第 4、9 节）。
// 目前包含 ffmpeg 检测、设置里和 ffmpeg 相关的两项、RevealInFolder 与 PickDirectory；PickFiles、GetEnv 等后续 PR 补充。
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
// mirror 只接受 ""（默认源）和 GetInstallOptions().mirrors 里列出的镜像；其他值返回 INVALID_ARGUMENT，
// detail 列出可选镜像，不会悄悄改用默认源（目前只有 Windows 有 "cn"，macOS / Linux 传 "cn" 会被拒绝）。
// 幂等：已有进行中的安装时返回同一个任务。这是任务管理器里的一个 ffmpeg_install 任务（batch 池），进度走 task:progress，任务状态走 task:status，ffmpeg 状态走 ffmpeg:status。
func (s *SystemService) InstallFFmpeg(mirror string) (store.Task, error) {
	return s.mgr.Install(context.Background(), mirror)
}

// GetInstallOptions 返回当前平台的安装选项：平台名、是否有下载源、可用的镜像列表。
// 前端只在 mirrors 非空时显示"使用国内镜像"开关。
func (s *SystemService) GetInstallOptions() (system.InstallOptions, error) {
	return s.mgr.GetInstallOptions()
}

// CancelFFmpegInstall 取消进行中的安装，已下载的部分保留以便下次续传。没有安装在进行时什么也不做。
func (s *SystemService) CancelFFmpegInstall() error {
	return s.mgr.CancelInstall()
}

// RevealInFolder 在系统文件管理器里显示 path（Windows 选中文件，macOS `open -R`，Linux 打开所在文件夹）；
// path 是文件夹时直接打开它。path 必须是绝对路径：空或相对路径返回 INVALID_ARGUMENT，不存在返回 NOT_FOUND，
// 无法启动文件管理器返回 PROCESS_FAILED。命令启动后立即返回。
func (s *SystemService) RevealInFolder(path string) error {
	return s.mgr.RevealInFolder(path)
}

// PickDirectory 弹出系统"选择文件夹"对话框，返回所选目录的绝对路径；用户取消返回空字符串（不是错误）。
// title 为空用默认标题。应用还没启动完成时返回 INTERNAL。
func (s *SystemService) PickDirectory(title string) (string, error) {
	ctx := s.mgr.AppContext()
	if ctx == nil {
		return "", apperr.New(apperr.Internal, "应用尚未初始化")
	}
	if title == "" {
		title = "选择文件夹"
	}
	dir, err := runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{Title: title, CanCreateDirectories: true})
	if err != nil {
		return "", apperr.Wrap(apperr.Internal, "打开文件夹选择对话框失败", err)
	}
	return dir, nil // 取消时 Wails 返回 ""
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
