package ffmpeg

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

const pvPath = "/data/tmp/live-preview/S1.jpg"

var wantPreviewTail = []string{
	"-map", "0:v:0", "-an", "-sn", "-dn", "-vf", "fps=2,scale=640:-2", "-c:v", "mjpeg", "-q:v", "5",
	"-protocol_whitelist", "file", "-f", "fifo", "-fifo_format", "image2", "-format_opts", "update=1:atomic_writing=1",
	"-queue_size", "4", "-drop_pkts_on_overflow", "1",
	"-attempt_recovery", "1", "-recover_any_error", "1", "-recovery_wait_time", "1", "-max_recovery_attempts", "0",
	"file:" + pvPath,
}

func TestPreviewOutputArgs(t *testing.T) {
	got := PreviewOutputArgs(pvPath)
	if strings.Join(got, "\x00") != strings.Join(wantPreviewTail, "\x00") {
		t.Fatalf("预览输出参数不对:\n got %v\nwant %v", got, wantPreviewTail)
	}
}

// endsWithPreview 断言 argv 以预览输出结尾，且主输出部分（前缀）与不带预览时完全相同。
func endsWithPreview(t *testing.T, name string, with, without []string) {
	t.Helper()
	n := len(wantPreviewTail)
	if len(with) != len(without)+n {
		t.Fatalf("%s: 带预览应恰好多出预览输出 %d 个参数: %d vs %d", name, n, len(with), len(without))
	}
	if strings.Join(with[:len(without)], "\x00") != strings.Join(without, "\x00") {
		t.Errorf("%s: 预览不能改变主输出参数:\n%v\n%v", name, with[:len(without)], without)
	}
	if strings.Join(with[len(without):], "\x00") != strings.Join(wantPreviewTail, "\x00") {
		t.Errorf("%s: 尾部不是预览输出: %v", name, with[len(without):])
	}
}

func TestPreviewArgsAppendedForAllPushKinds(t *testing.T) {
	filePlan := func(audio bool, pv string) []string {
		return BuildFilePushArgs(FilePushPlan{Input: "/a.mp4", HasAudio: audio, Scheme: "rtmp", URL: "rtmp://h/app/k", Enc: enc(), PreviewPath: pv})
	}
	screenPlan := func(tee, pv string) []string {
		return BuildScreenPushArgs(ScreenPushPlan{GOOS: "linux", Display: ":99", Scheme: "rtmp", URL: "rtmp://h/app/k", FPS: 30, Enc: enc(), Silent: true, ArchiveTee: tee, PreviewPath: pv})
	}
	tests := []struct {
		name          string
		with, without []string
	}{
		{"文件推流（有音轨）", filePlan(true, pvPath), filePlan(true, "")},
		{"文件推流（无音轨补静音）", filePlan(false, pvPath), filePlan(false, "")},
		{"屏幕推流", screenPlan("", pvPath), screenPlan("", "")},
		{"屏幕推流带 tee 存档", screenPlan("file:/arc/a.mp4", pvPath), screenPlan("file:/arc/a.mp4", "")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			endsWithPreview(t, tc.name, tc.with, tc.without)
			line := strings.Join(tc.with, " ")
			if strings.Count(line, "-fifo_format image2") != 1 {
				t.Fatalf("应恰好一个预览输出: %s", line)
			}
		})
	}
}

// tee 存档：预览是独立输出，不能出现在 tee 描述里；tee 描述必须仍在预览之前、只含网络与存档两路。
func TestPreviewIsOutsideTee(t *testing.T) {
	a := BuildScreenPushArgs(ScreenPushPlan{GOOS: "linux", Display: ":99", Scheme: "rtmp", URL: "rtmp://h/app/k", FPS: 30, Enc: enc(), ArchiveTee: "file:/arc/a.mp4", PreviewPath: pvPath})
	teeAt := idx(a, "tee", 0)
	if teeAt < 0 || a[teeAt-1] != "-f" {
		t.Fatalf("缺少 -f tee: %v", a)
	}
	desc := a[teeAt+1]
	if strings.Contains(desc, "image2") || strings.Contains(desc, "live-preview") || strings.Count(desc, "|") != 1 {
		t.Fatalf("tee 描述里不能有预览: %s", desc)
	}
	if idx(a, "-f", teeAt+2) < 0 || idx(a, "image2", teeAt+2) < 0 {
		t.Fatalf("预览输出必须在 tee 之后: %v", a)
	}
	// 主输出是 tee（重编码一次），预览输出有自己的 -vf（不复用主输出的滤镜链）。
	if strings.Count(strings.Join(a, " "), "-vf") != 2 {
		t.Fatalf("主输出与预览输出各有一个 -vf: %v", a)
	}
	// 预览输出之前的最后一个 -map 是 0:v:0，且预览输出不带音频。
	pv := a[teeAt+2:]
	// v0.24.2：fifo 封装没有默认编码器，预览这一路必须写 -c:v mjpeg。
	if !has(pv, "-an") || has(pv, "-c:a") || pv[idx(pv, "-c:v", 0)+1] != "mjpeg" {
		t.Fatalf("预览输出不带音频、编码器是 mjpeg: %v", pv)
	}
}

