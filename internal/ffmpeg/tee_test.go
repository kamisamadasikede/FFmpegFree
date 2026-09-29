package ffmpeg

import (
	"strings"
	"testing"
)

func TestTeeEscapeTable(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/a/b/screen-20260929-200000.mp4", "/a/b/screen-20260929-200000.mp4"},
		{"a_b-c.d/e", "a_b-c.d/e"},
		{`a b`, `a\ b`},
		{`a'b`, `a\'b`},
		{`a|b`, `a\|b`},
		{`a[b]`, `a\[b\]`},
		{`a,b`, `a\,b`},
		{`a:b`, `a\:b`},
		{`a=b`, `a\=b`},
		{`a;b`, `a\;b`},
		{`a\b`, `a\\b`},
		{`a#b?c%d&e(1)`, `a\#b\?c\%d\&e\(1\)`},
		{`"q"`, `\"q\"`},
		{"tab\there", "tab\\\there"},
		{"录屏/存档 测试", `录屏/存档\ 测试`},        // 中文不转义，空格转义
		{"日本語フォルダ/ファイル", "日本語フォルダ/ファイル"}, // 日文
		{"한국어 폴더", `한국어\ 폴더`},            // 韩文
		{`rtmp://127.0.0.1:1935/live/esc?k=v&x=1`, `rtmp\://127.0.0.1\:1935/live/esc\?k\=v\&x\=1`},
		{`srt://[::1]:9000?streamid=a,b;c`, `srt\://\[\:\:1\]\:9000\?streamid\=a\,b\;c`},
		{"", ""},
	}
	for _, tc := range tests {
		if got := TeeEscape(tc.in); got != tc.want {
			t.Errorf("TeeEscape(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
	// 契约 6.10 实测用的目录：含所有特殊字符
	dir := `/tmp/录 屏'|[x],y=z;c:d#f?g%h&i(1)`
	want := `/tmp/录\ 屏\'\|\[x\]\,y\=z\;c\:d\#f\?g\%h\&i\(1\)`
	if got := TeeEscape(dir); got != want {
		t.Errorf("%q → %q want %q", dir, got, want)
	}
}

func TestTeePathPerPlatform(t *testing.T) {
	tests := []struct {
		goos, in, want string
		bad            bool
	}{
		{"linux", "/home/me/Videos/screen-20260929-200000.mp4", "file:/home/me/Videos/screen-20260929-200000.mp4", false},
		{"linux", "/home/me/my videos/a.mp4", `file:/home/me/my\ videos/a.mp4`, false},
		{"linux", `/home/me/we\ird/a.mp4`, `file:/home/me/we\\ird/a.mp4`, false}, // Linux 上反斜杠是普通字符，照样转义
		{"windows", `C:\Users\me\Videos\FFmpegFree\screen-20260929-200000.mp4`, `file:C\:/Users/me/Videos/FFmpegFree/screen-20260929-200000.mp4`, false},
		{"windows", `C:\Users\我 的\视频\a.mp4`, `file:C\:/Users/我\ 的/视频/a.mp4`, false},
		{"windows", `D:\a'b\c[1]\x.mp4`, `file:D\:/a\'b/c\[1\]/x.mp4`, false},
		{"windows", `\\server\share\dir\a.mp4`, `file://server/share/dir/a.mp4`, false}, // UNC
		{"windows", `\\nas\共享 盘\a.mp4`, `file://nas/共享\ 盘/a.mp4`, false},
		{"windows", `C:/already/slash/a.mp4`, `file:C\:/already/slash/a.mp4`, false},
		{"windows", `\\?\C:\long\a.mp4`, "", true},
		{"windows", `\\.\PhysicalDrive0`, "", true},
		{"windows", `//?/C:/long/a.mp4`, "", true},
		{"windows", `//./COM1`, "", true},
		{"linux", `\\?\C:\x`, "", true},
		{"linux", "/tmp/a\nb.mp4", "", true},
		{"windows", "C:\\a\x00b", "", true},
	}
	for _, tc := range tests {
		got, err := TeePath(tc.goos, tc.in)
		if tc.bad {
			if err == nil {
				t.Errorf("%s %q 应被拒绝，得到 %q", tc.goos, tc.in, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s %q → %q, %v want %q", tc.goos, tc.in, got, err, tc.want)
		}
	}
}

func TestTeeDescription(t *testing.T) {
	got := TeeDescription("rtmp", "rtmp://127.0.0.1:1935/live/k?a=b", `file:/tmp/a.mp4`)
	want := `[f=flv:onfail=abort:protocol_whitelist=rtmp,tcp]rtmp\://127.0.0.1\:1935/live/k\?a\=b|` +
		`[f=mp4:onfail=abort:movflags=+frag_keyframe+empty_moov:flush_packets=1:protocol_whitelist=file]file:/tmp/a.mp4`
	if got != want {
		t.Fatalf("\n%s\n%s", got, want)
	}
	if got := TeeDescription("srt", "srt://h:9000?streamid=x", "file:/a.mp4"); !strings.HasPrefix(got, "[f=mpegts:onfail=abort:protocol_whitelist=srt,udp]srt\\://h\\:9000") {
		t.Fatal(got)
	}
	if got := TeeDescription("rtmps", "rtmps://h/a/k", "file:/a.mp4"); !strings.HasPrefix(got, "[f=flv:onfail=abort:protocol_whitelist=rtmps,tcp,tls,crypto]") {
		t.Fatal(got)
	}
	// slave 描述里 | 只有一个（分隔符）：URL 里的 | 会被转义
	if n := strings.Count(strings.ReplaceAll(got, `\|`, ""), "|"); n != 1 {
		t.Fatalf("应只有 1 个分隔符: %d", n)
	}
}

func TestBuildScreenPushArgsWithArchive(t *testing.T) {
	p := ScreenPushPlan{GOOS: "linux", Display: ":99", FPS: 15, Scheme: "rtmp", URL: "rtmp://h/a/k", Silent: true,
		Enc: LiveEncode{GOPFps: 15, VideoKbps: 1000, AudioKbps: 64}, Region: ScreenRegion{Desktop: true}, ArchiveTee: "file:/tmp/a.mp4"}
	a := BuildScreenPushArgs(p)
	line := strings.Join(a, " ")
	i := idx(a, "-flags", 0)
	if i < 0 || a[i+1] != "+global_header" || a[i+2] != "-f" || a[i+3] != "tee" || len(a) != i+5 {
		t.Fatalf("-flags +global_header -f tee <desc> 必须在末尾: %s", line)
	}
	if strings.Contains(line, "-flvflags") || strings.Contains(line, "-shortest") {
		t.Fatalf("tee 路径不带 -flvflags，屏幕采集不加 -shortest: %s", line)
	}
	if !strings.Contains(a[len(a)-1], "onfail=abort") || !strings.HasSuffix(a[len(a)-1], "file:/tmp/a.mp4") {
		t.Fatalf("%s", a[len(a)-1])
	}
	// 输出侧不再有全局 -protocol_whitelist（对 tee 的 slave 无效，白名单写在 slave 里）
	if n := strings.Count(line, "-protocol_whitelist"); n != 1 { // 只有 anullsrc 输入前的那个
		t.Fatalf("输出侧白名单应在 slave 里: %s", line)
	}
	// 无存档路径不变
	p.ArchiveTee = ""
	if line := strings.Join(BuildScreenPushArgs(p), " "); strings.Contains(line, "tee") {
		t.Fatal(line)
	}
}
