package localassets

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name string, n int) string {
	t.Helper()
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func get(t *testing.T, h http.Handler, method, url string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRegisterAndServeWhole(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.mp4", 1000)
	reg := New(Config{})
	e, err := reg.Register(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(e.URL, "/local/") || len(e.Token) != 32 || e.Mime != "video/mp4" || e.Size != 1000 {
		t.Fatalf("entry: %+v", e)
	}
	e2, _ := reg.Register(p)
	if e2.Token != e.Token {
		t.Fatal("同一文件应复用 token")
	}
	rec := get(t, reg.Handler(), "GET", e.URL, nil)
	if rec.Code != 200 || rec.Body.Len() != 1000 {
		t.Fatalf("code=%d len=%d", rec.Code, rec.Body.Len())
	}
	h := rec.Header()
	if h.Get("Accept-Ranges") != "bytes" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Content-Type") != "video/mp4" || h.Get("Cache-Control") != "no-store" {
		t.Fatalf("headers: %v", h)
	}
	head := get(t, reg.Handler(), "HEAD", e.URL, nil)
	if head.Code != 200 || head.Body.Len() != 0 || head.Header().Get("Content-Length") != "1000" {
		t.Fatalf("HEAD: %d %v", head.Code, head.Header())
	}
}

func TestRangeTruncation(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.mp3", 100)
	reg := New(Config{MaxRangeBytes: 10, MaxWholeBytes: 50})
	e, _ := reg.Register(p)
	h := reg.Handler()
	cases := []struct {
		rng, cr string
		code    int
		n       int
	}{
		{"bytes=0-4", "bytes 0-4/100", 206, 5},
		{"bytes=0-99", "bytes 0-9/100", 206, 10},  // 截断
		{"bytes=95-", "bytes 95-99/100", 206, 5},  // 开区间
		{"bytes=20-", "bytes 20-29/100", 206, 10}, // 开区间截断
		{"bytes=-5", "bytes 95-99/100", 206, 5},   // 后缀
		{"bytes=-80", "bytes 20-29/100", 206, 10}, // 后缀超限：从后缀起点取前 max 字节
		{"bytes=90-1000", "bytes 90-99/100", 206, 10},
		{"bytes=100-", "bytes */100", 416, 0},
		{"bytes=500-600", "bytes */100", 416, 0},
		{"bytes=0-1,5-6", "bytes */100", 416, 0}, // 多段
		{"bytes=abc", "bytes */100", 416, 0},
		{"bytes=5-2", "bytes */100", 416, 0},
		{"bytes=-0", "bytes */100", 416, 0},
	}
	for _, c := range cases {
		rec := get(t, h, "GET", e.URL, map[string]string{"Range": c.rng})
		if rec.Code != c.code {
			t.Errorf("%s: code=%d want %d", c.rng, rec.Code, c.code)
			continue
		}
		if rec.Header().Get("Content-Range") != c.cr {
			t.Errorf("%s: Content-Range=%q want %q", c.rng, rec.Header().Get("Content-Range"), c.cr)
		}
		if c.code == 206 && rec.Body.Len() != c.n {
			t.Errorf("%s: len=%d want %d", c.rng, rec.Body.Len(), c.n)
		}
	}
	// 精确断言几个
	rec := get(t, h, "GET", e.URL, map[string]string{"Range": "bytes=0-99"})
	if rec.Header().Get("Content-Range") != "bytes 0-9/100" || rec.Body.Len() != 10 {
		t.Fatalf("truncate: %v len=%d", rec.Header(), rec.Body.Len())
	}
	body, _ := io.ReadAll(rec.Body)
	for i, b := range body {
		if b != byte(i%251) {
			t.Fatal("内容不对")
		}
	}
	rec = get(t, h, "GET", e.URL, map[string]string{"Range": "bytes=-5"})
	if rec.Header().Get("Content-Range") != "bytes 95-99/100" || rec.Body.Len() != 5 {
		t.Fatalf("suffix: %v", rec.Header())
	}
	rec = get(t, h, "GET", e.URL, map[string]string{"Range": "bytes=-80"})
	if rec.Body.Len() != 10 || !strings.HasPrefix(rec.Header().Get("Content-Range"), "bytes 20-29/100") {
		t.Fatalf("suffix 截断（从后缀起点取前 max 字节）: %v len=%d", rec.Header(), rec.Body.Len())
	}
	rec = get(t, h, "GET", e.URL, map[string]string{"Range": "bytes=100-"})
	if rec.Header().Get("Content-Range") != "bytes */100" {
		t.Fatalf("416 Content-Range: %v", rec.Header())
	}
	// 无 Range：100 > MaxWholeBytes(50) → 413
	if rec := get(t, h, "GET", e.URL, nil); rec.Code != 413 {
		t.Fatalf("whole large: %d", rec.Code)
	}
	// 非 bytes 单位当作无 Range
	if rec := get(t, h, "GET", e.URL, map[string]string{"Range": "items=0-1"}); rec.Code != 413 {
		t.Fatalf("unit: %d", rec.Code)
	}
}

func TestParseRangeOverflow(t *testing.T) {
	if _, _, k := parseRange("bytes=0-99999999999999999999999", 100, 10); k != rangeInvalid {
		t.Fatal("溢出应 416")
	}
	if _, _, k := parseRange("bytes=+1-2", 100, 10); k != rangeInvalid {
		t.Fatal("符号应拒绝")
	}
}

func TestHandlerSecurity(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.pdf", 10)
	reg := New(Config{})
	e, _ := reg.Register(p)
	h := reg.Handler()
	for _, u := range []string{
		"/local/", "/local/xyz", "/local/" + e.Token + "/", "/local/" + e.Token + "/x", "/local/../" + e.Token,
		"/local/" + strings.ToUpper(e.Token), "/other/" + e.Token, "/local/" + e.Token + "%2f..", "/" + p, "/local/" + p,
	} {
		if rec := get(t, h, "GET", u, nil); rec.Code != 404 {
			t.Errorf("%s: %d", u, rec.Code)
		}
	}
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		if rec := get(t, h, m, e.URL, nil); rec.Code != 405 {
			t.Errorf("%s: %d", m, rec.Code)
		}
	}
	// 查询参数不影响
	if rec := get(t, h, "GET", e.URL+"?path=/etc/passwd", nil); rec.Code != 200 || rec.Body.Len() != 10 {
		t.Errorf("query: %d", rec.Code)
	}
}

func TestRegisterRejects(t *testing.T) {
	dir := t.TempDir()
	reg := New(Config{})
	if _, err := reg.Register("rel/a.mp4"); err != ErrNotAbs {
		t.Fatalf("相对路径: %v", err)
	}
	if _, err := reg.Register(""); err != ErrNotAbs {
		t.Fatalf("空: %v", err)
	}
	if _, err := reg.Register(filepath.Join(dir, "none.mp4")); err == nil {
		t.Fatal("不存在应失败")
	}
	if _, err := reg.Register(dir); err != ErrNotRegular {
		t.Fatalf("目录: %v", err)
	}
}

func TestSymlinkSwapAndDelete(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("符号链接需要权限")
	}
	dir := t.TempDir()
	p := writeFile(t, dir, "a.mp4", 10)
	secret := writeFile(t, dir, "secret.txt", 20)
	reg := New(Config{})
	e, _ := reg.Register(p)
	h := reg.Handler()
	if rec := get(t, h, "GET", e.URL, nil); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	// 换成指向别处的符号链接
	os.Remove(p)
	if err := os.Symlink(secret, p); err != nil {
		t.Fatal(err)
	}
	if rec := get(t, h, "GET", e.URL, nil); rec.Code != 404 {
		t.Fatalf("符号链接被换后应 404，got %d", rec.Code)
	}
	// 换成目录
	os.Remove(p)
	os.Mkdir(p, 0o755)
	if rec := get(t, h, "GET", e.URL, nil); rec.Code != 404 {
		t.Fatalf("目录: %d", rec.Code)
	}
	os.Remove(p)
	if rec := get(t, h, "GET", e.URL, nil); rec.Code != 404 {
		t.Fatalf("删除: %d", rec.Code)
	}
	// 登记时符号链接：解析到真实文件，可用；真实文件是目录则拒绝
	link := filepath.Join(dir, "link.mp4")
	os.Symlink(secret, link)
	le, err := reg.Register(link)
	if err != nil {
		t.Fatal(err)
	}
	if rec := get(t, h, "GET", le.URL, nil); rec.Code != 200 {
		t.Fatalf("link: %d", rec.Code)
	}
	// 链接改指向别的文件 → 404
	os.Remove(link)
	os.Symlink(p2(t, dir), link)
	if rec := get(t, h, "GET", le.URL, nil); rec.Code != 404 {
		t.Fatalf("链接改指向: %d", rec.Code)
	}
	dlink := filepath.Join(dir, "dlink")
	os.Symlink(dir, dlink)
	if _, err := reg.Register(dlink); err == nil {
		t.Fatal("指向目录的链接应拒绝")
	}
}

