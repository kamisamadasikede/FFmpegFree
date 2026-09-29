package task

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// PartPath 返回最终输出 final 对应的临时文件名：<name>.part.<原扩展名>（契约 v0.4）。
// 例如 /out/a.mp4 → /out/a.part.mp4。保留扩展名是为了让 ffmpeg 能按扩展名识别封装格式。
func PartPath(final string) string {
	ext := filepath.Ext(final)
	base := strings.TrimSuffix(final, ext)
	return base + ".part" + ext
}

// namer 在进程内登记"已被某个运行中任务占用"的输出路径。
//
// 只检查文件系统会有竞态：两个任务同时想写 out/a.mp4，都看到它不存在，就会写同一个 a.part.mp4。
// 所以选名和登记在同一把锁里完成，登记一直保留到任务结束（Manager 在任务结束时统一释放）。
type namer struct {
	mu   sync.Mutex
	held map[string]string // 规范化后的最终路径 → 占用它的任务 ID
}

func newNamer() *namer { return &namer{held: map[string]string{}} }

// defaultNamer 供不在任务管理器里运行的调用者（测试、一次性工具）使用。
var defaultNamer = newNamer()

var caseInsensitiveFS = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

func nameKey(p string) string {
	p = filepath.Clean(p)
	if caseInsensitiveFS {
		p = strings.ToLower(p)
	}
	return p
}

// reserve 选出一个不与文件系统、.part 文件或其他任务冲突的最终路径（重名依次追加 (1)、(2)），并登记占用。
func (n *namer) reserve(desired, owner string) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	ext := filepath.Ext(desired)
	base := strings.TrimSuffix(desired, ext)
	cand := desired
	for i := 1; ; i++ {
		if _, taken := n.held[nameKey(cand)]; !taken && !exists(cand) && !exists(PartPath(cand)) {
			n.held[nameKey(cand)] = owner
			return cand
		}
		cand = fmt.Sprintf("%s(%d)%s", base, i, ext)
	}
}

func (n *namer) release(p string) {
	n.mu.Lock()
	delete(n.held, nameKey(p))
	n.mu.Unlock()
}

func (n *namer) releaseOwner(owner string) {
	if owner == "" {
		return
	}
	n.mu.Lock()
	for k, o := range n.held {
		if o == owner {
			delete(n.held, k)
		}
	}
	n.mu.Unlock()
}

// UniquePath 在目标已存在时依次尝试 name(1).ext、name(2).ext ……直到找到不存在的路径（契约 6.5）。
// 注意：它只查文件系统（含 .part），不登记占用，不能防止两个并发任务选到同一个名字；
// 任务里选输出名请用 RunWithPart（走任务管理器的占用登记）。
func UniquePath(final string) string {
	ext := filepath.Ext(final)
	base := strings.TrimSuffix(final, ext)
	cand := final
	for i := 1; exists(cand) || exists(PartPath(cand)); i++ {
		cand = fmt.Sprintf("%s(%d)%s", base, i, ext)
	}
	return cand
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// errTargetExists 表示提交 .part 时最终路径已被别人占用。
var errTargetExists = errors.New("目标文件已存在")

// commitPart 把 part 提交为 final，绝不覆盖已存在的文件：
// 先用 os.Link 创建硬链接（目标已存在会失败），成功后删除 .part；
// 文件系统不支持硬链接（FAT / exFAT / 部分网络盘）时退回"先检查再 Rename"。
func commitPart(part, final string) error {
	err := os.Link(part, final)
	if err == nil {
		os.Remove(part)
		return nil
	}
	if errors.Is(err, fs.ErrExist) {
		return errTargetExists
	}
	if exists(final) {
		return errTargetExists
	}
	return os.Rename(part, final)
}

// RunWithPart 为写文件的任务提供统一的输出流程：
//
//  1. 在任务管理器里选出不冲突的最终路径（重名追加 (1)、(2)）并登记占用，创建输出目录；
//  2. 调用 produce(partPath) 让任务写到 <name>.part.<ext>；
//  3. produce 成功后无覆盖地提交为最终路径（os.Link；万一目标在此期间被别的程序创建，改用下一个 (n) 名字）；
//     失败或取消则删除 .part。
//
// 返回最终路径。produce 必须把完整输出写到 partPath；两遍编码等需要多次调用 ffmpeg 的任务
// 在同一个 produce 里依次运行即可（见 FFmpegRunner.BuildPassArgs）。ctx 不带任务信息时使用进程级登记表。
func RunWithPart(ctx context.Context, desired string, produce func(partPath string) error) (string, error) {
	n, owner := defaultNamer, ""
	if info, ok := InfoFrom(ctx); ok && info.m != nil {
		n, owner = info.m.namer, info.ID
	}
	final := n.reserve(desired, owner)
	release := func() { n.release(final) }
	defer func() { release() }()
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return "", fmt.Errorf("创建输出目录失败: %w", err)
	}
	part := PartPath(final)
	if err := produce(part); err != nil {
		os.Remove(part)
		return "", err
	}
	for {
		err := commitPart(part, final)
		if err == nil {
			return final, nil
		}
		if !errors.Is(err, errTargetExists) {
			os.Remove(part)
			return "", fmt.Errorf("重命名输出文件失败: %w", err)
		}
		// 目标被别的程序抢先创建：换下一个名字，把 .part 改名过去。
		next := n.reserve(desired, owner)
		nextPart := PartPath(next)
		if err := os.Rename(part, nextPart); err != nil {
			n.release(next)
			os.Remove(part)
			return "", fmt.Errorf("重命名输出文件失败: %w", err)
		}
		n.release(final)
		final, part = next, nextPart
	}
}

// MkTaskTemp 创建任务专属的临时目录（两遍编码的 -passlogfile 等），返回目录和清理函数。
// base 为空用系统临时目录。目录名带任务 ID 便于排查；清理函数是幂等的。
func MkTaskTemp(ctx context.Context, base string) (dir string, cleanup func(), err error) {
	if base != "" {
		if err := os.MkdirAll(base, 0o755); err != nil {
			return "", func() {}, err
		}
	}
	prefix := "task-"
	if info, ok := InfoFrom(ctx); ok && info.ID != "" {
		prefix += info.ID + "-"
	}
	dir, err = os.MkdirTemp(base, prefix)
	if err != nil {
		return "", func() {}, err
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}
