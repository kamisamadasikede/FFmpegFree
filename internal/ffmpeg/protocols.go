package ffmpeg

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Protocols 是 `ffmpeg -protocols` 里 Output 段支持的协议集合（小写）。
type Protocols map[string]bool

// ParseProtocols 解析 `ffmpeg -protocols` 的输出，只取 Output: 段（推流关心输出）。
func ParseProtocols(out string) Protocols {
	p := Protocols{}
	section := ""
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "Input:"):
			section = "in"
		case strings.HasPrefix(t, "Output:"):
			section = "out"
		case section == "out" && t != "":
			p[strings.ToLower(t)] = true
		}
	}
	return p
}

// ProtocolProbe 用 ffmpeg -protocols 检查协议支持，结果按 ffmpeg 路径缓存（成功的结果才缓存）。
type ProtocolProbe struct {
	Run func(ctx context.Context, exe string, args ...string) (string, error) // 默认 ExecRunner(10s)
	mu  sync.Mutex
	m   map[string]Protocols
}

// OutputProtocols 返回 exe 支持的输出协议。
func (pp *ProtocolProbe) OutputProtocols(ctx context.Context, exe string) (Protocols, error) {
	pp.mu.Lock()
	if p, ok := pp.m[exe]; ok {
		pp.mu.Unlock()
		return p, nil
	}
	pp.mu.Unlock()
	run := pp.Run
	if run == nil {
		run = ExecRunner(10 * time.Second)
	}
	out, err := run(ctx, exe, "-hide_banner", "-protocols")
	if err != nil {
		return nil, err
	}
	p := ParseProtocols(out)
	pp.mu.Lock()
	if pp.m == nil {
		pp.m = map[string]Protocols{}
	}
	pp.m[exe] = p
	pp.mu.Unlock()
	return p, nil
}
