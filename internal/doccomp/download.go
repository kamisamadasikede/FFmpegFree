package doccomp

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

// errChecksum 表示下载完成但 SHA-256 与清单不符（重下一次仍不符 → DOC_CHECKSUM_FAILED）。
var errChecksum = errors.New("SHA-256 校验失败")

// downloader 是带 Range 续传的下载器（同 ffmpeg/download.go 的做法，按契约 6.12.12 调整校验失败的处理）。
type downloader struct {
	client      *http.Client
	idleTimeout time.Duration // 连续这么久没有数据就中断本次尝试
	retries     int           // 每个地址断线后的额外重试次数（每次都从 .part 续传）
	retryWait   time.Duration
}

// fetch 把 urls 里第一个能下完的地址下到 partPath（续传），然后校验 SHA-256：
//   - 不通过：删掉 .part、调用 reset（进度归 0），换下一个地址从头重下一次；仍不通过返回 errChecksum。
//   - 所有地址都失败：返回最后一个错误，保留 .part。
func (d *downloader) fetch(ctx context.Context, urls []string, partPath, wantSHA string, wantSize int64, report func(done, total int64), reset func()) error {
	var lastErr error
	checksumFails := 0
	for _, u := range urls {
		var err error
		for attempt := 0; attempt <= d.retries; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(d.retryWait):
				}
			}
			err = d.fetchOnce(ctx, u, partPath, wantSize, report)
			if err == nil || ctx.Err() != nil {
				break
			}
			var he *httpStatusError
			if errors.As(err, &he) && he.code >= 400 && he.code < 500 && he.code != http.StatusRequestedRangeNotSatisfiable {
				break // 4xx 重试没有意义，换下一个地址
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			lastErr = fmt.Errorf("下载失败（%s）: %w", hostOf(u), err)
			continue
		}
		got, err := fileSHA256(ctx, partPath)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("读取下载文件失败: %w", err)
		}
		if strings.EqualFold(got, wantSHA) {
			return nil
		}
		os.Remove(partPath)
		reset()
		checksumFails++
		lastErr = fmt.Errorf("%w（%s）", errChecksum, hostOf(u))
		if checksumFails >= 2 {
			return lastErr // 从头重下过一次仍不通过
		}
	}
	if lastErr == nil {
		lastErr = errors.New("没有可用的下载地址")
	}
	return lastErr
}

func hostOf(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if i := strings.IndexByte(u, '/'); i >= 0 {
		u = u[:i]
	}
	return u
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return "HTTP " + strconv.Itoa(e.code) }

// fetchOnce 做一次请求：有 .part 就带 Range 续传；服务器不支持 Range（回 200）时从头写。
func (d *downloader) fetchOnce(ctx context.Context, url, partPath string, wantSize int64, report func(done, total int64)) error {
	var have int64
	if fi, err := os.Stat(partPath); err == nil {
		have = fi.Size()
	}
	if wantSize > 0 && have > wantSize {
		os.Remove(partPath)
		have = 0
	}
	if wantSize > 0 && have == wantSize {
		report(have, wantSize)
		return nil
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
	total := wantSize
	switch resp.StatusCode {
	case http.StatusPartialContent:
		start, size, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok || start != have {
			return fmt.Errorf("Content-Range 不匹配: %q", resp.Header.Get("Content-Range"))
		}
		if size > 0 {
			total = size
		}
		flag = os.O_WRONLY | os.O_APPEND
	case http.StatusOK:
		have = 0
		if resp.ContentLength > 0 {
			total = resp.ContentLength
		}
		flag = os.O_WRONLY | os.O_TRUNC
	case http.StatusRequestedRangeNotSatisfiable:
		os.Remove(partPath)
		return &httpStatusError{code: resp.StatusCode}
	default:
		return &httpStatusError{code: resp.StatusCode}
	}
	f, err := os.OpenFile(partPath, flag|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

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
			if ctx.Err() != nil {
				return ctx.Err()
			}
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

func parseContentRange(v string) (start, size int64, ok bool) {
	// bytes 100-199/200
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "bytes ") {
		return 0, 0, false
	}
	v = strings.TrimPrefix(v, "bytes ")
	dash := strings.IndexByte(v, '-')
	slash := strings.IndexByte(v, '/')
	if dash < 0 || slash < dash {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(v[:dash], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	if s := v[slash+1:]; s != "*" {
		if size, err = strconv.ParseInt(s, 10, 64); err != nil {
			return 0, 0, false
		}
	}
	return start, size, true
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

func fileSHA256(ctx context.Context, p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, ctxReader{ctx, f}); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
