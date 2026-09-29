package ffmpeg

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const pvPath = "/data/tmp/live-preview/S1.jpg"

var wantPreviewTail = []string{
	"-map", "0:v:0", "-an", "-sn", "-dn", "-vf", "fps=2,scale=640:-2", "-q:v", "5",
	"-protocol_whitelist", "file", "-f", "image2", "-update", "1", "-atomic_writing", "1", "file:" + pvPath,
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
			if strings.Count(line, "-f image2") != 1 {
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
	if !has(pv, "-an") || has(pv, "-c:a") || has(pv, "-c:v") {
		t.Fatalf("预览输出不带音频、不指定编码器: %v", pv)
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
		"-muxers":   "  E image2          image2 sequence\n",
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