func TestBuildPullPreviewArgs(t *testing.T) {
	tests := []struct {
		name string
		plan PullPreviewPlan
		ok   bool
	}{
		{"rtmp 拉流", PullPreviewPlan{URL: "rtmp://h/app/k", InputWhitelist: PullInputWhitelist("rtmp"), HasVideo: true, PreviewPath: pvPath}, true},
		{"srt 拉流", PullPreviewPlan{URL: "srt://h:9000?streamid=x", InputWhitelist: PullInputWhitelist("srt"), HasVideo: true, PreviewPath: pvPath}, true},
		{"http-flv 拉流", PullPreviewPlan{URL: "http://h/a.flv", InputWhitelist: PullInputWhitelist("http"), HasVideo: true, PreviewPath: pvPath}, true},
		{"纯音频没有预览", PullPreviewPlan{URL: "rtmp://h/app/k", InputWhitelist: "rtmp,tcp", HasVideo: false, PreviewPath: pvPath}, false},
		{"preview=false（没有预览路径）", PullPreviewPlan{URL: "rtmp://h/app/k", InputWhitelist: "rtmp,tcp", HasVideo: true}, false},
		{"缺白名单", PullPreviewPlan{URL: "rtmp://h/app/k", HasVideo: true, PreviewPath: pvPath}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, ok := BuildPullPreviewArgs(tc.plan)
			if ok != tc.ok {
				t.Fatalf("ok=%v want %v: %v", ok, tc.ok, a)
			}
			if !ok {
				if a != nil {
					t.Fatalf("不出预览时应返回 nil: %v", a)
				}
				return
			}
			in := "-protocol_whitelist " + tc.plan.InputWhitelist + " -fflags +nobuffer -analyzeduration 1000000 -probesize 1000000 -i " + tc.plan.URL
			if !strings.HasPrefix(strings.Join(a, " "), in+" ") {
				t.Fatalf("输入侧白名单必须在 -i 之前、探测缩短: %v", a)
			}
			if strings.Join(a[idx(a, "-i", 0)+2:], "\x00") != strings.Join(wantPreviewTail, "\x00") {
				t.Fatalf("拉流预览只有预览一路输出: %v", a)
			}
			if has(a, "copy") || has(a, "-f") && idx(a, "flv", 0) >= 0 {
				t.Fatalf("拉流预览不能有其他输出: %v", a)
			}
		})
	}
}

func TestPullInputWhitelist(t *testing.T) {
	for scheme, want := range map[string]string{"rtmp": "rtmp,tcp", "rtmps": "rtmps,tcp,tls,crypto", "srt": "srt,udp", "http": "http,https,tcp,tls,crypto", "https": "http,https,tcp,tls,crypto"} {
		if got := PullInputWhitelist(scheme); got != want {
			t.Errorf("%s: %s want %s", scheme, got, want)
		}
	}
}

func TestPreviewProbe(t *testing.T) {
	full := map[string]string{
		"-encoders": " V....D mjpeg               Motion JPEG\n V....D libx264\n",
		"-muxers":   "  E fifo            FIFO queue pseudo-muxer\n  E image2          image2 sequence\n",
		"-filters":  " ... fps               V->V       Force constant framerate.\n ... scale             V->V       Scale the input video\n",
	}
	calls := 0
	mk := func(m map[string]string, fail bool) func(context.Context, string, ...string) (string, error) {
		return func(_ context.Context, _ string, args ...string) (string, error) {
			calls++
			if fail {
				return "", errors.New("boom")
			}
			return m[args[len(args)-1]], nil
		}
	}
	pp := &PreviewProbe{Run: mk(full, false)}
	if !pp.Supported(context.Background(), "/x/ffmpeg") {
		t.Fatal("完整构建应支持预览")
	}
	n := calls
	if !pp.Supported(context.Background(), "/x/ffmpeg") || calls != n {
		t.Fatal("结果应按路径缓存")
	}
	for _, drop := range []string{"-encoders", "-muxers", "-filters"} {
		m := map[string]string{}
		for k, v := range full {
			m[k] = v
		}
		m[drop] = "\n"
		if (&PreviewProbe{Run: mk(m, false)}).Supported(context.Background(), "/y") {
			t.Errorf("缺 %s 应判不支持", drop)
		}
	}
	// v0.24.2：有 image2 但没有 fifo 封装（预览会反压推流）：不出预览。
	noFifo := map[string]string{}
	for k, v := range full {
		noFifo[k] = v
	}
	noFifo["-muxers"] = "  E image2          image2 sequence\n"
	if (&PreviewProbe{Run: mk(noFifo, false)}).Supported(context.Background(), "/z") {
		t.Error("缺 fifo 封装应判不支持")
	}
	// 探测命令失败：按不支持处理，且不缓存。
	bad := &PreviewProbe{Run: mk(full, true)}
	if bad.Supported(context.Background(), "/z") {
		t.Fatal("探测失败应判不支持")
	}
	bad.Run = mk(full, false)
	if !bad.Supported(context.Background(), "/z") {
		t.Fatal("失败结果不应缓存")
	}
	// 整词匹配：mjpeg_qsv 不算 mjpeg。
	if listsName(" V....D mjpeg_qsv  x\n", "mjpeg") {
		t.Fatal("应整词匹配")
	}
}

