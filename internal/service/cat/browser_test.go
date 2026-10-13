package cat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserFetchURL(t *testing.T) {
	if _, err := browserFetchURL("javascript:alert(1)"); err == nil {
		t.Fatal("脚本地址不该通过")
	}
	if _, err := browserFetchURL("file:///C:/Windows"); err == nil {
		t.Fatal("本地文件地址不该通过")
	}
	u, err := browserFetchURL("https://example.com/a")
	if err != nil || u.String() != "https://example.com/a" {
		t.Fatalf("网址 %v %v", u, err)
	}
}

func TestBrowserTitle(t *testing.T) {
	got := browserTitle("<html><head><title> 你好 &amp; 世界 </title></head></html>")
	if got != "你好 & 世界" {
		t.Fatalf("标题 %q", got)
	}
	if browserTitle("<html></html>") != "" {
		t.Fatal("没有标题时应为空")
	}
}

func TestOpenCatBrowser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><head><title>示例页</title></head><body><a href=\"/next\">下</a></body></html>"))
	}))
	defer srv.Close()
	u, err := browserFetchURL(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	page, err := fetchBrowserPage(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "示例页" || !strings.Contains(page.HTML, "下") || !strings.HasPrefix(page.URL, srv.URL) {
		t.Fatalf("页面 %+v", page)
	}
}
