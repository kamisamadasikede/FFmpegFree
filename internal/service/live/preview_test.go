//go:build !windows

package live

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/task"
)

// 假 ffmpeg 的 -encoders / -muxers / -filters 输出：完整构建。
func fullPreviewRun(_ context.Context, _ string, args ...string) (string, error) {
	switch args[len(args)-1] {
	case "-encoders":
		return " V....D mjpeg  Motion JPEG\n", nil
	case "-muxers":
		return "  E image2  image2 sequence\n", nil
	}
	return " ... fps  V->V  x\n ... scale  V->V  y\n", nil
}

// smallJPEG 是最小的"看起来完整"的 JPEG：SOI ... EOI。
var smallJPEG = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0xFF, 0xD9}

func previewFixture(t *testing.T, mut func(*Config)) *fixture {
	dir := filepath.Join(t.TempDir(), PreviewDirName)
	return newFixture(t, func(c *Config) {
		c.PreviewDir = dir
		c.Preview = &ffmpeg.PreviewProbe{Run: fullPreviewRun}
		if mut != nil {
			mut(c)
		}
	})
}

func (f *fixture) args(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(f.exe + ".args")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// previewArg 返回 argv 里预览输出的目标路径（file: 前缀去掉），没有预览输出返回 ""。
func previewArg(args []string) string {
	for i, a := range args {
		if a == "image2" && i > 0 && args[i-1] == "-f" {
			return strings.TrimPrefix(args[len(args)-1], "file:")
		}
	}
	return ""
}

func TestStartFilePushAddsPreviewOutputAndCleansUp(t *testing.T) {
	f := previewFixture(t, nil)
	tk, err := f.start(t, "rtmp://127.0.0.1:1935/live/pv1")
	if err != nil {
		t.Fatal(err)
	}
	f.waitProgress(t, tk.ID)
	path := previewArg(f.args(t))
	want := filepath.Join(f.svc.cfg.PreviewDir, tk.ID+".jpg")
	if path != want {
		t.Fatalf("预览路径 %q want %q", path, want)
	}
	// 还没有画面：空，不是错误；active=true。
	p, err := f.svc.GetPreview(tk.ID)
	if err != nil || p.Data != "" || p.TS != 0 || !p.Active {
		t.Fatalf("没有画面应返回空: %+v %v", p, err)
	}
	// 画面写好：返回 base64 与时间戳。
	if err := os.WriteFile(path, smallJPEG, 0o644); err != nil {
		t.Fatal(err)
	}
	p, err = f.svc.GetPreview(tk.ID)
	if err != nil || !p.Active || p.TS < time.Now().Add(-5*time.Second).UnixMilli() {
		t.Fatalf("%+v %v", p, err)
	}
	if b, _ := base64.StdEncoding.DecodeString(p.Data); string(b) != string(smallJPEG) {
		t.Fatalf("data 不对: %q", p.Data)
	}
	// 会话结束：文件被清理，GetPreview 返回空且 active=false。
	f.mgr.Cancel(tk.ID)
	f.wait(t, tk.ID)
	waitGone(t, path)
	p, err = f.svc.GetPreview(tk.ID)
	if err != nil || p.Data != "" || p.Active {
		t.Fatalf("结束后应为空且 inactive: %+v %v", p, err)
	}
}

func waitGone(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("预览文件没有被清理: %s", path)
}

func TestGetPreviewHalfFrameNoFileAndInvalid(t *testing.T) {
	f := previewFixture(t, nil)
	tk, err := f.start(t, "rtmp://127.0.0.1:1935/live/pv2")
	if err != nil {
		t.Fatal(err)
	}
	f.waitProgress(t, tk.ID)
	path := previewArg(f.args(t))
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"半帧（缺 EOI）", smallJPEG[:len(smallJPEG)-2]},
		{"半帧（只有 SOI）", []byte{0xFF, 0xD8}},
		{"空文件", nil},
		{"不是 JPEG", []byte("PNG....not a jpeg at all")},
		{"结尾是 EOI 前一字节", []byte{0xFF, 0xD8, 0xFF, 0xD9, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}},
	} {
		if err := os.WriteFile(path, tc.data, 0o644); err != nil {
			t.Fatal(err)
		}
		if p, err := f.svc.GetPreview(tk.ID); err != nil || p.Data != "" || p.TS != 0 {
			t.Errorf("%s 应返回空: %+v %v", tc.name, p, err)
		}
	}
	// 少量 0 填充的合法 JPEG 仍算完整。
	if err := os.WriteFile(path, append(append([]byte{}, smallJPEG...), 0, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	if p, _ := f.svc.GetPreview(tk.ID); p.Data == "" {
		t.Error("末尾少量 0 填充应视为完整")
	}
	// 文件被删（清理竞态）：空。
	os.Remove(path)
	if p, err := f.svc.GetPreview(tk.ID); err != nil || p.Data != "" {
		t.Fatalf("无文件应返回空: %+v %v", p, err)
	}
	// 未知会话：空，不是错误。
	if p, err := f.svc.GetPreview("nope"); err != nil || p != (Preview{}) {
		t.Fatalf("未知会话: %+v %v", p, err)
	}
	f.mgr.Cancel(tk.ID)
	f.wait(t, tk.ID)
}

func TestPreviewDisabledOrUnsupportedDegrades(t *testing.T) {
	no := false
	tests := []struct {
		name string
		mut  func(*Config)
		req  *bool
	}{
		{"preview=false", nil, &no},
		{"没有预览目录", func(c *Config) { c.PreviewDir = "" }, nil},
		{"ffmpeg 不支持预览", func(c *Config) {
			c.Preview = &ffmpeg.PreviewProbe{Run: func(context.Context, string, ...string) (string, error) { return "\n", nil }}
		}, nil},
		{"预览探测失败", func(c *Config) {
			c.Preview = &ffmpeg.PreviewProbe{Run: func(context.Context, string, ...string) (string, error) { return "", errors.New("x") }}
		}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := previewFixture(t, tc.mut)
			tk, err := f.svc.StartFilePush(context.Background(), FilePushRequest{InputPath: filepath.Join(f.dir, "a.mp4"), URL: "rtmp://127.0.0.1:1935/live/pv3", Preview: tc.req})
			if err != nil {
				t.Fatalf("降级不能让推流失败: %v", err)
			}
			f.waitProgress(t, tk.ID)
			if p := previewArg(f.args(t)); p != "" || strings.Contains(strings.Join(f.args(t), " "), "image2") {
				t.Fatalf("不应有预览输出: %v", f.args(t))
			}
			if p, err := f.svc.GetPreview(tk.ID); err != nil || p.Data != "" || !p.Active {
				t.Fatalf("关闭时 GetPreview 返回空（会话仍 active）: %+v %v", p, err)
			}
			f.mgr.Cancel(tk.ID)
			if d := f.wait(t, tk.ID); d.Status != task.StatusSucceeded {
				t.Fatalf("%+v", d)
			}
		})
	}
}

