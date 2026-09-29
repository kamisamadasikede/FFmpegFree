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
	return Binaries{}, apperr.New(apperr.FFmpegNotFound, "未找到可用的 ffmpeg，请先安装或手动指定 ffmpeg 所在位置")
}
