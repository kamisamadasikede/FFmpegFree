package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

func TestRevealCommand(t *testing.T) {
	cases := []struct {
		goos, path string
		dir        bool
		name       string
		args       []string
	}{
		{"windows", `C:\a b\x.mp4`, false, "explorer.exe", []string{`/select,"C:\a b\x.mp4"`}},
		{"windows", `D:\视频\我的 文件.mp4`, false, "explorer.exe", []string{`/select,"D:\视频\我的 文件.mp4"`}},
		{"windows", `C:\a b`, true, "explorer.exe", []string{`"C:\a b"`}},
		{"windows", `D:\输出 目录`, true, "explorer.exe", []string{`"D:\输出 目录"`}},
		{"darwin", "/Users/a/x.mp4", false, "open", []string{"-R", "/Users/a/x.mp4"}},
		{"darwin", "/Users/a", true, "open", []string{"/Users/a"}},
		{"linux", "/home/a/x.mp4", false, "xdg-open", []string{"/home/a"}},
		{"linux", "/home/a", true, "xdg-open", []string{"/home/a"}},
	}
	for _, c := range cases {
		n, a := revealCommand(c.goos, c.path, c.dir)
		if n != c.name || !reflect.DeepEqual(a, c.args) {
			t.Errorf("%v: got %s %q", c, n, a)
		}
	}
}

func TestRevealInFolderValidation(t *testing.T) {
	called := false
	start := func(string, ...string) error { called = true; return nil }
	code := func(err error) apperr.Code {
		var ae *apperr.AppError
		if errors.As(err, &ae) {
			return ae.Code
		}
		return ""
	}
	for _, p := range []string{"", "rel/x.mp4", "x.mp4"} {
		if c := code(revealIn("linux", start, p, nil)); c != apperr.InvalidArgument {
			t.Errorf("%q: %v", p, c)
		}
	}
	missing := filepath.Join(t.TempDir(), "nope.mp4")
	if c := code(revealIn("linux", start, missing, nil)); c != apperr.NotFound {
		t.Errorf("missing: %v", c)
	}
	if called {
		t.Fatal("校验失败不应启动命令")
	}
	f := filepath.Join(t.TempDir(), "中 文 a.mp4")
	os.WriteFile(f, []byte("x"), 0o644)
	var gotName string
	var gotArgs []string
	err := revealIn("linux", func(n string, a ...string) error { gotName, gotArgs = n, a; return nil }, f, nil)
	if err != nil || gotName != "xdg-open" || gotArgs[0] != filepath.Dir(f) {
		t.Fatalf("%v %s %v", err, gotName, gotArgs)
	}
	// 启动失败
	err = revealIn("linux", func(string, ...string) error { return errors.New("boom") }, f, nil)
	if code(err) != apperr.ProcessFailed {
		t.Fatalf("%v", err)
	}
}

func TestAppContextNilBeforeStart(t *testing.T) {
	if NewManager().AppContext() != nil {
		t.Fatal("Start 之前应为 nil")
	}
}

// ---- 范围限制：只允许任务输出或 defaultOutputDir 之内 ----

type revealFx struct {
	mgr      *Manager
	tasks    *task.Manager
	set      *memSettings
	root     string // 已 EvalSymlinks 的临时目录
	out      string // defaultOutputDir
	outside  string // 不在任何允许范围内的目录
	launched [][]string
}

