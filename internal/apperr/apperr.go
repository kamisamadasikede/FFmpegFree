// Package apperr 定义 Bind 方法对前端返回的统一错误（见 docs/architecture/contract.md 第 2 节）。
//
// Wails 会把 error.Error() 的内容作为前端 Promise 的 reject 值，
// 所以 Error() 返回 JSON 字符串，前端 api/ 层统一解析。
package apperr

import (
	"encoding/json"
	"errors"
)

// Code 是前后端约定的错误码。
type Code string

const (
	InvalidArgument     Code = "INVALID_ARGUMENT"
	NotFound            Code = "NOT_FOUND"
	FFmpegNotFound      Code = "FFMPEG_NOT_FOUND"
	TaskConflict        Code = "TASK_CONFLICT"
	IOError             Code = "IO_ERROR"
	ProcessFailed       Code = "PROCESS_FAILED"
	UnsupportedPlatform Code = "UNSUPPORTED_PLATFORM"
	Internal            Code = "INTERNAL"

	// PROBE_FAILED：文件存在但 ffprobe 无法解析（损坏、不是媒体文件、无可读流）。v0.8 新增。
	ProbeFailed Code = "PROBE_FAILED"

	// ConvertDiskFull：转换写输出文件时磁盘空间不足（ffmpeg 报 No space left on device / ENOSPC 等）。v0.9 新增。
	ConvertDiskFull Code = "CONVERT_DISK_FULL"
	// Canceled：调用因应用退出（根 ctx 取消）或调用方取消而中断。v0.9.1 新增。
	Canceled Code = "CANCELED"
	// Unsupported：该操作不支持这个对象（如没有注册重试工厂的任务类型不能 Retry）。v0.7.1 新增。
	Unsupported Code = "UNSUPPORTED"

	// 直播 / 录屏相关，后端返回（契约第 2 节）。
	// LIVE_PLAY_FAILED、LIVE_CORS_BLOCKED 是前端播放器自己产生的，不在这里定义。
	LiveURLInvalid         Code = "LIVE_URL_INVALID"
	LiveConnectFailed      Code = "LIVE_CONNECT_FAILED"
	LivePushRejected       Code = "LIVE_PUSH_REJECTED"
	LivePushInterrupted    Code = "LIVE_PUSH_INTERRUPTED"
	ScreenPermissionDenied Code = "SCREEN_PERMISSION_DENIED"
	// LiveSourceGone：屏幕推流所选的采集来源（窗口 / 屏幕）已不可用（窗口已关闭 / 最小化，屏幕已拔掉）。v0.14 新增。
	LiveSourceGone Code = "LIVE_SOURCE_GONE"

	// 文档转换（契约 v0.26，6.12.20）。
	DocEncrypted              Code = "DOC_ENCRYPTED"
	DocCorrupt                Code = "DOC_CORRUPT"
	DocTimeout                Code = "DOC_TIMEOUT"
	DocComponentCrashed       Code = "DOC_COMPONENT_CRASHED"
	DocComponentNotReady      Code = "DOC_COMPONENT_NOT_READY"
	DocDownloadFailed         Code = "DOC_DOWNLOAD_FAILED"
	DocChecksumFailed         Code = "DOC_CHECKSUM_FAILED"
	DocComponentInstallFailed Code = "DOC_COMPONENT_INSTALL_FAILED"
	DocFormatUnsupported      Code = "DOC_FORMAT_UNSUPPORTED"
	DocPDFInputUnsupported    Code = "DOC_PDF_INPUT_UNSUPPORTED"

	// v0.27 本机 Office / WPS（6.12.33）。
	DocPresentationBusy Code = "DOC_PRESENTATION_BUSY"
	DocEngineBusy       Code = "DOC_ENGINE_BUSY"
)

// AppError 同时用于 Bind 返回值和持久化到 tasks.error 列。
type AppError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`

	cause error
}

func (e *AppError) Error() string {
	b, err := json.Marshal(e)
	if err != nil {
		return `{"code":"INTERNAL","message":"错误序列化失败"}`
	}
	return string(b)
}

func (e *AppError) Unwrap() error { return e.cause }

// New 创建一个不带底层原因的错误。
func New(code Code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// Wrap 包装底层错误，底层错误的文本放进 Detail，方便排查。
func Wrap(code Code, message string, cause error) *AppError {
	e := &AppError{Code: code, Message: message, cause: cause}
	if cause != nil {
		e.Detail = cause.Error()
	}
	return e
}

// WithDetail 返回带自定义 Detail 的副本，例如子进程最后 50 行日志。
func (e *AppError) WithDetail(detail string) *AppError {
	c := *e
	c.Detail = detail
	return &c
}

// From 把任意 error 转成 AppError；已经是 AppError 的原样返回，其他归为 INTERNAL。
func From(err error) *AppError {
	if err == nil {
		return nil
	}
	var ae *AppError
	if errors.As(err, &ae) {
		return ae
	}
	return Wrap(Internal, "内部错误", err)
}

// Is 判断 err 链上是否有指定错误码。
func Is(err error, code Code) bool {
	var ae *AppError
	return errors.As(err, &ae) && ae.Code == code
}
