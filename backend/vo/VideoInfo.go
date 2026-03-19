package vo

import (
	"context"
	"os/exec"
)

type VideoInfo struct {
	Name           string   `json:"name"`
	Url            string   `json:"url"`
	Duration       string   `json:"duration"`     // 鍙互鐢?time.Duration 鎴?string 鏍煎紡
	Date           string   `json:"date"`         // 鏂囦欢淇敼鏃堕棿
	TargetFormat   string   `json:"targetFormat"` // 鏂囦欢淇敼鏃堕棿
	SteamUrl       string   `json:"steamurl"`
	StreamID       string   `json:"streamId"`
	Preset         string   `json:"preset"`
	Cover          string   `json:"cover"`
	Progress       int      `json:"progress"`
	ArchiveEnabled bool     `json:"archiveEnabled"`
	SegmentSeconds int      `json:"segmentSeconds"`
	RelayTargets   []string `json:"relayTargets"`
}

type ConvertingTask struct {
	VideoInfo  VideoInfo
	Context    context.Context
	Cancel     context.CancelFunc
	Cmd        *exec.Cmd
	OutputFile string // 娣诲姞杩欎竴琛?
}
