package vo

import (
	"context"
	"os/exec"
)

type VideoInfo struct {
	Name         string `json:"name"`
	Url          string `json:"url"`
	Duration     string `json:"duration"`     // 可以用 time.Duration 或 string 格式
	Date         string `json:"date"`         // 文件修改时间
	TargetFormat string `json:"targetFormat"` // 文件修改时间
}
type ConvertingTask struct {
	VideoInfo  VideoInfo
	Context    context.Context
	Cancel     context.CancelFunc
	Cmd        *exec.Cmd
	OutputFile string // 添加这一行

}
