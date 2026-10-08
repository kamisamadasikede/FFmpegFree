package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ---------- 存储根目录 <base>（契约 v0.24，6.15.1） ----------

// BaseKind 的取值。
const (
	BaseExeDir   = "exe_dir"
	BaseUserData = "user_data"
)

// 固定的子文件夹名。OutputDirName 同时是 6.12 输出目录规则里“应用数据目录内唯一放行”的文件夹名。
const (
	OutputDirName  = "output"
	UploadsDirName = "uploads"
)

// Storage 是启动时确定一次的存储根目录（不落库，运行期间不变）。
type Storage struct {
	Base     string // <base>
	Output   string // <base>/output（默认输出目录）
	Uploads  string // <base>/uploads（默认上传目录）
	BaseKind string // BaseExeDir | BaseUserData
	FellBack bool   // true = 程序所在文件夹不可写，回退到了用户数据目录（macOS .app 包不算回退）
}

// storageEnv 是 resolveStorage 的输入，测试里替换。
type storageEnv struct {
	goos     string
	exe      string                // 可执行文件路径（已 EvalSymlinks）；"" = 拿不到，直接回退
	appImage string                // $APPIMAGE
	dataDir  string                // 应用数据目录 <UserConfigDir>/FFmpegFree（macOS .app 包与非 Windows 的回退位置）
	localApp string                // Windows %LOCALAPPDATA%（回退位置的父目录；空时用 dataDir）
	probe    func(dir string) bool // 试写：MkdirAll + 建文件 + 写 1 字节 + 删除；成功返回 true
}

// ResolveStorage 按 6.15.1 确定 <base>，并尽量建好 output / uploads（建不出来不报错，用到时再报）。
// dataDir 是应用数据目录（paths.Resolve 的 Root）。
func ResolveStorage(dataDir string) Storage {
	env := storageEnv{goos: runtime.GOOS, appImage: os.Getenv("APPIMAGE"), dataDir: dataDir,
		localApp: os.Getenv("LOCALAPPDATA"), probe: probeWritable}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		env.exe = exe
	}
	st := resolveStorage(env)
	if st.FellBack || st.BaseKind == BaseUserData {
		_ = os.MkdirAll(st.Output, 0o755)
		_ = os.MkdirAll(st.Uploads, 0o755)
	}
	return st
}

func resolveStorage(env storageEnv) Storage {
	mk := func(base, kind string, fell bool) Storage {
		return Storage{Base: base, Output: filepath.Join(base, OutputDirName), Uploads: filepath.Join(base, UploadsDirName),
			BaseKind: kind, FellBack: fell}
	}
	fallback := env.dataDir
	if env.goos == "windows" && env.localApp != "" && filepath.IsAbs(env.localApp) {
		fallback = filepath.Join(env.localApp, appDirName) // 架构师定：Windows 回退到 %LocalAppData%\FFmpegFree
	}
	if env.exe == "" {
		return mk(fallback, BaseUserData, true)
	}
	if env.goos == "darwin" && inAppBundle(env.exe) {
		return mk(env.dataDir, BaseUserData, false) // .app 包：直接用用户数据目录，不算回退
	}
	exeDir := filepath.Dir(env.exe)
	if env.goos == "linux" && env.appImage != "" && filepath.IsAbs(env.appImage) {
		exeDir = filepath.Dir(env.appImage)
	}
	if env.probe(filepath.Join(exeDir, OutputDirName)) && env.probe(filepath.Join(exeDir, UploadsDirName)) {
		return mk(exeDir, BaseExeDir, false)
	}
	// 两个一起回退；试写时新建的空文件夹删掉（只删空的）
	for _, n := range []string{OutputDirName, UploadsDirName} {
		removeIfCreatedEmpty(filepath.Join(exeDir, n))
	}
	return mk(fallback, BaseUserData, true)
}

// inAppBundle：路径里有一段以 .app 结尾，且可执行文件在它的 Contents/MacOS/ 下。
func inAppBundle(exe string) bool {
	parts := strings.Split(filepath.ToSlash(exe), "/")
	for i := 0; i+3 < len(parts); i++ {
		if strings.HasSuffix(parts[i], ".app") && parts[i+1] == "Contents" && parts[i+2] == "MacOS" {
			return true
		}
	}
	return false
}

// createdByProbe 记录试写时新建的文件夹，回退时只删这些（而且只删空的）。
var createdByProbe = map[string]bool{}

// probeWritable 是 6.15.1 第 2 条的试写。
func probeWritable(dir string) bool {
	fi, err := os.Stat(dir)
	switch {
	case err == nil && !fi.IsDir():
		return false
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false
		}
		createdByProbe[dir] = true
	case err != nil:
		return false
	}
	return WriteTest(dir) == nil
}

// WriteTest 在 dir 里建一个探测文件、写 1 个字节、关闭并删除（不看权限位）。
func WriteTest(dir string) error {
	f, err := os.CreateTemp(dir, ".ffmpegfree-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_, werr := f.Write([]byte{0})
	cerr := f.Close()
	os.Remove(name)
	if werr != nil {
		return werr
	}
	return cerr
}

func removeIfCreatedEmpty(dir string) {
	if createdByProbe[dir] {
		_ = os.Remove(dir) // 非空时失败，正好不删
		delete(createdByProbe, dir)
	}
}

// ---------- 6.12 输出目录规则（v0.24.1 架构师改写） ----------

// InsideDataDir 报告 p 是否落在“输出目录禁止区”：应用数据目录本身及其下的一切，
// 但 <dataDir>/output 及其子文件夹除外（按固定文件夹名放行，与这次启动是否回退无关）。
// 比较前对两边做 EvalSymlinks（不存在的尾部原样接上），Windows / macOS 不区分大小写。
func InsideDataDir(dataDir, p string) bool {
	if dataDir == "" || p == "" {
		return false
	}
	if !Within(dataDir, p) {
		return false
	}
	return !Within(filepath.Join(dataDir, OutputDirName), p)
}

// Within 判断 p 是否等于 root 或在 root 之内（同 InsideDataDir 的比较方式）。
func Within(root, p string) bool {
	r, q := realish(root), realish(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		r, q = strings.ToLower(r), strings.ToLower(q)
	}
	if q == r {
		return true
	}
	if !strings.HasSuffix(r, string(filepath.Separator)) {
		r += string(filepath.Separator)
	}
	return strings.HasPrefix(q, r)
}

// realish 解析路径里已存在部分的符号链接，不存在的尾部原样接上。
func realish(p string) string {
	p = filepath.Clean(p)
	rest := ""
	for cur := p; ; {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}
