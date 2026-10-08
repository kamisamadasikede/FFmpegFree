package ffmpeg

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 直播预览（契约 6.10「预览画面」）：在主输出之外再加一路独立的输出，把同一路视频缩成 640 宽、每秒 2 帧的 JPEG，
// 用 -update 1 反复覆盖同一个文件。它是一个单独的输出（不放进 tee），有自己的 -vf，不影响主输出的帧率、分辨率、编码参数和码率统计。
//
// v0.24.5：预览输出包在 fifo 封装里（`-f fifo -fifo_format image2`），由 fifo 自己的线程写文件：
//   - 写预览变慢或卡住时（Windows 上杀毒扫描、读取端占着文件等），队列满了直接丢预览帧（drop_pkts_on_overflow），
//     不会反压到共用的解码器，主输出（推流）始终按源帧率走；
//   - 写预览失败时（Windows 上读取端打开着 <path>，改名会被拒绝）只记一行日志，1 秒后重试（attempt_recovery），
//     不会让整个 ffmpeg 以“Error muxing a packet”退出、把推流一起带走。
// 实测（ffmpeg 7.1，30 fps 源）：不包 fifo 时预览一卡住，主输出约 12 秒后完全停住；改名失败时整个推流立刻退出。包了 fifo 后两种情况主输出都保持 30 fps。

const (
	// PreviewFps 是预览帧率（只作用在预览这一路）。
	PreviewFps = 2
	// PreviewWidth 是预览宽度（高度按比例，-2 保证偶数）。
	PreviewWidth = 640
	// PreviewQuality 是 mjpeg 的 -q:v。
	PreviewQuality = 5
	// previewQueue 是 fifo 封装的队列长度（包数）；2 fps 下约 2 秒，满了丢预览帧。
	previewQueue = 4
)

// PreviewOutputArgs 返回预览输出的参数（放在主输出之后，input 序号 0 是主视频输入）。path 是预览 JPEG 的绝对路径。
// 顺序：-map 0:v:0 -an -sn -dn -vf fps=2,scale=640:-2 -c:v mjpeg -q:v 5 -protocol_whitelist file
// -f fifo -fifo_format image2 -format_opts update=1:atomic_writing=1 -queue_size 4 -drop_pkts_on_overflow 1
// -attempt_recovery 1 -recover_any_error 1 -recovery_wait_time 1 -max_recovery_attempts 0 file:<path>。
// atomic_writing 让 image2 先写 <path>.tmp 再改名，读取端不会读到半帧（读取端仍会校验 JPEG 首尾）。
// fifo 封装没有默认编码器，所以必须写 -c:v mjpeg。fps 滤镜只在这一路的 -vf 里，绝不出现在主输出上。
func PreviewOutputArgs(path string) []string {
	return []string{
		"-map", "0:v:0", "-an", "-sn", "-dn",
		"-vf", "fps=" + strconv.Itoa(PreviewFps) + ",scale=" + strconv.Itoa(PreviewWidth) + ":-2",
		"-c:v", "mjpeg", "-q:v", strconv.Itoa(PreviewQuality),
		"-protocol_whitelist", "file",
		"-f", "fifo", "-fifo_format", "image2", "-format_opts", "update=1:atomic_writing=1",
		"-queue_size", strconv.Itoa(previewQueue), "-drop_pkts_on_overflow", "1",
		"-attempt_recovery", "1", "-recover_any_error", "1", "-recovery_wait_time", "1", "-max_recovery_attempts", "0",
		"file:" + path,
	}
}

// PullPreviewPlan 描述一次"只出预览"的拉流会话：读远端流，只输出预览 JPEG（没有其他输出）。
type PullPreviewPlan struct {
	URL string // 已校验的拉流地址
	// InputWhitelist 是输入侧允许的协议（如 "http,https,tcp,tls,crypto"、"rtmp,tcp"、"srt,udp"），由调用方按地址协议决定。
	InputWhitelist string
	HasVideo       bool // 探测到流里有视频；纯音频拉流没有预览
	PreviewPath    string
}

// BuildPullPreviewArgs 生成拉流预览的参数（不含 -progress 等，由 Run 添加）。纯音频（HasVideo=false）或缺路径 / 白名单时返回 ok=false：没有预览可出。
func BuildPullPreviewArgs(p PullPreviewPlan) (args []string, ok bool) {
	if !p.HasVideo || p.PreviewPath == "" || p.InputWhitelist == "" || p.URL == "" {
		return nil, false
	}
	// 预览只要尽快出第一帧：缩短输入探测（默认 analyzeduration 5 秒、probesize 5 MB，会让第一帧晚 4~5 秒）。
	a := []string{"-protocol_whitelist", p.InputWhitelist, "-fflags", "+nobuffer", "-analyzeduration", "1000000", "-probesize", "1000000", "-i", p.URL}
	return append(a, PreviewOutputArgs(p.PreviewPath)...), true
}

// PreviewProbe 检查 ffmpeg 能不能出预览：需要 mjpeg 编码器、image2 和 fifo 封装（v0.24.5）、fps 和 scale 滤镜。结果按 ffmpeg 路径缓存（成功的才缓存）。
// 不支持（精简构建）时调用方降级为不加预览输出，不报错。
type PreviewProbe struct {
	Run func(ctx context.Context, exe string, args ...string) (string, error) // 默认 ExecRunner(10s)
	mu  sync.Mutex
	m   map[string]bool
}

// Supported 返回 exe 是否支持预览输出。探测命令失败按不支持处理（不缓存）。
func (pp *PreviewProbe) Supported(ctx context.Context, exe string) bool {
	pp.mu.Lock()
	if v, ok := pp.m[exe]; ok {
		pp.mu.Unlock()
		return v
	}
	pp.mu.Unlock()
	run := pp.Run
	if run == nil {
		run = ExecRunner(10 * time.Second)
	}
	need := []struct{ flag, name string }{{"-encoders", "mjpeg"}, {"-muxers", "image2"}, {"-muxers", "fifo"}, {"-filters", "fps"}, {"-filters", "scale"}}
	outs := map[string]string{}
	ok := true
	for _, n := range need {
		out, seen := outs[n.flag]
		if !seen {
			var err error
			if out, err = run(ctx, exe, "-hide_banner", n.flag); err != nil {
				return false
			}
			outs[n.flag] = out
		}
		if !listsName(out, n.name) {
			ok = false
		}
	}
	pp.mu.Lock()
	if pp.m == nil {
		pp.m = map[string]bool{}
	}
	pp.m[exe] = ok
	pp.mu.Unlock()
	return ok
}

// listsName 判断 -encoders / -muxers / -filters 的输出里有没有名字为 name 的条目（第二列，整词匹配）。
func listsName(out, name string) bool {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == name {
			return true
		}
	}
	return false
}

// PullInputWhitelist 返回拉流输入侧允许的协议（按地址 scheme；-protocol_whitelist 写在 -i 之前，只管这个输入）。
// http / https 同时放行两者和 tls / crypto，因为 HLS 之类的清单会跳转到子地址。
func PullInputWhitelist(scheme string) string {
	switch scheme {
	case "rtmps":
		return "rtmps,tcp,tls,crypto"
	case "srt":
		return "srt,udp"
	case "http", "https":
		return "http,https,tcp,tls,crypto"
	}
	return "rtmp,tcp"
}
