package live

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/fsutil"
	"FFmpegFree/internal/proc"
	"FFmpegFree/internal/task"
)

// 屏幕推流的本地存档（契约 6.10）：tee 一次编码写两路，存档是分片 mp4，直接写最终文件名（不走 RunWithPart），
// 任何终态都保留；只有"空壳"（ffprobe 读不出时长）会被删掉并清空 outputPath。

const (
	archivePrefix = "screen-"
	archiveExt    = ".mp4"
	archiveLayout = "20060102-150405" // 本地时间
	maxArchiveTry = 1000
	probeTimeout  = 10 * time.Second
)

// archiveBaseName 是存档文件名（不含扩展名和 (n)）：screen-<yyyyMMdd-HHmmss>，本地时间。文件名里没有任何用户输入；
// 仍过一遍共用的净化规则（直播额外禁止 | ' [ ]）作纵深防御，对这个固定格式是恒等变换（测试断言）。
func archiveBaseName(t time.Time) string {
	return fsutil.SanitizeArchiveName(archivePrefix + t.Format(archiveLayout))
}

// checkArchiveDir 校验 archiveDir：必须是绝对路径，不能是 \\?\ / \\.\ 开头（Windows 设备 / 长路径前缀），
// 不存在则创建，存在但不是目录则 INVALID_ARGUMENT。
func checkArchiveDir(dir string) (string, error) {
	for _, bad := range []string{`\\?\`, `\\.\`, "//?/", "//./"} {
		if strings.HasPrefix(dir, bad) {
			return "", invalidArg("存档目录不合法")
		}
	}
	if !filepath.IsAbs(dir) {
		return "", invalidArg("存档目录必须是绝对路径")
	}
	for _, r := range dir {
		if r < 0x20 || r == 0x7f {
			return "", invalidArg("存档目录含有不合法的字符")
		}
	}
	return filepath.Clean(dir), nil
}

// reserveArchive 创建存档目录并用 O_CREATE|O_EXCL 建一个空文件占位（不能只 Stat 后交给 ffmpeg：-y 会直接覆盖已有文件），
// 已存在则加 (n) 后缀（screen-…(1).mp4、(2)……）。返回占位文件的最终路径——只有这里创建成功的路径才归本任务，可以删。
func (s *Service) reserveArchive(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) || errors.Is(err, syscall.ENOTDIR) { // 路径上某一段是文件（Windows 报 ErrExist / ENOTDIR 之一）
			return "", invalidArg("存档目录不是文件夹")
		}
		return "", apperr.Wrap(apperr.IOError, "无法创建存档目录", err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", invalidArg("存档目录不是文件夹")
	}
	base := archiveBaseName(s.cfg.Now())
	if fsutil.OutputPathTooLong(s.cfg.GOOS, dir, base+"(999)", archiveExt) {
		return "", invalidArg("存档路径太长")
	}
	for i := 0; i < maxArchiveTry; i++ {
		name := base + archiveExt
		if i > 0 {
			name = base + "(" + strconv.Itoa(i) + ")" + archiveExt
		}
		p := filepath.Join(dir, name)
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			f.Close()
			return p, nil
		}
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return "", apperr.Wrap(apperr.IOError, "无法在存档目录创建文件", err)
	}
	return "", apperr.New(apperr.IOError, "存档目录里的同名文件太多")
}

// probeDuration 用 ffprobe 读存档的时长（秒）；读不出（没有 moov、没有分片、0 字节）返回错误。
func probeDuration(ctx context.Context, ffprobe, path string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := ffmpeg.NewCommand(ctx, ffprobe, "-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", "file:"+path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := proc.Run(cmd); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return 0, fmt.Errorf("%w: %v", errNoDuration, err)
		}
		return 0, err // ffprobe 没能运行（找不到、超时被杀）：不能据此说文件是空壳
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(out.String()), 64)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%w: %q", errNoDuration, strings.TrimSpace(out.String()))
	}
	return d, nil
}

// errNoDuration 表示 ffprobe 正常运行但读不出时长（空壳存档的判据）。ProbeDuration 的其他错误（ffprobe 起不来、超时）
// 不算空壳：宁可多留一个文件，也不误删可能完好的存档。
var errNoDuration = errors.New("读不出时长")

// archiveGuard 记录本任务自己创建的存档占位文件，并在任务结束时清理空壳。
type archiveGuard struct {
	s       *Service
	ffprobe string
	path    string // 本任务用 O_EXCL 创建的最终路径；只有它可以被删
}

// finish 在 Run 返回后调用：返回要交给任务管理器的输出路径——存档有内容返回它，空壳删除并返回 task.ClearOutputPath。
// 任何终态都走这里（强杀、失败、中断也一样）。删除失败只记日志（不含推流地址），不影响任务状态。
// 先于终态事件：任务管理器在 Run 返回之后才落终态，outputPath 已经是最终值。
func (g *archiveGuard) finish() string {
	if g.usable() {
		return g.path
	}
	if err := g.s.cfg.Remove(g.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		g.s.logf("删除空壳存档失败: %s: %v", g.path, err) // 只记日志；outputPath 仍然清空：空壳不是存档
	}
	return task.ClearOutputPath
}

// usable 判断存档是否有可播放内容：ffprobe 读得出时长即保留。没有 ffprobe 时退回"文件非空"。
func (g *archiveGuard) usable() bool {
	fi, err := os.Lstat(g.path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 {
		return false
	}
	if g.ffprobe == "" {
		return true
	}
	_, err = g.s.cfg.ProbeDuration(context.Background(), g.ffprobe, g.path)
	return err == nil || !errors.Is(err, errNoDuration)
}

// archiveRunner 在直播 Runner 之上加存档收尾。它同时实现 task.Finalizer（转发给 runner 释放会话）
// 和"未运行就结束"的清理（排队 / 提交时被取消，Run 从未执行）。
type archiveRunner struct {
	*runner
	g *archiveGuard
}

func (r *archiveRunner) Run(ctx context.Context, report func(task.Progress)) (string, error) {
	_, err := r.runner.Run(ctx, report)
	return r.g.finish(), err
}

// NeverRan 由任务管理器在 Run 没有执行就结束任务时调用（如刚提交就被取消）：占位文件是空的，删掉并清空 outputPath。
func (r *archiveRunner) NeverRan() string {
	os.Remove(r.g.path) // 空占位，只有本任务创建；失败无所谓，只是留下一个 0 字节文件
	return task.ClearOutputPath
}