// v0.24.2（老板：按源帧率推流）：主输出不带任何程序加的帧率 / 尺寸限制；fps=2 只在预览这一路；GOP 跟着源帧率走。
func TestMainOutputKeepsSourceFpsAndSize(t *testing.T) {
	mainOf := func(a []string) []string {
		n := len(PreviewOutputArgs(pvPath))
		return a[:len(a)-n]
	}
	for _, src := range []float64{24, 25, 30, 50, 59.94, 60} {
		e := LiveEncode{GOPFps: src, VideoKbps: 2500, AudioKbps: 128} // Fps=0：沿用源帧率
		a := BuildFilePushArgs(FilePushPlan{Input: "/a.mp4", HasAudio: true, Scheme: "rtmp", URL: "rtmp://h/app/k", Enc: e, PreviewPath: pvPath})
		m := strings.Join(mainOf(a), " ")
		for _, bad := range []string{"fps=", " -r ", "-framerate", "-fps_mode", "-vsync", " -s ", "scale=640", "-update"} {
			if strings.Contains(m, bad) {
				t.Errorf("src=%v 主输出不能有 %q: %s", src, bad, m)
			}
		}
		if mainOf(a)[idx(mainOf(a), "-vf", 0)+1] != "scale=trunc(iw/2)*2:trunc(ih/2)*2" {
			t.Errorf("src=%v 不设分辨率时主输出只补偶数，保持源尺寸: %s", src, m)
		}
		wantG := strconv.Itoa(int(src*2 + 0.5))
		if g := mainOf(a)[idx(mainOf(a), "-g", 0)+1]; g != wantG {
			t.Errorf("src=%v GOP 应为 2×源帧率 %s，实际 %s", src, wantG, g)
		}
		pv := strings.Join(a[len(mainOf(a)):], " ")
		if !strings.Contains(pv, "-vf fps=2,scale=640:-2") {
			t.Errorf("预览这一路应是 fps=2: %s", pv)
		}
		if strings.Count(strings.Join(a, " "), "fps=") != 1 {
			t.Errorf("fps 滤镜只能出现在预览这一路: %v", a)
		}
	}
	// 用户明确给了帧率：只作用在主输出，预览仍是 2 fps。
	a := BuildFilePushArgs(FilePushPlan{Input: "/a.mp4", HasAudio: true, Scheme: "rtmp", URL: "rtmp://h/app/k",
		Enc: LiveEncode{Fps: 60, GOPFps: 60, VideoKbps: 2500, AudioKbps: 128}, PreviewPath: pvPath})
	m := mainOf(a)
	if m[idx(m, "-vf", 0)+1] != "fps=60,scale=trunc(iw/2)*2:trunc(ih/2)*2" || m[idx(m, "-g", 0)+1] != "120" {
		t.Errorf("明确帧率应只加在主输出: %v", m)
	}
	if !strings.Contains(strings.Join(a[len(m):], " "), "-vf fps=2,") {
		t.Errorf("预览仍是 2 fps: %v", a[len(m):])
	}
	// 屏幕推流：采集端 -framerate 决定帧率，主输出没有 fps 滤镜，GOP = 2×采集帧率。
	for _, goos := range []string{"windows", "darwin", "linux"} {
		a := BuildScreenPushArgs(ScreenPushPlan{GOOS: goos, Display: ":0", Scheme: "rtmp", URL: "rtmp://h/app/k", FPS: 60, PreviewPath: pvPath,
			Region: ScreenRegion{Desktop: true}, Enc: LiveEncode{GOPFps: 60, VideoKbps: 2500, AudioKbps: 128}})
		m := mainOf(a)
		ms := strings.Join(m, " ")
		if m[idx(m, "-framerate", 0)+1] != "60" || strings.Contains(ms, "fps=") || strings.Contains(ms, " -r ") || m[idx(m, "-g", 0)+1] != "120" {
			t.Errorf("%s 屏幕推流主输出: %s", goos, ms)
		}
	}
}
