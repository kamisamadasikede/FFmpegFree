package ffmpeg

import (
	"strconv"
	"strings"
	"testing"
)

func TestTeePreviewBranchIsolated(t *testing.T) {
	plain := BuildFilePushArgs(FilePushPlan{Input: "/a.mp4", HasAudio: true, Scheme: "rtmp", URL: "rtmp://h/app/k", Enc: enc()})
	with := BuildFilePushArgs(FilePushPlan{Input: "/a.mp4", HasAudio: true, Scheme: "rtmp", URL: "rtmp://h/app/k", Enc: enc(), PreviewPort: 23456})
	line := strings.Join(with, " ")
	if strings.Contains(line, "fps=") || strings.Contains(line, "image2") || strings.Contains(line, " -r ") {
		t.Fatalf("预览不能再是 2 fps 图片，也不能改主输出帧率: %s", line)
	}
	if !strings.Contains(line, "-f tee") || !strings.Contains(line, "onfail=ignore") || !strings.Contains(line, "drop_pkts_on_overflow=1") {
		t.Fatalf("预览应是 tee 的一路: %s", line)
	}
	if !strings.Contains(line, "tcp\\://127.0.0.1\\:23456\\?tcp_nodelay\\=1") {
		t.Fatalf("预览目标没转义: %s", line)
	}
	// 编码参数（-vf 到 -ac）不因预览改变。
	encOf := func(a []string) string {
		i := idx(a, "-vf", 0)
		j := idx(a, "-protocol_whitelist", i)
		if strings.Contains(strings.Join(a, " "), "-f tee") {
			j = idx(a, "-flags", i)
		}
		return strings.Join(a[i:j], " ")
	}
	if encOf(plain) != encOf(with) {
		t.Fatalf("预览不能改变编码参数:\n%s\n%s", encOf(plain), encOf(with))
	}
	if strings.Count(line, "onfail=abort") != 1 || strings.Count(line, "onfail=ignore") != 1 {
		t.Fatalf("网络一路 abort、预览一路 ignore: %s", line)
	}
}

func TestPreviewTeeWithArchiveAndSRT(t *testing.T) {
	a := BuildScreenPushArgs(ScreenPushPlan{GOOS: "linux", Display: ":99", Scheme: "rtmp", URL: "rtmp://h/app/k", FPS: 30, Enc: enc(), ArchiveTee: "file:/arc/a.mp4", PreviewPort: 9})
	line := strings.Join(a, " ")
	tee := a[idx(a, "tee", 0)+1]
	if strings.Count(tee, "|") != 2 || !strings.Contains(tee, "file:/arc/a.mp4") || !strings.Contains(tee, "onfail=ignore") {
		t.Fatalf("存档 + 预览应是三路: %s", tee)
	}
	if strings.Contains(line, "fps=2") || strings.Contains(line, "image2") {
		t.Fatalf("不能再有图片预览: %s", line)
	}
	s := TeeSlaves("srt", "srt://h:9000?streamid=x", "", "tcp://127.0.0.1:9?tcp_nodelay=1")
	if !strings.HasPrefix(s, "[f=mpegts:") || !strings.Contains(s, "[f=flv:onfail=ignore") {
		t.Fatalf("SRT 网络一路是 mpegts，预览一路是 flv: %s", s)
	}
	// tee 先按 "|" 切分去一层转义，再按 ":" 解析选项又去一层：fifo_options 里的 ":" 在 argv 里必须是两个反斜杠（实测，只写一个预览分支打不开）。
	if !strings.Contains(s, `fifo_options=queue_size=120\\:drop_pkts_on_overflow=1:`) {
		t.Fatalf("fifo_options 的冒号要双重转义: %s", s)
	}
}

func TestMainOutputKeepsSourceFpsAndSize(t *testing.T) {
	for _, src := range []float64{24, 25, 30, 50, 59.94, 60} {
		e := LiveEncode{GOPFps: src, VideoKbps: 2500, AudioKbps: 128}
		a := BuildFilePushArgs(FilePushPlan{Input: "/a.mp4", HasAudio: true, Scheme: "rtmp", URL: "rtmp://h/app/k", Enc: e, PreviewPort: 7})
		line := strings.Join(a, " ")
		for _, bad := range []string{"fps=", " -r ", "-framerate", "-fps_mode", "-vsync", "scale=640"} {
			if strings.Contains(line, bad) {
				t.Errorf("src=%v 主输出不能有 %q: %s", src, bad, line)
			}
		}
		if a[idx(a, "-vf", 0)+1] != "scale=trunc(iw/2)*2:trunc(ih/2)*2" {
			t.Errorf("不设分辨率时应保持源尺寸: %s", line)
		}
		wantG := strconv.Itoa(int(src*2 + 0.5))
		if g := a[idx(a, "-g", 0)+1]; g != wantG {
			t.Errorf("src=%v GOP 应为 %s，实际 %s", src, wantG, g)
		}
	}
	a := BuildFilePushArgs(FilePushPlan{Input: "/a.mp4", HasAudio: true, Scheme: "rtmp", URL: "rtmp://h/app/k",
		Enc: LiveEncode{Fps: 60, GOPFps: 60, VideoKbps: 2500, AudioKbps: 128}, PreviewPort: 7})
	if a[idx(a, "-vf", 0)+1] != "fps=60,scale=trunc(iw/2)*2:trunc(ih/2)*2" || a[idx(a, "-g", 0)+1] != "120" {
		t.Errorf("明确帧率只加在主输出: %v", a)
	}
	if strings.Count(strings.Join(a, " "), "fps=") != 1 {
		t.Errorf("fps 滤镜只能有用户指定的那一个: %v", a)
	}
}

