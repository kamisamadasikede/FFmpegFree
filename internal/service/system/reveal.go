package system

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"FFmpegFree/internal/apperr"
)

// launcher 启动一个不需要等待结果的外部命令（打开文件管理器）。测试里替换。
type launcher func(name string, args ...string) error

// revealCommand 返回在文件管理器里显示 path 的命令。isDir 为 true 时打开这个文件夹本身。
//
//	Windows: explorer.exe /select,"<path>"（文件夹 explorer.exe "<path>"）；路径已经带好引号，
//	         启动时按原样拼进命令行（rawCmdLine），不能让 Go 再转义一次，否则 explorer 认不出 /select, 后面的路径

// macOS:   open -R <path>（文件夹直接 open）
// Linux:   xdg-open <文件所在文件夹>（文件夹直接打开）
func revealCommand(goos, path string, isDir bool) (string, []string) {
	switch goos {
	case "windows":
		if isDir {
			return "explorer.exe", []string{`"` + path + `"`}
		}
		return "explorer.exe", []string{`/select,"` + path + `"`}
	case "darwin":
		if isDir {
			return "open", []string{path}
		}
		return "open", []string{"-R", path}
	default:
		if isDir {
			return "xdg-open", []string{path}
		}
		return "xdg-open", []string{filepath.Dir(path)}
	}
}

// RevealInFolder 在系统文件管理器里显示 path（"打开输出位置"）。
//
// 只允许两类路径（防止前端被注入后拿它当"打开任意位置"的入口）：
//  1. 任务表里登记的输出文件（成功任务的 outputPath 等，含还在进行的任务）；
//  2. 当前设置里 defaultOutputDir 之内的路径（目录本身也可以）。
//
// 判断前先 Clean（"../" 穿越会被折叠掉）并 EvalSymlinks，用真实路径比较（Windows / macOS 不区分大小写），
// 所以符号链接指到允许范围之外的路径会被拒绝；实际打开的也是真实路径。
// 其余情况：空或相对路径、不在允许范围内都是 INVALID_ARGUMENT，不存在 NOT_FOUND，
// 找不到文件管理器命令或启动失败 PROCESS_FAILED。命令启动后立即返回，不等待窗口。
func (m *Manager) RevealInFolder(path string) error {
	ctx := context.Background()
	m.mu.Lock()
	tasks := m.cfg.Tasks
	m.mu.Unlock()
	allow := func(cleaned, real string) bool {
		if tasks != nil && tasks.IsTaskOutput(cleaned) {
			return true
		}
		if dir := m.DefaultOutputDir(ctx); dir != "" {
			if within(caseInsensitivePaths(runtime.GOOS), dir, real) {
				return true
			}
		}
		return false
	}
	start := m.launch
	if start == nil {
		start = startDetached
	}
	return revealIn(runtime.GOOS, start, path, allow)
}

func caseInsensitivePaths(goos string) bool { return goos == "windows" || goos == "darwin" }

// within 判断 real 是否等于 dir 或在 dir 之内。dir 会先做 EvalSymlinks（不存在则不允许）；
// real 必须已经是真实路径。按路径分隔符边界比较，"/a/out2" 不算在 "/a/out" 之内。
func within(fold bool, dir, real string) bool {
	if dir == "" || !filepath.IsAbs(dir) {
		return false
	}
	rd, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(key(fold, rd), key(fold, real))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func key(fold bool, p string) string {
	p = filepath.Clean(p)
	if fold {
		p = strings.ToLower(p)
	}
	return p
}

// revealIn 校验并打开。allow 收到 Clean 之后的原路径和 EvalSymlinks 之后的真实路径，返回是否允许；
// 传 nil 表示不做范围限制（只给不涉及范围的内部测试用）。
func revealIn(goos string, start launcher, path string, allow func(cleaned, real string) bool) error {
	if path == "" || !filepath.IsAbs(path) {
		return apperr.New(apperr.InvalidArgument, "路径必须是绝对路径").WithDetail(path)
	}
	path = filepath.Clean(path)
	if goos == "windows" && strings.Contains(path, `"`) {
		// Windows 文件名本来就不能含双引号；拒绝它，避免在手拼的命令行里破坏引号。
		return apperr.New(apperr.InvalidArgument, "路径里不能有双引号").WithDetail(path)
	}
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return apperr.New(apperr.NotFound, "文件或文件夹不存在").WithDetail(path)
		}
		return apperr.Wrap(apperr.IOError, "无法读取文件信息", err)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return apperr.Wrap(apperr.IOError, "无法解析路径", err)
	}
	if allow != nil && !allow(path, real) {
		return apperr.New(apperr.InvalidArgument, "只能打开任务输出文件或默认输出文件夹里的内容").WithDetail(path)
	}
	name, args := revealCommand(goos, real, fi.IsDir())
	if err := start(name, args...); err != nil {
		return apperr.Wrap(apperr.ProcessFailed, "无法打开文件管理器", err)
	}
	return nil
}

// AppContext 返回 Start 传入的应用 ctx（Wails 对话框需要它）；Start 之前为 nil。
func (m *Manager) AppContext() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appCtx
}

// rawCmdLine 把命令和参数用空格原样拼成一条命令行（不做任何转义）。Windows 下给 explorer 用：
// 调用方保证每个参数已经按需要带好引号。
func rawCmdLine(name string, args []string) string {
	return strings.Join(append([]string{name}, args...), " ")
}
