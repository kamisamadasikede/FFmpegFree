package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveStorageExeDirWritable(t *testing.T) {
	exeDir := t.TempDir()
	st := resolveStorage(storageEnv{goos: "linux", exe: filepath.Join(exeDir, "ffmpegfree"), dataDir: "/data/FFmpegFree", probe: probeWritable})
	if st.Base != exeDir || st.BaseKind != BaseExeDir || st.FellBack {
		t.Fatalf("%+v", st)
	}
	if st.Output != filepath.Join(exeDir, "output") || st.Uploads != filepath.Join(exeDir, "uploads") {
		t.Fatalf("%+v", st)
	}
	for _, d := range []string{st.Output, st.Uploads} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Fatalf("%s 应已建好: %v", d, err)
		}
		if ents, _ := os.ReadDir(d); len(ents) != 0 {
			t.Fatalf("试写文件应已删除: %v", ents)
		}
	}
}

// 一个不可写就两个一起回退，并删掉试写时新建的空文件夹。
func TestResolveStorageFallbackTogether(t *testing.T) {
	exeDir := t.TempDir()
	// uploads 是个普通文件：不是文件夹 = 试写失败
	if err := os.WriteFile(filepath.Join(exeDir, "uploads"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(t.TempDir(), "FFmpegFree")
	st := resolveStorage(storageEnv{goos: "linux", exe: filepath.Join(exeDir, "app"), dataDir: data, probe: probeWritable})
	if st.Base != data || st.BaseKind != BaseUserData || !st.FellBack {
		t.Fatalf("%+v", st)
	}
	if st.Output != filepath.Join(data, "output") || st.Uploads != filepath.Join(data, "uploads") {
		t.Fatalf("%+v", st)
	}
	if _, err := os.Stat(filepath.Join(exeDir, "output")); !os.IsNotExist(err) {
		t.Fatalf("试写时新建的空 output 应被删掉: %v", err)
	}
	// 原来就有的文件夹不删
	exe2 := t.TempDir()
	os.MkdirAll(filepath.Join(exe2, "output"), 0o755)
	os.WriteFile(filepath.Join(exe2, "uploads"), nil, 0o644)
	resolveStorage(storageEnv{goos: "linux", exe: filepath.Join(exe2, "app"), dataDir: data, probe: probeWritable})
	if fi, err := os.Stat(filepath.Join(exe2, "output")); err != nil || !fi.IsDir() {
		t.Fatalf("用户原有的 output 不应被删: %v", err)
	}
}

func TestResolveStorageReadOnlyExeDir(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("需要非 root 的 Unix 权限位")
	}
	exeDir := t.TempDir()
	os.Chmod(exeDir, 0o555)
	t.Cleanup(func() { os.Chmod(exeDir, 0o755) })
	data := filepath.Join(t.TempDir(), "FFmpegFree")
	st := resolveStorage(storageEnv{goos: "linux", exe: filepath.Join(exeDir, "app"), dataDir: data, probe: probeWritable})
	if !st.FellBack || st.Base != data {
		t.Fatalf("%+v", st)
	}
}

func TestResolveStoragePlatformRules(t *testing.T) {
	never := func(string) bool { return false }
	always := func(string) bool { return true }
	// Windows 回退到 %LocalAppData%\FFmpegFree（架构师定），不是 %AppData%（数据目录）
	local, roaming := "/home/u/AppData/Local", "/home/u/AppData/Roaming/FFmpegFree"
	if runtime.GOOS == "windows" {
		local, roaming = `C:\Users\u\AppData\Local`, `C:\Users\u\AppData\Roaming\FFmpegFree`
	}
	st := resolveStorage(storageEnv{goos: "windows", exe: filepath.Join(local, "x", "app.exe"), dataDir: roaming, localApp: local, probe: never})
	if st.Base != filepath.Join(local, "FFmpegFree") || !st.FellBack || st.BaseKind != BaseUserData {
		t.Fatalf("%+v", st)
	}
	// 没有 LOCALAPPDATA：退回 dataDir
	st = resolveStorage(storageEnv{goos: "windows", exe: "/x/app.exe", dataDir: "/roaming/FFmpegFree", probe: never})
	if st.Base != "/roaming/FFmpegFree" {
		t.Fatalf("%+v", st)
	}
	// macOS .app 包：直接用用户数据目录，不试写，不算回退
	probed := false
	st = resolveStorage(storageEnv{goos: "darwin", exe: "/Applications/FFmpegFree.app/Contents/MacOS/FFmpegFree", dataDir: "/Users/u/Library/Application Support/FFmpegFree",
		probe: func(string) bool { probed = true; return true }})
	if probed || st.BaseKind != BaseUserData || st.FellBack || st.Base != "/Users/u/Library/Application Support/FFmpegFree" {
		t.Fatalf("%+v probed=%v", st, probed)
	}
	// macOS 不在 .app 包里（开发时）：照常试写
	st = resolveStorage(storageEnv{goos: "darwin", exe: "/Users/u/dev/build/FFmpegFree", dataDir: "/d", probe: always})
	if st.Base != "/Users/u/dev/build" || st.BaseKind != BaseExeDir {
		t.Fatalf("%+v", st)
	}
	// Linux AppImage：用 $APPIMAGE 所在文件夹
	var seen []string
	st = resolveStorage(storageEnv{goos: "linux", exe: "/tmp/.mount_abc/usr/bin/ffmpegfree", appImage: "/home/u/Apps/FFmpegFree.AppImage", dataDir: "/d",
		probe: func(d string) bool { seen = append(seen, d); return true }})
	if st.Base != "/home/u/Apps" || len(seen) != 2 || seen[0] != "/home/u/Apps/output" || seen[1] != "/home/u/Apps/uploads" {
		t.Fatalf("%+v %v", st, seen)
	}
	// 拿不到可执行文件路径：回退
	st = resolveStorage(storageEnv{goos: "linux", dataDir: "/d", probe: always})
	if st.Base != "/d" || !st.FellBack {
		t.Fatalf("%+v", st)
	}
}

func TestInAppBundle(t *testing.T) {
	for p, want := range map[string]bool{
		"/Applications/FFmpegFree.app/Contents/MacOS/FFmpegFree": true,
		"/Users/u/Downloads/X.app/Contents/MacOS/bin":            true,
		"/Users/u/dev/FFmpegFree":                                false,
		"/Applications/FFmpegFree.app/Contents/Resources/x":      false,
	} {
		if inAppBundle(p) != want {
			t.Errorf("%s", p)
		}
	}
}

// v0.24.1 改写的 6.12：数据目录本身和它下面都禁止，<dataDir>/output 及其子文件夹放行（按固定名字，与是否回退无关）。
func TestInsideDataDir(t *testing.T) {
	data := filepath.Join(t.TempDir(), "FFmpegFree")
	os.MkdirAll(filepath.Join(data, "output", "sub"), 0o755)
	os.MkdirAll(filepath.Join(data, "logs"), 0o755)
	cases := map[string]bool{
		data:                                        true,
		filepath.Join(data, "logs"):                 true,
		filepath.Join(data, "thumbs", "new"):        true,
		filepath.Join(data, "uploads"):              true,
		filepath.Join(data, "outputs"):              true, // 只放行名字恰好是 output 的
		filepath.Join(data, "output"):               false,
		filepath.Join(data, "output", "sub"):        false,
		filepath.Join(data, "output", "not", "yet"): false,
		filepath.Dir(data):                          false,
		filepath.Join(t.TempDir(), "elsewhere"):     false,
	}
	for p, want := range cases {
		if got := InsideDataDir(data, p); got != want {
			t.Errorf("InsideDataDir(%s) = %v, want %v", p, got, want)
		}
	}
	if InsideDataDir("", data) {
		t.Error("dataDir 为空不检查")
	}
	// 符号链接：链接到 logs 的路径同样禁止，链接到 output 的放行
	if runtime.GOOS != "windows" {
		ln := filepath.Join(t.TempDir(), "ln")
		os.Symlink(filepath.Join(data, "logs"), ln)
		if !InsideDataDir(data, filepath.Join(ln, "x")) {
			t.Error("经符号链接进入 logs 应禁止")
		}
		ln2 := filepath.Join(t.TempDir(), "ln2")
		os.Symlink(filepath.Join(data, "output"), ln2)
		if InsideDataDir(data, ln2) {
			t.Error("经符号链接进入 output 应放行")
		}
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if InsideDataDir(data, filepath.Join(data, "OUTPUT")) {
			t.Error("不区分大小写的平台上 OUTPUT 也放行")
		}
	}
}
