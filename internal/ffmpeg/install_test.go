package ffmpeg

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ulikunitz/xz"
)

// ---- 测试辅助 ----

func sha(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func makeZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
	}
	zw.Close()
	return buf.Bytes()
}

func makeTarXz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	xw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(xw)
	for name, data := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg})
		tw.Write(data)
	}
	tw.Close()
	xw.Close()
	return buf.Bytes()
}

// 大一点的随机内容，便于中途断开。
func payload(n int, seed byte) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(int64(seed))).Read(b) // 伪随机，压缩后体积不变
	return b
}

// fakeServer 服务一个固定内容，支持 Range（http.ServeContent）。
type fakeServer struct {
	*httptest.Server
	data       []byte
	mu         sync.Mutex
	rangeHdrs  []string
	requests   int32
	noRange    bool  // 忽略 Range，总是返回 200 全量
	cutAfter   int64 // >0：本次响应写到这么多字节后强行断开连接（只生效一次）
	cutDone    bool
	statusCode int // 非 0：直接返回该状态码
}

func newFakeServer(t *testing.T, data []byte) *fakeServer {
	fs := &fakeServer{data: data}
	fs.Server = httptest.NewServer(http.HandlerFunc(fs.handle))
	t.Cleanup(fs.Close)
	return fs
}

func (f *fakeServer) handle(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&f.requests, 1)
	f.mu.Lock()
	f.rangeHdrs = append(f.rangeHdrs, r.Header.Get("Range"))
	cut, code, noRange := f.cutAfter, f.statusCode, f.noRange
	if cut > 0 && !f.cutDone {
		f.cutDone = true
	} else {
		cut = 0
	}
	f.mu.Unlock()
	if code != 0 {
		http.Error(w, "err", code)
		return
	}
	if noRange {
		r.Header.Del("Range")
	}
	if cut > 0 {
		// 先声明完整长度，只写一部分后劫持连接关闭，模拟中途断网。
		start := int64(0)
		if rg := r.Header.Get("Range"); rg != "" {
			fmt.Sscanf(rg, "bytes=%d-", &start)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(f.data)-1, len(f.data)))
			w.Header().Set("Content-Length", fmt.Sprint(int64(len(f.data))-start))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", fmt.Sprint(len(f.data)))
			w.WriteHeader(http.StatusOK)
		}
		w.Write(f.data[start : start+cut])
		w.(http.Flusher).Flush()
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
		return
	}
	http.ServeContent(w, r, "a.zip", time.Time{}, bytes.NewReader(f.data))
}

func (f *fakeServer) ranges() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.rangeHdrs...)
}

func testDownloader() *downloader {
	return &downloader{client: http.DefaultClient, idleTimeout: 5 * time.Second, retries: 0, retryWait: time.Millisecond}
}

// ---- 下载 ----

func TestFetchFresh(t *testing.T) {
	data := payload(300_000, 1)
	srv := newFakeServer(t, data)
	part := filepath.Join(t.TempDir(), "a.part")
	var lastDone int64
	err := testDownloader().fetch(context.Background(), []string{srv.URL}, part, sha(data), int64(len(data)), func(done, total int64) { lastDone = done })
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(part)
	if !bytes.Equal(got, data) || lastDone != int64(len(data)) {
		t.Fatalf("内容或进度不对: %d %d", len(got), lastDone)
	}
	if r := srv.ranges(); len(r) != 1 || r[0] != "" {
		t.Fatalf("全新下载不应带 Range: %v", r)
	}
}

