package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// 安装流程各阶段在整体进度条里占的区间（下载最耗时，占大头）。
const (
	fracDownloadEnd = 0.90
	fracExtractEnd  = 0.94
	fracValidateEnd = 0.98
)

// Installer 实现契约 9.3 的下载安装：下载（Range 续传）→ SHA256 → 解压到临时目录 →
// 用 Locator 校验 → macOS 必要时 ad-hoc 签名 → 原子改名进 BinDir。
// 不修改系统 PATH。同一时刻只允许一次安装（重复调用返回 ErrInstallBusy）。
type Installer struct {
	Manifest *Manifest
	Locator  *Locator // 校验用，其 Run 也用于 codesign
	Platform string   // 清单键，默认 PlatformKey()
	GOOS     string   // 默认 runtime.GOOS
	BinDir   string   // <数据目录>/bin
	TempDir  string   // <数据目录>/tmp，.part 与解压暂存目录都在这里（与 BinDir 同卷，保证改名原子）

	Client      *http.Client
	IdleTimeout time.Duration // 下载连续无数据的超时，默认 30s
	Retries     int           // 网络错误后每个地址额外重试次数，默认 2
	RetryWait   time.Duration // 默认 2s
	// ProgressInterval 是进度回调的最小间隔，默认 250ms（契约：每任务最多 4 次/秒）；负数表示不限制（测试用）。
	ProgressInterval time.Duration

	mu   sync.Mutex
	busy bool
}

// ErrInstallBusy 表示已经有安装在进行。
var ErrInstallBusy = errors.New("已有 ffmpeg 安装在进行")

// NewInstaller 用默认依赖创建 Installer。
func NewInstaller(m *Manifest, loc *Locator, binDir, tempDir string) *Installer {
	return &Installer{Manifest: m, Locator: loc, BinDir: binDir, TempDir: tempDir}
}

func (in *Installer) platform() string {
	if in.Platform != "" {
		return in.Platform
	}
	return PlatformKey()
}

func (in *Installer) goos() string {
	if in.GOOS != "" {
		return in.GOOS
	}
	return runtime.GOOS
}

func (in *Installer) downloader() *downloader {
	c := in.Client
	if c == nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.ResponseHeaderTimeout = 30 * time.Second
		c = &http.Client{Transport: tr} // 不设整体 Timeout：安装包很大，由空闲看门狗兜底
	}
	d := &downloader{client: c, idleTimeout: in.IdleTimeout, retries: in.Retries, retryWait: in.RetryWait}
	if d.idleTimeout <= 0 {
		d.idleTimeout = 30 * time.Second
	}
	if d.retries == 0 {
		d.retries = 2
	} else if d.retries < 0 {
		d.retries = 0
	}
	if d.retryWait <= 0 {
		d.retryWait = 2 * time.Second
	}
	return d
}

// Preflight 在真正开始前同步检查：mirror 合法、当前平台有下载源。
// 返回 *UnavailableError 表示平台不支持；其他错误表示参数不合法。
func (in *Installer) Preflight(mirror string) error {
	_, err := in.Manifest.Resolve(in.platform(), mirror)
	return err
}

// MirrorFallsBack 报告请求的镜像在当前平台是否会退回默认源（清单里没有该镜像的条目）。
func (in *Installer) MirrorFallsBack(mirror string) bool {
	plan, err := in.Manifest.Resolve(in.platform(), mirror)
	return err == nil && plan.MirrorFallback
}

// throttle 限制进度回调频率（契约：每任务最多 4 次/秒），阶段切换和结束一定放行。
type throttle struct {
	fn       ProgressFunc
	interval time.Duration
	last     time.Time
	phase    string
}

func (t *throttle) emit(p Progress, force bool) {
	if t.fn == nil {
		return
	}
	now := time.Now()
	if !force && p.Phase == t.phase && now.Sub(t.last) < t.interval {
		return
	}
	t.last, t.phase = now, p.Phase
	t.fn(p)
}

