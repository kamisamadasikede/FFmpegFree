package live

import (
	"errors"
	"strings"
	"testing"
)

func TestParsePushURLValid(t *testing.T) {
	longPass := strings.Repeat("p", 79)
	tests := []struct {
		name, in              string
		scheme, host          string
		port                  int
		ffmpeg, key, redacted string
	}{
		{"rtmp 默认端口", "rtmp://Live.Example.com/app/key", "rtmp", "live.example.com", 1935,
			"rtmp://live.example.com/app/key", "rtmp://live.example.com:1935/app/key", "rtmp://live.example.com/app/***"},
		{"rtmp 显式默认端口与上一项同键", "rtmp://live.example.com:1935/app/key/", "rtmp", "live.example.com", 1935,
			"rtmp://live.example.com:1935/app/key/", "rtmp://live.example.com:1935/app/key", "rtmp://live.example.com:1935/app/***"},
		{"scheme 大写、去空白", "  RTMP://h/a/k  ", "rtmp", "h", 1935, "rtmp://h/a/k", "rtmp://h:1935/a/k", "rtmp://h/a/***"},
		{"rtmps 默认 443", "rtmps://h/app/key", "rtmps", "h", 443, "rtmps://h/app/key", "rtmps://h:443/app/key", "rtmps://h/app/***"},
		{"流名在查询参数里", "rtmp://h/app?key=abc123", "rtmp", "h", 1935, "rtmp://h/app?key=abc123", "rtmp://h:1935/app?key=abc123", "rtmp://h/app?key=***"},
		{"userinfo", "rtmp://u:p@h:1936/live/abc123?token=xyz", "rtmp", "h", 1936,
			"rtmp://u:p@h:1936/live/abc123?token=xyz", "rtmp://h:1936/live/abc123?token=xyz", "rtmp://***@h:1936/live/***?token=***"},
		{"多段流名", "rtmp://h/live/a/b/c", "rtmp", "h", 1935, "rtmp://h/live/a/b/c", "rtmp://h:1935/live/a/b/c", "rtmp://h/live/***"},
		{"回环与内网允许", "rtmp://127.0.0.1:1935/live/k", "rtmp", "127.0.0.1", 1935, "rtmp://127.0.0.1:1935/live/k", "rtmp://127.0.0.1:1935/live/k", "rtmp://127.0.0.1:1935/live/***"},
		{"内网", "rtmp://192.168.1.10/live/k", "rtmp", "192.168.1.10", 1935, "rtmp://192.168.1.10/live/k", "rtmp://192.168.1.10:1935/live/k", "rtmp://192.168.1.10/live/***"},
		{"IPv6", "rtmp://[::1]:1935/live/k", "rtmp", "::1", 1935, "rtmp://[::1]:1935/live/k", "rtmp://[::1]:1935/live/k", "rtmp://[::1]:1935/live/***"},
		{"IDN 转 punycode", "rtmp://例え.jp/live/k", "rtmp", "xn--r8jz45g.jp", 1935, "rtmp://xn--r8jz45g.jp/live/k", "rtmp://xn--r8jz45g.jp:1935/live/k", "rtmp://xn--r8jz45g.jp/live/***"},
		{"srt 基本", "srt://h:9000", "srt", "h", 9000, "srt://h:9000", "srt://h:9000", "srt://h:9000"},
		{"srt 参数键小写化、值原样", "srt://h:9000?StreamID=publish:live/k&PassPhrase=" + longPass[:12], "srt", "h", 9000,
			"srt://h:9000?streamid=publish:live/k&passphrase=" + longPass[:12], "srt://h:9000?streamid=publish:live/k&passphrase=" + longPass[:12],
			"srt://h:9000?streamid=***&passphrase=***"},
		{"srt 键百分号解码", "srt://h:9000?pass%70hrase=0123456789&mode=CALLER", "srt", "h", 9000,
			"srt://h:9000?passphrase=0123456789&mode=CALLER", "srt://h:9000?passphrase=0123456789&mode=CALLER", "srt://h:9000?passphrase=***&mode=***"},
		{"srt 其余白名单参数", "srt://h:9000?latency=200000&connect_timeout=3000&maxbw=0&pkt_size=1316&pbkeylen=16", "srt", "h", 9000,
			"srt://h:9000?latency=200000&connect_timeout=3000&maxbw=0&pkt_size=1316&pbkeylen=16", "srt://h:9000?latency=200000&connect_timeout=3000&maxbw=0&pkt_size=1316&pbkeylen=16",
			"srt://h:9000?latency=***&connect_timeout=***&maxbw=***&pkt_size=***&pbkeylen=***"},
		{"passphrase 79 位", "srt://h:9000?passphrase=" + longPass, "srt", "h", 9000, "srt://h:9000?passphrase=" + longPass, "srt://h:9000?passphrase=" + longPass, "srt://h:9000?passphrase=***"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, err := ParsePushURL(tc.in)
			if err != nil {
				t.Fatalf("不应报错: %v", err)
			}
			if u.Scheme != tc.scheme || u.Host != tc.host || u.Port != tc.port || u.FFmpeg != tc.ffmpeg || u.Key != tc.key || u.Redacted != tc.redacted {
				t.Fatalf("got %+v\nwant scheme=%s host=%s port=%d ffmpeg=%s key=%s redacted=%s", u, tc.scheme, tc.host, tc.port, tc.ffmpeg, tc.key, tc.redacted)
			}
		})
	}
}

