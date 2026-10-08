package ffmpeg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
)

func liveSample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "live", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func firstLine(s string) string { l, _, _ := strings.Cut(s, "\n"); return l }

// 真实样本（ffmpeg 7.1.5 + MediaMTX 1.21.1）的分类。
func TestClassifyLiveErrorRealSamples(t *testing.T) {
	tests := []struct {
		file, scheme string
		started      bool
		code         apperr.Code
		first        string // detail 第一行；空 = 不断言
	}{
		{"off_rtmp.err", "rtmp", false, apperr.LiveConnectFailed, "scheme=rtmp"},
		{"off_rtmps.err", "rtmps", false, apperr.LiveConnectFailed, "scheme=rtmps"},
		{"off_srt.err", "srt", false, apperr.LiveConnectFailed, "scheme=srt"},
		{"dns.err", "rtmp", false, apperr.LiveConnectFailed, "scheme=rtmp"},
		{"rej_rtmp.err", "rtmp", false, apperr.LivePushRejected, ""},
		{"rej_srt.err", "srt", false, apperr.LiveConnectFailed, "scheme=srt"}, // 已知局限：SRT 被拒与服务器未开无法区分
		{"mid_rtmp.err", "rtmp", true, apperr.LivePushInterrupted, ""},
		{"mid_srt.err", "srt", true, apperr.LivePushInterrupted, ""},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			e := ClassifyLiveError(LiveClassifyInput{Tail: liveSample(t, tc.file), Scheme: tc.scheme, Started: tc.started})
			if e == nil || e.Code != tc.code {
				t.Fatalf("got %+v want %s", e, tc.code)
			}
			if tc.first != "" && firstLine(e.Detail) != tc.first {
				t.Fatalf("detail 首行=%q want %q", firstLine(e.Detail), tc.first)
			}
		})
	}
}

func TestClassifyLiveErrorStartedDecidesConnectVsInterrupted(t *testing.T) {
	e := ClassifyLiveError(LiveClassifyInput{Tail: liveSample(t, "off_rtmp.err"), Scheme: "rtmp", Started: true})
	if e.Code == apperr.LiveConnectFailed || e.Code == apperr.LivePushRejected {
		t.Fatalf("已开始不能是连接阶段的码: %+v", e)
	}
	e = ClassifyLiveError(LiveClassifyInput{Tail: liveSample(t, "mid_rtmp.err"), Scheme: "rtmp", Started: false})
	if e.Code == apperr.LivePushInterrupted {
		t.Fatalf("未开始不能是 INTERRUPTED: %+v", e)
	}
}

func TestClassifyLiveErrorIgnoresMetadataBlocks(t *testing.T) {
	tail := "Input #0, mov,mp4, from '/tmp/Connection refused.mp4':\n  Metadata:\n    title           : Broken pipe\n  Stream #0:0: Video: h264\nSomething odd happened\n"
	// 未开始：认不出 → INTERNAL；已开始：认不出 → LIVE_PUSH_INTERRUPTED + PushInterruptedMessage（v0.25.3），都不是元数据里的词触发的码。
	e := ClassifyLiveError(LiveClassifyInput{Tail: tail, Scheme: "rtmp", Started: false})
	if e.Code != apperr.Internal {
		t.Fatalf("未开始: 元数据里的词不应触发分类: %+v", e)
	}
	e = ClassifyLiveError(LiveClassifyInput{Tail: tail, Scheme: "rtmp", Started: true})
	if e.Code != apperr.LivePushInterrupted || e.Message != PushInterruptedMessage {
		t.Fatalf("已开始: 元数据里的词不应触发分类，认不出的是兜底的中断: %+v", e)
	}
}

// 契约 v0.25.3：推流开始以后进程被杀（stderr 什么都没有）是 LIVE_PUSH_INTERRUPTED，不是 INTERNAL。
func TestClassifyLiveErrorStartedKillIsInterrupted(t *testing.T) {
	for _, tail := range []string{"", "frame=  100 fps= 25 q=23.0 size=     512kB time=00:00:04.00\n", "Killed"} {
		e := ClassifyLiveError(LiveClassifyInput{Tail: tail, Scheme: "rtmp", Started: true})
		if e.Code != apperr.LivePushInterrupted || e.Message != PushInterruptedMessage {
			t.Fatalf("%q: %+v", tail, e)
		}
	}
}

