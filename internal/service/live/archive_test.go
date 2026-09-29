//go:build !windows

package live

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/fsutil"
	"FFmpegFree/internal/task"
)

var fixedNow = time.Date(2026, 9, 29, 20, 0, 0, 0, time.Local)

// archiveFixture：屏幕推流（x11）+ 固定时间 + 可注入的 ffprobe / 删除。
type archiveFixture struct {
	*fixture
	dir       string // archiveDir
	mu        sync.Mutex
	probe     func(path string) error // nil = 读得出
	removed   []string
	removeErr error
	orderLog  []string
}

func newArchiveFixture(t *testing.T) *archiveFixture {
	af := &archiveFixture{}
	af.fixture = x11Fixture(t, func(c *Config) {
		c.Now = func() time.Time { return fixedNow }
		c.ProbeDuration = func(_ context.Context, _ string, p string) (float64, error) {
			af.mu.Lock()
			f := af.probe
			af.mu.Unlock()
			if f != nil {
				if err := f(p); err != nil {
					return 0, err
				}
			}
			return 1.5, nil
		}
		c.Remove = func(p string) error {
			af.mu.Lock()
			defer af.mu.Unlock()
			af.removed = append(af.removed, p)
			af.orderLog = append(af.orderLog, "remove")
			if af.removeErr != nil {
				return af.removeErr
			}
			return os.Remove(p)
		}
	})
	af.dir = filepath.Join(af.fixture.dir, "arch")
	return af
}

func (af *archiveFixture) start(t *testing.T, url string) (task.Task, error) {
	return af.svc.StartScreenPush(context.Background(), ScreenPushRequest{URL: url, ArchiveDir: af.dir})
}

func (af *archiveFixture) argsLine() string {
	b, _ := os.ReadFile(af.exe + ".args")
	return strings.ReplaceAll(strings.TrimSpace(string(b)), "\n", " ")
}

// 存档文件名固定 screen-<yyyyMMdd-HHmmss>，净化是恒等变换（契约 6.10 第 4 条）。
func TestArchiveBaseNameIsIdentityUnderSanitize(t *testing.T) {
	for _, tm := range []time.Time{fixedNow, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), time.Date(1999, 12, 31, 23, 59, 59, 0, time.Local)} {
		raw := archivePrefix + tm.Format(archiveLayout)
		if got := archiveBaseName(tm); got != raw || fsutil.SanitizeArchiveName(raw) != raw {
			t.Fatalf("净化前后应相等: %q → %q", raw, archiveBaseName(tm))
		}
	}
	if archiveBaseName(fixedNow) != "screen-20260929-200000" {
		t.Fatal(archiveBaseName(fixedNow))
	}
}

func TestReserveArchiveExclusiveAndSuffix(t *testing.T) {
	af := newArchiveFixture(t)
	// 目录不存在会创建
	p0, err := af.svc.reserveArchive(af.dir)
	if err != nil || filepath.Base(p0) != "screen-20260929-200000.mp4" {
		t.Fatalf("%q %v", p0, err)
	}
	// 已有同名文件（预先放好带内容的）：不覆盖、不碰，加 (n)
	if err := os.WriteFile(p0, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	p1, err := af.svc.reserveArchive(af.dir)
	if err != nil || filepath.Base(p1) != "screen-20260929-200000(1).mp4" {
		t.Fatalf("%q %v", p1, err)
	}
	p2, err := af.svc.reserveArchive(af.dir)
	if err != nil || filepath.Base(p2) != "screen-20260929-200000(2).mp4" {
		t.Fatalf("%q %v", p2, err)
	}
	if b, _ := os.ReadFile(p0); string(b) != "precious" {
		t.Fatal("已有文件被改动")
	}
	for _, p := range []string{p1, p2} {
		if fi, err := os.Stat(p); err != nil || fi.Size() != 0 {
			t.Fatalf("占位应是 0 字节空文件: %v", err)
		}
	}
	// 并发占位：全部拿到不同的路径
	var wg sync.WaitGroup
	got := make([]string, 16)
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i], _ = af.svc.reserveArchive(af.dir)
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, p := range got {
		if p == "" || seen[p] {
			t.Fatalf("并发占位重复或失败: %v", got)
		}
		seen[p] = true
	}
}

