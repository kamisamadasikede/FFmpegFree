package doc

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
)

// forgedZip 写一个伪造的 zip：前面是 dirBytes 字节的“中央目录头”（46 字节、签名正确、文件名长度 0，Go 的读取器会一条条解析），
// 末尾是自己拼的 EOCD（可选带 zip64 locator + 记录）。返回文件大小。
func forgedZip(t *testing.T, path string, dirBytes int, eocd func(dirBytes, entries int) []byte) (entries int) {
	t.Helper()
	hdr := make([]byte, 46)
	copy(hdr, "PK\x01\x02")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries = dirBytes / 46
	dirBytes = entries * 46          // 只写整条目
	chunk := bytes.Repeat(hdr, 4096) // 每块 4096 条
	for written := 0; written < dirBytes; {
		n := len(chunk)
		if dirBytes-written < n {
			n = dirBytes - written
		}
		if _, err := f.Write(chunk[:n]); err != nil {
			t.Fatal(err)
		}
		written += n
	}
	if _, err := f.Write(eocd(dirBytes, entries)); err != nil {
		t.Fatal(err)
	}
	return entries
}

func eocdBytes(entries16 uint16, dirSize, dirOff uint32) []byte {
	b := make([]byte, 22)
	copy(b, "PK\x05\x06")
	binary.LittleEndian.PutUint16(b[8:], entries16)
	binary.LittleEndian.PutUint16(b[10:], entries16)
	binary.LittleEndian.PutUint32(b[12:], dirSize)
	binary.LittleEndian.PutUint32(b[16:], dirOff)
	return b
}

func TestForgedEOCDRejectedByPrecheck(t *testing.T) {
	dir := t.TempDir()
	const big = 98 << 20
	cases := []struct {
		name string
		eocd func(dirBytes, entries int) []byte
	}{
		// 小条目数 + 巨大 directorySize
		{"small-count-huge-size", func(n, e int) []byte { return eocdBytes(1, uint32(n), 0) }},
		// 条目数 zip32 回绕（2 236 962 条 mod 65536），directorySize 巨大
		{"wrapped-count-huge-size", func(n, e int) []byte { return eocdBytes(uint16(e%65536), uint32(n), 0) }},
		// 条目数回绕 + offset=0xFFFFFFFF，没有 zip64 locator：占位符没有 zip64 记录 → 伪造
		{"wrapped-count-offset-ffffffff", func(n, e int) []byte { return eocdBytes(uint16(e%65536), uint32(n), 0xFFFFFFFF) }},
		// directorySize=0xFFFFFFFF 占位符，没有 zip64
		{"size-ffffffff", func(n, e int) []byte { return eocdBytes(uint16(e%65536), 0xFFFFFFFF, 0) }},
		// 32 位目录大小很小、offset=0xFFFFFFFF 占位符，没有 zip64 记录：只能靠“占位符必须有 zip64 记录”这一条拦下
		{"offset-ffffffff-no-zip64", func(n, e int) []byte { return eocdBytes(uint16(e%65536), 1000, 0xFFFFFFFF) }},
		// EOCD 16/32 位字段全是占位符，zip64 记录存在且声明“条目数很小、目录很大”：必须读 zip64 记录取真实目录大小
		{"zip64-record-small-count-huge-dir", func(n, e int) []byte {
			rec := make([]byte, 56)
			copy(rec, "PK\x06\x06")
			binary.LittleEndian.PutUint64(rec[4:], 44)
			binary.LittleEndian.PutUint64(rec[24:], 5)
			binary.LittleEndian.PutUint64(rec[32:], 5)
			binary.LittleEndian.PutUint64(rec[40:], uint64(n))
			loc := make([]byte, 20)
			copy(loc, "PK\x06\x07")
			binary.LittleEndian.PutUint64(loc[8:], uint64(n)) // zip64 记录紧跟在假目录之后
			out := append(append([]byte{}, rec...), loc...)
			return append(out, eocdBytes(0xFFFF, 0xFFFFFFFF, 0xFFFFFFFF)...)
		}},
		// 条目数 0xFFFF + 指向不存在位置的 zip64 locator：读不到记录，且 32 位字段是占位符
		{"locator-points-nowhere", func(n, e int) []byte {
			loc := make([]byte, 20)
			copy(loc, "PK\x06\x07")
			binary.LittleEndian.PutUint64(loc[8:], 1<<40)
			return append(loc, eocdBytes(0xFFFF, 0xFFFFFFFF, 0xFFFFFFFF)...)
		}},
	}
	for _, c := range cases {
		p := filepath.Join(dir, c.name+".docx")
		forgedZip(t, p, big, c.eocd)
		if err := checkZipEntries(p); err == nil {
			t.Errorf("%s: 预检应拒绝", c.name)
		} else if ae, ok := err.(*apperr.AppError); !ok || ae.Code != apperr.InvalidArgument {
			t.Errorf("%s: 期望 INVALID_ARGUMENT，得到 %v", c.name, err)
		}
	}
}

