package media

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
)

const (
	// DefaultThumbWidth 是 width<=0 时使用的缩略图宽度。
	DefaultThumbWidth = 320
	minThumbWidth     = 16
	maxThumbWidth     = 1920

	defaultThumbTimeout = 20 * time.Second

	// 缓存上限：超过任意一个就按最后使用时间从旧到新删除，直到降到上限的 80%。
	defaultCacheMaxFiles = 1000
	defaultCacheMaxBytes = 200 << 20
	// 孤儿 .part 文件（进程崩溃遗留）超过这个时间就清掉。
	stalePartAge = time.Hour
)

// Thumb 是 Thumbnail 的返回值。
type Thumb struct {
	// Path 是缓存里的 jpg 绝对路径（<数据目录>/thumbs/<key>.jpg）。
	Path string `json:"path"`
	// DataURL 是同一张图的 data:image/jpeg;base64,... ，前端直接放进 <img src>，
	// 不需要本地 HTTP 服务（契约 v0.5 已取消）。
	DataURL string  `json:"dataUrl"`
	AtSec   float64 `json:"atSec"`
	Width   int     `json:"width"` // 请求的最大宽度（源视频更窄时不放大，实际宽度可能更小）
}

// thumbCache 管理 thumbs 目录：按 (path_key, mtime, size, atSec, width) 命名，带容量上限。
type thumbCache struct {
	dir      string
	maxFiles int
	maxBytes int64
	now      func() time.Time

	mu    sync.Mutex // 保护 locks 与清理
	locks map[string]*keyLock
}

type keyLock struct {
	mu   sync.Mutex
	refs int
}

func newThumbCache(dir string, maxFiles int, maxBytes int64, now func() time.Time) *thumbCache {
	if maxFiles <= 0 {
		maxFiles = defaultCacheMaxFiles
	}
	if maxBytes <= 0 {
		maxBytes = defaultCacheMaxBytes
	}
	if now == nil {
		now = time.Now
	}
	return &thumbCache{dir: dir, maxFiles: maxFiles, maxBytes: maxBytes, now: now, locks: map[string]*keyLock{}}
}

// cacheName 由路径 key、修改时间、大小、时间点（毫秒）和宽度算出缓存文件名（不含目录）。
func cacheName(pathKey string, mtime time.Time, size int64, atSec float64, width int) string {
	h := sha1.New()
	fmt.Fprintf(h, "%s\x00%d\x00%d\x00%d\x00%d", pathKey, mtime.UnixNano(), size, int64(math.Round(atSec*1000)), width)
	return hex.EncodeToString(h.Sum(nil)) + ".jpg"
}

func (c *thumbCache) path(name string) string { return filepath.Join(c.dir, name) }

// lookup 命中时刷新文件的修改时间（作为最后使用时间）并返回路径。
func (c *thumbCache) lookup(name string) (string, bool) {
	p := c.path(name)
	st, err := os.Stat(p)
	if err != nil || st.IsDir() || st.Size() == 0 {
		return "", false
	}
	now := c.now()
	_ = os.Chtimes(p, now, now)
	return p, true
}

// lock 取得某个缓存文件名的互斥锁，同一张缩略图并发请求只生成一次。
func (c *thumbCache) lock(name string) (unlock func()) {
	c.mu.Lock()
	l := c.locks[name]
	if l == nil {
		l = &keyLock{}
		c.locks[name] = l
	}
	l.refs++
	c.mu.Unlock()
	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		c.mu.Lock()
		if l.refs--; l.refs == 0 {
			delete(c.locks, name)
		}
		c.mu.Unlock()
	}
}

