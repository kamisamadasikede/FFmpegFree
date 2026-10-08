//go:build !windows

package ffmpeg

// 集成测试（v0.24.2）：真实 ffmpeg 按 BuildFilePushArgs 推 30 fps 的 testsrc2 到本机的 ffmpeg RTMP 监听端（-listen 1），
// 量收到的帧率：预览关 / 开 / 预览写不进去（目标路径是个目录，改名必失败——相当于 Windows 上读取端占着预览文件）三种情况，
// 主输出都必须保持源帧率。找不到 ffmpeg / ffprobe 时 Skip；-short 时跳过。

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func fpsTestBins(t *testing.T) (ffm, ffp string) {
	t.Helper()
	if testing.Short() {
		t.Skip("-short")
	}
	ffm = os.Getenv("FFMPEGFREE_FFMPEG")
	if ffm == "" {
		ffm, _ = exec.LookPath("ffmpeg")
	}
	if ffm == "" {
		t.Skip("没有 ffmpeg")
	}
	ffp = filepath.Join(filepath.Dir(ffm), "ffprobe")
	if _, err := os.Stat(ffp); err != nil {
		if ffp, _ = exec.LookPath("ffprobe"); ffp == "" {
			t.Skip("没有 ffprobe")
		}
	}
	return ffm, ffp
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// probeVideoFps 返回视频包数和按时间戳算的平均帧率（(包数-1)/(最后 pts-最先 pts)）。
func probeVideoFps(t *testing.T, ffp, path string) (n int, fps float64) {
	t.Helper()
	out, err := exec.Command(ffp, "-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var lo, hi float64
	for _, l := range strings.Fields(string(out)) {
		v, err := strconv.ParseFloat(strings.TrimSuffix(l, ","), 64)
		if err != nil {
			continue
		}
		if n == 0 || v < lo {
			lo = v
		}
		if n == 0 || v > hi {
			hi = v
		}
		n++
	}
	if n < 2 || hi <= lo {
		return n, 0
	}
	return n, float64(n-1) / (hi - lo)
}

func TestIntegrationFilePushKeepsSourceFps(t *testing.T) {
	ffm, ffp := fpsTestBins(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src30.mp4")
	const secs = 8
	gen := exec.Command(ffm, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30",
		"-f", "lavfi", "-i", "sine=f=440:r=44100", "-t", strconv.Itoa(secs), "-c:v", "libx264", "-preset", "ultrafast", "-g", "60", "-c:a", "aac", "-shortest", src)
	if b, err := gen.CombinedOutput(); err != nil {
		t.Skipf("生成测试源失败（精简构建？）: %v %s", err, b)
	}
	cases := []struct {
		name    string
		preview func(d string) string
	}{
		{"预览关", func(string) string { return "" }},
		{"预览开", func(d string) string { return filepath.Join(d, "pv.jpg") }},
		{"预览写不进去", func(d string) string {
			p := filepath.Join(d, "pv.jpg")
			os.MkdirAll(p, 0o755) // 目标是目录：每次改名都失败
			return p
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			port := freeTCPPort(t)
			url := fmt.Sprintf("rtmp://127.0.0.1:%d/live/k", port)
			got := filepath.Join(d, "got.flv")
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			sink := exec.CommandContext(ctx, ffm, "-hide_banner", "-loglevel", "error", "-y", "-listen", "1", "-i", url, "-c", "copy", "-f", "flv", got)
			var sinkErr bytes.Buffer
			sink.Stderr = &sinkErr
			if err := sink.Start(); err != nil {
				t.Fatal(err)
			}
			time.Sleep(500 * time.Millisecond)
			pv := tc.preview(d)
			args := BuildFilePushArgs(FilePushPlan{Input: src, HasAudio: true, Scheme: "rtmp", URL: url, PreviewPath: pv,
				Enc: LiveEncode{GOPFps: 30, VideoKbps: 1500, AudioKbps: 128}})
			push := exec.CommandContext(ctx, ffm, append([]string{"-hide_banner", "-nostats", "-y", "-nostdin"}, args...)...)
			var pushErr bytes.Buffer
			push.Stderr = &pushErr
			start := time.Now()
			if err := push.Run(); err != nil {
				t.Fatalf("推流失败: %v\n%s", err, tail(pushErr.String(), 20))
			}
			wall := time.Since(start)
			if err := sink.Wait(); err != nil {
				t.Logf("接收端退出: %v %s", err, sinkErr.String())
			}
			n, fps := probeVideoFps(t, ffp, got)
			t.Logf("%s: 收到 %d 帧，平均 %.2f fps，推流用时 %v", tc.name, n, fps, wall.Round(10*time.Millisecond))
			if fps < 29 || n < (secs-1)*30 {
				t.Fatalf("主输出应保持源帧率 30 fps：%d 帧 %.2f fps\n%s", n, fps, tail(pushErr.String(), 20))
			}
			if wall > time.Duration(secs+4)*time.Second {
				t.Fatalf("推流被拖慢：%d 秒的源用了 %v", secs, wall)
			}
			if tc.name == "预览开" {
				if b, err := os.ReadFile(pv); err != nil || len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
					t.Fatalf("预览开时应写出 JPEG: %v", err)
				}
			}
		})
	}
}

func tail(s string, n int) string {
	l := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, "\n")
}
