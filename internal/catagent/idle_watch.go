package catagent

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// DefaultIdleTimeout 是普通对话一轮的无响应超时（v0.53.1 §6.19.28.9）：
// stdout / stderr / 会话文件任何一行输出都会重新计时；连续这么久没有输出才停掉这一轮。
// 定时任务（TurnOptions.Timeout > 0）仍按总时长，不走这里。测试可改小。
var DefaultIdleTimeout = 10 * time.Minute

// idleCheckEvery 是检查间隔上限（测试可改小）。
var idleCheckEvery = 5 * time.Second

// ReasonIdleTimeout 是无响应超时的 CAT_REPLY_FAILED reason。
const ReasonIdleTimeout = "reason=idle_timeout"

// MsgIdleTimeout 是无响应超时的文案（前端同文）。
const MsgIdleTimeout = "CLI 已经 10 分钟没有响应，已停止这一轮。"

var errIdleTimeout = errors.New("cat turn idle timeout")

// idleWatch 在 ctx 上挂一个看门狗：超过 idle 没有 touch 就以 errIdleTimeout 取消 ctx
// （runCmd 的 cmd.Cancel 负责结束整个进程树）。
type idleWatch struct {
	mu     sync.Mutex
	last   time.Time
	now    func() time.Time
	idle   time.Duration
	cancel context.CancelCauseFunc
	stop   chan struct{}
	once   sync.Once
}

// newTurnContext：timeout > 0 → 总时长（定时任务）；否则 → 无响应超时。
func newTurnContext(ctx context.Context, timeout time.Duration, now func() time.Time) (context.Context, *idleWatch, func()) {
	if timeout > 0 {
		c, cancel := context.WithTimeout(ctx, timeout)
		return c, nil, cancel
	}
	if now == nil {
		now = time.Now
	}
	c, cancel := context.WithCancelCause(ctx)
	w := &idleWatch{last: now(), now: now, idle: DefaultIdleTimeout, cancel: cancel, stop: make(chan struct{})}
	every := idleCheckEvery
	if w.idle/4 < every {
		every = w.idle / 4
	}
	if every <= 0 {
		every = time.Millisecond
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-c.Done():
				return
			case <-t.C:
				w.mu.Lock()
				silent := w.now().Sub(w.last)
				w.mu.Unlock()
				if silent >= w.idle {
					cancel(errIdleTimeout)
					return
				}
			}
		}
	}()
	return c, w, func() { w.once.Do(func() { close(w.stop) }); cancel(nil) }
}

func (w *idleWatch) touch() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.last = w.now()
	w.mu.Unlock()
}

// idleTimedOut 报告 ctx 是不是因无响应被停掉的。
func idleTimedOut(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), errIdleTimeout)
}

// activityReader 每读到数据就 touch。
type activityReader struct {
	r io.Reader
	w *idleWatch
}

func (a activityReader) Read(p []byte) (int, error) {
	n, err := a.r.Read(p)
	if n > 0 {
		a.w.touch()
	}
	return n, err
}

// activityWriter 每写入数据就 touch（stderr）。
type activityWriter struct {
	inner io.Writer
	w     *idleWatch
}

func (a activityWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		a.w.touch()
	}
	return a.inner.Write(p)
}
