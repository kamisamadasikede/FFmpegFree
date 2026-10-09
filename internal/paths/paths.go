// Package paths 负责应用数据目录和路径规范化（见契约第 1 节）。
//
// v2 起所有数据都放在用户数据目录，不再写进程序目录，也不再依赖当前工作目录。
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const appDirName = "FFmpegFree"

// Dirs 是应用运行期用到的目录集合。
type Dirs struct {
	Root   string // <UserConfigDir>/FFmpegFree
	Bin    string // 自动安装的 ffmpeg / ffprobe
	Thumbs string // 缩略图缓存
	Logs   string // 任务日志
	Temp   string // 任务临时目录（两遍编码日志等）
	DB     string // SQLite 文件路径
}

// Resolve 计算数据目录。root 为空时使用 os.UserConfigDir()；测试可以传临时目录。
func Resolve(root string) (Dirs, error) {
	if root == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return Dirs{}, fmt.Errorf("定位用户数据目录失败: %w", err)
		}
		root = filepath.Join(base, appDirName)
	}
	return Dirs{
		Root:   root,
		Bin:    filepath.Join(root, "bin"),
		Thumbs: filepath.Join(root, "thumbs"),
		Logs:   filepath.Join(root, "logs"),
		Temp:   filepath.Join(root, "tmp"),
		DB:     filepath.Join(root, "app.db"),
	}, nil
}

// Ensure 创建所有目录。
func (d Dirs) Ensure() error {
	for _, dir := range []string{d.Root, d.Bin, d.Thumbs, d.Logs, d.Temp} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}
	return nil
}

// DefaultOutputDir 返回系统"视频"目录下的 FFmpegFree 子目录：
// Windows 为 %USERPROFILE%\Videos，macOS 为 ~/Movies，
// Linux 优先读 XDG_VIDEOS_DIR（环境变量或 ~/.config/user-dirs.dirs），读不到用 ~/Videos。
func DefaultOutputDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(videosDir(runtime.GOOS, home, os.Getenv), appDirName), nil
}

func videosDir(goos, home string, getenv func(string) string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, "Movies")
	case "windows":
		return filepath.Join(home, "Videos")
	}
	if v := expandXDG(getenv("XDG_VIDEOS_DIR"), home); v != "" {
		return v
	}
	cfg := getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	if data, err := os.ReadFile(filepath.Join(cfg, "user-dirs.dirs")); err == nil {
		if v := parseUserDirs(string(data), "XDG_VIDEOS_DIR", home); v != "" {
			return v
		}
	}
	return filepath.Join(home, "Videos")
}

// parseUserDirs 解析 xdg-user-dirs 生成的文件，格式如 XDG_VIDEOS_DIR="$HOME/视频"。
func parseUserDirs(content, key, home string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		return expandXDG(strings.Trim(strings.TrimSpace(v), `"`), home)
	}
	return ""
}

func expandXDG(v, home string) string {
	if v == "" {
		return ""
	}
	v = strings.Replace(v, "$HOME", home, 1)
	if !filepath.IsAbs(v) {
		return ""
	}
	// xdg 规范：值等于 $HOME 表示该目录被禁用。
	if filepath.Clean(v) == filepath.Clean(home) {
		return ""
	}
	return filepath.Clean(v)
}

// caseInsensitiveFS 表示当前平台默认文件系统大小写不敏感。
var caseInsensitiveFS = CaseInsensitiveGOOS(runtime.GOOS)

// Normalize 把路径转成干净的绝对路径，返回用于显示的 path 和用于唯一约束的 key。
// Windows 和 macOS 上 key 转小写，避免同一个文件因大小写不同被记录两次。
func Normalize(p string) (path string, key string, err error) {
	if strings.TrimSpace(p) == "" {
		return "", "", fmt.Errorf("路径为空")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", "", err
	}
	abs = filepath.Clean(abs)
	key = abs
	if caseInsensitiveFS {
		key = strings.ToLower(abs)
	}
	return abs, key, nil
}

// CaseInsensitiveGOOS 表示该平台默认文件系统大小写不敏感（windows、darwin），与 Normalize 的 key 口径一致。
func CaseInsensitiveGOOS(goos string) bool { return goos == "windows" || goos == "darwin" }

// KeyFor 按 goos 的口径给一个已是绝对路径的 p 算唯一约束 key（Clean；windows / darwin 转小写，其他原样）。
// 与 Normalize 返回的 key 同一规则，只是不做 Abs、平台可指定（给需要跨平台测试或重算 key 的调用方用）。
func KeyFor(goos, p string) string {
	k := filepath.Clean(p)
	if CaseInsensitiveGOOS(goos) {
		k = strings.ToLower(k)
	}
	return k
}

// Key 是当前平台的 KeyFor。
func Key(p string) string { return KeyFor(runtime.GOOS, p) }