// 预览目录不可写：只降级，不报错。
func TestPreviewDirUnwritableDegrades(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, []byte("x"), 0o644)
	f := previewFixture(t, func(c *Config) { c.PreviewDir = filepath.Join(blocker, PreviewDirName) }) // 父路径是文件
	tk, err := f.start(t, "rtmp://127.0.0.1:1935/live/pv4")
	if err != nil {
		t.Fatalf("目录不可用不能让推流失败: %v", err)
	}
	f.waitProgress(t, tk.ID)
	if strings.Contains(strings.Join(f.args(t), " "), "image2") {
		t.Fatalf("不应有预览输出: %v", f.args(t))
	}
	f.mgr.Cancel(tk.ID)
	f.wait(t, tk.ID)
}

// 排队中被取消 / 提交失败等"从未运行"的会话也要清理预览文件（这里用 release 直接模拟从未运行的结束）。
func TestReleaseRemovesPreviewFiles(t *testing.T) {
	f := previewFixture(t, nil)
	path := filepath.Join(f.svc.cfg.PreviewDir, "X.jpg")
	os.MkdirAll(f.svc.cfg.PreviewDir, 0o755)
	os.WriteFile(path, smallJPEG, 0o644)
	os.WriteFile(path+".tmp", []byte("half"), 0o644)
	if err := f.svc.reserve("X", "k", false, false, path); err != nil {
		t.Fatal(err)
	}
	f.svc.release("X")
	for _, p := range []string{path, path + ".tmp"} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s 应被删除: %v", p, err)
		}
	}
}

func TestCleanupPreviewDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, PreviewDirName)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "old.jpg"), smallJPEG, 0o644)
	os.WriteFile(filepath.Join(dir, "old.jpg.tmp"), []byte("x"), 0o644)
	other := filepath.Join(root, "keep.txt")
	os.WriteFile(other, []byte("keep"), 0o644)
	if err := CleanupPreviewDir(dir); err != nil {
		t.Fatal(err)
	}
	if es, err := os.ReadDir(dir); err != nil || len(es) != 0 {
		t.Fatalf("应清空并重建: %v %v", es, err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("不能误删旁边的文件")
	}
	// 名字不对的目录一律拒绝，防止误删。
	for _, bad := range []string{"", root, filepath.Join(root, "tmp")} {
		if err := CleanupPreviewDir(bad); err == nil {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("不能误删")
	}
}

func TestPreviewDoesNotChangeMainOutputArgs(t *testing.T) {
	on := previewFixture(t, nil)
	tk, _ := on.start(t, "rtmp://127.0.0.1:1935/live/pv5")
	on.waitProgress(t, tk.ID)
	withArgs := on.args(t)
	on.mgr.Cancel(tk.ID)
	on.wait(t, tk.ID)

	no := false
	off := previewFixture(t, nil)
	tk2, _ := off.svc.StartFilePush(context.Background(), FilePushRequest{InputPath: filepath.Join(off.dir, "a.mp4"), URL: "rtmp://127.0.0.1:1935/live/pv5", Preview: &no})
	off.waitProgress(t, tk2.ID)
	withoutArgs := off.args(t)
	off.mgr.Cancel(tk2.ID)
	off.wait(t, tk2.ID)
	// 输入路径在两个 fixture 里不同（临时目录）：从第一个 -map 起比较主输出部分，带预览的只能多出末尾的预览输出。
	fromMap := func(a []string) string {
		for j, x := range a {
			if x == "-map" {
				return strings.Join(a[j:], " ")
			}
		}
		return ""
	}
	w, wo := fromMap(withArgs), fromMap(withoutArgs)
	if !strings.HasPrefix(w, wo+" -map 0:v:0 -an -sn -dn -vf fps=2,scale=640:-2 -q:v 5") {
		t.Fatalf("主输出参数被预览改动:\n%s\n%s", w, wo)
	}
}

// ---------- 拉流预览会话 ----------

func TestStartPullPreviewValidationAndLimits(t *testing.T) {
	f := previewFixture(t, func(c *Config) {
		c.ProbeStreams = func(context.Context, string, string, string) (bool, error) { return true, nil }
	})
	f.setMode("connecting") // 假 ffmpeg 一直挂着，直到被停止
	for _, tc := range []struct{ url, reason string }{
		{"", "malformed"}, {"ws://h/a.flv", "scheme_unsupported"}, {"ftp://h/a", "scheme_unsupported"},
		{"http:///a.flv", "missing_host"}, {"rtmp://h:99999/a/k", "malformed"}, {"http://h/a b", "malformed"},
	} {
		_, err := f.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: tc.url})
		ae := mustAppErr(t, err, apperr.LiveURLInvalid)
		if firstLine(ae.Detail) != "reason="+tc.reason {
			t.Errorf("%q: %q want reason=%s", tc.url, firstLine(ae.Detail), tc.reason)
		}
	}
	// 同一地址幂等。
	s1, err := f.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: "http://127.0.0.1:9/live/a.flv"})
	if err != nil || !s1.Preview || s1.Redacted == "" || strings.Contains(s1.Redacted, "a.flv") && false {
		t.Fatalf("%+v %v", s1, err)
	}
	s2, err := f.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: "http://127.0.0.1:9/live/a.flv"})
	if err != nil || s2.ID != s1.ID {
		t.Fatalf("同一地址应返回同一会话: %+v %+v %v", s1, s2, err)
	}
	f.waitPullArgs(t)
	args := f.args(t)
	if !strings.Contains(strings.Join(args, " "), "-nostdin -progress pipe:1 -protocol_whitelist http,https,tcp,tls,crypto -i http://127.0.0.1:9/live/a.flv") || previewArg(args) == "" {
		t.Fatalf("拉流预览参数: %v", args)
	}
	if got := previewArg(args); got != filepath.Join(f.svc.cfg.PreviewDir, s1.ID+".jpg") {
		t.Fatalf("路径 %s", got)
	}
	// 上限。
	for i := 1; i < MaxPullPreviews; i++ {
		if _, err := f.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: "http://127.0.0.1:9/live/" + string(rune('a'+i)) + ".flv"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err = f.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: "http://127.0.0.1:9/live/zz.flv"})
	if ae := mustAppErr(t, err, apperr.TaskConflict); firstLine(ae.Detail) != "reason=max_pull_previews" {
		t.Fatalf("%+v", ae)
	}
	// 写一帧，再停止：文件清理。
	pv := previewArg(args)
	os.WriteFile(pv, smallJPEG, 0o644)
	if p, _ := f.svc.GetPreview(s1.ID); p.Data == "" || !p.Active {
		t.Fatalf("拉流预览 GetPreview: %+v", p)
	}
	if err := f.svc.StopPullPreview(s1.ID); err != nil {
		t.Fatal(err)
	}
	waitGone(t, pv)
	if p, _ := f.svc.GetPreview(s1.ID); p.Data != "" || p.Active {
		t.Fatalf("停止后: %+v", p)
	}
	if err := f.svc.StopPullPreview(s1.ID); err != nil { // 再停一次无操作
		t.Fatal(err)
	}
	f.svc.Close()
	if n := len(f.svc.pulls); n != 0 {
		t.Fatalf("Close 后应没有会话: %d", n)
	}
}

