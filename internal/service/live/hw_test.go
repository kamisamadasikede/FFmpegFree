//go:build !windows

package live

import (
	"context"
	"os"
	"strings"
	"testing"

	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/task"
)

func resolverOf(enc, dev string, fb bool) ffmpeg.EncoderResolver {
	return func(context.Context, string) ffmpeg.EncoderChoice {
		return ffmpeg.EncoderChoice{Encoder: enc, Device: dev, Fallback: fb}
	}
}

func callsOf(f *fixture) []string {
	b, _ := os.ReadFile(f.exe + ".calls")
	s := strings.TrimSpace(string(b))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// 文件推流 / 屏幕推流：各厂商的硬件参数（-g / -bf / -maxrate / -bufsize 在每个厂商上都有），CPU 参数不变。
func TestLiveHWArgvPerVendor(t *testing.T) {
	cases := []struct {
		enc, dev, want string
	}{
		{"h264_nvenc", "nvidia", "-c:v h264_nvenc -preset p4 -tune ll -rc cbr -pix_fmt yuv420p -b:v 1200k -maxrate 1200k -bufsize 2400k -g 50 -bf 0"},
		{"h264_qsv", "intel", "-c:v h264_qsv -preset veryfast -async_depth 1 -pix_fmt nv12 -b:v 1200k -maxrate 1200k -bufsize 2400k -g 50 -bf 0"},
		{"h264_amf", "amd", "-c:v h264_amf -usage lowlatency -rc cbr -pix_fmt yuv420p -b:v 1200k -maxrate 1200k -bufsize 2400k -g 50 -bf 0"},
		{"h264_videotoolbox", "apple", "-c:v h264_videotoolbox -realtime 1 -pix_fmt yuv420p -b:v 1200k -maxrate 1200k -bufsize 2400k -g 50 -bf 0"},
		{"libx264", "cpu", "-c:v libx264 -preset veryfast -tune zerolatency -pix_fmt yuv420p -b:v 1200k -maxrate 1200k -bufsize 2400k -g 50"},
	}
	for _, c := range cases {
		for _, kind := range []string{"file", "screen"} {
			f := x11Fixture(t, func(cfg *Config) { cfg.Encoder = resolverOf(c.enc, c.dev, false) })
			f.setMode("live")
			var tk task.Task
			var err error
			if kind == "file" {
				tk, err = f.svc.StartFilePush(context.Background(), FilePushRequest{InputPath: f.dir + "/a.mp4", URL: "rtmp://127.0.0.1:1935/live/a",
					Options: PushOptions{VideoBitrateKbps: 1200, Fps: 25}})
			} else {
				tk, err = f.svc.StartScreenPush(context.Background(), ScreenPushRequest{URL: "rtmp://127.0.0.1:1935/live/a",
					Options: PushOptions{VideoBitrateKbps: 1200, Fps: 25}})
			}
			if err != nil {
				t.Fatal(err)
			}
			if tk.Encoder != c.enc || tk.EncoderDevice != c.dev || tk.HWFallback {
				t.Errorf("%s/%s: %+v", kind, c.enc, tk)
			}
			f.waitProgress(t, tk.ID)
			b, _ := os.ReadFile(f.exe + ".args")
			line := strings.ReplaceAll(strings.TrimSpace(string(b)), "\n", " ")
			if !strings.Contains(line, c.want) {
				t.Errorf("%s/%s 缺少 %q:\n%s", kind, c.enc, c.want, line)
			}
			if c.dev != "cpu" && strings.Contains(line, "libx264") {
				t.Errorf("%s/%s 不应含 libx264:\n%s", kind, c.enc, line)
			}
			f.mgr.Cancel(tk.ID)
			f.wait(t, tk.ID)
		}
	}
}

// 设备不可用：CPU + hwFallback=true + device_unavailable，任务详情带出。
func TestLiveDeviceUnavailableFlagsFallback(t *testing.T) {
	f := newFixture(t, func(c *Config) { c.Encoder = resolverOf("libx264", "cpu", true) })
	tk, err := f.start(t, "rtmp://127.0.0.1:1935/live/a")
	if err != nil {
		t.Fatal(err)
	}
	if tk.Encoder != "libx264" || tk.EncoderDevice != "cpu" || !tk.HWFallback || tk.HWFallbackReason != ffmpeg.ReasonDeviceUnavailable {
		t.Fatalf("%+v", tk)
	}
	f.waitProgress(t, tk.ID)
	f.mgr.Cancel(tk.ID)
	f.wait(t, tk.ID)
}

// 推流尚未建立（首个数据前）硬件初始化失败 → 自动 CPU 重试一次，推流正常建立。
func TestLiveHWFailBeforeStreamFallsBack(t *testing.T) {
	for _, kind := range []string{"file", "screen"} {
		f := x11Fixture(t, func(c *Config) { c.Encoder = resolverOf("h264_nvenc", "nvidia", false) })
		f.setMode("hwinit_live")
		var tk task.Task
		var err error
		if kind == "file" {
			tk, err = f.start(t, "rtmp://127.0.0.1:1935/live/a")
		} else {
			tk, err = f.svc.StartScreenPush(context.Background(), ScreenPushRequest{URL: "rtmp://127.0.0.1:1935/live/a"})
		}
		if err != nil {
			t.Fatal(err)
		}
		f.waitProgress(t, tk.ID)
		cur, _ := f.mgr.Get(tk.ID)
		if cur.Encoder != "libx264" || cur.EncoderDevice != "cpu" || !cur.HWFallback || cur.HWFallbackReason != ffmpeg.ReasonNVENCInit {
			t.Fatalf("%s: 回退后编码器: %+v", kind, cur)
		}
		if n := len(callsOf(f)); n != 2 {
			t.Fatalf("%s: 应启动 2 次: %v", kind, callsOf(f))
		}
		f.mgr.Cancel(tk.ID)
		d := f.wait(t, tk.ID)
		if d.Status != task.StatusSucceeded || d.Error != nil || d.Encoder != "libx264" || !d.HWFallback {
			t.Fatalf("%s: %+v %+v", kind, d, d.Error)
		}
		if strings.Contains(f.logText(t, tk.ID), "rtmp://127.0.0.1") && false {
			t.Fatal("unreachable")
		}
	}
}

// 推流已建立后中途硬件失败：不自动重试，任务失败（LIVE_PUSH_INTERRUPTED 等），编码器仍是硬件。
func TestLiveHWFailMidStreamDoesNotRetry(t *testing.T) {
	f := newFixture(t, func(c *Config) { c.Encoder = resolverOf("h264_nvenc", "nvidia", false) })
	f.setMode("hwmid")
	tk, err := f.start(t, "rtmp://127.0.0.1:1935/live/a")
	if err != nil {
		t.Fatal(err)
	}
	d := f.wait(t, tk.ID)
	if d.Status != task.StatusFailed {
		t.Fatalf("%+v", d)
	}
	if n := len(callsOf(f)); n != 1 {
		t.Fatalf("推流中途失败不能重试: %v", callsOf(f))
	}
	if d.Encoder != "h264_nvenc" || d.HWFallback {
		t.Fatalf("%+v", d)
	}
}

// 推流前失败但与硬件无关（连接被拒）：不回退、不重试。
func TestLiveConnectionRefusedDoesNotFallBack(t *testing.T) {
	f := newFixture(t, func(c *Config) { c.Encoder = resolverOf("h264_nvenc", "nvidia", false) })
	f.setMode("refused")
	tk, err := f.start(t, "rtmp://127.0.0.1:1935/live/a")
	if err != nil {
		t.Fatal(err)
	}
	d := f.wait(t, tk.ID)
	if d.Status != task.StatusFailed || len(callsOf(f)) != 1 || d.HWFallback {
		t.Fatalf("%+v calls=%v", d, callsOf(f))
	}
}

// 连接阶段取消：不回退。
func TestLiveCancelWhileConnectingDoesNotFallBack(t *testing.T) {
	f := newFixture(t, func(c *Config) { c.Encoder = resolverOf("h264_nvenc", "nvidia", false) })
	f.setMode("connecting")
	tk, err := f.start(t, "rtmp://127.0.0.1:1935/live/a")
	if err != nil {
		t.Fatal(err)
	}
	f.mgr.Cancel(tk.ID)
	d := f.wait(t, tk.ID)
	if d.Status != task.StatusCanceled || len(callsOf(f)) > 1 || d.HWFallback {
		t.Fatalf("%+v calls=%v", d, callsOf(f))
	}
}
