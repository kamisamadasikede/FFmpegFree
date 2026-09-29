package doc

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
)

func writePDF(t *testing.T, path string, size int) {
	t.Helper()
	b := make([]byte, size)
	copy(b, "%PDF-1.4\n")
	for i := 9; i < size; i++ {
		b[i] = byte(i * 7)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenPDFValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	open := func(p string) error { _, err := e.svc.OpenPDF(ctx, p); return err }

	wantCode(t, open(""), apperr.InvalidArgument)
	wantCode(t, open("rel.pdf"), apperr.InvalidArgument)
	wantCode(t, open(filepath.Join(e.dir, "nope.pdf")), apperr.NotFound)
	d := filepath.Join(e.dir, "d.pdf")
	os.Mkdir(d, 0o755)
	wantCode(t, open(d), apperr.InvalidArgument)
	txt := filepath.Join(e.dir, "a.txt")
	writePDF(t, txt, 100)
	wantCode(t, open(txt), apperr.InvalidArgument) // 扩展名
	fake := filepath.Join(e.dir, "fake.pdf")
	os.WriteFile(fake, []byte("hello, not a pdf"), 0o644)
	ae := wantCode(t, open(fake), apperr.InvalidArgument)
	if ae.Message != "不是 PDF 文件" {
		t.Fatal(ae.Message)
	}
	// %PDF- 在 1024 字节之后不算
	late := filepath.Join(e.dir, "late.pdf")
	os.WriteFile(late, append(bytes.Repeat([]byte{'x'}, 1100), []byte("%PDF-1.4")...), 0o644)
	wantCode(t, open(late), apperr.InvalidArgument)
	// %PDF- 在前 1024 字节内（有 BOM / 垃圾头）可以
	junk := filepath.Join(e.dir, "junk.PDF")
	os.WriteFile(junk, append(bytes.Repeat([]byte{'x'}, 500), []byte("%PDF-1.7\n")...), 0o644)
	if err := open(junk); err != nil {
		t.Fatal(err)
	}
	// 空文件
	empty := filepath.Join(e.dir, "empty.pdf")
	os.WriteFile(empty, nil, 0o644)
	wantCode(t, open(empty), apperr.InvalidArgument)
	// 无读权限（root 下不生效，跳过）
	if os.Geteuid() != 0 {
		np := filepath.Join(e.dir, "noperm.pdf")
		writePDF(t, np, 100)
		os.Chmod(np, 0)
		wantCode(t, open(np), apperr.IOError)
	}
	// 大小上限：稀疏文件 512 MiB+1
	big := filepath.Join(e.dir, "big.pdf")
	writePDF(t, big, 100)
	os.Truncate(big, MaxPDFBytes+1)
	wantCode(t, open(big), apperr.InvalidArgument)
	os.Truncate(big, MaxPDFBytes)
	src, err := e.svc.OpenPDF(ctx, big)
	if err != nil || src.Size != MaxPDFBytes || src.URL == "" {
		t.Fatalf("512 MiB 应可打开且带 url: %+v %v", src, err)
	}
	// FIFO 不是普通文件
	if fifo := filepath.Join(e.dir, "p.pdf"); mkfifo(fifo) == nil {
		wantCode(t, open(fifo), apperr.InvalidArgument)
	}
}

func TestOpenPDFHandleReuseAndURLThreshold(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	small := filepath.Join(e.dir, "小.pdf")
	writePDF(t, small, 5000)
	a, err := e.svc.OpenPDF(ctx, small)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "小.pdf" || a.Size != 5000 || a.Path != small || a.URL != "" || len(a.ID) != 32 {
		t.Fatalf("%+v", a)
	}
	b, _ := e.svc.OpenPDF(ctx, small)
	if b.ID != a.ID {
		t.Fatal("同一路径应复用同一 id")
	}
	if e.reg.Len() != 0 {
		t.Fatal("≤64 MiB 不登记 /local/<token>")
	}
	// 恰好 64 MiB：仍不给 url；64 MiB+1：给
	edge := filepath.Join(e.dir, "edge.pdf")
	writePDF(t, edge, 100)
	os.Truncate(edge, WholeLoadBytes)
	if s, err := e.svc.OpenPDF(ctx, edge); err != nil || s.URL != "" {
		t.Fatalf("64 MiB 整不应有 url: %+v %v", s, err)
	}
	os.Truncate(edge, WholeLoadBytes+1)
	s, err := e.svc.OpenPDF(ctx, edge)
	if err != nil || !strings.HasPrefix(s.URL, "/local/") || len(s.URL) != len("/local/")+32 {
		t.Fatalf("%+v %v", s, err)
	}
	if s.URL == "/local/"+s.ID {
		t.Fatal("token 与 id 是两回事")
	}
	// 记录里
	list, _ := e.svc.ListRecentPDFs(ctx, 0)
	if len(list) != 2 || list[0].Name != "edge.pdf" || !list[0].Exists {
		t.Fatalf("%+v", list)
	}
}

func TestReadPDFChunk(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := filepath.Join(e.dir, "c.pdf")
	const size = 2*1024*1024 + 123
	writePDF(t, p, size)
	want, _ := os.ReadFile(p)
	src, _ := e.svc.OpenPDF(ctx, p)

	// 循环读整份，拼起来逐字节一致
	var got []byte
	var off int64
	for i := 0; ; i++ {
		c, err := e.svc.ReadPDFChunk(src.ID, off, ChunkBytes)
		if err != nil {
			t.Fatal(err)
		}
		if c.Offset != off || c.Length != len(c.Data) {
			t.Fatalf("%+v", c)
		}
		got = append(got, c.Data...)
		off += int64(c.Length)
		if c.EOF {
			break
		}
		if i > 10 {
			t.Fatal("没有 eof")
		}
	}
	if !bytes.Equal(got, want) {
		t.Fatal("内容不一致")
	}
	// 边界
	c, err := e.svc.ReadPDFChunk(src.ID, size-10, 1000)
	if err != nil || c.Length != 10 || !c.EOF {
		t.Fatalf("末尾短块: %+v %v", c, err)
	}
	c, _ = e.svc.ReadPDFChunk(src.ID, size-10, 10)
	if c.Length != 10 || !c.EOF {
		t.Fatalf("恰好读到末尾 eof=true: %+v", c)
	}
	c, _ = e.svc.ReadPDFChunk(src.ID, size-11, 10)
	if c.Length != 10 || c.EOF {
		t.Fatalf("差一个字节 eof=false: %+v", c)
	}
	c, err = e.svc.ReadPDFChunk(src.ID, size, 10)
	if err != nil || c.Length != 0 || !c.EOF || c.Data == nil || c.Size != size {
		t.Fatalf("offset==size: %+v %v", c, err)
	}
	// offset > MaxInt64-length 一律 INVALID_ARGUMENT（防 offset+length 溢出）
	_, err = e.svc.ReadPDFChunk(src.ID, math.MaxInt64, 10)
	wantCode(t, err, apperr.InvalidArgument)
	_, err = e.svc.ReadPDFChunk(src.ID, math.MaxInt64-5, ChunkBytes)
	wantCode(t, err, apperr.InvalidArgument)
	_, err = e.svc.ReadPDFChunk(src.ID, math.MaxInt64-10, 11)
	wantCode(t, err, apperr.InvalidArgument)
	// 恰好 offset == MaxInt64-length 不溢出：越过文件末尾，返回 eof
	if c, err = e.svc.ReadPDFChunk(src.ID, math.MaxInt64-10, 10); err != nil || c.Length != 0 || !c.EOF || c.Size != size {
		t.Fatalf("%+v %v", c, err)
	}
	// 参数越界
	for _, l := range []int{0, -1, ChunkBytes + 1, math.MaxInt32} {
		_, err = e.svc.ReadPDFChunk(src.ID, 0, l)
		wantCode(t, err, apperr.InvalidArgument)
	}
	_, err = e.svc.ReadPDFChunk(src.ID, -1, 10)
	wantCode(t, err, apperr.InvalidArgument)
	_, err = e.svc.ReadPDFChunk(src.ID, math.MinInt64, 10)
	wantCode(t, err, apperr.InvalidArgument)
	// 句柄：只查登记表，不拼路径
	for _, bad := range []string{"", "nope", p, "../" + src.ID, strings.ToUpper(src.ID)} {
		_, err = e.svc.ReadPDFChunk(bad, 0, 10)
		wantCode(t, err, apperr.NotFound)
	}
	// 重启后句柄失效：新 Service 不认旧 id
	e2 := New(Config{Recent: e.st})
	_, err = e2.ReadPDFChunk(src.ID, 0, 10)
	wantCode(t, err, apperr.NotFound)
}

func TestReadPDFChunkRevalidatesEachCall(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := filepath.Join(e.dir, "r.pdf")
	writePDF(t, p, 4096)
	src, _ := e.svc.OpenPDF(ctx, p)
	if _, err := e.svc.ReadPDFChunk(src.ID, 0, 10); err != nil {
		t.Fatal(err)
	}
	// 文件被截短：eof
	os.Truncate(p, 100)
	c, err := e.svc.ReadPDFChunk(src.ID, 50, 1000)
	if err != nil || c.Length != 50 || !c.EOF || c.Size != 100 {
		t.Fatalf("%+v %v", c, err)
	}
	// 超过 512 MiB
	os.Truncate(p, MaxPDFBytes+1)
	_, err = e.svc.ReadPDFChunk(src.ID, 0, 10)
	wantCode(t, err, apperr.InvalidArgument)
	// 硬链接保住原 inode，避免后面新建的文件复用同一个 inode 号让 SameFile 误判为同一个文件
	if err := os.Link(p, filepath.Join(e.dir, "keep-inode.pdf")); err != nil {
		t.Skip("文件系统不支持硬链接:", err)
	}
	// 被换成目录 → NOT_FOUND（与登记时不是同一个文件）
	os.Remove(p)
	os.Mkdir(p, 0o755)
	_, err = e.svc.ReadPDFChunk(src.ID, 0, 10)
	wantCode(t, err, apperr.NotFound)
	// 被删
	os.Remove(p)
	_, err = e.svc.ReadPDFChunk(src.ID, 0, 10)
	wantCode(t, err, apperr.NotFound)
	// 被换成 FIFO（不能阻塞）
	if mkfifo(p) == nil {
		_, err = e.svc.ReadPDFChunk(src.ID, 0, 10)
		wantCode(t, err, apperr.NotFound)
		os.Remove(p)
	}
	// 被换成另一个同名普通文件（SameFile 不同）→ NOT_FOUND
	writePDF(t, p, 4096)
	_, err = e.svc.ReadPDFChunk(src.ID, 0, 10)
	wantCode(t, err, apperr.NotFound)
	// 换成符号链接指向别处 → NOT_FOUND
	os.Remove(p)
	other := filepath.Join(e.dir, "other.pdf")
	writePDF(t, other, 4096)
	if os.Symlink(other, p) == nil {
		_, err = e.svc.ReadPDFChunk(src.ID, 0, 10)
		wantCode(t, err, apperr.NotFound)
		// 重新 OpenPDF 得到同一个 id（同路径复用），并刷新登记的文件信息
		src2, err := e.svc.OpenPDF(ctx, p)
		if err != nil || src2.ID != src.ID {
			t.Fatalf("%+v %v", src2, err)
		}
		if _, err := e.svc.ReadPDFChunk(src.ID, 0, 10); err != nil {
			t.Fatal(err)
		}
	}
}

func TestChunkDataIsBase64InJSON(t *testing.T) {
	b := mustJSON(t, PDFChunk{Offset: 1, Length: 3, EOF: true, Data: []byte("abc")})
	if !strings.Contains(b, `"data":"YWJj"`) {
		t.Fatal(b)
	}
	if b := mustJSON(t, PDFChunk{Data: []byte{}}); !strings.Contains(b, `"data":""`) {
		t.Fatal(b)
	}
}

func TestListAndRemoveRecent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	var srcs []PDFSource
	for i := 0; i < 3; i++ {
		p := filepath.Join(e.dir, fmt.Sprintf("f%d.pdf", i))
		writePDF(t, p, 200+i)
		s, err := e.svc.OpenPDF(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		srcs = append(srcs, s)
	}
	list, err := e.svc.ListRecentPDFs(ctx, 0)
	if err != nil || len(list) != 3 || list[0].Name != "f2.pdf" || list[2].Name != "f0.pdf" {
		t.Fatalf("%+v %v", list, err)
	}
	if l, _ := e.svc.ListRecentPDFs(ctx, 2); len(l) != 2 {
		t.Fatal(len(l))
	}
	// 文件被删：记录保留，exists=false
	os.Remove(filepath.Join(e.dir, "f1.pdf"))
	list, _ = e.svc.ListRecentPDFs(ctx, 0)
	if len(list) != 3 || list[1].Exists || !list[0].Exists {
		t.Fatalf("%+v", list)
	}
	// 再次打开同一个文件：更新 openedAt 置顶，不重复
	if _, err := e.svc.OpenPDF(ctx, filepath.Join(e.dir, "f0.pdf")); err != nil {
		t.Fatal(err)
	}
	list, _ = e.svc.ListRecentPDFs(ctx, 0)
	if len(list) != 3 || list[0].Name != "f0.pdf" {
		t.Fatalf("%+v", list)
	}
	// 删除：不存在的忽略、不删文件
	if err := e.svc.RemoveRecentPDFs(ctx, []string{list[0].ID, "nope"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "f0.pdf")); err != nil {
		t.Fatal("不应删文件")
	}
	list, _ = e.svc.ListRecentPDFs(ctx, 0)
	if len(list) != 2 {
		t.Fatal(len(list))
	}
	// 句柄被撤销
	_, err = e.svc.ReadPDFChunk(srcs[0].ID, 0, 10)
	wantCode(t, err, apperr.NotFound)
	if _, err := e.svc.ReadPDFChunk(srcs[2].ID, 0, 10); err != nil {
		t.Fatal("其他句柄不受影响", err)
	}
	// 上限 500
	ids := make([]string, 501)
	wantCode(t, e.svc.RemoveRecentPDFs(ctx, ids), apperr.InvalidArgument)
	if err := e.svc.RemoveRecentPDFs(ctx, make([]string, 500)); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RemoveRecentPDFs(ctx, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveRecentRevokesLocalToken(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := filepath.Join(e.dir, "large.pdf")
	writePDF(t, p, 1000)
	os.Truncate(p, WholeLoadBytes+4096) // 稀疏大文件
	src, err := e.svc.OpenPDF(ctx, p)
	if err != nil || src.URL == "" {
		t.Fatalf("%+v %v", src, err)
	}
	srv := e.reg.Handler()
	do := func(method, hdr string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, src.URL, nil)
		if hdr != "" {
			req.Header.Set("Range", hdr)
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		return w
	}
	// GET Range 206，HEAD 200 且带长度，无 Range 的整体请求 > 32 MiB → 413
	if w := do("GET", "bytes=0-99"); w.Code != http.StatusPartialContent || w.Body.Len() != 100 || !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-1.4")) {
		t.Fatalf("Range: %d len=%d", w.Code, w.Body.Len())
	}
	if w := do("HEAD", ""); w.Code != http.StatusOK || w.Header().Get("Content-Length") != strconv.FormatInt(WholeLoadBytes+4096, 10) || w.Body.Len() != 0 {
		t.Fatalf("HEAD 应 200 且带完整 Content-Length（不受 32 MiB 限制）: %d %q", w.Code, w.Header().Get("Content-Length"))
	}
	if w := do("GET", ""); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("大文件无 Range 应 413: %d", w.Code)
	}
	// 撤销：记录、句柄、token 都失效，HEAD / GET 都 404
	list, _ := e.svc.ListRecentPDFs(ctx, 0)
	if err := e.svc.RemoveRecentPDFs(ctx, []string{list[0].ID}); err != nil {
		t.Fatal(err)
	}
	if w := do("GET", "bytes=0-99"); w.Code != http.StatusNotFound {
		t.Fatalf("撤销后 GET 应 404: %d", w.Code)
	}
	if w := do("HEAD", ""); w.Code != http.StatusNotFound {
		t.Fatalf("撤销后 HEAD 应 404: %d", w.Code)
	}
	if e.reg.Len() != 0 {
		t.Fatal("登记表应为空")
	}
	_, err = e.svc.ReadPDFChunk(src.ID, 0, 10)
	wantCode(t, err, apperr.NotFound)
	// 重新打开得到新 id、新 token
	src2, err := e.svc.OpenPDF(ctx, p)
	if err != nil || src2.ID == src.ID || src2.URL == src.URL {
		t.Fatalf("%+v %v", src2, err)
	}
}

func TestOpenPDFWithoutLocalAssetsHasEmptyURL(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.Local = nil })
	p := filepath.Join(e.dir, "large.pdf")
	writePDF(t, p, 1000)
	os.Truncate(p, WholeLoadBytes+1)
	src, err := e.svc.OpenPDF(context.Background(), p)
	if err != nil || src.URL != "" {
		t.Fatalf("%+v %v", src, err)
	}
}

