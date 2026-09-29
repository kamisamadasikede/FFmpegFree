package ffmpeg

import (
	"strings"
	"testing"
)

const sampleProgress = `frame=120
fps=29.97
stream_0_0_q=28.0
bitrate=1234.5kbits/s
total_size=2048000
out_time_us=4004000
out_time_ms=4004000
out_time=00:00:04.004000
dup_frames=0
drop_frames=2
speed=2.31x
progress=continue
frame=240
fps=30.00
bitrate=N/A
total_size=N/A
out_time_us=N/A
out_time=N/A
speed=N/A
progress=continue
frame=300
out_time_us=10000000
speed= 1.5x
progress=end
`

func feedAll(p *ProgressParser, s string) []ProgressUpdate {
	var out []ProgressUpdate
	for _, l := range strings.Split(s, "\n") {
		if u, ok := p.Feed(l); ok {
			out = append(out, u)
		}
	}
	return out
}

func TestProgressParser(t *testing.T) {
	var p ProgressParser
	ups := feedAll(&p, sampleProgress)
	if len(ups) != 3 {
		t.Fatalf("应解析出 3 个块: %d", len(ups))
	}
	a := ups[0]
	if a.OutTimeSec != 4.004 || a.Speed != "2.31x" || a.SpeedX != 2.31 || a.Fps != 29.97 || a.BitrateKbps != 1234.5 ||
		a.Frame != 120 || a.Dropped != 2 || a.TotalSize != 2048000 || a.End {
		t.Fatalf("%+v", a)
	}
	// N/A 字段保持零值，且不会带上上一块的数据
	b := ups[1]
	if b.OutTimeSec != 0 || b.Speed != "" || b.BitrateKbps != 0 || b.TotalSize != 0 || b.Frame != 240 || b.End {
		t.Fatalf("%+v", b)
	}
	c := ups[2]
	if !c.End || c.OutTimeSec != 10 || c.SpeedX != 1.5 {
		t.Fatalf("%+v", c)
	}
}

func TestProgressParserFallbacksAndNoise(t *testing.T) {
	var p ProgressParser
	// 只有 out_time_ms（微秒）
	ups := feedAll(&p, "out_time_ms=2500000\nprogress=continue\n")
	if len(ups) != 1 || ups[0].OutTimeSec != 2.5 {
		t.Fatalf("%+v", ups)
	}
	// 只有 out_time 时钟格式
	ups = feedAll(&p, "out_time=01:02:03.500000\nprogress=continue\n")
	if len(ups) != 1 || ups[0].OutTimeSec != 3723.5 {
		t.Fatalf("%+v", ups)
	}
	// 负值（ffmpeg 起始时间戳为负时会输出）忽略
	ups = feedAll(&p, "out_time_us=-500\nout_time=-00:00:00.000500\nprogress=continue\n")
	if len(ups) != 1 || ups[0].OutTimeSec != 0 {
		t.Fatalf("%+v", ups)
	}
	// 噪声行、未知 key、CRLF
	ups = feedAll(&p, "garbage line\r\nfoo=bar\r\nspeed=3x\r\nprogress=continue\r\n")
	if len(ups) != 1 || ups[0].SpeedX != 3 {
		t.Fatalf("%+v", ups)
	}
	// out_time_us 优先于 out_time_ms
	ups = feedAll(&p, "out_time_ms=1\nout_time_us=7000000\nprogress=continue\n")
	if ups[0].OutTimeSec != 7 {
		t.Fatalf("%+v", ups)
	}
}

func TestTailBuffer(t *testing.T) {
	tb := NewTailBuffer(3)
	for _, l := range []string{"a", "", "b\r\n", "c", "d", "e"} {
		tb.Add(l)
	}
	if got := tb.String(); got != "c\nd\ne" {
		t.Fatalf("%q", got)
	}
	if NewTailBuffer(0).max != 50 {
		t.Fatal("默认 50 行")
	}
	tb = NewTailBuffer(5)
	tb.Add("x")
	if got := tb.Lines(); len(got) != 1 || got[0] != "x" {
		t.Fatalf("%v", got)
	}
}

func TestLineWriter(t *testing.T) {
	var got []string
	w := newLineWriter(func(s string) { got = append(got, s) })
	w.Write([]byte("ab"))
	w.Write([]byte("c\nde\r"))
	w.Write([]byte("f\n\n\nlast"))
	if strings.Join(got, "|") != "abc|de|f" {
		t.Fatalf("%v", got)
	}
}
