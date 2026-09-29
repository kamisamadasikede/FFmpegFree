package ffmpeg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Progress 是下载 / 安装过程的进度快照。任务管理器接入后把它映射成 task:progress。
type Progress struct {
	Phase    string  // download | verify | extract | validate
	Done     int64   // 已下载字节（download 阶段有效）
	Total    int64   // 总字节（未知为 0）
	Fraction float64 // 整个安装流程的进度 0~1
	Speed    float64 // 字节/秒，仅 download 阶段
	EtaSec   float64 // 剩余秒数，未知为 0
}

// ProgressFunc 接收进度回调。实现必须快速返回（下载循环里同步调用），且可并发安全地被调用一次一个。
type ProgressFunc func(Progress)

// 阶段名。
const (
	PhaseDownload = "download"
	PhaseVerify   = "verify"
	PhaseExtract  = "extract"
	PhaseValidate = "validate"
)

// ErrChecksum 表示下载完成但 SHA256 与清单不符。
var ErrChecksum = errors.New("SHA256 校验失败")

// downloader 负责带 Range 续传的下载。
type downloader struct {
	client      *http.Client
	idleTimeout time.Duration // 连续这么久没有数据就中断本次尝试
	retries     int           // 每个地址在网络错误后的额外重试次数（每次都从 .part 续传）
	retryWait   time.Duration
}

// fetch 把 urls[0..] 中第一个成功的地址下载到 partPath（续传），返回时文件完整且 SHA256 与 want 一致。
// 失败时保留 .part（校验失败除外：内容已坏，删除以免下次续传后继续失败）。
// report 收到本文件已下载字节数（含续传前已有的部分）。
func (d *downloader) fetch(ctx context.Context, urls []string, partPath, wantSHA string, wantSize int64, report func(done, total int64)) error {
	var lastErr error
	for _, u := range urls {
		for attempt := 0; attempt <= d.retries; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(d.retryWait):
				}
			}
			err := d.fetchOnce(ctx, u, partPath, wantSize, report)
			if err == nil {
				lastErr = nil
				break
			}
			lastErr = fmt.Errorf("下载 %s 失败: %w", u, err)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var he *httpStatusError
			if errors.As(err, &he) && he.code >= 400 && he.code < 500 && he.code != http.StatusRequestedRangeNotSatisfiable {
				break // 4xx 重试没有意义，换下一个地址
			}
		}
		if lastErr != nil {
			continue
		}
		// 数据齐了，校验。
		got, err := fileSHA256(partPath)
		if err != nil {
			return fmt.Errorf("读取下载文件失败: %w", err)
		}
		if !strings.EqualFold(got, wantSHA) {
			os.Remove(partPath)
			lastErr = fmt.Errorf("%w: 期望 %s，实际 %s（来源 %s）", ErrChecksum, wantSHA, got, u)
			continue // 换下一个地址（镜像可能给了坏文件）
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("没有可用的下载地址")
	}
	return lastErr
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return "HTTP " + strconv.Itoa(e.code) }

// fetchOnce 做一次请求：有 .part 就带 Range 续传。
func (d *downloader) fetchOnce(ctx context.Context, url, partPath string, wantSize int64, report func(done, total int64)) error {
	var have int64
	if fi, err := os.Stat(partPath); err == nil {
		have = fi.Size()
	}
	if wantSize > 0 && have > wantSize {
		os.Remove(partPath) // 比预期还大，肯定坏了
		have = 0
	}
	if wantSize > 0 && have == wantSize {
		report(have, wantSize)
		return nil // 已经下完，交给上层校验
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "FFmpegFree")
	if have > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(have, 10)+"-")
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var flag int
	var total int64
	switch resp.StatusCode {
	case http.StatusPartialContent:
		start, size, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok || start != have {
			return fmt.Errorf("服务器返回了不匹配的 Content-Range: %q", resp.Header.Get("Content-Range"))
		}
		total = size
		flag = os.O_WRONLY | os.O_APPEND
	case http.StatusOK:
		// 服务器不支持 Range（或没有 .part），从头写。
		have = 0
		total = resp.ContentLength
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case http.StatusRequestedRangeNotSatisfiable:
		// .part 的长度不合法：删掉，让上层重试时从头下。
		os.Remove(partPath)
		return &httpStatusError{code: resp.StatusCode}
	default:
		return &httpStatusError{code: resp.StatusCode}
	}
	if total <= 0 {
		total = wantSize
	}

	f, err := os.OpenFile(partPath, flag|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	// 空闲看门狗：连续 idleTimeout 没有读到数据就取消请求。
	idle := time.AfterFunc(d.idleTimeout, cancel)
	defer idle.Stop()

	buf := make([]byte, 128<<10)
	done := have
	report(done, total)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			idle.Reset(d.idleTimeout)
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			report(done, total)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if total > 0 && done != total {
		return fmt.Errorf("下载不完整: %d/%d 字节", done, total)
	}
	return nil
}

// parseContentRange 解析 "bytes 100-999/1000"。
func parseContentRange(h string) (start, total int64, ok bool) {
	rest, found := strings.CutPrefix(h, "bytes ")
	if !found {
		return 0, 0, false
	}
	rng, tot, found := strings.Cut(rest, "/")
	if !found {
		return 0, 0, false
	}
	s, _, found := strings.Cut(rng, "-")
	if !found {
		return 0, 0, false
	}
	var err error
	if start, err = strconv.ParseInt(s, 10, 64); err != nil {
		return 0, 0, false
	}
	if tot == "*" {
		return start, 0, true
	}
	if total, err = strconv.ParseInt(tot, 10, 64); err != nil {
		return 0, 0, false
	}
	return start, total, true
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