// cleanup 删除过期 .part，并把总文件数 / 总大小压到上限的 80% 以下（最旧的先删）。
func (c *thumbCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type fileInfo struct {
		path string
		mod  time.Time
		size int64
	}
	var files []fileInfo
	var total int64
	now := c.now()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		p := c.path(e.Name())
		if strings.Contains(e.Name(), ".part.") {
			if now.Sub(info.ModTime()) > stalePartAge {
				_ = os.Remove(p)
			}
			continue
		}
		if !strings.HasSuffix(e.Name(), ".jpg") {
			continue
		}
		files = append(files, fileInfo{p, info.ModTime(), info.Size()})
		total += info.Size()
	}
	if len(files) <= c.maxFiles && total <= c.maxBytes {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	targetFiles, targetBytes := c.maxFiles*8/10, c.maxBytes*8/10
	n := len(files)
	for _, f := range files {
		if n <= targetFiles && total <= targetBytes {
			break
		}
		if os.Remove(f.path) == nil {
			n--
			total -= f.size
		}
	}
}

// maxThumbAt 是 atSec 的上限（秒，约 115 天）：再大的值没有意义，还会在换算成毫秒时溢出 int64（如 1e300）。
const maxThumbAt = 1e7

// errThumbTimeout 标记"生成缩略图超时"，超时不会退回第 0 秒重试。
var errThumbTimeout = errors.New("生成缩略图超时")

// imagePatternExts 是 ffmpeg 用 image2 解封装的图片扩展名。文件名里带 % 时（如 a%03d.png），
// image2 会把它当成序列模板而找不到文件，所以要加 -pattern_type none。
// 只对这些扩展名加：其他解封装器（mp4、gif、png_pipe……）不认识这个选项，加了反而报错（ffmpeg 7.1 实测）。
var imagePatternExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".bmp": true, ".webp": true,
	".tif": true, ".tiff": true, ".ppm": true, ".pgm": true, ".pbm": true, ".pam": true,
}

// ffmpegPatternArgs 返回放在 ffmpeg -i 之前的 -pattern_type none（仅当文件名带 % 且是 image2 支持的图片）。
func ffmpegPatternArgs(in string) []string {
	if strings.Contains(filepath.Base(in), "%") && imagePatternExts[strings.ToLower(filepath.Ext(in))] {
		return []string{"-pattern_type", "none"}
	}
	return nil
}

// thumbArgs 生成截图命令行。
//
//   - -ss 放在 -i 前（输入侧快速定位）；
//   - 输入用 `file:` 前缀：路径以 `-` 开头、含冒号或 Windows 盘符时不会被当成选项或协议；
//     图片文件名里有 % 时再加 -pattern_type none，避免被当成序列模板；
//   - -map 0:V:0 只选真正的视频流：大写 V 排除封面图（mp3 的专辑图），没有画面的文件报 "matches no streams"；
//   - scale 用 min(width, iw) 不放大，-2 保证高度为偶数；ffmpeg 默认会按旋转元数据自动转正；
//   - 输出扩展名是 .jpg 但仍显式 -f image2 -update 1，避免 image2 对单文件名的告警。
func thumbArgs(in, out string, atSec float64, width int) []string {
	a := []string{"-hide_banner", "-nostdin", "-v", "error", "-ss", strconv.FormatFloat(atSec, 'f', 3, 64)}
	a = append(a, ffmpegPatternArgs(in)...)
	return append(a,
		"-i", "file:"+in,
		"-map", "0:V:0", "-an", "-sn", "-dn",
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=w='min(%d,iw)':h=-2", width),
		"-pix_fmt", "yuvj420p", "-q:v", "3",
		"-f", "image2", "-update", "1", "-y",
		"file:"+out,
	)
}

// runThumb 生成一张缩略图到 out（调用方保证 out 是 .part 临时文件），返回实际使用的时间点。
// 目标时间超出视频长度时 ffmpeg 不出图，此时退回第 0 秒再试一次（返回 0）；超时、取消不重试。
func runThumb(ctx context.Context, ffmpegExe, in, out string, atSec float64, width int, timeout time.Duration) (float64, error) {
	err := runThumbOnce(ctx, ffmpegExe, in, out, atSec, width, timeout)
	if err == nil {
		return atSec, nil
	}
	var ae *apperr.AppError
	if atSec > 0 && errors.As(err, &ae) && ae.Code == apperr.ProcessFailed && !errors.Is(err, errThumbTimeout) && ctx.Err() == nil {
		return 0, runThumbOnce(ctx, ffmpegExe, in, out, 0, width, timeout)
	}
	return atSec, err
}