func TestOpenPDFWithoutStore(t *testing.T) {
	s := New(Config{})
	p := filepath.Join(t.TempDir(), "a.pdf")
	writePDF(t, p, 300)
	src, err := s.OpenPDF(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if c, err := s.ReadPDFChunk(src.ID, 0, 100); err != nil || c.Length != 100 {
		t.Fatal(c, err)
	}
	if err := s.RemoveRecentPDFs(context.Background(), []string{"x"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleTableEviction(t *testing.T) {
	h := newHandles()
	first, _, _ := h.put("/a/0.pdf", "k0", "/a/0.pdf", nil)
	var ev []*handle
	for i := 1; i <= maxHandles; i++ {
		_, e, _ := h.put(fmt.Sprintf("/a/%d.pdf", i), fmt.Sprintf("k%d", i), "", nil)
		ev = append(ev, e...)
	}
	if len(ev) != 1 || ev[0].id != first.id {
		t.Fatalf("应淘汰最旧的一个: %d", len(ev))
	}
	if _, ok := h.get(first.id); ok {
		t.Fatal("最旧的应已失效")
	}
}

// 转换产出的 PDF 能用 OpenPDF 打开、ReadPDFChunk 读回（端到端）。
func TestConvertThenOpenPDF(t *testing.T) {
	e := newEnv(t)
	in := filepath.Join(e.dir, "x.docx")
	makeDocx(t, in, "端到端 e2e")
	tk := e.convertOK(t, in, "")
	// 转换产出不自动进历史（契约 6.12.4 第 6 点）
	if l, _ := e.svc.ListRecentPDFs(context.Background(), 0); len(l) != 0 {
		t.Fatalf("转换不应自动写入历史: %+v", l)
	}
	src, err := e.svc.OpenPDF(context.Background(), tk.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.svc.ReadPDFChunk(src.ID, 0, 8)
	if err != nil || string(c.Data) != "%PDF-1.3" && !strings.HasPrefix(string(c.Data), "%PDF-") {
		t.Fatalf("%q %v", c.Data, err)
	}
}