func TestCheckArchiveDir(t *testing.T) {
	for _, bad := range []string{`\\?\C:\x`, `\\.\PhysicalDrive0`, "//?/C:/x", "//./COM1", "relative/dir", "./x", "/tmp/a\nb"} {
		if _, err := checkArchiveDir(bad); !apperr.Is(err, apperr.InvalidArgument) {
			t.Errorf("%q 应 INVALID_ARGUMENT: %v", bad, err)
		}
	}
	if d, err := checkArchiveDir("/tmp/x/../y/"); err != nil || d != "/tmp/y" {
		t.Fatalf("%q %v", d, err)
	}
	af := newArchiveFixture(t)
	// 存档目录是个文件
	file := filepath.Join(af.fixture.dir, "afile")
	os.WriteFile(file, []byte("x"), 0o644)
	if _, err := af.svc.reserveArchive(file); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("目录是文件: %v", err)
	}
	if _, err := af.svc.reserveArchive(filepath.Join(file, "sub")); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("路径中间是文件: %v", err)
	}
	// Start 层：相对路径同步拒绝，不建任务不占会话
	_, err := af.svc.StartScreenPush(context.Background(), ScreenPushRequest{URL: "rtmp://127.0.0.1:1935/live/a", ArchiveDir: "rel"})
	mustAppErr(t, err, apperr.InvalidArgument)
	if n, _ := af.svc.ActiveSessions(); n != 0 {
		t.Fatal("失败不应占会话")
	}
}

// 缺 tee：UNSUPPORTED，detail 单独一行 missing=tee；没有存档时不需要 tee。
func TestArchiveMissingTeeIsUnsupported(t *testing.T) {
	af := newArchiveFixture(t)
	af.svc.cfg.Protocols = &ffmpeg.ProtocolProbe{Run: func(context.Context, string, ...string) (string, error) {
		return "Output:\n  file\n  rtmp\n  srt\n", nil
	}}
	_, err := af.start(t, "rtmp://127.0.0.1:1935/live/a")
	ae := mustAppErr(t, err, apperr.Unsupported)
	if ae.Detail != "missing=tee" {
		t.Fatalf("detail 应只有一行 missing=tee: %q", ae.Detail)
	}
	if _, statErr := os.Stat(af.dir); statErr == nil {
		if ents, _ := os.ReadDir(af.dir); len(ents) != 0 {
			t.Fatalf("缺 tee 不应留下占位文件: %v", ents)
		}
	}
	if n, _ := af.svc.ActiveSessions(); n != 0 {
		t.Fatal("失败不应占会话")
	}
	// 无存档不需要 tee
	af.setMode("live")
	tk, err := af.svc.StartScreenPush(context.Background(), ScreenPushRequest{URL: "rtmp://127.0.0.1:1935/live/a"})
	if err != nil {
		t.Fatalf("无存档不检查 tee: %v", err)
	}
	af.mgr.Cancel(tk.ID)
	af.wait(t, tk.ID)
}

