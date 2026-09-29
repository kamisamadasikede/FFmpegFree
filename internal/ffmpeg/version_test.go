package ffmpeg

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantOK     bool
		wantRaw    string
		wantMajor  int
		wantKnown  bool
		acceptable bool
	}{
		{"发行版 Ubuntu", "ffmpeg version 6.1.1-3ubuntu5 Copyright (c) 2000-2023 the FFmpeg developers\nbuilt with gcc 13", true, "6.1.1-3ubuntu5", 6, true, true},
		{"n 前缀 tag", "ffmpeg version n7.0 Copyright (c) 2000-2024", true, "n7.0", 7, true, true},
		{"n 前缀带补丁", "ffmpeg version n7.1.1-4-gabc Copyright", true, "n7.1.1-4-gabc", 7, true, true},
		{"macOS Homebrew", "ffmpeg version 7.1.1 Copyright (c) 2000-2025", true, "7.1.1", 7, true, true},
		{"ffprobe", "ffprobe version 6.0-static https://johnvansickle.com/ffmpeg/  Copyright", true, "6.0-static", 6, true, true},
		{"只有主版本", "ffmpeg version 8 Copyright", true, "8", 8, true, true},
		{"过低 4.4", "ffmpeg version 4.4.2-0ubuntu0.22.04.1 Copyright", true, "4.4.2-0ubuntu0.22.04.1", 4, true, false},
		{"过低 5.1", "ffmpeg version 5.1.6-0+deb12u1 Copyright", true, "5.1.6-0+deb12u1", 5, true, false},
		{"git 主干构建", "ffmpeg version N-12345-gabc1234 Copyright", true, "N-12345-gabc1234", 0, false, true},
		{"git 构建 gyan.dev 日期版", "ffmpeg version 2024-05-20-git-abcdef-full_build-www.gyan.dev Copyright", true, "2024-05-20-git-abcdef-full_build-www.gyan.dev", 0, false, true},
		{"BtbN 主干构建", "ffmpeg version N-118000-g1234567-20250101 Copyright", true, "N-118000-g1234567-20250101", 0, false, true},
		{"横幅在前", "some banner\nffmpeg version 6.0 Copyright", true, "6.0", 6, true, true},
		{"CRLF", "ffmpeg version 6.1.1-essentials_build Copyright\r\nbuilt with gcc\r\n", true, "6.1.1-essentials_build", 6, true, true},
		{"不是 ffmpeg", "hello world", false, "", 0, false, false},
		{"空", "", false, "", 0, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, ok := ParseVersion(c.in)
			if ok != c.wantOK {
				t.Fatalf("ok=%v, 期望 %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if v.Raw != c.wantRaw || v.Major != c.wantMajor || v.Known != c.wantKnown {
				t.Fatalf("得到 %+v", v)
			}
			if v.Acceptable() != c.acceptable {
				t.Fatalf("Acceptable=%v, 期望 %v", v.Acceptable(), c.acceptable)
			}
		})
	}
}

func TestMissingEncoders(t *testing.T) {
	out := `Encoders:
 V..... = Video
 ------
 V....D libx264              libx264 H.264 / AVC (codec h264)
 V....D libx264rgb           libx264 H.264 / AVC (codec h264)
 A....D aac                  AAC (Advanced Audio Coding)
`
	if m := missingEncoders(out, "libx264", "aac"); len(m) != 0 {
		t.Fatalf("不应缺失: %v", m)
	}
	// libx264rgb 不能冒充 libx264；aac_at 不能冒充 aac。
	out2 := " V....D libx264rgb  x\n A....D aac_at  y\n"
	if m := missingEncoders(out2, "libx264", "aac"); len(m) != 2 {
		t.Fatalf("应缺失两个: %v", m)
	}
}