func newRevealFx(t *testing.T) *revealFx {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &revealFx{root: root, out: filepath.Join(root, "out"), outside: filepath.Join(root, "outside"), set: newMemSettings()}
	for _, d := range []string{f.out, f.outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(context.Background(), filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	f.tasks = task.NewManager(task.Config{Store: st, LogDir: filepath.Join(root, "logs"), BatchConcurrency: 1,
		ProgressInterval: -1, Logf: func(string, ...any) {}})
	t.Cleanup(func() { f.tasks.Shutdown(2 * time.Second) })
	f.mgr = NewManager()
	f.mgr.cfg = Config{Settings: f.set, Tasks: f.tasks}
	f.mgr.launch = func(name string, args ...string) error {
		f.launched = append(f.launched, append([]string{name}, args...))
		return nil
	}
	return f
}

func (f *revealFx) setOut(t *testing.T, dir string) {
	t.Helper()
	if err := f.set.SetSetting(context.Background(), SettingDefaultOutputDir, dir); err != nil {
		t.Fatal(err)
	}
}

func (f *revealFx) file(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func revealCode(err error) apperr.Code {
	var ae *apperr.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

func (f *revealFx) runTask(t *testing.T, out string) task.Task {
	t.Helper()
	tk, err := f.tasks.Submit(task.Spec{Type: task.TypeConvert, OutputPath: out}, task.RunnerFunc(func(ctx context.Context, _ func(task.Progress)) (string, error) {
		return task.RunWithPart(ctx, out, func(part string) error { return os.WriteFile(part, []byte("v"), 0o644) })
	}))
	if err != nil {
		t.Fatal(err)
	}
	var got task.Task
	waitFor(t, func() bool {
		got, _ = f.tasks.Get(tk.ID)
		return got.Status == task.StatusSucceeded
	})
	return got
}

func TestRevealAllowsPathInsideDefaultOutputDir(t *testing.T) {
	f := newRevealFx(t)
	f.setOut(t, f.out)
	in := f.file(t, filepath.Join(f.out, "sub", "a.mp4"))
	for _, p := range []string{in, filepath.Dir(in), f.out} {
		f.launched = nil
		if err := f.mgr.RevealInFolder(p); err != nil {
			t.Fatalf("%s 应允许: %v", p, err)
		}
		if len(f.launched) != 1 {
			t.Fatalf("%s 应启动一次文件管理器: %v", p, f.launched)
		}
	}
}

func TestRevealAllowsRegisteredTaskOutputOutsideDefaultDir(t *testing.T) {
	f := newRevealFx(t)
	// 没设置 defaultOutputDir：输出在源文件旁边
	done := f.runTask(t, filepath.Join(f.outside, "conv.mp4"))
	if err := f.mgr.RevealInFolder(done.OutputPath); err != nil || len(f.launched) != 1 {
		t.Fatalf("任务输出应允许: %v %v", err, f.launched)
	}
	// 同目录里别的文件不在任务表里 → 拒绝
	other := f.file(t, filepath.Join(f.outside, "other.mp4"))
	if c := revealCode(f.mgr.RevealInFolder(other)); c != apperr.InvalidArgument {
		t.Fatalf("未登记的同目录文件应拒绝: %v", c)
	}
	// 任务输出所在文件夹本身也不是登记的输出 → 拒绝
	if c := revealCode(f.mgr.RevealInFolder(f.outside)); c != apperr.InvalidArgument {
		t.Fatalf("输出所在文件夹本身应拒绝: %v", c)
	}
	// 记录被删之后不再是登记的输出
	if err := f.tasks.Remove([]string{done.ID}, false); err != nil {
		t.Fatal(err)
	}
	if c := revealCode(f.mgr.RevealInFolder(done.OutputPath)); c != apperr.InvalidArgument {
		t.Fatalf("记录删除后应拒绝: %v", c)
	}
}

func TestRevealRejectsDotDotTraversal(t *testing.T) {
	f := newRevealFx(t)
	f.setOut(t, f.out)
	secret := f.file(t, filepath.Join(f.outside, "secret.txt"))
	f.file(t, filepath.Join(f.out, "ok.mp4"))
	// out/../outside/secret.txt 折叠后落在 outside
	for _, p := range []string{
		filepath.Join(f.out, "..", "outside", "secret.txt"),
		f.out + string(filepath.Separator) + ".." + string(filepath.Separator) + "outside",
		filepath.Join(f.out, "sub", "..", "..", "outside", "secret.txt"),
		filepath.Join(f.out, "..") + string(filepath.Separator), // out 的上级
	} {
		if c := revealCode(f.mgr.RevealInFolder(p)); c != apperr.InvalidArgument {
			t.Errorf("%s 应 INVALID_ARGUMENT，得到 %q", p, c)
		}
	}
	// 名字前缀相同的兄弟目录（out2）不算在 out 之内
	sib := f.file(t, filepath.Join(f.root, "out2", "x.mp4"))
	if c := revealCode(f.mgr.RevealInFolder(sib)); c != apperr.InvalidArgument {
		t.Errorf("兄弟目录 out2 应拒绝: %v", c)
	}
	if len(f.launched) != 0 {
		t.Fatalf("被拒绝的路径不应启动文件管理器: %v", f.launched)
	}
	_ = secret
	// 合法的带 .. 但折叠后仍在 out 内
	if err := f.mgr.RevealInFolder(filepath.Join(f.out, "sub", "..", "ok.mp4")); err != nil {
		t.Errorf("折叠后仍在 out 内应允许: %v", err)
	}
}

func TestRevealRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 上创建符号链接需要权限")
	}
	f := newRevealFx(t)
	f.setOut(t, f.out)
	secret := f.file(t, filepath.Join(f.outside, "secret.txt"))
	// out/linkdir -> outside（目录链接）；out/linkfile -> outside/secret.txt（文件链接）
	if err := os.Symlink(f.outside, filepath.Join(f.out, "linkdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(f.out, "linkfile")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(f.out, "linkdir"),
		filepath.Join(f.out, "linkdir", "secret.txt"),
		filepath.Join(f.out, "linkfile"),
	} {
		if c := revealCode(f.mgr.RevealInFolder(p)); c != apperr.InvalidArgument {
			t.Errorf("%s 符号链接逃逸应 INVALID_ARGUMENT，得到 %q", p, c)
		}
	}
	// defaultOutputDir 自己是指向别处的链接：用真实路径比较，链接里的内容按真实位置判断
	link := filepath.Join(f.root, "outlink")
	if err := os.Symlink(f.out, link); err != nil {
		t.Fatal(err)
	}
	in := f.file(t, filepath.Join(f.out, "real.mp4"))
	f.setOut(t, link)
	if err := f.mgr.RevealInFolder(in); err != nil {
		t.Errorf("defaultOutputDir 是链接时，真实路径在其内应允许: %v", err)
	}
	if err := f.mgr.RevealInFolder(filepath.Join(link, "real.mp4")); err != nil {
		t.Errorf("经链接访问 defaultOutputDir 内文件应允许: %v", err)
	}
	// 任务输出被换成指向别处的符号链接：拒绝
	f.setOut(t, "")
	done := f.runTask(t, filepath.Join(f.outside, "t.mp4"))
	if err := os.Remove(done.OutputPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, done.OutputPath); err != nil {
		t.Fatal(err)
	}
	if c := revealCode(f.mgr.RevealInFolder(done.OutputPath)); c != apperr.InvalidArgument {
		t.Errorf("任务输出被换成符号链接应拒绝: %v", c)
	}
	if len(f.launched) != 2 {
		t.Errorf("只有两次合法调用应启动: %v", f.launched)
	}
}

func TestRevealWithoutDefaultDirOrTasksRejects(t *testing.T) {
	f := newRevealFx(t)
	p := f.file(t, filepath.Join(f.outside, "a.mp4"))
	if c := revealCode(f.mgr.RevealInFolder(p)); c != apperr.InvalidArgument {
		t.Fatalf("没有 defaultOutputDir 也没有任务登记时应拒绝: %v", c)
	}
	// 没有任务管理器（降级）时同样只看 defaultOutputDir
	m := NewManager()
	m.launch = func(string, ...string) error { t.Fatal("不应启动"); return nil }
	if c := revealCode(m.RevealInFolder(p)); c != apperr.InvalidArgument {
		t.Fatalf("无任务管理器应拒绝: %v", c)
	}
}

func TestWithinCaseInsensitive(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	dir := filepath.Join(root, "Out")
	os.MkdirAll(dir, 0o755)
	real := filepath.Join(root, "out", "a.mp4")
	if within(false, dir, real) {
		t.Fatal("区分大小写时 Out 与 out 不同")
	}
	if !within(true, dir, real) {
		t.Fatal("不区分大小写时应视为同一目录")
	}
}

func TestRawCmdLine(t *testing.T) {
	name, args := revealCommand("windows", `C:\a b\中文 x.mp4`, false)
	if got, want := rawCmdLine(name, args), `explorer.exe /select,"C:\a b\中文 x.mp4"`; got != want {
		t.Fatalf("file cmdline = %q, want %q", got, want)
	}
	name, args = revealCommand("windows", `C:\a b`, true)
	if got, want := rawCmdLine(name, args), `explorer.exe "C:\a b"`; got != want {
		t.Fatalf("dir cmdline = %q, want %q", got, want)
	}
}

func TestRevealWindowsRejectsQuoteInPath(t *testing.T) {
	err := revealIn("windows", func(string, ...string) error { t.Fatal("must not launch"); return nil }, `/tmp/a"b`, nil)
	if err == nil {
		t.Fatal("want error for path containing a double quote")
	}
}