func TestArchiveCommandLineAndTaskFields(t *testing.T) {
	af := newArchiveFixture(t)
	af.setMode("live2")
	tk, err := af.start(t, "rtmp://127.0.0.1:1935/live/SECRETKEYabc?token=TOKENxyz")
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(af.dir, "screen-20260929-200000.mp4")
	if tk.OutputPath != wantPath {
		t.Fatalf("outputPath = 存档最终路径: %q", tk.OutputPath)
	}
	af.waitProgress(t, tk.ID)
	line := af.argsLine()
	for _, want := range []string{"-flags +global_header -f tee ", "[f=flv:onfail=abort:protocol_whitelist=rtmp,tcp]", "[f=mp4:onfail=abort:movflags=+frag_keyframe+empty_moov:flush_packets=1:protocol_whitelist=file]file:" + wantPath} {
		if !strings.Contains(line, want) {
			t.Errorf("缺少 %q:\n%s", want, line)
		}
	}
	if !strings.Contains(tk.Params, `"archiveDir":`) || strings.Contains(tk.Params, "SECRETKEY") {
		t.Fatalf("params: %s", tk.Params)
	}
	// 有存档时没有 bitrateKbps（tee 下 total_size 为 N/A，不轮询补）；fps 等其他指标照常
	time.Sleep(100 * time.Millisecond)
	f := af.em
	f.mu.Lock()
	for _, e := range f.evts {
		if p, ok := e.payload.(task.ProgressEvent); ok && p.ID == tk.ID && p.BitrateKbps != 0 {
			t.Fatalf("有存档不应有 bitrateKbps: %+v", p)
		}
	}
	f.mu.Unlock()
	if n, hasArchive := af.svc.ActiveSessions(); n != 1 || !hasArchive {
		t.Fatalf("应报告有带存档的会话（Shutdown 等 16 秒）: %d %v", n, hasArchive)
	}
	af.mgr.Cancel(tk.ID)
	af.wait(t, tk.ID)
	if n, hasArchive := af.svc.ActiveSessions(); n != 0 || hasArchive {
		t.Fatal("结束后释放")
	}
}

