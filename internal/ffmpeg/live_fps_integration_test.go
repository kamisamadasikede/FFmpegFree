//go:build !windows

package ffmpeg

// 集成测试（v0.24.5 起，v0.25 改成 tee 预览分支）：真实 ffmpeg 按 BuildFilePushArgs 推 30 fps 的 testsrc2 到本机的 ffmpeg RTMP 监听端（-listen 1），
// 量收到的帧率：没有预览分支 / 预览分支有人读 / 预览分支的 TCP 对端完全不读（接收缓冲只有几 KB）三种情况，
// 主输出都必须保持源帧率；有人读时预览分支必须真的收到 FLV（防止预览分支因为参数错误打开失败，被 onfail=ignore 吞掉）。
// 找不到 ffmpeg / ffprobe 时 Skip；-short 时跳过。

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
	"sync/atomic"
	"syscall"
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
	n, fps, err := countVideoFps(ffp, path)
	if err != nil {
		t.Fatal(err)
	}
	return n, fps
}

// countVideoFps 是 probeVideoFps 的不带 *testing.T 版本，可以在非测试 goroutine 里调用。
func countVideoFps(ffp, path string) (n int, fps float64, err error) {
	out, err := exec.Command(ffp, "-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, 0, fmt.Errorf("ffprobe %s: %v", path, err)
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
		return n, 0, nil
	}
	return n, float64(n-1) / (hi - lo), nil
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
		preview func(t *testing.T) (int, func() int64, func())
	}{
		{"预览关", func(*testing.T) (int, func() int64, func()) { return 0, nil, nil }},
		{"预览开", readPreviewTCP},
		{"预览没人读", stallPreviewTCP},
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
			pvPort, pvBytes, release := tc.preview(t)
			args := BuildFilePushArgs(FilePushPlan{Input: src, HasAudio: true, Scheme: "rtmp", URL: url, PreviewPort: pvPort,
				Enc: LiveEncode{GOPFps: 30, VideoKbps: 1500, AudioKbps: 128}})
			push := exec.CommandContext(ctx, ffm, append([]string{"-hide_banner", "-nostats", "-y", "-nostdin"}, args...)...)
			var pushErr bytes.Buffer
			push.Stderr = &pushErr
			start := time.Now()
			// 对端完全不读时，正片推完后 ffmpeg 会卡在预览分支的收尾（fifo 要把队列写完），这不是推流变慢。
			// 所以在源时长 + 1 秒时先数主输出已经收到多少帧（证明推流期间没被拖慢），再断开预览连接让 ffmpeg 收尾。
			// 计时器 goroutine 里数帧，结果经通道交回测试 goroutine（不能直接写共享变量，也不能在那里调 t.Fatal）。
			type midResult struct {
				n   int
				err error
			}
			midCh := make(chan midResult, 1)
			if release != nil {
				timer := time.AfterFunc(time.Duration(secs)*time.Second+time.Second, func() {
					n, _, err := countVideoFps(ffp, got)
					midCh <- midResult{n, err}
					release()
				})
				defer timer.Stop()
			}
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
			if release != nil {
				mid := <-midCh // 推流只有在 release 之后才能收尾，所以计时器一定已经触发
				if mid.err != nil {
					t.Fatal(mid.err)
				}
				midFrames := mid.n
				t.Logf("%s: 源时长 + 1 秒时主输出已收到 %d 帧", tc.name, midFrames)
				if midFrames < (secs-1)*30 {
					t.Fatalf("预览分支卡住时推流被拖慢：源时长 + 1 秒时只收到 %d 帧", midFrames)
				}
			}
			if wall > time.Duration(secs+4)*time.Second {
				t.Fatalf("推流被拖慢：%d 秒的源用了 %v", secs, wall)
			}
			// 断开卡住的预览连接后 tee 会报 "Slave muxer #1 failed"（预期，onfail=ignore），其余情况不该出现。
			if (release == nil && strings.Contains(pushErr.String(), "Slave muxer")) || strings.Contains(pushErr.String(), "Unknown option") {
				t.Fatalf("tee 分支打开失败:\n%s", tail(pushErr.String(), 20))
			}
			if pvBytes != nil {
				time.Sleep(200 * time.Millisecond)
				b := pvBytes()
				t.Logf("%s: 预览分支对端收到 %d 字节", tc.name, b)
				if tc.name == "预览开" && b < 100_000 {
					t.Fatalf("预览分支几乎没有数据: %d 字节", b)
				}
				if b <= 0 {
					t.Fatal("预览分支没有连上")
				}
			}

		})
	}
}

func readPreviewTCP(t *testing.T) (int, func() int64, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var n atomic.Int64
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 64<<10)
		for {
			k, err := c.Read(buf)
			n.Add(int64(k))
			if err != nil {
				return
			}
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, n.Load, nil
}

// stallPreviewTCP 接受连接后只读一次（证明连上了），之后完全不读；接收缓冲设成 4 KB，让 ffmpeg 很快写不进去。
func stallPreviewTCP(t *testing.T) (int, func() int64, func()) {
	t.Helper()
	lc := net.ListenConfig{Control: func(_, _ string, rc syscall.RawConn) error {
		var serr error
		rc.Control(func(fd uintptr) { serr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 4096) })
		return serr
	}}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var n atomic.Int64
	conns := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		k, _ := c.Read(make([]byte, 13))
		n.Add(int64(k))
		conns <- c
	}()
	release := func() {
		select {
		case c := <-conns:
			c.Close()
		default:
		}
	}
	t.Cleanup(release)
	return ln.Addr().(*net.TCPAddr).Port, n.Load, release
}

func tail(s string, n int) string {
	l := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, "\n")
}
