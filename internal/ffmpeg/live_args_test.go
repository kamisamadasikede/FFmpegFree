package ffmpeg

import (
	"strings"
	"testing"
)

func idx(a []string, s string, from int) int {
	for i := from; i < len(a); i++ {
		if a[i] == s {
			return i
		}
	}
	return -1
}

func has(a []string, s string) bool { return idx(a, s, 0) >= 0 }

func enc() LiveEncode { return LiveEncode{GOPFps: 25, VideoKbps: 2500, AudioKbps: 128} }

func TestBuildFilePushArgsWithAudio(t *testing.T) {
	a := BuildFilePushArgs(FilePushPlan{Input: "/a/b c.mp4", HasAudio: true, Scheme: "rtmp", URL: "rtmp://h/app/k", Enc: enc()})
	line := strings.Join(a, " ")
	if a[0] != "-protocol_whitelist" || a[1] != "file" {
		t.Fatalf("输入侧必须先放行 file: %v", a)
	}
	if has(a, "-shortest") || strings.Contains(line, "anullsrc") {
		t.Fatalf("有音轨不补 anullsrc、不加 -shortest: %s", line)
	}
	if strings.Contains(line, "copy") {
		t.Fatalf("始终重编码，不能 copy: %s", line)
	}
	for _, want := range []string{"-re", "file:/a/b c.mp4", "0:v:0", "0:a:0", "libx264", "veryfast", "zerolatency", "yuv420p", "aac", "-g 50", "-b:v 2500k", "-maxrate 2500k", "-bufsize 5000k", "-b:a 128k", "-ar 44100", "-ac 2", "-f flv"} {
		if !strings.Contains(line, want) {
			t.Errorf("缺少 %q: %s", want, line)
		}
	}
	lastI := -1
	for i, x := range a {
		if x == "-i" {
			lastI = i
		}
	}
	out := idx(a, "-protocol_whitelist", lastI)
	if out < 0 || a[out+1] != "rtmp,tcp" || a[len(a)-1] != "rtmp://h/app/k" || idx(a, "-f", out) != out+2 {
		t.Fatalf("输出侧白名单位置不对: %v", a)
	}
	if has(a, "-stream_loop") {
		t.Fatalf("loop=false 不应循环: %s", line)
	}
}

func TestBuildFilePushArgsNoAudioNeedsShortest(t *testing.T) {
	a := BuildFilePushArgs(FilePushPlan{Input: "/a.mp4", Loop: true, HasAudio: false, Scheme: "srt", URL: "srt://h:9000?streamid=x", Enc: enc()})
	line := strings.Join(a, " ")
	if !strings.Contains(line, "-f lavfi -i anullsrc=r=44100:cl=stereo") || !has(a, "-shortest") || !strings.Contains(line, "-map 1:a") {
		t.Fatalf("补 anullsrc 必须加 -shortest: %s", line)
	}
	if !strings.Contains(line, "-stream_loop -1") {
		t.Fatalf("loop: %s", line)
	}
	i := idx(a, "lavfi", 0)
	if a[i-3] != "-protocol_whitelist" || a[i-2] != "file" {
		t.Fatalf("anullsrc 输入前应写 -protocol_whitelist file: %v", a)
	}
	if !strings.Contains(line, "-protocol_whitelist srt,udp -f mpegts srt://h:9000?streamid=x") {
		t.Fatalf("srt 输出: %s", line)
	}
	if has(a, "-flvflags") {
		t.Fatalf("mpegts 不应有 flvflags: %s", line)
	}
	if idx(a, "-shortest", 0) > idx(a, "-protocol_whitelist", i+2) {
		t.Fatalf("-shortest 应在输出白名单之前: %s", line)
	}
}

func TestBuildFilePushArgsWhitelistPerScheme(t *testing.T) {
	for scheme, want := range map[string]string{"rtmp": "rtmp,tcp", "rtmps": "rtmps,tcp,tls,crypto", "srt": "srt,udp"} {
		a := BuildFilePushArgs(FilePushPlan{Input: "/a.mp4", HasAudio: true, Scheme: scheme, URL: scheme + "://h:1/a/b", Enc: enc()})
		if !strings.Contains(strings.Join(a, " "), "-protocol_whitelist "+want+" -f ") {
			t.Errorf("%s: %v", scheme, a)
		}
	}
}

func TestLiveVideoFilterEven(t *testing.T) {
	tests := []struct {
		e    LiveEncode
		want string
	}{
		{LiveEncode{}, "scale=trunc(iw/2)*2:trunc(ih/2)*2"},
		{LiveEncode{Width: 1281}, "scale=1280:-2"},
		{LiveEncode{Height: 721}, "scale=-2:720"},
		{LiveEncode{Width: 1281, Height: 721, Fps: 30}, "fps=30,scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2"},
	}
	for _, tc := range tests {
		if got := liveVideoFilter(tc.e); got != tc.want {
			t.Errorf("%+v → %q want %q", tc.e, got, tc.want)
		}
	}
}

func TestParseProtocols(t *testing.T) {
	out := "Supported file protocols:\nInput:\n  file\n  rtmp\n  srt\nOutput:\n  file\n  rtmp\n  rtmps\n  tee\n"
	p := ParseProtocols(out)
	if !p["rtmp"] || !p["rtmps"] || !p["tee"] || p["srt"] {
		t.Fatalf("只看 Output 段: %v", p)
	}
}
