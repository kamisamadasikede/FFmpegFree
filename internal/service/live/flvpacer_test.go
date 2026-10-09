package live

import (
	"sync"
	"testing"
	"time"
)

// flvPacer：数据一阵一阵到（每 500 毫秒一次性到 500 毫秒的帧），放出来要匀速、顺序不变、时间戳跳变不卡住。
func TestFLVPacerSmoothsBursts(t *testing.T) {
	var mu sync.Mutex
	var got []flvTag
	var at []time.Time
	p := newFLVPacer(func(tg flvTag) {
		mu.Lock()
		got = append(got, tg)
		at = append(at, time.Now())
		mu.Unlock()
	})
	p.push(flvTag{raw: []byte{18}, script: true})
	p.push(flvTag{raw: []byte{9}, video: true, seq: true})
	const fps, burst = 50, 25 // 20 ms 一帧，500 ms 一阵
	for b := 0; b < 5; b++ {
		for i := 0; i < burst; i++ {
			n := b*burst + i
			p.push(flvTag{raw: []byte{9}, video: true, keyframe: i == 0, ts: int32(n * 1000 / fps)})
		}
		time.Sleep(500 * time.Millisecond)
	}
	p.close()
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2+5*burst {
		t.Fatalf("放出 %d 个 tag，应为 %d", len(got), 2+5*burst)
	}
	if !got[0].script || !got[1].seq {
		t.Fatal("metadata、序列头要立即按原顺序放出")
	}
	for i := 3; i < len(got); i++ {
		if got[i].ts < got[i-1].ts {
			t.Fatalf("顺序变了: %d < %d", got[i].ts, got[i-1].ts)
		}
	}
	// 前 4 阵（close 之前）：相邻两帧的间隔不超过 60 毫秒（不限速时每阵之间是 500 毫秒的空档）。
	var maxGap time.Duration
	for i := 3; i < 2+4*burst; i++ {
		maxGap = max(maxGap, at[i].Sub(at[i-1]))
	}
	if maxGap > 60*time.Millisecond {
		t.Fatalf("应匀速放出，最大间隔 %v", maxGap)
	}
}

func TestFLVPacerTimestampJumps(t *testing.T) {
	out := make(chan flvTag, 16)
	p := newFLVPacer(func(tg flvTag) { out <- tg })
	defer p.close()
	recv := func(want int32) {
		t.Helper()
		select {
		case tg := <-out:
			if tg.ts != want {
				t.Fatalf("ts %d，应为 %d", tg.ts, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("ts %d 被扣住了", want)
		}
	}
	p.push(flvTag{raw: []byte{9}, video: true, ts: 100000})
	recv(100000)
	p.push(flvTag{raw: []byte{9}, video: true, ts: 500}) // 倒退：重新定锚，立即放出
	recv(500)
	p.push(flvTag{raw: []byte{9}, video: true, ts: 60000}) // 往前跳一分钟：不扣一分钟
	recv(60000)
}