// Install 执行安装并返回安装后 BinDir 里 ffmpeg 的校验信息。
// 失败时保留 TempDir 里的 .part 以便下次续传（校验失败的除外，那份数据已经坏了）；
// ctx 取消同样保留 .part。
func (in *Installer) Install(ctx context.Context, mirror string, progress ProgressFunc) (Info, error) {
	plan, err := in.Manifest.Resolve(in.platform(), mirror)
	if err != nil {
		return Info{}, err
	}
	in.mu.Lock()
	if in.busy {
		in.mu.Unlock()
		return Info{}, ErrInstallBusy
	}
	in.busy = true
	in.mu.Unlock()
	defer func() { in.mu.Lock(); in.busy = false; in.mu.Unlock() }()

	if err := os.MkdirAll(in.TempDir, 0o755); err != nil {
		return Info{}, fmt.Errorf("创建临时目录失败: %w", err)
	}
	if err := os.MkdirAll(in.BinDir, 0o755); err != nil {
		return Info{}, fmt.Errorf("创建 bin 目录失败: %w", err)
	}
	tp := &throttle{fn: progress, interval: in.ProgressInterval}
	if tp.interval == 0 {
		tp.interval = 250 * time.Millisecond
	}
	dl := in.downloader()

	// 1. 下载 + 校验
	var totalBytes int64
	for _, s := range plan.Sources {
		totalBytes += s.Archive.Size
	}
	var doneBefore int64 // 已完成的压缩包字节数
	started := time.Now()
	var startBytes int64 = -1
	var partPaths []string
	for _, s := range plan.Sources {
		a := s.Archive
		part := filepath.Join(in.TempDir, fmt.Sprintf("ffmpeg-%s-%s.part", plan.Version, a.SHA256[:12]))
		partPaths = append(partPaths, part)
		err := dl.fetch(ctx, s.URLs, part, a.SHA256, a.Size, func(done, total int64) {
			cur := doneBefore + done
			if startBytes < 0 {
				startBytes = cur // 续传时不把已有部分算进速度
			}
			p := Progress{Phase: PhaseDownload, Done: cur, Total: totalBytes}
			if totalBytes > 0 {
				p.Fraction = fracDownloadEnd * float64(cur) / float64(totalBytes)
			}
			if el := time.Since(started).Seconds(); el > 0.5 {
				p.Speed = float64(cur-startBytes) / el
				if p.Speed > 0 && totalBytes > cur {
					p.EtaSec = float64(totalBytes-cur) / p.Speed
				}
			}
			tp.emit(p, false)
		})
		if err != nil {
			return Info{}, err
		}
		doneBefore += a.Size
	}
	tp.emit(Progress{Phase: PhaseExtract, Done: totalBytes, Total: totalBytes, Fraction: fracDownloadEnd}, true)

	// 2. 解压到暂存目录（只取 ffmpeg / ffprobe）
	stage, err := os.MkdirTemp(in.TempDir, "ffmpeg-stage-")
	if err != nil {
		return Info{}, fmt.Errorf("创建暂存目录失败: %w", err)
	}
	defer os.RemoveAll(stage)
	var names []string
	for i, s := range plan.Sources {
		if err := extractItems(partPaths[i], s.Archive.Type, s.Archive.Extract, stage); err != nil {
			return Info{}, fmt.Errorf("解压失败: %w", err)
		}
		for _, it := range s.Archive.Extract {
			names = append(names, it.Name)
		}
	}
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	tp.emit(Progress{Phase: PhaseValidate, Done: totalBytes, Total: totalBytes, Fraction: fracExtractEnd}, true)

	// 3. 权限 + 校验（macOS 失败时 ad-hoc 签名再验一次）
	staged := Binaries{
		FFmpeg:  filepath.Join(stage, in.Locator.exeName("ffmpeg")),
		FFprobe: filepath.Join(stage, in.Locator.exeName("ffprobe")),
	}
	if in.goos() != "windows" {
		for _, n := range names {
			if err := os.Chmod(filepath.Join(stage, n), 0o755); err != nil {
				return Info{}, fmt.Errorf("设置可执行权限失败: %w", err)
			}
		}
	}
	if err := in.validate(ctx, staged); err != nil {
		return Info{}, err
	}
	tp.emit(Progress{Phase: PhaseValidate, Done: totalBytes, Total: totalBytes, Fraction: fracValidateEnd}, true)

	// 4. 原子改名进 bin/
	if err := in.commit(stage, names); err != nil {
		return Info{}, err
	}
	final := Binaries{
		FFmpeg:  filepath.Join(in.BinDir, in.Locator.exeName("ffmpeg")),
		FFprobe: filepath.Join(in.BinDir, in.Locator.exeName("ffprobe")),
	}
	info, state, reason := in.Locator.Check(ctx, SourceBundled, final)
	if state != StateReady {
		return Info{}, fmt.Errorf("安装后校验未通过: %s", reason)
	}
	for _, p := range partPaths {
		os.Remove(p) // 成功后才清理已下载的压缩包
	}
	tp.emit(Progress{Phase: PhaseValidate, Done: totalBytes, Total: totalBytes, Fraction: 1}, true)
	return info, nil
}

