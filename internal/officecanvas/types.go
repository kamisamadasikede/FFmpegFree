package officecanvas

import "FFmpegFree/internal/apperr"

// 状态（契约 6.19.16.3，对齐语音识别 / 文档组件）。
const (
	StateChecking    = "checking"
	StateReady       = "ready"
	StateMissing     = "missing"
	StateOutdated    = "outdated"
	StateDownloading = "downloading"
	StatePreparing   = "preparing"
	StateFailed      = "failed"
)

// 来源：一期只有 downloaded。
const SourceDownloaded = "downloaded"

// 事件名（契约 6.19.16.3）。
const (
	EventCanvas   = "office:canvas"
	EventProgress = "office:canvas-progress"
)

// Status 是 OfficeCanvasStatus（契约 6.19.16.3）。
type Status struct {
	State         string           `json:"state"`
	Version       string           `json:"version"`
	Source        string           `json:"source"` // ready 时 downloaded；一期无系统安装
	CanDownload   bool             `json:"canDownload"`
	DownloadBytes int64            `json:"downloadBytes"`
	InstallBytes  int64            `json:"installBytes,omitempty"`
	Phase         string           `json:"phase,omitempty"`
	ReceivedBytes int64            `json:"receivedBytes,omitempty"`
	Error         *apperr.AppError `json:"error,omitempty"`
	Path          string           `json:"-"` // 组件根目录；不回前端
}

// Progress 是 office:canvas-progress 的 payload（≤4 次/秒）。
type Progress struct {
	Phase         string   `json:"phase"`
	ReceivedBytes int64    `json:"receivedBytes"`
	TotalBytes    int64    `json:"totalBytes"`
	Progress      *float64 `json:"progress,omitempty"`
}

// 用户可见文案（契约 6.19.16；不得出现 grok / CLI / ffmpeg / 旗标名）。
const (
	MsgNotPublished   = "暂时无法下载办公画布组件，请稍后重试。"
	MsgMissingGuide   = "办公画布组件还没准备好，发布后即可下载。"
	MsgNotReady       = "需要先下载办公画布组件。"
	MsgDownloadFailed = "办公画布组件下载失败，请检查网络后重试。"
	MsgExportPrefix   = "导出失败："
	MsgExportNotReady = "导出失败：组件尚未就绪"
	MsgExportNoDeck   = "导出失败：找不到演示文稿"
	MsgExportStub     = "导出失败：暂时无法导出"
)