// 伪造文件走完整入口（ConvertToPDF）：INVALID_ARGUMENT，且预检不去解析假目录。内存断言取粗粒度阈值：
// 修复前 98 MiB 的伪造文件会让 Go 读取器把 220 万条假目录项都解析进 File 列表，累计分配约 580 MiB；修复后应只有几 MB。
// 这里用累计分配量（TotalAlloc）的增量，比“当前堆”稳定；阈值 64 MiB 远低于修复前，也远高于正常波动。
func TestForgedEOCDDoesNotAllocate(t *testing.T) {
	e := newEnv(t)
	p := filepath.Join(e.dir, "forged.docx")
	forgedZip(t, p, 98<<20, func(n, e int) []byte { return eocdBytes(1, uint32(n), 0) })

	// 对照：不做预检、直接交给 archive/zip，会大量分配（实测约 580 MiB；断言 ≥128 MiB，保证测试确实覆盖了攻击路径）
	var b0, b1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&b0)
	if zr, err := zip.OpenReader(p); err == nil {
		zr.Close()
	}
	runtime.ReadMemStats(&b1)
	ctl := (b1.TotalAlloc - b0.TotalAlloc) >> 20
	t.Logf("对照：直接 zip.OpenReader 的累计分配 %d MiB", ctl)
	if ctl < 128 { // 对照组必须真的很大，否则这个测试没有测到攻击路径
		t.Fatalf("对照组只分配了 %d MiB：伪造文件没有触发 Go 读取器的大量分配，测试无效", ctl)
	}

	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	_, err := e.svc.ConvertToPDF(context.Background(), []string{p}, "")
	runtime.ReadMemStats(&m1)
	ae := wantCode(t, err, apperr.InvalidArgument)
	if !strings.Contains(ae.Detail, "中央目录") {
		t.Fatalf("detail=%q", ae.Detail)
	}
	if d := (m1.TotalAlloc - m0.TotalAlloc) >> 20; d > 64 {
		t.Fatalf("伪造文件的校验累计分配了 %d MiB，预检应在解析目录之前拒绝", d)
	}
}

// 合法 zip64（条目数 > 65535）的预检行为保持不变：10 万+1 拒绝，恰好 10 万通过；目录字节数在上限内。
func TestZip64LegitEntryCountsUnchanged(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string, total int) string {
		p := filepath.Join(dir, name)
		f, _ := os.Create(p)
		zw := zip.NewWriter(f)
		w, _ := zw.Create("word/document.xml")
		w.Write([]byte("<x/>"))
		for i := 1; i < total; i++ {
			if _, err := zw.CreateHeader(&zip.FileHeader{Name: "x/" + strings.Repeat("a", i%7) + string(rune('a'+i%26)) + itoa(i), Method: zip.Store}); err != nil {
				t.Fatal(err)
			}
		}
		zw.Close()
		f.Close()
		return p
	}
	over := mk("over.docx", MaxZipEntries+1)
	edge := mk("edge.docx", MaxZipEntries)
	if d, ok := readZipDir(over); !ok || d.entries != MaxZipEntries+1 || d.bogus {
		t.Fatalf("over: %+v %v", d, ok)
	}
	if err := checkZipEntries(over); err == nil || !strings.Contains(err.(*apperr.AppError).Detail, "100000") {
		t.Fatalf("10 万+1 应因条目数拒绝: %v", err)
	}
	d, ok := readZipDir(edge)
	if !ok || d.entries != MaxZipEntries || d.bogus {
		t.Fatalf("edge: %+v %v", d, ok)
	}
	if d.dirBytes > MaxZipDirBytes {
		t.Fatalf("恰好 10 万条的正常目录 %d 字节超过了 MaxZipDirBytes=%d，上限设小了", d.dirBytes, MaxZipDirBytes)
	}
	if err := checkZipEntries(edge); err != nil {
		t.Fatalf("恰好 10 万条应通过: %v", err)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}
