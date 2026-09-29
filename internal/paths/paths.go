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

// DefaultOutputDir 返回默认输出目录 ~/Videos/FFmpegFree。
func DefaultOutputDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Videos", appDirName), nil
}

// caseInsensitiveFS 表示当前平台默认文件系统大小写不敏感。
var caseInsensitiveFS = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

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