func p2(t *testing.T, dir string) string { return writeFile(t, dir, "other.txt", 5) }

func TestReplacedFileSameName(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.mp4", 10)
	reg := New(Config{})
	e, _ := reg.Register(p)
	// 删掉再建同名新文件（不同 inode）→ 旧 token 应 404（Windows 上 SameFile 语义可能不同，只在 unix 断言）
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	os.Rename(p, filepath.Join(dir, "old.mp4")) // 保留旧文件，避免 inode 被复用
	writeFile(t, dir, "a.mp4", 12)
	if rec := get(t, reg.Handler(), "GET", e.URL, nil); rec.Code != 404 {
		t.Fatalf("被替换后应 404: %d", rec.Code)
	}
	e2, err := reg.Register(p)
	if err != nil || e2.Token == e.Token {
		t.Fatalf("重新登记应换新 token: %v %v", err, e2)
	}
	if rec := get(t, reg.Handler(), "GET", e2.URL, nil); rec.Code != 200 || rec.Body.Len() != 12 {
		t.Fatal(rec.Code)
	}
}

func TestEvictionAndRevoke(t *testing.T) {
	dir := t.TempDir()
	reg := New(Config{MaxEntries: 3})
	var es []Entry
	for i := 0; i < 5; i++ {
		p := writeFile(t, dir, string(rune('a'+i))+".mp4", 5)
		e, err := reg.Register(p)
		if err != nil {
			t.Fatal(err)
		}
		es = append(es, e)
	}
	if reg.Len() != 3 {
		t.Fatalf("len=%d", reg.Len())
	}
	h := reg.Handler()
	for i, e := range es {
		want := 200
		if i < 2 {
			want = 404
		}
		if rec := get(t, h, "GET", e.URL, nil); rec.Code != want {
			t.Errorf("%d: %d want %d", i, rec.Code, want)
		}
	}
	reg.Revoke(es[4].Token)
	reg.Revoke("nope")
	if rec := get(t, h, "GET", es[4].URL, nil); rec.Code != 404 || reg.Len() != 2 {
		t.Fatal("revoke")
	}
}

func TestTokensUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		tok, err := newToken()
		if err != nil || !validToken(tok) || seen[tok] {
			t.Fatal("token")
		}
		seen[tok] = true
	}
}

func TestContentTypes(t *testing.T) {
	for ext, want := range map[string]string{
		"a.MP4": "video/mp4", "a.webm": "video/webm", "a.mov": "video/quicktime", "a.mkv": "video/x-matroska",
		"a.mp3": "audio/mpeg", "a.wav": "audio/wav", "a.aac": "audio/aac", "a.png": "image/png",
		"a.jpg": "image/jpeg", "a.pdf": "application/pdf", "a.xyz": "application/octet-stream",
	} {
		if got := ContentType(ext); got != want {
			t.Errorf("%s: %s want %s", ext, got, want)
		}
	}
}

func TestHeadProbe(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "big.mp4", 100)
	reg := New(Config{MaxRangeBytes: 10, MaxWholeBytes: 50})
	e, _ := reg.Register(p)
	h := reg.Handler()
	// 大文件：GET 无 Range 413，但 HEAD 200 且无 body，带 Accept-Ranges / Content-Length / nosniff
	if rec := get(t, h, "GET", e.URL, nil); rec.Code != 413 {
		t.Fatal(rec.Code)
	}
	rec := get(t, h, "HEAD", e.URL, nil)
	if rec.Code != 200 || rec.Body.Len() != 0 || rec.Header().Get("Content-Length") != "100" ||
		rec.Header().Get("Accept-Ranges") != "bytes" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("HEAD: %d %v", rec.Code, rec.Header())
	}
	// HEAD + Range：206 头部，长度截断，无 body
	rec = get(t, h, "HEAD", e.URL, map[string]string{"Range": "bytes=0-99"})
	if rec.Code != 206 || rec.Body.Len() != 0 || rec.Header().Get("Content-Length") != "10" || rec.Header().Get("Content-Range") != "bytes 0-9/100" {
		t.Fatalf("HEAD range: %d %v", rec.Code, rec.Header())
	}
	if rec := get(t, h, "HEAD", e.URL, map[string]string{"Range": "bytes=200-"}); rec.Code != 416 {
		t.Fatalf("HEAD 越界: %d", rec.Code)
	}
	// token 失效 / 不存在 / 文件被删：HEAD 404
	if rec := get(t, h, "HEAD", "/local/"+strings.Repeat("0", 32), nil); rec.Code != 404 {
		t.Fatalf("未知 token: %d", rec.Code)
	}
	reg.Revoke(e.Token)
	if rec := get(t, h, "HEAD", e.URL, nil); rec.Code != 404 {
		t.Fatalf("Revoke 后: %d", rec.Code)
	}
	e2, _ := reg.Register(p)
	os.Remove(p)
	if rec := get(t, h, "HEAD", e2.URL, nil); rec.Code != 404 {
		t.Fatalf("文件删除后: %d", rec.Code)
	}
}

