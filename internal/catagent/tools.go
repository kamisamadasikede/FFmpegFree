package catagent

import (
	"os"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/paths"
)

// 只读文本扩展名白名单（契约：白名单 TBD；一期保守列表）。
var textExtAllow = map[string]struct{}{
	".txt": {}, ".md": {}, ".markdown": {}, ".json": {}, ".yaml": {}, ".yml": {},
	".toml": {}, ".xml": {}, ".csv": {}, ".tsv": {}, ".go": {}, ".ts": {}, ".tsx": {},
	".js": {}, ".jsx": {}, ".vue": {}, ".css": {}, ".scss": {}, ".html": {}, ".htm": {},
	".py": {}, ".rs": {}, ".java": {}, ".kt": {}, ".c": {}, ".h": {}, ".cpp": {}, ".hpp": {},
	".cs": {}, ".rb": {}, ".php": {}, ".sh": {}, ".bash": {}, ".zsh": {}, ".ps1": {},
	".sql": {}, ".ini": {}, ".cfg": {}, ".conf": {}, ".env": {}, ".gitignore": {},
	".srt": {}, ".vtt": {}, ".ass": {}, ".ssa": {},
}

// MaxReadBytes 是只读文本上限（1 MiB）。
const MaxReadBytes = 1 << 20

// IsWriteOrExec 判断工具是否属于一期禁止的写 / 执行类。
func IsWriteOrExec(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "write", "write_file", "exec", "run", "shell", "command":
		return true
	default:
		return false
	}
}

// ResolveUnderRoot 把工具请求里的 rel 按“相对项目根”解析（契约 6.19.10.5）。
// 一律拒绝：根为空、绝对路径（含盘符 / UNC / 以分隔符开头）、含 .. 跳出根、
// 以及解析符号链接 / 目录联接后真实路径不在根的真实路径之内的；比较按目录边界，Windows / macOS 不区分大小写（同 6.8）。
// rel 为空或 "." 表示根本身。
func ResolveUnderRoot(root, rel string) (abs string, ok bool) {
	if strings.TrimSpace(root) == "" {
		return "", false
	}
	root = filepath.Clean(root)
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return root, true
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", false
	}
	target := filepath.Clean(filepath.Join(root, rel))
	relOut, err := filepath.Rel(root, target)
	if err != nil || relOut == ".." || strings.HasPrefix(relOut, ".."+string(filepath.Separator)) {
		return "", false
	}
	// 真实路径检查：根与目标都解析已存在部分的符号链接 / junction，再按目录边界比较。
	if !paths.Within(root, target) {
		return "", false
	}
	return target, true
}

// ListDir 列出目录名（一期只读工具）。
func ListDir(root, rel string) (string, error) {
	abs, ok := ResolveUnderRoot(root, rel)
	if !ok || abs == "" {
		return "", os.ErrPermission
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		b.WriteString(name)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// ReadText 读取白名单文本文件。
func ReadText(root, rel string) (string, error) {
	abs, ok := ResolveUnderRoot(root, rel)
	if !ok {
		return "", os.ErrPermission
	}
	ext := strings.ToLower(filepath.Ext(abs))
	base := filepath.Base(abs)
	if _, allow := textExtAllow[ext]; !allow {
		// 无扩展名但叫 Dockerfile / Makefile / LICENSE 等常见文本名
		low := strings.ToLower(base)
		if low != "dockerfile" && low != "makefile" && low != "license" && low != "licence" && low != "readme" {
			return "", os.ErrInvalid
		}
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if fi.IsDir() || fi.Size() > MaxReadBytes {
		return "", os.ErrInvalid
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// HandleToolRequests 处理工具请求：写/跑一律拒绝并返回用户可见说明；只读尽量执行。
// projectRoot 是对话所属项目的文件夹（6.19.10.5）；对话不属于项目时为空，所有项目工具一律拒绝。
// 被拒绝的请求作为工具失败结果回给适配器，不中断这一轮。
func HandleToolRequests(projectRoot string, reqs []ToolRequest) []ToolResult {
	projectPath := projectRoot
	var out []ToolResult
	for _, r := range reqs {
		res := ToolResult{ID: r.ID}
		if IsWriteOrExec(r.Kind) {
			res.OK = false
			res.Content = MsgToolWriteRef
			out = append(out, res)
			continue
		}
		switch strings.ToLower(strings.TrimSpace(r.Kind)) {
		case "list_dir", "listdir", "list":
			if projectPath == "" {
				res.OK = false
				res.Content = MsgToolNoProject
			} else {
				s, err := ListDir(projectPath, r.Path)
				if err != nil {
					res.OK = false
					res.Content = "无法列出目录。"
				} else {
					res.OK = true
					res.Content = s
				}
			}
		case "read_text", "read", "read_file":
			if projectPath == "" {
				res.OK = false
				res.Content = MsgToolNoProject
			} else {
				s, err := ReadText(projectPath, r.Path)
				if err != nil {
					res.OK = false
					res.Content = "无法读取文件。"
				} else {
					res.OK = true
					res.Content = s
				}
			}
		default:
			res.OK = false
			res.Content = MsgToolWriteRef
		}
		out = append(out, res)
	}
	return out
}
