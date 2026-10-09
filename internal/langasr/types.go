package langasr

import "FFmpegFree/internal/apperr"

// 状态（契约 6.18.3，对齐文档组件）。
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

// 事件名（契约 6.18.5）。
const (
	EventAsr      = "lang:asr"
	EventProgress = "lang:asr-progress"
)

// Status 是 LangAsrStatus（契约 6.18.3）。
type Status struct {
	State         string           `json:"state"`
	Version       string           `json:"version"`
	Source        string           `json:"source"` // ready 时 downloaded
	Tier          string           `json:"tier"`   // standard | hd
	CanDownload   bool             `json:"canDownload"`
	DownloadBytes int64            `json:"downloadBytes"`
	InstallBytes  int64            `json:"installBytes,omitempty"`
	Phase         string           `json:"phase,omitempty"`
	ReceivedBytes int64            `json:"receivedBytes,omitempty"`
	Error         *apperr.AppError `json:"error,omitempty"`
	Path          string           `json:"-"` // 组件根目录；不回前端
}

// Progress 是 lang:asr-progress 的 payload。
type Progress struct {
	Phase         string   `json:"phase"`
	ReceivedBytes int64    `json:"receivedBytes"`
	TotalBytes    int64    `json:"totalBytes"`
	Progress      *float64 `json:"progress,omitempty"`
}

// SubtitleCue 是可编辑字幕条目（契约 6.18.5）。
type SubtitleCue struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
}

// 用户可见文案（契约 6.18.8 + 产品 10-09）。
const (
	MsgNotReady       = "需要先下载语音识别组件。"
	MsgEmpty          = "这段音频里没有识别到有效内容。"
	MsgFailed         = "识别没完成，请稍后重试。" // 产品定稿；契约 v0.29.1 已对齐
	MsgDownloadFailed = "语音识别组件下载失败，请检查网络后重试。"
	MsgChecksumFailed = "下载的语音识别组件校验失败，请重试。"
	MsgNotPublished   = "暂时无法下载语音识别组件，请稍后重试。"
	MsgMissingGuide   = "语音识别组件还没准备好，发布后即可下载。"
)