func TestParsePushURLInvalidReasons(t *testing.T) {
	tests := []struct{ name, in, reason string }{
		{"空串", "", ReasonMalformed},
		{"只有空白", "   ", ReasonMalformed},
		{"超长", "rtmp://h/a/" + strings.Repeat("x", 2048), ReasonMalformed},
		{"含空白", "rtmp://h/a/k k", ReasonMalformed},
		{"含控制字符", "rtmp://h/a/k\x01", ReasonMalformed},
		{"含竖线", "rtmp://h/a/k|x", ReasonMalformed},
		{"含反斜杠", "rtmp://h/a\\k", ReasonMalformed},
		{"含双引号", "rtmp://h/a/\"k", ReasonMalformed},
		{"含单引号", "rtmp://h/a/'k", ReasonMalformed},
		{"没有 scheme", "h/a/k", ReasonMalformed},
		{"scheme 空", "://h/a", ReasonMalformed},
		{"file", "file:///etc/passwd", ReasonSchemeUnsupported},
		{"http", "http://h/a/k", ReasonSchemeUnsupported},
		{"rtsp", "rtsp://h/a/k", ReasonSchemeUnsupported},
		{"udp", "udp://h:1234", ReasonSchemeUnsupported},
		{"tcp", "tcp://h:1234", ReasonSchemeUnsupported},
		{"concat", "concat://h/a", ReasonSchemeUnsupported},
		{"subfile", "subfile://h/a", ReasonSchemeUnsupported},
		{"data", "data://h/a", ReasonSchemeUnsupported},
		{"host 为空", "rtmp:///app/key", ReasonMissingHost},
		{"host 为空带端口", "rtmp://:1935/app/key", ReasonMissingHost},
		{"host 为空 srt", "srt://:9000", ReasonMissingHost},
		{"端口 0", "rtmp://h:0/a/k", ReasonMalformed},
		{"端口越界", "rtmp://h:65536/a/k", ReasonMalformed},
		{"端口非数字", "rtmp://h:abc/a/k", ReasonMalformed},
		{"端口为空", "rtmp://h:/a/k", ReasonMalformed},
		{"IPv6 缺括号", "rtmp://::1/a/k", ReasonMalformed},
		{"IPv6 括号未闭合", "rtmp://[::1/a/k", ReasonMalformed},
		{"rtmp 无应用名", "rtmp://h/", ReasonMalformed},
		{"rtmp 无路径", "rtmp://h", ReasonMalformed},
		{"srt 缺端口", "srt://h?streamid=a", ReasonMalformed},
		{"srt 不在白名单的参数", "srt://h:9000?foo=bar", ReasonParamNotAllowed},
		{"srt 解码后不在白名单", "srt://h:9000?%66oo=bar", ReasonParamNotAllowed},
		{"srt 未列出的常见参数", "srt://h:9000?oheadbw=25", ReasonParamNotAllowed},
		{"srt listener", "srt://h:9000?mode=listener", ReasonParamNotAllowed},
		{"srt rendezvous 大写键", "srt://h:9000?MODE=rendezvous", ReasonParamNotAllowed},
		{"srt 同名重复", "srt://h:9000?latency=1&LATENCY=2", ReasonParamNotAllowed},
		{"srt passphrase 太短", "srt://h:9000?passphrase=abc", ReasonMalformed},
		{"srt passphrase 9 位", "srt://h:9000?passphrase=012345678", ReasonMalformed},
		{"srt passphrase 80 位", "srt://h:9000?passphrase=" + strings.Repeat("p", 80), ReasonMalformed},
		{"srt pbkeylen 非法", "srt://h:9000?pbkeylen=20", ReasonMalformed},
		{"srt latency 非整数", "srt://h:9000?latency=abc", ReasonMalformed},
		{"srt pkt_size 超限", "srt://h:9000?pkt_size=1500", ReasonMalformed},
		{"srt streamid 超长", "srt://h:9000?streamid=" + strings.Repeat("s", 513), ReasonMalformed},
		{"srt 键百分号非法", "srt://h:9000?%zz=1", ReasonMalformed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParsePushURL(tc.in)
			var ue *URLError
			if !errors.As(err, &ue) {
				t.Fatalf("应报 URLError: %v", err)
			}
			if ue.Reason != tc.reason {
				t.Fatalf("reason=%q want %q (%s)", ue.Reason, tc.reason, ue.Message)
			}
			// message 不得回显地址原文的任何有意义片段。
			for _, frag := range []string{"secret", "example.com"} {
				if strings.Contains(ue.Message, frag) {
					t.Fatalf("message 含地址片段: %s", ue.Message)
				}
			}
		})
	}
}

func TestParsePushURLNeverEchoesInputInMessage(t *testing.T) {
	in := "http://h.example.com/app/supersecretkey"
	_, err := ParsePushURL(in)
	if err == nil || strings.Contains(err.Error(), "supersecretkey") || strings.Contains(err.Error(), "h.example.com") {
		t.Fatalf("%v", err)
	}
}