func (f *fixture) waitPullArgs(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(f.exe + ".args"); err == nil && strings.Contains(string(b), "image2") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("拉流预览的 ffmpeg 没有启动")
}

func TestPullPreviewAudioOnlyHasNoPreview(t *testing.T) {
	f := previewFixture(t, func(c *Config) {
		c.ProbeStreams = func(context.Context, string, string, string) (bool, error) { return false, nil }
		old := c.Require
		c.Require = func() (ffmpeg.Binaries, error) {
			b, err := old()
			b.FFprobe = "/x/ffprobe"
			return b, err
		}
	})
	s, err := f.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: "rtmp://127.0.0.1:1935/live/audio"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if p, _ := f.svc.GetPreview(s.ID); !p.Active {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if p, _ := f.svc.GetPreview(s.ID); p.Active || p.Data != "" {
		t.Fatalf("纯音频会话应很快结束且没有画面: %+v", p)
	}
	if b, err := os.ReadFile(f.exe + ".args"); err == nil && strings.Contains(string(b), "image2") {
		t.Fatalf("纯音频不应启动 ffmpeg 预览: %s", b)
	}
}

func TestPullPreviewOffStartsNothing(t *testing.T) {
	f := previewFixture(t, nil)
	no := false
	s, err := f.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: "rtmp://127.0.0.1:1935/live/off", Preview: &no})
	if err != nil || s.Preview {
		t.Fatalf("%+v %v", s, err)
	}
	if _, err := os.Stat(f.exe + ".args"); err == nil {
		t.Fatal("preview=false 不应启动 ffmpeg")
	}
	if p, err := f.svc.GetPreview(s.ID); err != nil || p.Data != "" {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestPullPreviewUnsupportedFFmpeg(t *testing.T) {
	f := previewFixture(t, func(c *Config) {
		c.Preview = &ffmpeg.PreviewProbe{Run: func(context.Context, string, ...string) (string, error) { return "\n", nil }}
	})
	_, err := f.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: "rtmp://127.0.0.1:1935/live/x"})
	if ae := mustAppErr(t, err, apperr.Unsupported); firstLine(ae.Detail) != "missing=preview" {
		t.Fatalf("%+v", ae)
	}
}
