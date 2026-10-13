package cat

import (
	"context"
	"errors"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"golang.org/x/net/html/charset"
)

const maxBrowserBytes = 1500 * 1024

// browserDesktopUA 是常见桌面 Chrome UA，避免空壳 bot 身份被拒。
const browserDesktopUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// browserHTTPTimeout 是取网页的整次超时；测试可改小以覆盖超时路径。
var browserHTTPTimeout = 20 * time.Second

// BrowserOpenRequest 是要打开的网址。
type BrowserOpenRequest struct {
	URL string `json:"url"`
}

// BrowserPage 是取回来的网页。HTML 还没放进预览，调用方负责嵌进页面。
type BrowserPage struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	HTML  string `json:"html"`
}

var (
	browserTitleRE = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	browserTagRE   = regexp.MustCompile(`(?is)<[^>]*>`)
)

// OpenCatBrowser 取回这个网址的网页，供侧栏浏览器嵌进去。
func (s *Service) OpenCatBrowser(ctx context.Context, req BrowserOpenRequest) (BrowserPage, error) {
	_ = s
	u, err := browserFetchURL(req.URL)
	if err != nil {
		return BrowserPage{}, err
	}
	return fetchBrowserPage(ctx, u)
}

func browserFetchURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, apperr.New(apperr.InvalidArgument, "网址不对。")
	}
	return u, nil
}

func fetchBrowserPage(ctx context.Context, u *url.URL) (BrowserPage, error) {
	client := &http.Client{
		Timeout: browserHTTPTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return apperr.New(apperr.IOError, "页面没有打开。")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return apperr.New(apperr.InvalidArgument, "网址不对。")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return BrowserPage{}, apperr.New(apperr.InvalidArgument, "网址不对。")
	}
	req.Header.Set("User-Agent", browserDesktopUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,image/*;q=0.8,*/*;q=0.5")
	resp, err := client.Do(req)
	if err != nil {
		return BrowserPage{}, browserFetchError(err)
	}
	defer resp.Body.Close()
	final := resp.Request.URL.String()
	if _, err := browserFetchURL(final); err != nil {
		return BrowserPage{}, err
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "image/") {
		return BrowserPage{URL: final, Title: "图片", HTML: browserImagePage(final)}, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBrowserBytes+1))
	if err != nil {
		return BrowserPage{}, apperr.New(apperr.IOError, "页面没有打开。")
	}
	if len(body) > maxBrowserBytes {
		return BrowserPage{}, apperr.New(apperr.InvalidArgument, "这个页面太大。")
	}
	if resp.StatusCode >= 400 && !looksLikeHTML(ct, body) {
		return BrowserPage{}, apperr.New(apperr.IOError, "页面没有打开。")
	}
	if !looksLikeHTML(ct, body) {
		return BrowserPage{}, apperr.New(apperr.InvalidArgument, "这个地址不是网页。")
	}
	text, err := decodeBrowser(body, resp.Header.Get("Content-Type"))
	if err != nil {
		if err == io.ErrUnexpectedEOF {
			return BrowserPage{}, apperr.New(apperr.InvalidArgument, "这个页面太大。")
		}
		return BrowserPage{}, apperr.New(apperr.IOError, "页面没有打开。")
	}
	return BrowserPage{URL: final, Title: browserTitle(text), HTML: text}, nil
}


func browserFetchError(err error) error {
	if isBrowserTimeout(err) {
		return apperr.New(apperr.IOError, "打开超时，请检查网络或换网址。")
	}
	return apperr.New(apperr.IOError, "页面没有打开。")
}

func isBrowserTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if os.IsTimeout(err) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}
func looksLikeHTML(contentType string, body []byte) bool {
	if strings.Contains(contentType, "html") || strings.Contains(contentType, "xml") {
		return true
	}
	s := strings.ToLower(strings.TrimSpace(string(body[:min(len(body), 256)])))
	return strings.HasPrefix(s, "<!doctype html") || strings.HasPrefix(s, "<html") || strings.HasPrefix(s, "<head") || strings.HasPrefix(s, "<body")
}

func decodeBrowser(body []byte, contentType string) (string, error) {
	r, err := charset.NewReader(strings.NewReader(string(body)), contentType)
	if err != nil {
		return string(body), nil
	}
	b, err := io.ReadAll(io.LimitReader(r, maxBrowserBytes+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxBrowserBytes {
		return "", io.ErrUnexpectedEOF
	}
	return strings.ReplaceAll(string(b), "\x00", ""), nil
}

func browserTitle(s string) string {
	m := browserTitleRE.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	t := html.UnescapeString(browserTagRE.ReplaceAllString(m[1], ""))
	t = strings.Join(strings.Fields(t), " ")
	r := []rune(t)
	if len(r) > 80 {
		t = string(r[:80])
	}
	return t
}

func browserImagePage(raw string) string {
	esc := html.EscapeString(raw)
	return "<!doctype html><html><head><title>图片</title></head><body style=\"margin:0;background:#111\"><img alt=\"\" src=\"" + esc + "\" style=\"max-width:100%\"></body></html>"
}
