package catagent

import (
	"os"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/apperr"
)

// MediaKind 按扩展名判断能不能在对话里预览。第二个返回值是浏览器用来显示的类型。
func MediaKind(name string) (kind, mime string, ok bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image", "image/png", true
	case ".jpg", ".jpeg":
		return "image", "image/jpeg", true
	case ".gif":
		return "image", "image/gif", true
	case ".webp":
		return "image", "image/webp", true
	case ".bmp":
		return "image", "image/bmp", true
	case ".ico":
		return "image", "image/x-icon", true
	case ".avif":
		return "image", "image/avif", true
	case ".mp4", ".m4v":
		return "video", "video/mp4", true
	case ".webm":
		return "video", "video/webm", true
	case ".mov":
		return "video", "video/quicktime", true
	case ".mkv":
		return "video", "video/x-matroska", true
	case ".avi":
		return "video", "video/x-msvideo", true
	case ".mp3":
		return "audio", "audio/mpeg", true
	case ".wav":
		return "audio", "audio/wav", true
	case ".m4a":
		return "audio", "audio/mp4", true
	case ".ogg", ".opus":
		return "audio", "audio/ogg", true
	case ".flac":
		return "audio", "audio/flac", true
	case ".weba":
		return "audio", "audio/webm", true
	case ".pdf":
		return "pdf", "application/pdf", true
	case ".docx":
		return "docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true
	case ".xlsx", ".xlsm", ".xls":
		return "xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", true
	case ".pptx":
		return "pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", true
	case ".md", ".markdown", ".mdown", ".mkd", ".mdx":
		return "markdown", "text/markdown", true
	case ".txt", ".text", ".log":
		return "text", "text/plain", true
	case ".csv":
		return "csv", "text/csv", true
	case ".tsv":
		return "csv", "text/tab-separated-values", true
	case ".html", ".htm":
		return "html", "text/html", true
	case ".svg":
		return "svg", "image/svg+xml", true
	default:
		return "", "", false
	}
}

// ClassifyPreviewPath 确认这是本机上一个能预览的普通文件。
func ClassifyPreviewPath(path string) (kind, mime, name string, err error) {
	p := strings.TrimSpace(path)
	if p == "" || !filepath.IsAbs(p) {
		return "", "", "", apperr.New(apperr.InvalidArgument, "请选择本机上的文件")
	}
	p = filepath.Clean(p)
	base := filepath.Base(p)
	if reservedDevicePath(p) {
		return "", "", "", apperr.New(apperr.InvalidArgument, base+" 在 Windows 上是设备名，没法预览。")
	}
	kind, mime, ok := MediaKind(base)
	if !ok {
		return "", "", "", apperr.New(apperr.InvalidArgument, "这个文件不能在这里预览")
	}
	fi, statErr := os.Stat(p)
	if statErr != nil {
		return "", "", "", apperr.New(apperr.NotFound, "找不到这个文件")
	}
	if !fi.Mode().IsRegular() {
		return "", "", "", apperr.New(apperr.InvalidArgument, "请选择本机上的文件")
	}
	return kind, mime, base, nil
}
