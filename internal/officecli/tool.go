// Package officecli integrates the native OfficeCLI renderer, never OOXML rendering in the browser.
package officecli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"FFmpegFree/internal/proc"
)

const Version = "1.0.157"
const MaxHTML = 48 << 20

var digests = map[string]string{
	"win-x64":     "bd64eba587e56cc71709cdd1f38ae9ac77cd65a9c8ea744b0996897c0557e343",
	"win-arm64":   "786e08b1ccf09c3a2107912537fe25fa82970b30145e3c59e09d83bf4032b065",
	"linux-x64":   "f8eefe88dbfa7464bee421e5421efe57fc26bc3180b30e56eef58594c8245c94",
	"linux-arm64": "60db926c8a1371984b9e99622a834777befb02b6f73d0f3fff914ffcc2760d2f",
	"mac-x64":     "178c1e9ebaf44c24d2b4b359064a5dea6eb7b2f539809871a3de1c09599dff9f",
	"mac-arm64":   "0dae0990a9cc29688a5d54469f984c34e5b15ab6bbd194a0ffdc571bb9040124",
}

func platform() string {
	osName := map[string]string{"windows": "win", "darwin": "mac", "linux": "linux"}[runtime.GOOS]
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
	return osName + "-" + arch
}

func ToolPath() string {
	root, _ := os.UserCacheDir()
	name := "officecli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(root, "ffc", "tools", "officecli", Version, name)
}

func Available() bool { return validBinary(ToolPath(), digests[platform()]) }

// Install fetches an immutable release, verifies its official SHA256 and atomically publishes it.
// Callers serialize installation; no shell scripts, PATH changes or global npm packages are used.
func Install(ctx context.Context) (string, error) {
	dest := ToolPath()
	digest := digests[platform()]
	if digest == "" {
		return "", fmt.Errorf("OfficeCLI 不支持此平台 %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if validBinary(dest, digest) {
		return dest, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return "", err
	}
	asset := "officecli-" + platform()
	if runtime.GOOS == "windows" {
		asset += ".exe"
	}
	client := &http.Client{Timeout: 70 * time.Second}
	var last error
	for _, base := range []string{"https://d.officecli.ai/releases/download/", "https://github.com/iOfficeAI/OfficeCLI/releases/download/"} {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		last = func() error {
			req, err := http.NewRequestWithContext(ctx, "GET", base+"v"+Version+"/"+asset, nil)
			if err != nil {
				return err
			}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return fmt.Errorf("OfficeCLI 下载 HTTP %d", resp.StatusCode)
			}
			f, err := os.CreateTemp(filepath.Dir(dest), "download-*")
			if err != nil {
				return err
			}
			defer os.Remove(f.Name())
			h := sha256.New()
			n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 80<<20))
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			if n >= 80<<20 || hex.EncodeToString(h.Sum(nil)) != digest {
				return fmt.Errorf("OfficeCLI 下载校验失败")
			}
			if err = os.Chmod(f.Name(), 0700); err != nil {
				return err
			}
			return os.Rename(f.Name(), dest)
		}()
		if last == nil {
			return dest, nil
		}
	}
	return "", fmt.Errorf("OfficeCLI 安装失败，可重试或使用静态 Office 预览: %w", last)
}

func validBinary(path, digest string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == digest
}

func Command(ctx context.Context, exe string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, exe, args...)
	// Direct disk reads also see modifications by PptxGenJS/PowerPoint. The managed skill
	// uses the same settings and saves each increment, avoiding a stale resident cache.
	c.Env = append(os.Environ(), "OFFICECLI_SKIP_UPDATE=1", "OFFICECLI_NO_AUTO_RESIDENT=1", "OFFICECLI_RESIDENT_FLUSH=each")
	proc.Configure(c)
	c.Cancel = func() error { return proc.Kill(c) }
	return c
}

// Render invokes the CLI HTML engine. Output is bounded and temporary files are always removed.
func Render(ctx context.Context, exe, file string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "ffc-ppt-render-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "preview.html")
	c := Command(ctx, exe, "view", file, "html", "-o", output)
	log := &limitedWriter{limit: 8192}
	c.Stdout = log
	c.Stderr = log
	if err = proc.Run(c); err != nil {
		return "", fmt.Errorf("PPT 渲染失败: %w (%s)", err, log.String())
	}
	f, err := os.Open(output)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxHTML+1))
	if err != nil {
		return "", err
	}
	if len(b) > MaxHTML {
		return "", fmt.Errorf("PPT 预览超过 48 MiB，请拆分演示文稿或压缩图片")
	}
	return string(b), nil
}

type limitedWriter struct {
	limit int
	b     strings.Builder
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if left := w.limit - w.b.Len(); left > 0 {
		if len(p) > left {
			p = p[:left]
		}
		w.b.Write(p)
	}
	return n, nil
}
func (w *limitedWriter) String() string { return w.b.String() }
