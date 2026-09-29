package live

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"rtmp://u:p@h:1935/live/abc123?token=xyz", "rtmp://***@h:1935/live/***?token=***"},
		{"srt://h:9000?streamid=a&passphrase=b", "srt://h:9000?streamid=***&passphrase=***"},
		{"rtmp://h/app/key", "rtmp://h/app/***"},
		{"rtmp://h/app", "rtmp://h/app"},
		{"rtmp://h/live/a/b/c#frag", "rtmp://h/live/***"},
		{"rtmps://h:443/app/key?x=1&y=2", "rtmps://h:443/app/***?x=***&y=***"},
		{"rtmp://[::1]:1935/live/k", "rtmp://[::1]:1935/live/***"},
		{"RTMP://H/App/Key", "rtmp://H/App/***"},
		{"tcp://127.0.0.1:1935?tcp_nodelay=0", "tcp://127.0.0.1:1935?tcp_nodelay=***"},
		{"tls://127.0.0.1:1997", "tls://127.0.0.1:1997"},
		{"srt://h:9000?bare", "srt://h:9000?***"},
		{"not a url", InvalidURLText},
		{"", InvalidURLText},
		{"rtmp://", InvalidURLText},
		{"rtmp://%zz/a", InvalidURLText},
	}
	for _, tc := range tests {
		if got := RedactURL(tc.in); got != tc.want {
			t.Errorf("RedactURL(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

// 用真实 ffmpeg 样本（ffmpeg 7.1.5 + MediaMTX 1.21.1 抓取）验证行脱敏：任何秘密片段都不能留下。
func TestRedactorOnRealSamples(t *testing.T) {
	dir := filepath.Join("..", "ffmpeg", "testdata", "live")
	tests := []struct {
		file, url string
		secrets   []string
	}{
		{"off_rtmp.err", "rtmp://127.0.0.1:1999/live/secretkey123", []string{"secretkey123"}},
		{"off_rtmps.err", "rtmps://127.0.0.1:1997/live/secretkey123", []string{"secretkey123"}},
		{"dns.err", "rtmp://nonexistent.invalid/live/secretkey123", []string{"secretkey123"}},
		{"rej_rtmp.err", "rtmp://127.0.0.1:2935/live/secretkey123?user=wrong&pass=wrong", []string{"secretkey123", "wrong"}},
		{"off_srt.err", "srt://127.0.0.1:1998?streamid=publish:secretsid&passphrase=secretpass123", []string{"secretsid", "secretpass123"}},
		{"rej_srt.err", "srt://127.0.0.1:2890?streamid=publish:live/secretkey123:wrong:wrong", []string{"secretkey123", "wrong"}},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join(dir, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			red := NewRedactor(tc.url)
			var out []string
			for _, l := range strings.Split(string(b), "\n") {
				out = append(out, red(l))
			}
			all := strings.Join(out, "\n")
			for _, s := range tc.secrets {
				if strings.Contains(all, s) {
					t.Fatalf("残留秘密片段 %q:\n%s", s, all)
				}
			}
			if !strings.Contains(all, "127.0.0.1") && !strings.Contains(all, "nonexistent.invalid") {
				t.Fatalf("host 应该保留用于排查:\n%s", all)
			}
		})
	}
}

func TestRedactorForms(t *testing.T) {
	url := "rtmp://user:pa%24s@h:1935/live/my%20key/x?token=tok%2Fen&sig=abcdef"
	red := NewRedactor(url)
	lines := []string{
		"Error opening output " + url + ": Input/output error",
		"decoded form rtmp://user:pa$s@h:1935/live/my key/x?token=tok/en&sig=abcdef",
		"[rtmp @ 0x1] Cannot open connection tcp://h:1935?tcp_nodelay=0",
		"stream my%20key seen, token tok%2Fen, sig abcdef, password pa$s, user user",
		"nested URL: (rtmp://other/app/topsecret).",
	}
	for _, l := range lines {
		got := red(l)
		for _, s := range []string{"my%20key", "my key", "tok%2Fen", "tok/en", "abcdef", "pa$s", "pa%24s", "topsecret", "user:"} {
			if strings.Contains(got, s) {
				t.Errorf("行 %q 脱敏后仍含 %q: %q", l, s, got)
			}
		}
	}
	if got := red("Error opening output " + url + ": x"); !strings.Contains(got, "rtmp://***@h:1935/live/***?token=***&sig=***") {
		t.Errorf("整体地址应替换成 RedactURL 的结果: %q", got)
	}
	if got := red("[rtmp @ 0x1] Cannot open connection tcp://h:1935?tcp_nodelay=0"); !strings.Contains(got, "tcp://h:1935?tcp_nodelay=***") {
		t.Errorf("残留 URL 应交给 RedactURL: %q", got)
	}
	// 短片段（< 3 字符）不当秘密，避免把普通文字全替换掉。
	if got := NewRedactor("rtmp://h/live/ab")("ab is fine"); got != "ab is fine" {
		t.Errorf("过短的片段不应被替换: %q", got)
	}
}