func TestClassifyLiveErrorUnknownIsInternalNotProcessFailed(t *testing.T) {
	e := ClassifyLiveError(LiveClassifyInput{Tail: "some weird failure\nConversion failed!", Scheme: "srt"})
	if e == nil || e.Code != apperr.Internal || !strings.Contains(e.Detail, "some weird failure") {
		t.Fatalf("%+v", e)
	}
}

func TestClassifyLiveErrorTeeSlaveLines(t *testing.T) {
	tail := "[tcp @ 0x7f4e] Connection to tcp://127.0.0.1:1999?tcp_nodelay=0 failed: Connection refused\n" +
		"[tee @ 0x55] Slave '[f=flv:onfail=abort]rtmp://127.0.0.1:1999/live/k': error opening: Connection refused\n" +
		"[tee @ 0x55] Slave muxer #0 failed, aborting.\n[out#0/tee @ 0x55] Could not write header (incorrect codec parameters ?): Connection refused\n"
	e := ClassifyLiveError(LiveClassifyInput{Tail: tail, Scheme: "rtmp", Screen: true})
	if e.Code != apperr.LiveConnectFailed || firstLine(e.Detail) != "scheme=rtmp" {
		t.Fatalf("%+v", e)
	}
}

func TestClassifyLiveErrorScreen(t *testing.T) {
	tests := []struct {
		tail string
		code apperr.Code
	}{
		{"[avfoundation @ 0x1] Failed to create AV capture input device: Cannot use FaceTime\nscreen recording permission not authorized", apperr.ScreenPermissionDenied},
		{"[x11grab @ 0x55] Cannot open display :0, error 1.\n:0+0,0: Input/output error", apperr.UnsupportedPlatform},
		{"[x11grab @ 0x55] Cannot open display :99, error 1.", apperr.UnsupportedPlatform},
	}
	for _, tc := range tests {
		e := ClassifyLiveError(LiveClassifyInput{Tail: tc.tail, Scheme: "rtmp", Screen: true})
		if e.Code != tc.code {
			t.Errorf("%q → %s want %s", tc.tail, e.Code, tc.code)
		}
	}
	if e := ClassifyLiveError(LiveClassifyInput{Tail: "[x11grab @ 0x55] Cannot open display :0", Scheme: "rtmp"}); e.Code == apperr.UnsupportedPlatform {
		t.Fatalf("文件推流不做采集分类: %+v", e)
	}
}

// 脱敏之后的真实样本：任务日志和 Classify 看到的都是 *** 形式（ffmpeg 7.1.5 + MediaMTX 1.21.1，集成测试里抓取，源路径与口令已替换）。
// 覆盖鉴权失败、推流中途服务器被杀（Broken pipe，进程退出码 224，退出码由 ffmpeg.Run 的测试覆盖）、SRT 连接被拒。
func TestClassifyLiveErrorRedactedRealSamples(t *testing.T) {
	tests := []struct {
		file, scheme string
		started      bool
		code         apperr.Code
		first        string
	}{
		{"rej_rtmp_redacted.err", "rtmp", false, apperr.LivePushRejected, ""},
		{"mid_rtmp_redacted.err", "rtmp", true, apperr.LivePushInterrupted, ""},
		{"off_srt_redacted.err", "srt", false, apperr.LiveConnectFailed, "scheme=srt"},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			sample := liveSample(t, tc.file)
			if !strings.Contains(sample, "***") && tc.file != "mid_rtmp_redacted.err" {
				t.Fatal("样本应已脱敏")
			}
			for _, leak := range []string{"secret", "wrong", "WRONG", "passphrase"} {
				if strings.Contains(sample, leak) {
					t.Fatalf("样本含敏感词 %q", leak)
				}
			}
			e := ClassifyLiveError(LiveClassifyInput{Tail: sample, Scheme: tc.scheme, Started: tc.started})
			if e == nil || e.Code != tc.code {
				t.Fatalf("got %+v want %s", e, tc.code)
			}
			if tc.first != "" && firstLine(e.Detail) != tc.first {
				t.Fatalf("detail 首行=%q want %q", firstLine(e.Detail), tc.first)
			}
			if strings.Contains(e.Detail, "127.0.0.1:1") && strings.Contains(e.Detail, "live/ok") {
				t.Fatalf("detail 不应带原始地址: %s", e.Detail)
			}
		})
	}
}

