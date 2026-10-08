//go:build !windows

package live

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

// 调查用：FFMPEGFREE_HLS_URL=<地址> FFMPEGFREE_HLS_OUT=<目录> FFMPEGFREE_HLS_SECS=150
func TestInvestigatePull(t *testing.T) {
	u := os.Getenv("FFMPEGFREE_HLS_URL")
	out := os.Getenv("FFMPEGFREE_HLS_OUT")
	if u == "" || out == "" {
		t.Skip("调查用")
	}
	secs, _ := strconv.Atoi(os.Getenv("FFMPEGFREE_HLS_SECS"))
	if secs == 0 {
		secs = 150
	}
	r := newRealFixture(t, 0)
	r.svc.cfg.Logf = func(f string, a ...any) {
		if os.Getenv("FFMPEGFREE_HLS_LOG") != "" {
			t.Logf(f, a...)
		}
	}
	r.svc.cfg.Emit = func(name string, p any) { t.Logf("event %s %+v", name, p) }
	ps, err := r.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: u})
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("FFMPEGFREE_HLS_SERVE") != "" { // 只开预览，把地址写进 url.txt，开 secs 秒
		_ = os.WriteFile(filepath.Join(out, "url.txt"), []byte(ps.PreviewURL), 0o644)
		time.Sleep(time.Duration(secs) * time.Second)
		return
	}
	get := func(ctx context.Context, file string) (int64, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ps.PreviewURL, nil)
		req.Header.Set("Origin", "wails://wails")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		f, _ := os.Create(file)
		defer f.Close()
		ar, _ := os.Create(file + ".arr")
		defer ar.Close()
		pr, pw := io.Pipe()
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			tags, _ := parseFLVArrivals(pr)
			for _, tg := range tags {
				fmt.Fprintf(ar, "%d %d %d %v %v\n", tg.at.UnixMicro(), tg.kind, tg.ts, tg.keyframe, tg.seq)
			}
		}()
		n, err := io.Copy(io.MultiWriter(f, pw), resp.Body)
		pw.Close()
		wg.Wait()
		return n, err
	}
	time.Sleep(3 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(secs)*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, err := get(ctx, filepath.Join(out, "long.flv"))
		t.Logf("long: %d bytes %v", n, err)
	}()
	i := 0
	for ctx.Err() == nil {
		d := time.Duration(1500+rand.Intn(3000)) * time.Millisecond
		c2, c2cancel := context.WithTimeout(ctx, d)
		if n, err := get(c2, filepath.Join(out, fmt.Sprintf("join%03d.flv", i))); n == 0 && i < 8 {
			t.Logf("join %d: %d %v", i, n, err)
		}
		c2cancel()
		i++
		time.Sleep(time.Duration(rand.Intn(700)) * time.Millisecond)
	}
	wg.Wait()
	t.Logf("joins: %d", i)
}
