package langasr

import (
	"context"
	"sync"
)

// HWGate：高清 ASR 运行时，硬件编码并发 ≤ 1（契约 6.18.2）。
type HWGate struct {
	mu       sync.Mutex
	hdActive int
	hwSem    chan struct{} // cap 1；仅 hdActive>0 时使用
}

func (g *HWGate) ensure() {
	if g.hwSem == nil {
		g.hwSem = make(chan struct{}, 1)
	}
}

// EnterHD 标记开始一次高清 ASR。
func (g *HWGate) EnterHD() {
	g.mu.Lock()
	g.ensure()
	g.hdActive++
	g.mu.Unlock()
}

// LeaveHD 结束一次高清 ASR。
func (g *HWGate) LeaveHD() {
	g.mu.Lock()
	if g.hdActive > 0 {
		g.hdActive--
	}
	g.mu.Unlock()
}

// HDActive 是否有高清 ASR 在跑。
func (g *HWGate) HDActive() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.hdActive > 0
}

// AcquireHWEncode：若高清 ASR 在跑，则占用唯一硬件编码名额；否则立即返回空 release。
func (g *HWGate) AcquireHWEncode(ctx context.Context) (release func(), err error) {
	g.mu.Lock()
	active := g.hdActive
	g.ensure()
	sem := g.hwSem
	g.mu.Unlock()
	if active <= 0 {
		return func() {}, nil
	}
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
