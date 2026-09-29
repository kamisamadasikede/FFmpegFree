package ffmpeg

import (
	"strconv"
	"strings"
)

// ProgressUpdate 是 `-progress pipe:1` 输出的一个完整块（以 progress=continue|end 结尾）。
type ProgressUpdate struct {
	OutTimeSec  float64 // 已输出的媒体时长（秒）；ffmpeg 还没给出（N/A）时为 0
	Speed       string  // 如 "2.3x"，N/A 时为空
	SpeedX      float64 // 数值形式，未知为 0
	Fps         float64
	BitrateKbps float64 // 未知为 0
	Frame       int64
	Dropped     int64
	TotalSize   int64
	End         bool // progress=end
}

// ProgressParser 逐行解析 `-progress pipe:1` 的 key=value 输出。
// 使用方式：对每一行调用 Feed，返回 ok=true 时表示凑齐了一个块。
// 该输出的字段顺序不固定，未知的 key 一律忽略；值为 N/A 的字段保持零值。
type ProgressParser struct {
	cur ProgressUpdate
}

// Feed 处理一行输入。
func (p *ProgressParser) Feed(line string) (ProgressUpdate, bool) {
	line = strings.TrimSpace(line)
	key, val, found := strings.Cut(line, "=")
	if !found {
		return ProgressUpdate{}, false
	}
	key, val = strings.TrimSpace(key), strings.TrimSpace(val)
	switch key {
	case "out_time_us", "out_time_ms":
		// out_time_ms 在 ffmpeg 里实际也是微秒（历史遗留），两个 key 同样处理；
		// out_time_us 优先，出现后不再被 out_time_ms 覆盖。
		if n, ok := parseInt(val); ok {
			if key == "out_time_us" || p.cur.OutTimeSec == 0 {
				p.cur.OutTimeSec = float64(n) / 1e6
			}
		}
	case "out_time":
		if p.cur.OutTimeSec == 0 {
			if s, ok := parseClock(val); ok {
				p.cur.OutTimeSec = s
			}
		}
	case "speed":
		if val != "" && val != "N/A" {
			p.cur.Speed = val
			if f, err := strconv.ParseFloat(strings.TrimSuffix(val, "x"), 64); err == nil {
				p.cur.SpeedX = f
			}
		}
	case "fps":
		p.cur.Fps, _ = strconv.ParseFloat(val, 64)
	case "bitrate":
		if val != "N/A" {
			p.cur.BitrateKbps, _ = strconv.ParseFloat(strings.TrimSuffix(val, "kbits/s"), 64)
		}
	case "frame":
		p.cur.Frame, _ = strconv.ParseInt(val, 10, 64)
	case "drop_frames":
		p.cur.Dropped, _ = strconv.ParseInt(val, 10, 64)
	case "total_size":
		p.cur.TotalSize, _ = strconv.ParseInt(val, 10, 64)
	case "progress":
		u := p.cur
		u.End = val == "end"
		p.cur = ProgressUpdate{}
		return u, true
	}
	return ProgressUpdate{}, false
}

func parseInt(s string) (int64, bool) {
	if s == "" || s == "N/A" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// parseClock 解析 "HH:MM:SS.micro"。
func parseClock(s string) (float64, bool) {
	if s == "" || s == "N/A" || strings.HasPrefix(s, "-") {
		return 0, false
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, e1 := strconv.ParseFloat(parts[0], 64)
	m, e2 := strconv.ParseFloat(parts[1], 64)
	sec, e3 := strconv.ParseFloat(parts[2], 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return 0, false
	}
	return h*3600 + m*60 + sec, true
}

// TailBuffer 保存最近 N 行文本（环形），用于把 ffmpeg stderr 的末尾放进错误 detail。
type TailBuffer struct {
	max   int
	lines []string
	start int
	n     int
}

// NewTailBuffer 创建容量为 max 行的缓冲，max<=0 时取 50（契约：detail 带最后 50 行日志）。
func NewTailBuffer(max int) *TailBuffer {
	if max <= 0 {
		max = 50
	}
	return &TailBuffer{max: max, lines: make([]string, max)}
}

// Add 追加一行（忽略空行）。
func (t *TailBuffer) Add(line string) {
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return
	}
	if t.n < t.max {
		t.lines[(t.start+t.n)%t.max] = line
		t.n++
		return
	}
	t.lines[t.start] = line
	t.start = (t.start + 1) % t.max
}

// Lines 按时间顺序返回缓冲内容。
func (t *TailBuffer) Lines() []string {
	out := make([]string, 0, t.n)
	for i := 0; i < t.n; i++ {
		out = append(out, t.lines[(t.start+i)%t.max])
	}
	return out
}

// String 用换行连接 Lines。
func (t *TailBuffer) String() string { return strings.Join(t.Lines(), "\n") }
