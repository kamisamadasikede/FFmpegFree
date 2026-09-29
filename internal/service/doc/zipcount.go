package doc

import (
	"encoding/binary"
	"io"
	"os"

	"FFmpegFree/internal/apperr"
)

// MaxZipEntries 是 docx / pptx / xlsx 压缩包允许的最大条目数。正常 Office 文件一般只有几十到几千个条目
// （一个 5000 页的 pptx 约 5000 个 slide 加上媒体 / 版式），10 万足够宽松。超过按“不是有效的 OOXML 文件”处理
// （INVALID_ARGUMENT，与 256 MiB 单条目上限的 zip 炸弹防护同一个错误码）。
const MaxZipEntries = 100_000

func errTooManyEntries(_ uint64) error {
	return apperr.New(apperr.InvalidArgument, "不是有效的 OOXML 文件").WithDetail("压缩包条目数超过 100000")
}

// zipEntryCount 只读文件尾部的 End Of Central Directory（含 zip64 记录）取条目总数，不解析中央目录，
// 所以对条目特别多的压缩包也不会先分配几百 MB 再检查。读不出来（不是 zip、被截断等）返回 ok=false，
// 由后面的 zip.OpenReader 给出正常的错误。
func zipEntryCount(path string) (n uint64, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.Size() < 22 {
		return 0, false
	}
	size := fi.Size()
	tail := int64(22 + 65535)
	if tail > size {
		tail = size
	}
	buf := make([]byte, tail)
	if _, err := f.ReadAt(buf, size-tail); err != nil && err != io.EOF {
		return 0, false
	}
	eocd := -1
	for i := len(buf) - 22; i >= 0; i-- {
		if buf[i] == 'P' && buf[i+1] == 'K' && buf[i+2] == 5 && buf[i+3] == 6 {
			eocd = i
			break
		}
	}
	if eocd < 0 {
		return 0, false
	}
	total := uint64(binary.LittleEndian.Uint16(buf[eocd+10:]))
	if total != 0xFFFF {
		return total, true
	}
	// zip64：EOCD 前面 20 字节是 zip64 locator，指向 zip64 EOCD 记录
	loc := int64(eocd) - 20
	if loc < 0 || string(buf[loc:loc+4]) != "PK\x06\x07" {
		return total, true
	}
	off := int64(binary.LittleEndian.Uint64(buf[loc+8:]))
	if off < 0 || off+56 > size {
		return 0, false
	}
	rec := make([]byte, 56)
	if _, err := f.ReadAt(rec, off); err != nil || string(rec[:4]) != "PK\x06\x06" {
		return 0, false
	}
	return binary.LittleEndian.Uint64(rec[32:]), true
}

// checkZipEntries 在打开压缩包之前检查条目数上限。
func checkZipEntries(path string) error {
	if n, ok := zipEntryCount(path); ok && n > MaxZipEntries {
		return errTooManyEntries(n)
	}
	return nil
}