func TestFetchResumesFromPartial(t *testing.T) {
	data := payload(300_000, 2)
	srv := newFakeServer(t, data)
	part := filepath.Join(t.TempDir(), "a.part")
	os.WriteFile(part, data[:120_000], 0o644)
	var first int64 = -1
	err := testDownloader().fetch(context.Background(), []string{srv.URL}, part, sha(data), int64(len(data)), func(done, total int64) {
		if first < 0 {
			first = done
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(part)
	if !bytes.Equal(got, data) {
		t.Fatal("续传后内容不一致")
	}
	if r := srv.ranges(); len(r) != 1 || r[0] != "bytes=120000-" {
		t.Fatalf("应带 Range: %v", r)
	}
	if first != 120_000 {
		t.Fatalf("进度应从已有字节数开始: %d", first)
	}
}

func TestFetchRetriesAfterMidStreamCutWithRange(t *testing.T) {
	data := payload(500_000, 3)
	srv := newFakeServer(t, data)
	srv.cutAfter = 200_000
	d := testDownloader()
	d.retries = 2
	part := filepath.Join(t.TempDir(), "a.part")
	if err := d.fetch(context.Background(), []string{srv.URL}, part, sha(data), int64(len(data)), func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(part)
	if !bytes.Equal(got, data) {
		t.Fatal("断线重试后内容不一致")
	}
	r := srv.ranges()
	if len(r) != 2 || r[0] != "" || !strings.HasPrefix(r[1], "bytes=") {
		t.Fatalf("第二次请求应为续传: %v", r)
	}
}

func TestFetchFailureKeepsPartial(t *testing.T) {
	data := payload(500_000, 4)
	srv := newFakeServer(t, data)
	srv.cutAfter = 150_000
	part := filepath.Join(t.TempDir(), "a.part")
	err := testDownloader().fetch(context.Background(), []string{srv.URL}, part, sha(data), int64(len(data)), func(int64, int64) {})
	if err == nil {
		t.Fatal("断线且不重试应失败")
	}
	fi, serr := os.Stat(part)
	if serr != nil || fi.Size() == 0 || fi.Size() > 150_000 {
		t.Fatalf("失败后应保留部分文件: %v %v", fi, serr)
	}
	// 之后再来一次，能接着下完
	if err := testDownloader().fetch(context.Background(), []string{srv.URL}, part, sha(data), int64(len(data)), func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(part)
	if !bytes.Equal(got, data) {
		t.Fatal("续传结果不一致")
	}
}

func TestFetchServerIgnoresRange(t *testing.T) {
	data := payload(200_000, 5)
	srv := newFakeServer(t, data)
	srv.noRange = true
	part := filepath.Join(t.TempDir(), "a.part")
	os.WriteFile(part, data[:50_000], 0o644)
	if err := testDownloader().fetch(context.Background(), []string{srv.URL}, part, sha(data), int64(len(data)), func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(part)
	if !bytes.Equal(got, data) {
		t.Fatal("服务器不支持 Range 时应从头覆盖写")
	}
}

func TestFetchChecksumMismatch(t *testing.T) {
	data := payload(100_000, 6)
	srv := newFakeServer(t, data)
	part := filepath.Join(t.TempDir(), "a.part")
	err := testDownloader().fetch(context.Background(), []string{srv.URL}, part, sha([]byte("other")), int64(len(data)), func(int64, int64) {})
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("期望 ErrChecksum: %v", err)
	}
	if _, serr := os.Stat(part); !os.IsNotExist(serr) {
		t.Fatal("校验失败应删除坏文件，避免下次续传继续失败")
	}
}

func TestFetchAlreadyCompleteSkipsNetwork(t *testing.T) {
	data := payload(100_000, 7)
	srv := newFakeServer(t, data)
	part := filepath.Join(t.TempDir(), "a.part")
	os.WriteFile(part, data, 0o644)
	if err := testDownloader().fetch(context.Background(), []string{srv.URL}, part, sha(data), int64(len(data)), func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&srv.requests) != 0 {
		t.Fatal("已下载完整时不应再发请求")
	}
}

func TestFetchFallsBackToNextURL(t *testing.T) {
	data := payload(100_000, 8)
	bad := newFakeServer(t, data)
	bad.statusCode = 404
	good := newFakeServer(t, data)
	part := filepath.Join(t.TempDir(), "a.part")
	if err := testDownloader().fetch(context.Background(), []string{bad.URL, good.URL}, part, sha(data), int64(len(data)), func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	// 镜像给了坏文件（SHA 不符）也应换下一个
	corrupt := newFakeServer(t, payload(100_000, 9))
	os.Remove(part)
	if err := testDownloader().fetch(context.Background(), []string{corrupt.URL, good.URL}, part, sha(data), int64(len(data)), func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
}

func TestFetchCancelKeepsPartial(t *testing.T) {
	data := payload(400_000, 10)
	srv := newFakeServer(t, data)
	ctx, cancel := context.WithCancel(context.Background())
	part := filepath.Join(t.TempDir(), "a.part")
	err := testDownloader().fetch(ctx, []string{srv.URL}, part, sha(data), int64(len(data)), func(done, total int64) {
		if done > 100_000 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled: %v", err)
	}
	if fi, serr := os.Stat(part); serr != nil || fi.Size() < 100_000 {
		t.Fatalf("取消后应保留 .part: %v %v", fi, serr)
	}
}

func TestParseContentRange(t *testing.T) {
	s, tot, ok := parseContentRange("bytes 100-999/1000")
	if !ok || s != 100 || tot != 1000 {
		t.Fatal(s, tot, ok)
	}
	if _, tot, ok := parseContentRange("bytes 5-9/*"); !ok || tot != 0 {
		t.Fatal("未知总长")
	}
	for _, bad := range []string{"", "items 1-2/3", "bytes x-2/3", "bytes 1-2"} {
		if _, _, ok := parseContentRange(bad); ok {
			t.Fatalf("%q 应解析失败", bad)
		}
	}
}

// ---- 解压 ----

func TestExtractZipOnlyRequested(t *testing.T) {
	z := makeZip(t, map[string][]byte{
		"top/bin/ffmpeg.exe":  []byte("FF"),
		"top/bin/ffprobe.exe": []byte("FP"),
		"top/bin/ffplay.exe":  []byte("PLAY"),
		"top/doc/x.html":      []byte("doc"),
		"../evil":             []byte("evil"),
	})
	src := filepath.Join(t.TempDir(), "a.zip")
	os.WriteFile(src, z, 0o644)
	dst := t.TempDir()
	err := extractItems(src, "zip", []ExtractItem{{"top/bin/ffmpeg.exe", "ffmpeg.exe"}, {"top/bin/ffprobe.exe", "ffprobe.exe"}}, dst)
	if err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(dst)
	if len(ents) != 2 {
		t.Fatalf("只应提取两个文件: %v", ents)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "ffprobe.exe")); string(b) != "FP" {
		t.Fatal("内容不对")
	}
	// 缺失条目
	if err := extractItems(src, "zip", []ExtractItem{{"top/bin/nope", "nope"}}, dst); err == nil {
		t.Fatal("缺少条目应报错")
	}
}

func TestExtractTarXz(t *testing.T) {
	x := makeTarXz(t, map[string][]byte{"pkg/ffmpeg": []byte("FF"), "pkg/ffprobe": []byte("FP"), "pkg/readme": []byte("r")})
	src := filepath.Join(t.TempDir(), "a.tar.xz")
	os.WriteFile(src, x, 0o644)
	dst := t.TempDir()
	if err := extractItems(src, "tar.xz", []ExtractItem{{"pkg/ffmpeg", "ffmpeg"}, {"./pkg/ffprobe", "ffprobe"}}, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "ffmpeg")); string(b) != "FF" {
		t.Fatal("ffmpeg 内容不对")
	}
	if err := extractItems(src, "tar.xz", []ExtractItem{{"pkg/missing", "m"}}, t.TempDir()); err == nil {
		t.Fatal("缺少条目应报错")
	}
	if err := extractItems(src, "rar", nil, dst); err == nil {
		t.Fatal("不支持的类型应报错")
	}
}

// ---- 安装 ----

// contentRunner：读取被执行文件的内容来决定行为。"GOOD" 开头 = 合格 6.1；"OLD" = 4.4；其余无法运行。
func contentRunner(_ context.Context, exe string, args ...string) (string, error) {
	b, err := os.ReadFile(exe)
	if err != nil {
		return "", err
	}
	name := strings.TrimSuffix(filepath.Base(exe), ".exe")
	for _, a := range args {
		if a == "-encoders" {
			return " V....D libx264  x\n A....D aac  x\n", nil
		}
	}
	switch {
	case bytes.HasPrefix(b, []byte("GOOD")):
		return name + " version 6.1 x\n", nil
	case bytes.HasPrefix(b, []byte("OLD")):
		return name + " version 4.4 x\n", nil
	}
	return "", errors.New("exec format error")
}

type installFixture struct {
	in   *Installer
	srv  *fakeServer
	root string
}

func newInstallFixture(t *testing.T, ffmpegBody, ffprobeBody string) *installFixture {
	t.Helper()
	root := t.TempDir()
	z := makeZip(t, map[string][]byte{"ffmpeg": []byte(ffmpegBody), "ffprobe": []byte(ffprobeBody)})
	srv := newFakeServer(t, z)
	m := &Manifest{Platforms: map[string]Platform{"linux-amd64": {
		Available: true, Version: "6.1",
		Archives: []Archive{{URL: srv.URL + "/ff.zip", SHA256: sha(z), Size: int64(len(z)), Type: "zip",
			Extract: []ExtractItem{{"ffmpeg", "ffmpeg"}, {"ffprobe", "ffprobe"}}}},
	}}}
	loc := &Locator{Run: contentRunner, LookPath: func(string) (string, error) { return "", errors.New("no") }, GOOS: "linux"}
	in := NewInstaller(m, loc, filepath.Join(root, "bin"), filepath.Join(root, "tmp"))
	in.Platform, in.GOOS = "linux-amd64", "linux"
	in.RetryWait, in.ProgressInterval = time.Millisecond, -1
	return &installFixture{in: in, srv: srv, root: root}
}

func TestInstallSuccess(t *testing.T) {
	f := newInstallFixture(t, "GOOD-ffmpeg-bytes", "GOOD-ffprobe-bytes")
	var mu sync.Mutex
	var phases []string
	var last Progress
	info, err := f.in.Install(context.Background(), "", func(p Progress) {
		mu.Lock()
		defer mu.Unlock()
		if len(phases) == 0 || phases[len(phases)-1] != p.Phase {
			phases = append(phases, p.Phase)
		}
		if p.Fraction < last.Fraction {
			t.Errorf("进度不应回退: %v -> %v", last.Fraction, p.Fraction)
		}
		last = p
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.Source != SourceBundled || info.FFmpeg != filepath.Join(f.in.BinDir, "ffmpeg") || info.Version != "6.1" {
		t.Fatalf("%+v", info)
	}
	for _, n := range []string{"ffmpeg", "ffprobe"} {
		fi, err := os.Stat(filepath.Join(f.in.BinDir, n))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm()&0o111 == 0 {
			t.Fatalf("%s 应有可执行权限: %v", n, fi.Mode())
		}
	}
	if strings.Join(phases, ",") != "download,extract,validate" || last.Fraction != 1 {
		t.Fatalf("阶段: %v last=%+v", phases, last)
	}
	// 成功后清理 .part 与暂存目录
	if len(f.in.PartFiles()) != 0 {
		t.Fatal("成功后应清理 .part")
	}
	ents, _ := os.ReadDir(f.in.TempDir)
	if len(ents) != 0 {
		t.Fatalf("暂存目录应被清理: %v", ents)
	}
	// 安装结果能被 Locator 作为 bundled 找到
	loc := f.in.Locator
	loc.BinDir = f.in.BinDir
	res, _ := loc.Locate(context.Background(), "")
	if res.State != StateReady || res.Info.Source != SourceBundled {
		t.Fatalf("%+v", res)
	}
}

func TestInstallValidationFailureLeavesBinUntouched(t *testing.T) {
	f := newInstallFixture(t, "BAD-not-executable", "GOOD-ffprobe")
	// bin/ 里已有旧的可用版本
	os.MkdirAll(f.in.BinDir, 0o755)
	os.WriteFile(filepath.Join(f.in.BinDir, "ffmpeg"), []byte("GOOD-old"), 0o755)
	_, err := f.in.Install(context.Background(), "", nil)
	if err == nil || !strings.Contains(err.Error(), "校验") {
		t.Fatalf("期望校验失败: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(f.in.BinDir, "ffmpeg"))
	if string(b) != "GOOD-old" {
		t.Fatalf("校验失败不应动 bin/: %q", b)
	}
	if _, serr := os.Stat(filepath.Join(f.in.BinDir, "ffprobe")); !os.IsNotExist(serr) {
		t.Fatal("校验失败不应写入 ffprobe")
	}
	// 压缩包已下载完整且 SHA 正确，保留 .part 供重试
	if len(f.in.PartFiles()) != 1 {
		t.Fatalf("校验失败应保留 .part: %v", f.in.PartFiles())
	}
}

func TestInstallOutdatedRejected(t *testing.T) {
	f := newInstallFixture(t, "OLD-ffmpeg", "OLD-ffprobe")
	if _, err := f.in.Install(context.Background(), "", nil); err == nil {
		t.Fatal("下载到低版本也应拒绝")
	}
}

func TestInstallReplacesExistingAtomically(t *testing.T) {
	f := newInstallFixture(t, "GOOD-new-ffmpeg", "GOOD-new-ffprobe")
	os.MkdirAll(f.in.BinDir, 0o755)
	os.WriteFile(filepath.Join(f.in.BinDir, "ffmpeg"), []byte("GOOD-old-ffmpeg"), 0o755)
	os.WriteFile(filepath.Join(f.in.BinDir, "ffprobe"), []byte("GOOD-old-ffprobe"), 0o755)
	if _, err := f.in.Install(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(f.in.BinDir, "ffmpeg"))
	p, _ := os.ReadFile(filepath.Join(f.in.BinDir, "ffprobe"))
	if string(b) != "GOOD-new-ffmpeg" || string(p) != "GOOD-new-ffprobe" {
		t.Fatalf("应整体替换: %q %q", b, p)
	}
	ents, _ := os.ReadDir(f.in.BinDir)
	if len(ents) != 2 {
		t.Fatalf("bin/ 里不应残留备份或临时文件: %v", ents)
	}
}

func TestCommitRollsBackOnFailure(t *testing.T) {
	f := newInstallFixture(t, "x", "y")
	os.MkdirAll(f.in.BinDir, 0o755)
	os.WriteFile(filepath.Join(f.in.BinDir, "ffmpeg"), []byte("OLD-ffmpeg"), 0o755)
	stage := t.TempDir()
	os.WriteFile(filepath.Join(stage, "ffmpeg"), []byte("NEW-ffmpeg"), 0o755)
	// ffprobe 不存在于暂存目录 → 第二步失败 → 第一步应回滚
	err := f.in.commit(stage, []string{"ffmpeg", "ffprobe"})
	if err == nil {
		t.Fatal("应失败")
	}
	b, _ := os.ReadFile(filepath.Join(f.in.BinDir, "ffmpeg"))
	if string(b) != "OLD-ffmpeg" {
		t.Fatalf("应回滚到旧文件: %q", b)
	}
	if _, serr := os.Stat(filepath.Join(f.in.BinDir, "ffprobe")); !os.IsNotExist(serr) {
		t.Fatal("不应留下 ffprobe")
	}
}

func TestInstallChecksumMismatch(t *testing.T) {
	f := newInstallFixture(t, "GOOD-a", "GOOD-b")
	p := f.in.Manifest.Platforms["linux-amd64"]
	p.Archives[0].SHA256 = strings.Repeat("0", 64)
	f.in.Manifest.Platforms["linux-amd64"] = p
	_, err := f.in.Install(context.Background(), "", nil)
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("期望 ErrChecksum: %v", err)
	}
	if _, serr := os.Stat(filepath.Join(f.in.BinDir, "ffmpeg")); !os.IsNotExist(serr) {
		t.Fatal("SHA 不符不应安装")
	}
}

func TestInstallFailureKeepsPartialAndResumes(t *testing.T) {
	// 用较大的载荷，确保断开点落在压缩包中间。
	root := t.TempDir()
	big := payload(600_000, 11)
	z := makeZip(t, map[string][]byte{"ffmpeg": append([]byte("GOOD"), big...), "ffprobe": []byte("GOOD-probe")})
	srv := newFakeServer(t, z)
	srv.cutAfter = 5_000
	m := &Manifest{Platforms: map[string]Platform{"linux-amd64": {Available: true, Version: "6.1",
		Archives: []Archive{{URL: srv.URL, SHA256: sha(z), Size: int64(len(z)), Type: "zip",
			Extract: []ExtractItem{{"ffmpeg", "ffmpeg"}, {"ffprobe", "ffprobe"}}}}}}}
	loc := &Locator{Run: contentRunner, LookPath: func(string) (string, error) { return "", errors.New("no") }, GOOS: "linux"}
	in := NewInstaller(m, loc, filepath.Join(root, "bin"), filepath.Join(root, "tmp"))
	in.Platform, in.GOOS, in.Retries, in.RetryWait = "linux-amd64", "linux", -1, time.Millisecond // 不自动重试

	if _, err := in.Install(context.Background(), "", nil); err == nil {
		t.Fatal("断线应失败")
	}
	parts := in.PartFiles()
	if len(parts) != 1 {
		t.Fatalf("失败后应保留 .part: %v", parts)
	}
	if fi, _ := os.Stat(parts[0]); fi.Size() == 0 || fi.Size() >= int64(len(z)) {
		t.Fatalf(".part 大小应为部分数据: %d/%d", fi.Size(), len(z))
	}
	// 再次安装（服务器恢复正常）应从断点续传并成功。
	if _, err := in.Install(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	r := srv.ranges()
	if len(r) != 2 || !strings.HasPrefix(r[1], "bytes=") {
		t.Fatalf("第二次安装应带 Range 续传: %v", r)
	}
	if len(in.PartFiles()) != 0 {
		t.Fatal("成功后应清理 .part")
	}
}

func TestInstallCancel(t *testing.T) {
	f := newInstallFixture(t, "GOOD-"+string(payload(300_000, 1)), "GOOD-p")
	ctx, cancel := context.WithCancel(context.Background())
	_, err := f.in.Install(ctx, "", func(p Progress) {
		if p.Phase == PhaseDownload && p.Done > 50_000 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled: %v", err)
	}
	if len(f.in.PartFiles()) != 1 {
		t.Fatal("取消应保留 .part")
	}
	if _, serr := os.Stat(filepath.Join(f.in.BinDir, "ffmpeg")); !os.IsNotExist(serr) {
		t.Fatal("取消不应安装")
	}
}

func TestInstallBusy(t *testing.T) {
	f := newInstallFixture(t, "GOOD-a", "GOOD-b")
	gate := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	done := make(chan error, 1)
	go func() {
		_, err := f.in.Install(context.Background(), "", func(p Progress) {
			once.Do(func() { close(started) })
			<-gate
		})
		done <- err
	}()
	<-started
	if _, err := f.in.Install(context.Background(), "", nil); !errors.Is(err, ErrInstallBusy) {
		t.Fatalf("期望 ErrInstallBusy: %v", err)
	}
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestInstallPreflight(t *testing.T) {
	f := newInstallFixture(t, "GOOD-a", "GOOD-b")
	if err := f.in.Preflight(""); err != nil {
		t.Fatal(err)
	}
	if err := f.in.Preflight("eu"); err == nil || IsUnavailable(err) {
		t.Fatalf("非法镜像应是参数错误: %v", err)
	}
	f.in.Platform = "plan9-mips"
	if err := f.in.Preflight(""); !IsUnavailable(err) {
		t.Fatalf("未知平台应是 unavailable: %v", err)
	}
	f.in.Platform = "linux-amd64"
	if !f.in.MirrorFallsBack("cn") || f.in.MirrorFallsBack("") {
		t.Fatal("没有 cn 条目时 cn 应退回默认源")
	}
}

func TestInstallMirrorFallbackOrder(t *testing.T) {
	f := newInstallFixture(t, "GOOD-a", "GOOD-b")
	mirror := newFakeServer(t, []byte("garbage"))
	mirror.statusCode = 503
	p := f.in.Manifest.Platforms["linux-amd64"]
	p.Archives[0].Mirrors = map[string][]string{"cn": {mirror.URL + "/x.zip"}}
	f.in.Manifest.Platforms["linux-amd64"] = p
	f.in.Retries = -1
	if _, err := f.in.Install(context.Background(), "cn", nil); err != nil {
		t.Fatalf("镜像失败应退回默认地址: %v", err)
	}
	if atomic.LoadInt32(&mirror.requests) == 0 {
		t.Fatal("应先尝试镜像")
	}
}

// 端到端：真实 shell 脚本 + ExecRunner + 真实 zip / tar.xz，仅 unix。
func TestInstallEndToEndWithScripts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell 脚本假二进制仅在 unix 上运行")
	}
	script := func(name string) []byte {
		return []byte(fmt.Sprintf(`#!/bin/sh
case "$*" in
  *-encoders*) printf ' V....D libx264  x\n A....D aac  x\n' ;;
  *-version*) echo '%s version 7.0-test Copyright' ;;
esac
`, name))
	}
	for _, typ := range []string{"zip", "tar.xz"} {
		t.Run(typ, func(t *testing.T) {
			files := map[string][]byte{"pkg/bin/ffmpeg": script("ffmpeg"), "pkg/bin/ffprobe": script("ffprobe"), "pkg/README": []byte("x")}
			var data []byte
			if typ == "zip" {
				data = makeZip(t, files)
			} else {
				data = makeTarXz(t, files)
			}
			srv := newFakeServer(t, data)
			root := t.TempDir()
			m := &Manifest{Platforms: map[string]Platform{PlatformKey(): {Available: true, Version: "7.0",
				Archives: []Archive{{URL: srv.URL, SHA256: sha(data), Size: int64(len(data)), Type: typ,
					Extract: []ExtractItem{{"pkg/bin/ffmpeg", "ffmpeg"}, {"pkg/bin/ffprobe", "ffprobe"}}}}}}}
			in := NewInstaller(m, NewLocator(""), filepath.Join(root, "bin"), filepath.Join(root, "tmp"))
			info, err := in.Install(context.Background(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			if info.Version != "7.0-test" || info.Major != 7 {
				t.Fatalf("%+v", info)
			}
		})
	}
}