// 无存档会话：ActiveSessions 不报告存档（Shutdown 仍是 8 秒）；有存档时 rate 也随之缺省，无存档有值。
func TestNoArchiveStillHasBitrate(t *testing.T) {
	af := newArchiveFixture(t)
	af.setMode("live2")
	tk, err := af.svc.StartScreenPush(context.Background(), ScreenPushRequest{URL: "rtmp://127.0.0.1:1935/live/a"})
	if err != nil {
		t.Fatal(err)
	}
	af.waitProgress(t, tk.ID)
	if _, hasArchive := af.svc.ActiveSessions(); hasArchive {
		t.Fatal("无存档不应标记")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		af.em.mu.Lock()
		got := false
		for _, e := range af.em.evts {
			if p, ok := e.payload.(task.ProgressEvent); ok && p.ID == tk.ID && p.BitrateKbps > 0 {
				got = true
			}
		}
		af.em.mu.Unlock()
		if got {
			af.mgr.Cancel(tk.ID)
			af.wait(t, tk.ID)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("无存档应有 bitrateKbps")
}

// 强杀（宽限期到）后存档有内容：canceled + outputPath 非空，error 为空。
func TestArchiveKeptAfterForceKill(t *testing.T) {
	af := newArchiveFixture(t)
	af.setMode("ignoreq")
	af.svc.cfg.Grace = 300 * time.Millisecond
	tk, err := af.start(t, "rtmp://127.0.0.1:1935/live/a")
	if err != nil {
		t.Fatal(err)
	}
	af.waitProgress(t, tk.ID)
	os.WriteFile(tk.OutputPath, []byte("moov+fragments"), 0o644)
	af.mgr.Cancel(tk.ID)
	d := af.wait(t, tk.ID)
	if d.Status != task.StatusCanceled || d.Error != nil || d.OutputPath != tk.OutputPath {
		t.Fatalf("强杀且存档保留应 canceled + outputPath 非空: %+v %+v", d, d.Error)
	}
	assertStatusPayloadsHaveNoError(t, af.fixture, tk.ID, task.StatusCanceled)
	if ev := af.em.statusEvents(tk.ID); ev[len(ev)-1].OutputPath != tk.OutputPath {
		t.Fatalf("终态事件应带 outputPath: %+v", ev[len(ev)-1])
	}
	if _, err := os.Stat(tk.OutputPath); err != nil || len(af.removed) != 0 {
		t.Fatalf("存档不应被删: %v %v", err, af.removed)
	}
}

// 正常停止（q 后退出码 0）：succeeded，存档保留。
func TestArchiveKeptAfterGracefulStop(t *testing.T) {
	af := newArchiveFixture(t)
	af.setMode("live")
	tk, _ := af.start(t, "rtmp://127.0.0.1:1935/live/a")
	af.waitProgress(t, tk.ID)
	os.WriteFile(tk.OutputPath, []byte("data"), 0o644)
	af.mgr.Cancel(tk.ID)
	d := af.wait(t, tk.ID)
	if d.Status != task.StatusSucceeded || d.Error != nil || d.OutputPath != tk.OutputPath || len(af.removed) != 0 {
		t.Fatalf("%+v %+v %v", d, d.Error, af.removed)
	}
}

// 空壳（ffprobe 读不出时长）：删除、outputPath 清空，且清理发生在终态事件之前（事件、快照、库一致）。
func TestArchiveShellDeletedAndOutputPathCleared(t *testing.T) {
	for _, st := range []string{"refused", "broken", "live"} { // failed / failed / succeeded
		t.Run(st, func(t *testing.T) {
			af := newArchiveFixture(t)
			af.setMode(st)
			af.probe = func(string) error { return fmt.Errorf("%w: moov atom not found", errNoDuration) }
			tk, err := af.start(t, "rtmp://127.0.0.1:1935/live/a")
			if err != nil {
				t.Fatal(err)
			}
			if st == "live" {
				af.waitProgress(t, tk.ID)
				af.mgr.Cancel(tk.ID)
			}
			d := af.wait(t, tk.ID)
			if d.OutputPath != "" {
				t.Fatalf("空壳应清空 outputPath: %+v", d)
			}
			if _, err := os.Stat(tk.OutputPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("空壳文件应被删: %v", err)
			}
			stored, _ := af.mgr.Get(tk.ID)
			ev := af.em.statusEvents(tk.ID)
			last := ev[len(ev)-1]
			if stored.OutputPath != "" || last.OutputPath != "" || last.Status.Active() {
				t.Fatalf("事件 / 库 / 快照必须一致（空）: %+v %+v", stored, last)
			}
			if terminals := countTerminal(ev); terminals != 1 {
				t.Fatalf("终态事件应只有 1 条: %d", terminals)
			}
		})
	}
}

func countTerminal(ev []task.StatusEvent) int {
	n := 0
	for _, e := range ev {
		if !e.Status.Active() {
			n++
		}
	}
	return n
}

// 清理先于终态事件：Remove 被调用的那一刻，任务的终态事件还没发出。
func TestArchiveCleanupHappensBeforeTerminalEvent(t *testing.T) {
	af := newArchiveFixture(t)
	af.setMode("refused")
	af.probe = func(string) error { return errNoDuration }
	var taskID atomic_string
	var sawTerminal bool
	inner := af.svc.cfg.Remove
	af.svc.cfg.Remove = func(p string) error {
		if id := taskID.get(); id != "" {
			sawTerminal = countTerminal(af.em.statusEvents(id)) > 0
		} else {
			sawTerminal = countTerminal(af.em.allStatusEvents()) > 0
		}
		return inner(p)
	}
	tk, err := af.start(t, "rtmp://127.0.0.1:1935/live/a")
	if err != nil {
		t.Fatal(err)
	}
	taskID.set(tk.ID)
	af.wait(t, tk.ID)
	if sawTerminal {
		t.Fatal("Remove 被调用时终态事件已经发出")
	}
}

type atomic_string struct {
	mu sync.Mutex
	v  string
}

func (a *atomic_string) get() string  { a.mu.Lock(); defer a.mu.Unlock(); return a.v }
func (a *atomic_string) set(s string) { a.mu.Lock(); a.v = s; a.mu.Unlock() }

// 删除失败：只记日志，任务状态和 error 不变；outputPath 仍清空（空壳不是存档）。
func TestArchiveShellDeleteFailureOnlyLogged(t *testing.T) {
	af := newArchiveFixture(t)
	af.setMode("ignoreq")
	af.svc.cfg.Grace = 200 * time.Millisecond
	af.probe = func(string) error { return errNoDuration }
	af.removeErr = errors.New("read-only file system")
	var logs []string
	var lm sync.Mutex
	af.svc.cfg.Logf = func(f string, a ...any) { lm.Lock(); logs = append(logs, fmt.Sprintf(f, a...)); lm.Unlock() }
	tk, err := af.svc.StartScreenPush(context.Background(), ScreenPushRequest{URL: "rtmp://127.0.0.1:1935/live/SECRETKEYabc", ArchiveDir: af.dir})
	if err != nil {
		t.Fatal(err)
	}
	af.waitProgress(t, tk.ID)
	af.mgr.Cancel(tk.ID)
	d := af.wait(t, tk.ID)
	if d.Status != task.StatusCanceled || d.Error != nil {
		t.Fatalf("删除失败不改任务状态: %+v %+v", d, d.Error)
	}
	assertStatusPayloadsHaveNoError(t, af.fixture, tk.ID, task.StatusCanceled)
	lm.Lock()
	defer lm.Unlock()
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "删除空壳存档失败") || !strings.Contains(joined, tk.OutputPath) {
		t.Fatalf("应记日志（含路径）: %q", joined)
	}
	assertNoSecrets(t, joined, "SECRETKEYabc", "127.0.0.1", "1935")
	if _, err := os.Stat(tk.OutputPath); err != nil {
		t.Fatalf("删除失败文件仍在: %v", err)
	}
}

// 只删本任务自己用 O_EXCL 创建的文件：已有同名文件永远不删，即使 ffprobe 读不出。
func TestArchiveNeverDeletesPreexistingFiles(t *testing.T) {
	af := newArchiveFixture(t)
	af.setMode("refused")
	af.probe = func(string) error { return errNoDuration }
	os.MkdirAll(af.dir, 0o755)
	existing := filepath.Join(af.dir, "screen-20260929-200000.mp4")
	other := filepath.Join(af.dir, "unrelated.txt")
	os.WriteFile(existing, []byte("someone else's recording"), 0o644)
	os.WriteFile(other, []byte("x"), 0o644)
	tk, err := af.start(t, "rtmp://127.0.0.1:1935/live/a")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(tk.OutputPath) != "screen-20260929-200000(1).mp4" {
		t.Fatalf("同名应加 (1): %q", tk.OutputPath)
	}
	d := af.wait(t, tk.ID)
	if d.OutputPath != "" {
		t.Fatalf("%+v", d)
	}
	if b, _ := os.ReadFile(existing); string(b) != "someone else's recording" {
		t.Fatal("已有文件被动了")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("不相关文件被删")
	}
	if len(af.removed) != 1 || af.removed[0] != tk.OutputPath {
		t.Fatalf("只应删本任务的占位文件: %v", af.removed)
	}
}

// ffprobe 自己起不来（不是"读不出时长"）：不能据此判空壳，保留文件。
func TestArchiveKeptWhenProbeCannotRun(t *testing.T) {
	af := newArchiveFixture(t)
	af.setMode("ignoreq")
	af.svc.cfg.Grace = 200 * time.Millisecond
	af.probe = func(string) error { return errors.New("ffprobe: executable file not found") }
	tk, _ := af.start(t, "rtmp://127.0.0.1:1935/live/a")
	af.waitProgress(t, tk.ID)
	os.WriteFile(tk.OutputPath, []byte("data"), 0o644)
	af.mgr.Cancel(tk.ID)
	d := af.wait(t, tk.ID)
	if d.OutputPath != tk.OutputPath || len(af.removed) != 0 {
		t.Fatalf("%+v %v", d, af.removed)
	}
}

// 0 字节文件不用 ffprobe 也是空壳。
func TestArchiveZeroByteIsShell(t *testing.T) {
	af := newArchiveFixture(t)
	af.setMode("refused")
	called := false
	af.svc.cfg.ProbeDuration = func(context.Context, string, string) (float64, error) { called = true; return 1, nil }
	tk, _ := af.start(t, "rtmp://127.0.0.1:1935/live/a")
	d := af.wait(t, tk.ID)
	if d.OutputPath != "" || called {
		t.Fatalf("0 字节直接判空壳: %+v called=%v", d, called)
	}
	if _, err := os.Stat(tk.OutputPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("应被删")
	}
}

// 连接被拒的分类不受存档影响（tee 的失败行）。
func TestArchiveConnectFailureClassification(t *testing.T) {
	af := newArchiveFixture(t)
	af.setMode("refused")
	tk, _ := af.start(t, secretURL)
	d := af.wait(t, tk.ID)
	if d.Status != task.StatusFailed || d.Error == nil || d.Error.Code != apperr.LiveConnectFailed || firstLine(d.Error.Detail) != "scheme=rtmp" {
		t.Fatalf("%+v %+v", d, d.Error)
	}
	assertNoSecrets(t, d.Error.Detail+d.Title+d.Params+af.logText(t, tk.ID)+af.em.jsonAll())
}

// 提交失败（如应用正在退出）时回滚占位文件和会话。
func TestArchivePlaceholderRolledBackOnSubmitFailure(t *testing.T) {
	af := newArchiveFixture(t)
	af.mgr.Shutdown(time.Second)
	_, err := af.start(t, "rtmp://127.0.0.1:1935/live/a")
	if err == nil {
		t.Fatal("应失败")
	}
	if n, _ := af.svc.ActiveSessions(); n != 0 {
		t.Fatal("会话应释放")
	}
	if ents, _ := os.ReadDir(af.dir); len(ents) != 0 {
		t.Fatalf("占位文件应删除: %v", ents)
	}
}

// tee 命令里的存档路径按契约转义：目录含所有特殊字符。
func TestArchiveCommandEscapesSpecialDir(t *testing.T) {
	af := newArchiveFixture(t)
	af.setMode("live")
	af.dir = filepath.Join(af.fixture.dir, `录 屏'|[x],y=z;c:d#f?g%h&i(1)`)
	tk, err := af.start(t, "rtmp://127.0.0.1:1935/live/a")
	if err != nil {
		t.Fatal(err)
	}
	af.waitProgress(t, tk.ID)
	desc := ""
	b, _ := os.ReadFile(af.exe + ".args")
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, "onfail=abort") {
			desc = l
		}
	}
	want := "file:" + ffmpeg.TeeEscape(tk.OutputPath)
	if !strings.HasSuffix(desc, want) || strings.Count(strings.ReplaceAll(desc, `\|`, ""), "|") != 1 {
		t.Fatalf("存档路径未按规则转义:\n%s\nwant suffix %s", desc, want)
	}
	af.mgr.Cancel(tk.ID)
	af.wait(t, tk.ID)
}

// 排队即被取消（Run 从未执行）也清理空占位文件：NeverRanner。
func TestArchiveNeverRanCleansPlaceholder(t *testing.T) {
	af := newArchiveFixture(t)
	path, err := af.svc.reserveArchive(af.dir)
	if err != nil {
		t.Fatal(err)
	}
	r := &archiveRunner{runner: &runner{s: af.svc, id: "x"}, g: &archiveGuard{s: af.svc, path: path}}
	if out := r.NeverRan(); out != task.ClearOutputPath {
		t.Fatalf("%q", out)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("占位应被删")
	}
}
