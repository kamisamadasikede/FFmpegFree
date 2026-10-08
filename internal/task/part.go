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
	held map[string]holder // 规范化后的最终路径 → 占用者
}

type holder struct {
	owner string // 任务 ID
	path  string // 原样的最终路径
}

func newNamer() *namer { return &namer{held: map[string]holder{}} }

// suffixStyle 是重名后缀格式（契约 6.14.5）：convert 用带空格的 "a (1).mp4"，其余类型 "a(1).mp4"。
type suffixStyle int

const (
	styleCompact suffixStyle = iota // a(1).mp4
	styleSpaced                     // a (1).mp4
)

// styleFor 按任务类型选择重名格式：只有 convert 带空格。
func styleFor(t Type) suffixStyle {
	if t == TypeConvert {
		return styleSpaced
	}
	return styleCompact
}

func numbered(base string, i int, ext string, st suffixStyle) string {
	if st == styleSpaced {
		return fmt.Sprintf("%s (%d)%s", base, i, ext)
	}
	return fmt.Sprintf("%s(%d)%s", base, i, ext)
}

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

// reserve 选出一个不与文件系统、.part 文件或其他任务冲突的最终路径（重名按 st 依次追加 (1)、(2)），并登记占用。
func (n *namer) reserve(desired, owner string, st suffixStyle) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	cand := n.pickLocked(desired, st)
	n.held[nameKey(cand)] = holder{owner: owner, path: cand}
	return cand
}

// peek 同 reserve 但不登记（PreviewOutputName 的“将保存为”提示）。
func (n *namer) peek(desired string, st suffixStyle) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.pickLocked(desired, st)
}

func (n *namer) freeLocked(cand string) bool {
	_, taken := n.held[nameKey(cand)]
	return !taken && !exists(cand) && !exists(PartPath(cand))
}

func (n *namer) pickLocked(desired string, st suffixStyle) string {
	ext := filepath.Ext(desired)
	base := strings.TrimSuffix(desired, ext)
	cand := desired
	for i := 1; ; i++ {
		if n.freeLocked(cand) {
			return cand
		}
		cand = numbered(base, i, ext, st)
	}
}

// hold 原样占用 p（原地重试沿用原输出名）：p 磁盘上没有、没有 .part、没被占时登记并返回 true。
func (n *namer) hold(p, owner string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.freeLocked(p) {
		return false
	}
	n.held[nameKey(p)] = holder{owner: owner, path: p}
	return true
}

// claim 让 owner 占用 p，不看磁盘（原地重转沿用自己的输出名，契约 6.17.3 第 2 步）：被别的任务占着时返回 false。
func (n *namer) claim(p, owner string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if h, ok := n.held[nameKey(p)]; ok && h.owner != owner {
		return false
	}
	n.held[nameKey(p)] = holder{owner: owner, path: p}
	return true
}

// takeHeld 返回 owner 已占的名字（提交时 / 重试时占的）。占着的名字在此期间被别的程序在磁盘上建了同名文件（或 .part）时，
// 释放它并返回 false，由调用方顺延。
func (n *namer) takeHeld(owner string) (string, bool) {
	if owner == "" {
		return "", false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for k, h := range n.held {
		if h.owner != owner {
			continue
		}
		if !exists(h.path) && !exists(PartPath(h.path)) {
			return h.path, true
		}
		delete(n.held, k)
	}
	return "", false
}

// heldByOther 判断 p 是否被 owner 以外的任务占着。
func (n *namer) heldByOther(p, owner string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	h, ok := n.held[nameKey(p)]
	return ok && h.owner != owner
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
	for k, h := range n.held {
		if h.owner == owner {
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
		cand = numbered(base, i, ext, styleCompact)
	}
	return cand
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// errTargetExists 表示提交 .part 时最终路径已被别人占用。
var errTargetExists = errors.New("目标文件已存在")

// 文件系统操作的入口，测试里替换以模拟磁盘满等错误。
var (
	mkdirAll   = os.MkdirAll
	linkFile   = os.Link
	renameFile = renameNoReplace // 见 part_windows.go / part_other.go
)

// commitPart 把 part 提交为 final，绝不覆盖已存在的文件：
// 先用 os.Link 创建硬链接（目标已存在会失败），成功后删除 .part；
// 文件系统不支持硬链接（FAT / exFAT / 部分网络盘）时退回"先检查再 Rename"。
func commitPart(part, final string) error {
	err := linkFile(part, final)
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
	return renameFile(part, final)
}

// RunWithPart 为写文件的任务提供统一的输出流程：
//
//  1. 任务已在提交 / 原地重试时占了名字（convert，契约 6.14.5）就直接用它，只有占着的名字被别的程序在磁盘上建了同名文件时才顺延，
//     新名字落库并随 running 的 task:status.outputPath 告知前端；没占过名字的在任务管理器里选出不冲突的最终路径
//     （重名后缀按任务类型：convert "a (1).mp4"，其余 "a(1).mp4"）并登记占用。然后创建输出目录；
//  2. 调用 produce(partPath) 让任务写到 <name>.part.<ext>；
//  3. produce 成功后无覆盖地提交为最终路径（os.Link；万一目标在此期间被别的程序创建，改用下一个 (n) 名字）；
//     失败或取消则删除 .part。
//
// 返回最终路径。produce 必须把完整输出写到 partPath；需要多次调用 ffmpeg 的任务可以在同一个 produce 里依次运行。ctx 不带任务信息时使用进程级登记表。
func RunWithPart(ctx context.Context, desired string, produce func(partPath string) error) (string, error) {
	n, owner, st := defaultNamer, "", styleCompact
	info, inTask := InfoFrom(ctx)
	if inTask && info.m != nil {
		n, owner, st = info.m.namer, info.ID, styleFor(info.Type)
	}
	final, held := n.takeHeld(owner)
	if !held {
		final = n.reserve(desired, owner, st)
	}
	// 名字和任务记录里的不一样（提交时占的名字被抢了而顺延）时，落库并告知前端。
	announce := func(p string) {
		if inTask && info.m != nil {
			info.m.updateOutput(info.ID, p)
		}
	}
	if held || st == styleSpaced {
		announce(final)
	}
	release := func() { n.release(final) }
	defer func() { release() }()
	if err := mkdirAll(filepath.Dir(final), 0o755); err != nil {
		return "", outputIOError("创建输出目录失败", err)
	}
	part := PartPath(final)
	// produce 发生 panic 时也要清理 .part（panic 会继续向上传播，由 Manager 的 safeRun 转成任务失败）。
	committed := false
	defer func() {
		if !committed {
			os.Remove(part)
		}
	}()
	if err := produce(part); err != nil {
		return "", err
	}
	for {
		err := commitPart(part, final)
		if err == nil {
			committed = true
			return final, nil
		}
		if !errors.Is(err, errTargetExists) {
			os.Remove(part)
			return "", outputIOError("保存输出文件失败", err)
		}
		// 目标被别的程序抢先创建：换下一个名字，把 .part 改名过去。
		next := n.reserve(desired, owner, st)
		nextPart := PartPath(next)
		if err := renameFile(part, nextPart); err != nil {
			n.release(next)
			os.Remove(part)
			return "", outputIOError("保存输出文件失败", err)
		}
		n.release(final)
		final, part = next, nextPart
		if st == styleSpaced {
			announce(final)
		}
	}
}