func TestLRUEviction(t *testing.T) {
	dir := t.TempDir()
	reg := New(Config{MaxEntries: 2})
	a, _ := reg.Register(writeFile(t, dir, "a.mp4", 5))
	b, _ := reg.Register(writeFile(t, dir, "b.mp4", 5))
	h := reg.Handler()
	// 请求命中 a → a 变成最近使用；再登记 c 应淘汰 b
	if rec := get(t, h, "GET", a.URL, nil); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	c, _ := reg.Register(writeFile(t, dir, "c.mp4", 5))
	if get(t, h, "GET", b.URL, nil).Code != 404 || get(t, h, "GET", a.URL, nil).Code != 200 || get(t, h, "GET", c.URL, nil).Code != 200 {
		t.Fatal("LRU：b 应被淘汰")
	}
}

func TestDefaultCapacity512(t *testing.T) {
	if r := New(Config{}); r.cfg.MaxEntries != 512 {
		t.Fatal(r.cfg.MaxEntries)
	}
}

// edit 与 doc 分表：各自 512 项，互不挤占；MultiHandler 两张表都能查到。
func TestSeparateTablesAndMultiHandler(t *testing.T) {
	dir := t.TempDir()
	edit, doc := New(Config{MaxEntries: 2}), New(Config{MaxEntries: 2})
	e1, _ := edit.Register(writeFile(t, dir, "a.mp4", 5))
	d1, _ := doc.Register(writeFile(t, dir, "d1.pdf", 6))
	for i := 0; i < 5; i++ { // doc 表被挤满、淘汰，不影响 edit 表
		doc.Register(writeFile(t, dir, "x"+string(rune('a'+i))+".pdf", 5))
	}
	h := MultiHandler(edit, doc)
	if rec := get(t, h, "GET", e1.URL, nil); rec.Code != 200 || rec.Body.Len() != 5 {
		t.Fatalf("edit 表的项不应被 doc 挤掉: %d", rec.Code)
	}
	if rec := get(t, h, "GET", d1.URL, nil); rec.Code != 404 {
		t.Fatalf("doc 表里最旧的应被淘汰: %d", rec.Code)
	}
	d2, _ := doc.Register(writeFile(t, dir, "last.pdf", 7))
	if rec := get(t, h, "GET", d2.URL, nil); rec.Code != 200 || rec.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("%d", rec.Code)
	}
	// 单表 Handler 只认自己的表
	if rec := get(t, edit.Handler(), "GET", d2.URL, nil); rec.Code != 404 {
		t.Fatalf("%d", rec.Code)
	}
}