// gdigrab 校验之后窗口消失：LIVE_SOURCE_GONE，detail 只有 kind=window（不带标题）。
func TestClassifyLiveErrorWindowGone(t *testing.T) {
	tail := "[gdigrab @ 000001] Can't find window '机密标题', aborting.\n[in#0 @ 000002] Error opening input: Input/output error\nError opening input file title=机密标题.\n"
	e := ClassifyLiveError(LiveClassifyInput{Tail: tail, Scheme: "rtmp", Screen: true})
	if e.Code != apperr.LiveSourceGone || e.Detail != "kind=window" || strings.Contains(e.Message+e.Detail, "机密") {
		t.Fatalf("%+v", e)
	}
	// 文件推流不走这条
	if e := ClassifyLiveError(LiveClassifyInput{Tail: tail, Scheme: "rtmp"}); e.Code == apperr.LiveSourceGone {
		t.Fatal("非屏幕推流不应识别")
	}
}

// 设计走查 G3：拉流失败有自己的分类和文字，不出现推流的文字。
func TestClassifyPullError(t *testing.T) {
	if PullFailedMessage != "拉流失败，请检查直播地址和网络。" {
		t.Fatalf("文案: %q", PullFailedMessage)
	}
	cases := []struct {
		name, tail string
		code       apperr.Code
	}{
		{"拒绝连接", "[tcp @ 0x1] Connection to tcp://127.0.0.1:1935?tcp_nodelay=0 failed: Connection refused\n[in#0 @ 0x2] Error opening input: Connection refused\nError opening input file rtmp://127.0.0.1:1935/live/***.", apperr.LiveConnectFailed},
		{"HLS 404", "[in#0 @ 0x1] Error opening input: Server returned 404 Not Found\nError opening input file http://h/***.\nError opening input files: Server returned 404 Not Found", apperr.LiveConnectFailed},
		{"DNS", "[tcp @ 0x1] Failed to resolve hostname nope.invalid: Name or service not known", apperr.LiveConnectFailed},
		{"RTMP 服务器报错", "[rtmp @ 0x1] Server error: stream not found\n[in#0 @ 0x2] Error opening input: Operation not permitted", apperr.LiveConnectFailed},
		{"SRT 握手失败", "[srt @ 0x1] Connection to srt://127.0.0.1:9000?streamid=*** failed: Input/output error", apperr.LiveConnectFailed},
		{"读不出画面参数", "[flv @ 0x1] Could not find codec parameters for stream 0 (Video: h264, none): unspecified size\n[out#0/flv @ 0x2] Could not write header (incorrect codec parameters ?): Invalid argument", apperr.Internal},
	}
	for _, c := range cases {
		e := ClassifyPullError(LiveClassifyInput{Tail: c.tail, Scheme: "rtmp"})
		if e.Code != c.code || e.Message != PullFailedMessage {
			t.Errorf("%s: %s %q", c.name, e.Code, e.Message)
		}
		if strings.Contains(e.Message, "推流") || strings.Contains(strings.ToLower(e.Message), "ffmpeg") {
			t.Errorf("%s: 拉流文字不能出现推流或 ffmpeg: %q", c.name, e.Message)
		}
		if c.code == apperr.LiveConnectFailed && !strings.HasPrefix(e.Detail, "scheme=rtmp\n") {
			t.Errorf("%s: detail 首行应是 scheme=: %q", c.name, e.Detail)
		}
	}
	// 推流的文字不变。
	if e := ClassifyLiveError(LiveClassifyInput{Tail: "something odd"}); e.Message != "推流启动失败" {
		t.Errorf("推流文字被改了: %q", e.Message)
	}
	if e := ClassifyLiveError(LiveClassifyInput{Tail: "Connection refused"}); e.Message != "连接推流服务器失败" {
		t.Errorf("推流文字被改了: %q", e.Message)
	}
}