func TestBuildPullRemuxArgs(t *testing.T) {
	a, ok := BuildPullRemuxArgs(PullRemuxPlan{URL: "rtmp://h/a/k", InputWhitelist: "rtmp,tcp", Port: 9, Video: true, Audio: true})
	if !ok || !strings.Contains(strings.Join(a, " "), "-c copy") || strings.Contains(strings.Join(a, " "), "fps=") {
		t.Fatalf("拉流应是转封装: %v", a)
	}
	if _, ok := BuildPullRemuxArgs(PullRemuxPlan{URL: "rtmp://h/a", InputWhitelist: "rtmp,tcp", Port: 9}); ok {
		t.Fatal("没有可映射的流不应生成参数")
	}
	v, ok := BuildPullRemuxArgs(PullRemuxPlan{URL: "rtmp://h/a", InputWhitelist: "rtmp,tcp", Port: 9, Video: true})
	if !ok || !has(v, "-an") {
		t.Fatalf("不支持的音频应去掉: %v", v)
	}
	u, ok := BuildPullRemuxArgs(PullRemuxPlan{URL: "http://h/a.flv", InputWhitelist: PullInputWhitelist("http"), Port: 9, Unknown: true})
	if !ok || !strings.Contains(strings.Join(u, " "), "-map 0:v:0? -map 0:a:0?") {
		t.Fatalf("探测失败用可选 map: %v", u)
	}
}

// 契约 v0.25.1：RTMP 的探测窗口要盖住一个完整 GOP（5 秒 / 5 MB），其他仍是 1 秒；都有 -rw_timeout；HLS 从最新的分片开始。
func TestBuildPullRemuxArgsProbeWindowAndHLS(t *testing.T) {
	a, _ := BuildPullRemuxArgs(PullRemuxPlan{URL: "rtmp://h/a/k", InputWhitelist: "rtmp,tcp", Port: 9, Video: true, Audio: true})
	j := strings.Join(a, " ")
	if !strings.Contains(j, "-analyzeduration 5000000 -probesize 5000000 -rw_timeout 8000000 -i rtmp://h/a/k") || strings.Contains(j, "live_start_index") || has(a, "-re") {
		t.Fatalf("非 HLS: %v", a)
	}
	sr, _ := BuildPullRemuxArgs(PullRemuxPlan{URL: "srt://h:1?streamid=read:a", InputWhitelist: "srt,udp", Port: 9, Video: true, Audio: true})
	if !strings.Contains(strings.Join(sr, " "), "-analyzeduration 1000000 -probesize 1000000 -rw_timeout 8000000") {
		t.Fatalf("SRT 仍是 1 秒窗口: %v", sr)
	}
	h, _ := BuildPullRemuxArgs(PullRemuxPlan{URL: "http://h/live/a/index.m3u8", InputWhitelist: PullInputWhitelist("http"), Port: 9, Video: true, Audio: true, HLS: true})
	j = strings.Join(h, " ")
	if !strings.Contains(j, "-analyzeduration 1000000 -probesize 1000000 -rw_timeout 8000000 -live_start_index -1 -i http://h/live/a/index.m3u8") || has(h, "-re") {
		t.Fatalf("HLS 应从最新分片开始、不加 -re: %v", h)
	}
}

func TestLooksLikeHLS(t *testing.T) {
	for _, c := range []struct {
		url, format string
		want        bool
	}{
		{"http://127.0.0.1:8888/live/a/index.m3u8", "", true},
		{"https://h/x/PLAYLIST.M3U8?token=a.flv", "", true},
		{"https://h/x/a.m3u8#frag", "", true},
		{"http://h/live/a.flv", "", false},
		{"http://h/live/a.flv?x=.m3u8", "", false},
		{"rtmp://h/live/a", "", false},
		{"http://h/play?id=1", "hls", true},
		{"http://h/play?id=1", "flv", false},
		{"", "mov,mp4,m4a,3gp,3g2,mj2", false},
	} {
		if got := LooksLikeHLS(c.url, c.format); got != c.want {
			t.Errorf("LooksLikeHLS(%q, %q) = %v", c.url, c.format, got)
		}
	}
}

func TestPreviewPlayable(t *testing.T) {
	v, a, bad := PreviewPlayable("h264", "aac")
	if !v || !a || bad {
		t.Fatalf("h264+aac 应可播")
	}
	v, a, bad = PreviewPlayable("h264", "opus")
	if !v || a || bad {
		t.Fatalf("音频不支持时应只出画面: %v %v %v", v, a, bad)
	}
	if _, _, bad = PreviewPlayable("hevc", "aac"); !bad {
		t.Fatal("HEVC 默认不支持")
	}
	if _, _, bad = PreviewPlayable("", "aac"); bad {
		t.Fatal("纯 AAC 可以播")
	}
	if _, _, bad = PreviewPlayable("", "opus"); !bad {
		t.Fatal("纯 Opus 不支持")
	}
}
