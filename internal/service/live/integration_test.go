//go:build !windows

package live

// 集成测试：真实 ffmpeg + 本地 MediaMTX。找不到二进制时 Skip（不让 CI 默认失败）。
//   FFMPEGFREE_MEDIAMTX  MediaMTX 可执行文件（默认 /workspace/tools/mediamtx/mediamtx，再找 PATH 里的 mediamtx）
//   FFMPEGFREE_FFMPEG    ffmpeg 可执行文件（默认 PATH 里的 ffmpeg，ffprobe 取同目录或 PATH）

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/task"
)

func findBin(t *testing.T, env string, defaults ...string) string {
	t.Helper()
	if p := os.Getenv(env); p != "" {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("%s=%s 不存在", env, p)
		}
		return p
	}
	for _, d := range defaults {
		if filepath.IsAbs(d) {
			if _, err := os.Stat(d); err == nil {
				return d
			}
		} else if p, err := exec.LookPath(d); err == nil {
			return p
		}
	}
	t.Skipf("找不到 %s（设置环境变量 %s 指向它才会运行集成测试）", defaults[len(defaults)-1], env)
	return ""
}

func freePort(t *testing.T, udp bool) int {
	t.Helper()
	if udp {
		c, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).Port
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

type mtx struct {
	cmd       *exec.Cmd
	rtmp, srt int
	hls       int
	dir       string
	logPath   string
}

// startMediaMTX 启动一个只开 RTMP / SRT 的 MediaMTX：路径 auth/ 需要账号 pub / secretpass 才能发布，其余路径可随意发布；所有人可读。
func startMediaMTX(t *testing.T) *mtx { return startMediaMTXHLS(t, "") }

// startMediaMTXHLS 同 startMediaMTX；hlsVariant 非空时另开 HLS（lowLatency / mpegts / fmp4），地址见 hlsURL。
func startMediaMTXHLS(t *testing.T, hlsVariant string) *mtx {
	bin := findBin(t, "FFMPEGFREE_MEDIAMTX", "/workspace/tools/mediamtx/mediamtx", "mediamtx")
	m := &mtx{dir: t.TempDir(), rtmp: freePort(t, false), srt: freePort(t, true)}
	hls := "hls: no"
	if hlsVariant != "" {
		m.hls = freePort(t, false)
		hls = fmt.Sprintf("hls: yes\nhlsAddress: 127.0.0.1:%d\nhlsVariant: %s\nhlsAlwaysRemux: yes", m.hls, hlsVariant)
	}
	cfg := fmt.Sprintf(`logLevel: info
api: no
metrics: no
pprof: no
playback: no
rtsp: no
%s
webrtc: no
moq: no
rtmp: yes
rtmpAddress: 127.0.0.1:%d
srt: yes
srtAddress: 127.0.0.1:%d
authMethod: internal
authInternalUsers:
  - user: any
    pass:
    permissions:
      - action: read
      - action: playback
  - user: any
    pass:
    permissions:
      - action: publish
        path: ~^live
  - user: pub
    pass: secretpass
    permissions:
      - action: publish
        path: ~^auth
paths:
  all_others:
`, hls, m.rtmp, m.srt)
	cfgPath := filepath.Join(m.dir, "mtx.yml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	m.logPath = filepath.Join(m.dir, "mtx.log")
	lf, _ := os.Create(m.logPath)
	m.cmd = exec.Command(bin, cfgPath)
	m.cmd.Dir = m.dir
	m.cmd.Stdout, m.cmd.Stderr = lf, lf
	m.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := m.cmd.Start(); err != nil {
		t.Skipf("启动 MediaMTX 失败: %v", err)
	}
	t.Cleanup(m.stop)
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", m.rtmp), 200*time.Millisecond)
		if err == nil {
			c.Close()
			return m
		}
		if time.Now().After(deadline) {
			b, _ := os.ReadFile(m.logPath)
			t.Skipf("MediaMTX 没有起来: %s", b)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (m *mtx) stop() {
	if m.cmd != nil && m.cmd.Process != nil {
		syscall.Kill(-m.cmd.Process.Pid, syscall.SIGKILL)
		m.cmd.Wait()
		m.cmd = nil
	}
}

func (m *mtx) rtmpURL(path string) string { return fmt.Sprintf("rtmp://127.0.0.1:%d/%s", m.rtmp, path) }
func (m *mtx) hlsURL(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/%s/index.m3u8", m.hls, path)
}
func (m *mtx) srtURL(q string) string { return fmt.Sprintf("srt://127.0.0.1:%d?%s", m.srt, q) }

type realFx struct {
	*fixture
	ffmpeg, ffprobe string
	withAudio       string
	noAudio         string
}

// newRealFixture 用真实 ffmpeg（Media 用 fakeMedia，按生成的源文件填 HasAudio）。
func newRealFixture(t *testing.T, grace time.Duration) *realFx {
	ff := findBin(t, "FFMPEGFREE_FFMPEG", "ffmpeg")
	probe := filepath.Join(filepath.Dir(ff), "ffprobe")
	if _, err := os.Stat(probe); err != nil {
		probe = findBin(t, "FFMPEGFREE_FFPROBE", "ffprobe")
	}
	out, _ := exec.Command(ff, "-hide_banner", "-encoders").Output()
	if !strings.Contains(string(out), "libx264") {
		t.Skip("ffmpeg 没有 libx264")
	}
	f := newFixture(t, func(c *Config) {
		c.Require = func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{FFmpeg: ff, FFprobe: probe}, nil }
		c.Protocols = &ffmpeg.ProtocolProbe{}
		if grace > 0 {
			c.Grace = grace
		}
	})
	rf := &realFx{fixture: f, ffmpeg: ff, ffprobe: probe}
	rf.withAudio = filepath.Join(f.dir, "av.mp4")
	rf.noAudio = filepath.Join(f.dir, "v.mp4")
	gen := func(dst string, audio bool, sec int) {
		args := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25"}
		if audio {
			args = append(args, "-f", "lavfi", "-i", "sine=frequency=440")
		}
		args = append(args, "-t", fmt.Sprint(sec), "-c:v", "libx264", "-pix_fmt", "yuv420p")
		if audio {
			args = append(args, "-c:a", "aac")
		}
		if b, err := exec.Command(ff, append(args, dst)...).CombinedOutput(); err != nil {
			t.Skipf("生成测试源失败: %v %s", err, b)
		}
	}
	gen(rf.withAudio, true, 20)
	gen(rf.noAudio, false, 3)
	return rf
}

func (r *realFx) push(t *testing.T, input, url string, loop bool, hasAudio bool) (task.Task, error) {
	r.svc.cfg.Media = fakeMedia{info: mediaInfo(hasAudio)}
	return r.svc.StartFilePush(context.Background(), FilePushRequest{InputPath: input, URL: url, Loop: loop,
		Options: PushOptions{VideoBitrateKbps: 500, Fps: 25}})
}

// probeStream 用 ffprobe 从服务器拉流，返回读到的流数量；拉不到返回 0。
func (r *realFx) probeStream(t *testing.T, url string) int {
	// 推流端的首个 progress 与服务器端登记流之间有竞态，重试几次。
	n := 0
	for i := 0; i < 8 && n == 0; i++ {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		n = r.probeOnce(t, url)
	}
	return n
}

func (r *realFx) probeOnce(t *testing.T, url string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, r.ffprobe, "-v", "error", "-rw_timeout", "5000000", "-show_entries", "stream=codec_type", "-of", "csv=p=0", url).CombinedOutput()
	if err != nil {
		t.Logf("ffprobe %v: %s", err, out)
		return 0
	}
	return len(strings.Fields(string(out)))
}

func TestIntegrationRTMPSuccessPullAndGracefulStop(t *testing.T) {
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	tk, err := r.push(t, r.withAudio, m.rtmpURL("live/ok"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	r.waitProgress(t, tk.ID)
	if n := r.probeStream(t, m.rtmpURL("live/ok")); n < 2 {
		b, _ := os.ReadFile(m.logPath)
		t.Logf("mediamtx log:\n%s", b)
		t.Fatalf("从 MediaMTX 拉流应读到音视频两路，实际 %d 路", n)
	}
	begin := time.Now()
	r.mgr.Cancel(tk.ID)
	d := r.wait(t, tk.ID)
	if d.Status != task.StatusSucceeded || d.Error != nil {
		t.Fatalf("优雅停止应 succeeded 且 error 为空: %+v %+v", d, d.Error)
	}
	if el := time.Since(begin); el > 4*time.Second {
		t.Fatalf("q 之后应很快退出，实际 %v", el)
	}
	assertStatusPayloadsHaveNoError(t, r.fixture, tk.ID, task.StatusSucceeded)
	t.Logf("日志尾部:\n%s", tailLines(r.logText(t, tk.ID), 6))
}

func TestIntegrationSRTSuccessPull(t *testing.T) {
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	tk, err := r.push(t, r.withAudio, m.srtURL("streamid=publish:live/srt1"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	r.waitProgress(t, tk.ID)
	if n := r.probeStream(t, m.srtURL("streamid=read:live/srt1")); n < 2 {
		t.Fatalf("SRT 拉流应读到音视频，实际 %d 路", n)
	}
	r.mgr.Cancel(tk.ID)
	if d := r.wait(t, tk.ID); d.Status != task.StatusSucceeded || d.Error != nil {
		t.Fatalf("%+v %+v", d, d.Error)
	}
}

// 无音轨源补 anullsrc：不加 -shortest 会永远不结束；loop=false 应在源播完后自然结束（succeeded）。
func TestIntegrationNoAudioSourceEndsNaturally(t *testing.T) {
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	begin := time.Now()
	tk, err := r.push(t, r.noAudio, m.rtmpURL("live/noaudio"), false, false)
	if err != nil {
		t.Fatal(err)
	}
	d := r.wait(t, tk.ID)
	if d.Status != task.StatusSucceeded || d.Error != nil {
		t.Fatalf("播完应自然 succeeded: %+v %+v\n%s", d, d.Error, tailLines(r.logText(t, tk.ID), 8))
	}
	if el := time.Since(begin); el > 8*time.Second {
		t.Fatalf("3 秒的源应在几秒内结束，实际 %v（-shortest 没生效？）", el)
	}
}

func TestIntegrationConnectRefused(t *testing.T) {
	r := newRealFixture(t, 0)
	port := freePort(t, false) // 关掉监听后没有服务
	for _, tc := range []struct{ name, url, scheme string }{
		{"rtmp", fmt.Sprintf("rtmp://127.0.0.1:%d/live/SECRETKEYabc", port), "rtmp"},
		{"srt", fmt.Sprintf("srt://127.0.0.1:%d?streamid=publish:live/SECRETKEYabc", freePort(t, true)), "srt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tk, err := r.push(t, r.withAudio, tc.url, true, true)
			if err != nil {
				t.Fatal(err)
			}
			d := r.wait(t, tk.ID)
			if d.Status != task.StatusFailed || d.Error == nil || d.Error.Code != apperr.LiveConnectFailed || firstLine(d.Error.Detail) != "scheme="+tc.scheme {
				t.Fatalf("%+v %+v", d, d.Error)
			}
			if r.progressed(tk.ID) {
				t.Fatal("连接失败前不应有 task:progress")
			}
			assertNoSecrets(t, d.Error.Detail+d.Title+d.Params+r.logText(t, tk.ID)+r.em.jsonAll())
			t.Logf("%s detail:\n%s", tc.name, d.Error.Detail)
		})
	}
}

func TestIntegrationAuthRejected(t *testing.T) {
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	// 正确口令可以推
	ok, err := r.push(t, r.withAudio, m.rtmpURL("auth/k1?user=pub&pass=secretpass"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	r.waitProgress(t, ok.ID)
	r.mgr.Cancel(ok.ID)
	if d := r.wait(t, ok.ID); d.Status != task.StatusSucceeded {
		t.Fatalf("%+v %+v", d, d.Error)
	}
	// 错口令 → LIVE_PUSH_REJECTED
	tk, err := r.push(t, r.withAudio, m.rtmpURL("auth/k2?user=pub&pass=WRONGpass"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	d := r.wait(t, tk.ID)
	if d.Status != task.StatusFailed || d.Error == nil || d.Error.Code != apperr.LivePushRejected {
		t.Fatalf("%+v %+v", d, d.Error)
	}
	t.Logf("rejected detail:\n%s", d.Error.Detail)
	for _, s := range []string{"WRONGpass", "secretpass", "pass=", "k2"} {
		if strings.Contains(d.Error.Detail+d.Title+d.Params+r.logText(t, tk.ID)+r.em.jsonAll(), s) && s != "pass=" {
			t.Fatalf("泄露 %q", s)
		}
	}
}

func TestIntegrationServerKilledMidStream(t *testing.T) {
	m := startMediaMTX(t)
	r := newRealFixture(t, 0)
	tk, err := r.push(t, r.withAudio, m.rtmpURL("live/mid"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	r.waitProgress(t, tk.ID)
	time.Sleep(500 * time.Millisecond)
	m.stop()
	d := r.wait(t, tk.ID)
	if d.Error != nil {
		t.Logf("interrupted detail:\n%s", d.Error.Detail)
	}
	if d.Status != task.StatusInterrupted || d.Error == nil || d.Error.Code != apperr.LivePushInterrupted {
		t.Fatalf("推流中途杀掉服务器应 LIVE_PUSH_INTERRUPTED: %+v %+v\n%s", d, d.Error, tailLines(r.logText(t, tk.ID), 8))
	}
}

func TestIntegrationForceKillIsCanceled(t *testing.T) {
	m := startMediaMTX(t)
	r := newRealFixture(t, time.Millisecond) // 宽限期 1ms：q 之后几乎立即强杀
	tk, err := r.push(t, r.withAudio, m.rtmpURL("live/force"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	r.waitProgress(t, tk.ID)
	r.mgr.Cancel(tk.ID)
	d := r.wait(t, tk.ID)
	if d.Status != task.StatusCanceled || d.Error != nil {
		t.Fatalf("强杀应 canceled 且 error 为空: %+v %+v", d, d.Error)
	}
	assertStatusPayloadsHaveNoError(t, r.fixture, tk.ID, task.StatusCanceled)
}

func tailLines(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, "\n")
}

// 取消之后 ffmpeg 非零退出 → canceled（不是 failed / LIVE_PUSH_INTERRUPTED）。真实 ffmpeg：先 SIGKILL 服务器（不等待），
// 紧接着 Cancel；ffmpeg 收到 q 后写文件尾时 Broken pipe，以非零码退出（实测 224），契约要求"取消后非零一律 canceled"。
// 服务器死亡到 ffmpeg 下一次写包之间有约一个包间隔（几十毫秒），Cancel 在微秒级发出，所以不会误判成断流。
func TestIntegrationNonzeroExitAfterCancelIsCanceled(t *testing.T) {
	for i := 0; i < 3; i++ {
		m := startMediaMTX(t)
		r := newRealFixture(t, 0)
		tk, err := r.push(t, r.withAudio, m.rtmpURL("live/nz"), true, true)
		if err != nil {
			t.Fatal(err)
		}
		r.waitProgress(t, tk.ID)
		time.Sleep(300 * time.Millisecond)
		syscall.Kill(-m.cmd.Process.Pid, syscall.SIGKILL)
		if err := r.mgr.Cancel(tk.ID); err != nil {
			t.Fatal(err)
		}
		d := r.wait(t, tk.ID)
		if d.Status != task.StatusCanceled || d.Error != nil {
			t.Fatalf("第 %d 次：取消后非零退出应 canceled 且 error 为空: %+v %+v\n%s", i, d, d.Error, tailLines(r.logText(t, tk.ID), 6))
		}
		assertStatusPayloadsHaveNoError(t, r.fixture, tk.ID, task.StatusCanceled)
		t.Logf("第 %d 次日志尾部:\n%s", i, tailLines(r.logText(t, tk.ID), 3))
		m.stop()
	}
}
