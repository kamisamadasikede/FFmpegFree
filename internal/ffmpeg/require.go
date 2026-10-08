package ffmpeg

import (
	"sync"

	"FFmpegFree/internal/apperr"
)

var (
	curMu sync.RWMutex
	cur   *Binaries
)

// SetCurrent 由 Manager 在检测结果变化时调用：ready 时传入可用路径，否则传 nil。
func SetCurrent(b *Binaries) {
	curMu.Lock()
	defer curMu.Unlock()
	if b == nil {
		cur = nil
		return
	}
	c := *b
	cur = &c
}

// Current 返回当前可用的 ffmpeg / ffprobe，没有时 ok=false。
func Current() (b Binaries, ok bool) {
	curMu.RLock()
	defer curMu.RUnlock()
	if cur == nil {
		return Binaries{}, false
	}
	return *cur, true
}

// Require 是依赖 ffmpeg 的 Service 的统一门控（契约第 9.5 节）：
// 转换、剪辑、直播、探测、缩略图等入口先调用它，未就绪时直接把返回的错误交给前端。
// 就绪时返回 ffmpeg / ffprobe 的绝对路径，调用方一律用这个路径启动子进程。
func Require() (Binaries, error) {
	if b, ok := Current(); ok {
		return b, nil
	}
	return Binaries{}, apperr.New(apperr.FFmpegNotFound, "未找到可用的转换组件，请先安装或手动指定转换组件所在位置")
}

// RequireProbe 在 Require 的基础上要求 ffprobe 也可用（媒体探测、缩略图前调用）。
// v1 兼容目录只有 ffmpeg 时返回 FFMPEG_NOT_FOUND，detail 说明缺的是 ffprobe，前端引导用户安装。
func RequireProbe() (Binaries, error) {
	b, err := Require()
	if err != nil {
		return Binaries{}, err
	}
	if b.FFprobe == "" {
		return Binaries{}, apperr.New(apperr.FFmpegNotFound, "转换组件不完整（缺少媒体信息读取程序），请重新安装转换组件或手动指定完整的转换组件所在位置").
			WithDetail("当前使用的 ffmpeg 来自 v1 目录，没有附带 ffprobe")
	}
	return b, nil
}
