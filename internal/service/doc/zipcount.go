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

// MaxZipDirBytes 是中央目录字节数上限。每个条目的中央目录头固定 46 字节，再加文件名 / 扩展字段；
// 按每条平均 96 字节留余量（Office 的部件名一般 20～50 字节），10 万条约 9.6 MB。architect 建议的“约 46 字节 × 10 万 = 4.6 MB”
// 只算了固定头，会把带正常部件名、条目数接近 6 万的 pptx 误拒，所以放宽到每条 96 字节；它的作用是给“伪造 EOCD 让 Go 的
// zip 读取器去解析几十 MB 假目录”封顶，10 MB 级别的目录不会造成明显内存压力。
const MaxZipDirBytes = MaxZipEntries * 96

func errDirTooLarge() error {
	return reasonErr(apperr.InvalidArgument, "不是有效的 OOXML 文件", reasonTooLarge, "压缩包中央目录超过 9600000 字节")
}

func errBadDirectory() error {
	return reasonErr(apperr.InvalidArgument, "不是有效的 OOXML 文件", reasonInvalidOOXML, "压缩包目录信息无效（zip64 记录缺失或损坏）")
}

func errTooManyEntries(_ uint64) error {
	return reasonErr(apperr.InvalidArgument, "不是有效的 OOXML 文件", reasonTooLarge, "压缩包条目数超过 100000")
}

// zipDir 是从 EOCD（必要时加 zip64 记录）读出的中央目录概况。
type zipDir struct {
	entries  uint64
	dirBytes uint64
	bogus    bool // EOCD 的 32 位字段是 0xFFFFFFFF 占位符，但没有可用的 zip64 记录：伪造或损坏
}

// readZipDir 只读文件尾部的 End Of Central Directory（含 zip64 记录），不解析中央目录，
// 所以对条目特别多的压缩包也不会先分配几百 MB 再检查。读不出来（不是 zip、被截断等）返回 ok=false，
// 由后面的 zip.OpenReader 给出正常的错误。
//
// 16 位条目数不可信：伪造的 EOCD 可以写一个很小的条目数（或让它在 65536 处回绕），同时把中央目录大小写得很大。
// 所以条目数、目录大小、目录偏移三个字段里只要有一个是占位符 0xFFFF / 0xFFFFFFFF，就必须去读 zip64 记录；
// 调用方再对“条目数”和“目录字节数”各设一个上限。
func readZipDir(path string) (d zipDir, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return d, false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.Size() < 22 {
		return d, false
	}
	size := fi.Size()
	tail := int64(22 + 65535)
	if tail > size {
		tail = size
	}
	buf := make([]byte, tail)
	if _, err := f.ReadAt(buf, size-tail); err != nil && err != io.EOF {
		return d, false
	}
	eocd := -1
	for i := len(buf) - 22; i >= 0; i-- {
		if buf[i] == 'P' && buf[i+1] == 'K' && buf[i+2] == 5 && buf[i+3] == 6 {
			eocd = i
			break
		}
	}
	if eocd < 0 {
		return d, false
	}
	total16 := uint64(binary.LittleEndian.Uint16(buf[eocd+10:]))
	size32 := uint64(binary.LittleEndian.Uint32(buf[eocd+12:]))
	off32 := uint64(binary.LittleEndian.Uint32(buf[eocd+16:]))
	d = zipDir{entries: total16, dirBytes: size32}
	if total16 != 0xFFFF && size32 != 0xFFFFFFFF && off32 != 0xFFFFFFFF {
		return d, true
	}
	placeholder32 := size32 == 0xFFFFFFFF || off32 == 0xFFFFFFFF
	// zip64：EOCD 前面 20 字节是 zip64 locator，指向 zip64 EOCD 记录
	loc := int64(eocd) - 20
	if loc < 0 || string(buf[loc:loc+4]) != "PK\x06\x07" {
		d.bogus = placeholder32
		return d, true
	}
	off := int64(binary.LittleEndian.Uint64(buf[loc+8:]))
	rec := make([]byte, 56)
	if off < 0 || off+56 > size {
		d.bogus = placeholder32
		return d, true
	}
	if _, err := f.ReadAt(rec, off); err != nil || string(rec[:4]) != "PK\x06\x06" {
		d.bogus = placeholder32
		return d, true
	}
	d.entries = binary.LittleEndian.Uint64(rec[32:])
	d.dirBytes = binary.LittleEndian.Uint64(rec[40:])
	return d, true
}

// zipEntryCount 返回 EOCD（或 zip64 记录）声明的条目总数。
func zipEntryCount(path string) (n uint64, ok bool) {
	d, ok := readZipDir(path)
	return d.entries, ok
}

// checkZipEntries 在打开压缩包之前做预检：条目数上限、中央目录字节数上限、占位符无 zip64 记录的伪造 EOCD。
// 都返回 INVALID_ARGUMENT（与 256 MiB 单条目的 zip 炸弹防护同一个错误码）。
func checkZipEntries(path string) error {
	d, ok := readZipDir(path)
	if !ok {
		return nil
	}
	switch {
	case d.bogus:
		return errBadDirectory()
	case d.entries > MaxZipEntries:
		return errTooManyEntries(d.entries)
	case d.dirBytes > MaxZipDirBytes:
		return errDirTooLarge()
	}
	return nil
}