// validate 用 Locator 的同一套规则校验暂存的二进制；macOS 上失败时先 codesign -s - 再试一次。
func (in *Installer) validate(ctx context.Context, b Binaries) error {
	_, state, reason := in.Locator.Check(ctx, SourceBundled, b)
	if state == StateReady {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.goos() == "darwin" {
		for _, f := range []string{b.FFmpeg, b.FFprobe} {
			if _, err := in.Locator.run()(ctx, "codesign", "-s", "-", "--force", f); err != nil {
				return fmt.Errorf("校验失败（%s），ad-hoc 签名 %s 也失败: %w", reason, filepath.Base(f), err)
			}
		}
		_, state, reason2 := in.Locator.Check(ctx, SourceBundled, b)
		if state == StateReady {
			return nil
		}
		reason = reason2
	}
	return fmt.Errorf("下载的 ffmpeg 校验未通过: %s", reason)
}

// commit 把暂存文件改名到 BinDir。已有同名文件先挪到暂存目录里备份，任何一步失败都回滚，
// 避免 bin/ 里出现新旧混杂（例如新 ffmpeg + 旧 ffprobe）。
func (in *Installer) commit(stage string, names []string) error {
	type moved struct{ dst, backup string }
	var done []moved
	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			m := done[i]
			os.Remove(m.dst)
			if m.backup != "" {
				os.Rename(m.backup, m.dst)
			}
		}
	}
	for _, n := range names {
		src := filepath.Join(stage, n)
		dst := filepath.Join(in.BinDir, n)
		m := moved{dst: dst}
		if _, err := os.Stat(dst); err == nil {
			m.backup = filepath.Join(stage, "old-"+n)
			if err := os.Rename(dst, m.backup); err != nil {
				rollback()
				return fmt.Errorf("替换 %s 失败（文件可能正在使用）: %w", n, err)
			}
		}
		if err := os.Rename(src, dst); err != nil {
			if m.backup != "" {
				os.Rename(m.backup, dst)
			}
			rollback()
			return fmt.Errorf("安装 %s 失败: %w", n, err)
		}
		done = append(done, m)
	}
	return nil
}

// PartFiles 返回 TempDir 里现存的 .part 文件（测试与排查用）。
func (in *Installer) PartFiles() []string {
	m, _ := filepath.Glob(filepath.Join(in.TempDir, "ffmpeg-*.part"))
	return m
}

// IsUnavailable 判断错误是否是"当前平台没有下载源"。
func IsUnavailable(err error) bool {
	var u *UnavailableError
	return errors.As(err, &u)
}
