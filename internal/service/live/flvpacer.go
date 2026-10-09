package live

import (
	"sync"
	"time"
)

// flvPacer 让 HLS 拉流的预览按时间戳匀速送进分发器（契约 v0.25.1）。
//
// HLS 一次到一整个分片：ffmpeg -c copy 转出来的 FLV 每隔一个分片时长（MediaMTX 实测 2 秒）一次性到 2 秒的数据，
// 然后什么都没有。播放器的缓冲在 0 和 2 秒之间来回，追帧（倍速 / 跳到最新）会跳到 GOP 中间，
// 解码缺参考帧就是灰色花屏，一直到下一个关键帧。这里给每个 tag 排一个放出时间：
// 第一帧到达时定锚，之后按 锚点 + (ts − 锚点 ts) 放出；某个 tag 到达时已经晚于它的放出时间（数据断档），
// 就把锚点后移到它到达的时刻，延迟增加这次的晚到量。一两个分片之后延迟稳定在“分片的到达抖动”，输出和 RTMP 一样匀速。
// 时间戳跳变（倒退或者一下子往前跳太多）、积压太多时重新定锚，宁可一次性放出，也不无限扣着。
type flvPacer struct {
	out func(flvTag)
	now func() time.Time

	mu     sync.Mutex
	q      []pacedTag
	qBytes int
	wake   chan struct{}
	closed bool
	done   chan struct{}
	exited chan struct{}

	anchored   bool
	anchorWall time.Time
	anchorTS   int32
	lastTS     int32
	// 收缩：开头定锚时可能已经比需要的晚（拿到的分片早就生成好了），延迟只增不减就一直多扣着。
	// 每 10 秒看一次这段时间里最小的余量（放出时间 − 到达时间），还有富余就把锚点提前一点（每次最多 250 毫秒，
	// 相当于这 10 秒里放得快 2.5%），留 50 毫秒的余量。
	winStart time.Time
	winMin   time.Duration
}

type pacedTag struct {
	t  flvTag
	at time.Time
}

const (
	pacerLateSlack = 40 * time.Millisecond // 晚到不超过这么多不算断档（音视频交织、调度抖动）
	pacerMaxHold   = 8 * time.Second       // 某个 tag 要扣这么久以上：时间戳往前跳了，重新定锚
	pacerMaxQueue  = 30 * time.Second      // 积压的媒体时长上限（源时钟比本机快，越积越多）
	pacerMaxBytes  = 64 << 20
	pacerWindow    = 10 * time.Second
	pacerKeepSlack = 50 * time.Millisecond
	pacerMaxShrink = 250 * time.Millisecond
)

func newFLVPacer(out func(flvTag)) *flvPacer {
	p := &flvPacer{out: out, now: time.Now, wake: make(chan struct{}, 1), done: make(chan struct{}), exited: make(chan struct{})}
	go p.loop()
	return p
}

// push 排队一个 tag（读 ffmpeg 的协程调用，不阻塞）。
func (p *flvPacer) push(t flvTag) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.q = append(p.q, pacedTag{t: t, at: p.now()})
	p.qBytes += len(t.raw)
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// close 把还在排队的 tag 立即放完，等放完再返回。
func (p *flvPacer) close() {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.done)
	}
	p.mu.Unlock()
	<-p.exited
}

func (p *flvPacer) loop() {
	defer close(p.exited)
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		p.mu.Lock()
		if len(p.q) == 0 {
			closed := p.closed
			p.mu.Unlock()
			if closed {
				return
			}
			select {
			case <-p.wake:
			case <-p.done:
			}
			continue
		}
		head := p.q[0]
		wait := p.scheduleLocked(head)
		if wait > 0 && !p.closed {
			p.mu.Unlock()
			timer.Reset(wait)
			select {
			case <-timer.C:
			case <-p.wake: // 新数据可能触发“积压太多”，重新算一遍
				if !timer.Stop() {
					<-timer.C
				}
			case <-p.done:
				if !timer.Stop() {
					<-timer.C
				}
			}
			continue
		}
		p.q[0] = pacedTag{}
		p.q = p.q[1:]
		p.qBytes -= len(head.t.raw)
		p.mu.Unlock()
		p.out(head.t)
	}
}

// scheduleLocked 返回队首还要等多久（<=0 立即放出），必要时调整锚点。
func (p *flvPacer) scheduleLocked(h pacedTag) time.Duration {
	if !(h.t.video || h.t.audio) || h.t.seq {
		return 0 // metadata、序列头：立即放出
	}
	if !p.anchored {
		p.reanchor(h)
		return 0
	}
	// 时间戳倒退（远端重连、分片不连续）
	if h.t.ts < p.lastTS-1000 {
		p.reanchor(h)
		return 0
	}
	due := p.anchorWall.Add(time.Duration(h.t.ts-p.anchorTS) * time.Millisecond)
	if late := h.at.Sub(due); late > pacerLateSlack {
		// 数据断档：这个 tag 到的时候就已经过了放出时间，延迟加上这次的晚到量。
		p.anchorWall = p.anchorWall.Add(late)
		due = h.at
	}
	if due.Sub(h.at) > pacerMaxHold {
		p.reanchor(h)
		return 0
	}
	if last := p.q[len(p.q)-1].t; (last.video || last.audio) && time.Duration(last.ts-h.t.ts)*time.Millisecond > pacerMaxQueue || p.qBytes > pacerMaxBytes {
		p.reanchor(h)
		return 0
	}
	p.lastTS = max(p.lastTS, h.t.ts)
	if slack := due.Sub(h.at); slack < p.winMin {
		p.winMin = slack
	}
	if now := p.now(); now.Sub(p.winStart) >= pacerWindow {
		if p.winMin > 2*pacerKeepSlack {
			shrink := min(p.winMin-pacerKeepSlack, pacerMaxShrink)
			p.anchorWall = p.anchorWall.Add(-shrink)
			due = due.Add(-shrink)
		}
		p.winStart, p.winMin = now, time.Duration(1<<62)
	}
	return due.Sub(p.now())
}

func (p *flvPacer) reanchor(h pacedTag) {
	p.anchored = true
	p.anchorWall = p.now()
	p.anchorTS = h.t.ts
	p.lastTS = h.t.ts
	p.winStart, p.winMin = p.anchorWall, time.Duration(1<<62)
}