// 不设 Last-Modified / ETag，也不处理条件请求（WebView2 对 304 有已知问题）。
func TestNoConditionalHeaders(t *testing.T) {
	dir := t.TempDir()
	reg := New(Config{})
	e, _ := reg.Register(writeFile(t, dir, "a.mp4", 50))
	rec := get(t, reg.Handler(), "GET", e.URL, map[string]string{
		"If-None-Match": `"x"`, "If-Modified-Since": "Wed, 21 Oct 2099 07:28:00 GMT", "If-Range": `"x"`, "Range": "bytes=0-9"})
	if rec.Code != 206 || rec.Header().Get("Last-Modified") != "" || rec.Header().Get("ETag") != "" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
}

// 契约 6.13 原型实测的 12 种请求（阈值等比缩小：4 MiB → 40 字节，32 MiB → 320 字节；文件 400 字节）。
func TestContractRangeMatrix(t *testing.T) {
	dir := t.TempDir()
	reg := New(Config{MaxRangeBytes: 40, MaxWholeBytes: 320})
	e, _ := reg.Register(writeFile(t, dir, "big.mp4", 400))
	h := reg.Handler()
	type c struct {
		method, rng string
		code, n     int
		cr          string
	}
	for _, x := range []c{
		{"GET", "bytes=0-", 206, 40, "bytes 0-39/400"},
		{"GET", "bytes=0-9", 206, 10, "bytes 0-9/400"},
		{"GET", "bytes=-10", 206, 10, "bytes 390-399/400"},
		{"GET", "bytes=-100000", 206, 40, "bytes 0-39/400"},
		{"GET", "bytes=99999999-", 416, 0, "bytes */400"},
		{"GET", "bytes=0-1,5-9", 416, 0, "bytes */400"},
		{"GET", "bytes=abc", 416, 0, "bytes */400"},
		{"POST", "", 405, 0, ""},
		{"HEAD", "bytes=0-9", 206, 0, "bytes 0-9/400"},
		{"GET", "", 413, 0, ""},
		{"HEAD", "", 200, 0, ""},
		{"GET", "bytes=400-", 416, 0, "bytes */400"},
	} {
		hdr := map[string]string{}
		if x.rng != "" {
			hdr["Range"] = x.rng
		}
		rec := get(t, h, x.method, e.URL, hdr)
		if rec.Code != x.code || (x.code == 206 && x.method == "GET" && rec.Body.Len() != x.n) || rec.Header().Get("Content-Range") != x.cr {
			t.Errorf("%s %q: %d len=%d cr=%q", x.method, x.rng, rec.Code, rec.Body.Len(), rec.Header().Get("Content-Range"))
		}
		if x.method == "POST" && rec.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("405 缺 Allow")
		}
		if x.method == "HEAD" && rec.Body.Len() != 0 {
			t.Errorf("HEAD 有正文")
		}
	}
}
