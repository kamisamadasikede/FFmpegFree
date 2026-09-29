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