func runThumbOnce(ctx context.Context, ffmpegExe, in, out string, atSec float64, width int, timeout time.Duration) error {
	_ = os.Remove(out)
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := ffmpeg.NewCommand(cctx, ffmpegExe, thumbArgs(in, out, atSec, width)...)
	stderr := newTailWriter(maxStderrBytes)
	cmd.Stderr = stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if cctx.Err() == context.DeadlineExceeded {
		return apperr.Wrap(apperr.ProcessFailed, "生成缩略图超时", errThumbTimeout)
	}
	st, statErr := os.Stat(out)
	if runErr == nil && statErr == nil && st.Size() > 0 {
		return nil
	}
	tail := lastLines(stderr.String(), 50)
	if strings.Contains(tail, "matches no streams") || strings.Contains(tail, "does not contain any stream") {
		return apperr.New(apperr.InvalidArgument, "该文件没有视频画面，无法生成缩略图").WithDetail(tail)
	}
	if runErr == nil {
		runErr = errors.New("ffmpeg 没有输出图片")
	}
	return apperr.Wrap(apperr.ProcessFailed, "生成缩略图失败", runErr).WithDetail(tail)
}

const (
	maxProbeStdoutBytes = 16 << 20 // ffprobe JSON 输出上限：畸形文件可能有海量流 / 标签
	maxStderrBytes      = 256 << 10
)

// headWriter 只保留前 max 字节，超出的丢弃（仍返回写入成功，避免子进程因管道写满而卡住），并记录溢出。
type headWriter struct {
	max  int
	buf  bytes.Buffer
	over bool
}

func (w *headWriter) Write(p []byte) (int, error) {
	if room := w.max - w.buf.Len(); room > 0 {
		if len(p) <= room {
			w.buf.Write(p)
		} else {
			w.buf.Write(p[:room])
			w.over = true
		}
	} else if len(p) > 0 {
		w.over = true
	}
	return len(p), nil
}

// tailWriter 只保留最后 max 字节（错误信息需要的是 stderr 的末尾）。
type tailWriter struct {
	max int
	buf []byte
}

func newTailWriter(max int) *tailWriter { return &tailWriter{max: max} }

func (w *tailWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	if len(w.buf) > 2*w.max { // 摊销：超过 2 倍再截，避免每次都拷贝
		w.buf = append([]byte(nil), w.buf[len(w.buf)-w.max:]...)
	}
	return len(p), nil
}

func (w *tailWriter) String() string {
	b := w.buf
	if len(b) > w.max {
		b = b[len(b)-w.max:]
	}
	return string(b)
}

// lastLines 返回 s 的最后 n 行（去掉空行）。
func lastLines(s string, n int) string {
	tb := ffmpeg.NewTailBuffer(n)
	for _, l := range strings.Split(s, "\n") {
		tb.Add(l)
	}
	return tb.String()
}

// dataURL 读取缓存里的 jpg 并编码成 data URL。
func dataURL(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b), nil
}

// clampWidth 把请求宽度限制在 [16, 1920]，<=0 用默认值。
func clampWidth(w int) int {
	switch {
	case w <= 0:
		return DefaultThumbWidth
	case w < minThumbWidth:
		return minThumbWidth
	case w > maxThumbWidth:
		return maxThumbWidth
	}
	return w
}

// defaultThumbAt 是 Probe 生成默认缩略图的时间点：时长的 10%，最多 10 秒，精确到 0.1 秒；未知时长取 0。
func defaultThumbAt(duration float64) float64 {
	if duration <= 0 {
		return 0
	}
	at := math.Min(duration*0.1, 10)
	return math.Floor(at*10) / 10
}
