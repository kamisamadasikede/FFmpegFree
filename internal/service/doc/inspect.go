package doc

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/richardlehane/mscfb"

	"FFmpegFree/internal/apperr"
)

// ---------- 添加 / 提交 / 运行前的轻量检查（契约 6.12.16、6.12.19） ----------

var oleMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

func errEncrypted() *apperr.AppError {
	return apperr.New(apperr.DocEncrypted, "这个文件有密码保护，不能转换。请先去掉密码再添加。")
}

func errCorrupt(detail string) *apperr.AppError {
	return apperr.New(apperr.DocCorrupt, "文件打不开，可能已损坏或不是有效的文档。").WithDetail(detail)
}

// inspectDoc 检查一个文档：空文件 / 容器损坏 → DOC_CORRUPT，加密 → DOC_ENCRYPTED；返回 sheetCount（6.12.16）。
// ext 是规范化后的扩展名。不会 panic（OLE 解析器的 panic 也按损坏处理）。
func inspectDoc(ctx context.Context, path, ext string) (sheetCount int, err error) {
	defer func() {
		if r := recover(); r != nil {
			sheetCount, err = 0, errCorrupt(fmt.Sprintf("panic=%v", r))
		}
	}()
	f, fi, err := openRegular(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if fi.Size() == 0 {
		return 0, errCorrupt("empty")
	}
	head := make([]byte, 8)
	n, err := io.ReadFull(f, head)
	if err != nil && n == 0 {
		return 0, apperr.Wrap(apperr.IOError, "读取文件失败", err)
	}
	head = head[:n]
	isOLE := bytes.Equal(head, oleMagic)
	switch ext {
	case "docx", "xlsx", "pptx":
		if isOLE {
			enc, err := oleHasStreams(f, "EncryptionInfo", "EncryptedPackage")
			if err != nil {
				return 0, err
			}
			if enc {
				return 0, errEncrypted()
			}
			return unknownSheets(ext), nil // 旧格式改了扩展名：交给组件按内容识别（6.12.19）
		}
		return inspectOOXML(ctx, path, ext)
	case "odt", "ods", "odp":
		return inspectODF(ctx, path, ext)
	case "doc", "xls", "ppt":
		if !isOLE {
			if textLike(head) { // 网页 / RTF 另存的 .doc / .xls（很常见），交给组件按内容识别
				return unknownSheets(ext), nil
			}
			return 0, errCorrupt("ole_header")
		}
		return inspectOLE(ctx, f, ext)
	case "csv":
		return 1, nil
	case "pdf":
		_, err := inspectPDF(ctx, path) // 6.12.60
		return 0, err
	}
	return 0, nil // rtf / txt / html / md：只检查非空和可读
}

func unknownSheets(ext string) int {
	if familyOf(ext) == FamilySheet {
		return -1
	}
	return 0
}

// textLike：开头是 RTF / HTML / XML / BOM 的文本文件。
func textLike(head []byte) bool {
	h := bytes.TrimLeft(head, " \t\r\n")
	return bytes.HasPrefix(h, []byte("{\\rtf")) || bytes.HasPrefix(h, []byte("<")) || bytes.HasPrefix(h, []byte{0xEF, 0xBB, 0xBF}) ||
		bytes.HasPrefix(h, []byte{0xFF, 0xFE}) || bytes.HasPrefix(h, []byte{0xFE, 0xFF})
}

func openZip(path string) (*zip.ReadCloser, error) {
	if err := checkZipEntries(path); err != nil {
		return nil, errCorrupt("zip_dir")
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, errCorrupt("not_zip")
	}
	if len(zr.File) > MaxZipEntries {
		zr.Close()
		return nil, errCorrupt("zip_entries")
	}
	return zr, nil
}

func inspectOOXML(ctx context.Context, path, ext string) (int, error) {
	zr, err := openZip(path)
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	need := map[string]string{"docx": "word/document.xml", "xlsx": "xl/workbook.xml", "pptx": "ppt/presentation.xml"}[ext]
	part := findEntry(zr, need)
	if part == nil {
		return 0, errCorrupt("missing=" + need)
	}
	if ext != "xlsx" {
		return 0, nil
	}
	n, err := countElements(ctx, part, "sheet", "")
	if err != nil {
		return -1, nil
	}
	return n, nil
}

const odfTableNS = "urn:oasis:names:tc:opendocument:xmlns:table:1.0"

func inspectODF(ctx context.Context, path, ext string) (int, error) {
	zr, err := openZip(path)
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	man := findEntry(zr, "META-INF/manifest.xml")
	content := findEntry(zr, "content.xml")
	if man == nil || content == nil {
		return 0, errCorrupt("missing=content.xml")
	}
	enc, err := zipEntryContains(man, []byte("<manifest:encryption-data"))
	if err != nil {
		return 0, errCorrupt("manifest")
	}
	if enc {
		return 0, errEncrypted()
	}
	if ext != "ods" {
		return 0, nil
	}
	n, err := countElements(ctx, content, "table", odfTableNS)
	if err != nil {
		return -1, nil
	}
	return n, nil
}

// zipEntryContains 流式查找（不整份读入；跨块边界也能找到）。
func zipEntryContains(f *zip.File, needle []byte) (bool, error) {
	rc, err := f.Open()
	if err != nil {
		return false, err
	}
	defer rc.Close()
	r := io.LimitReader(rc, maxEntryBytes)
	buf := make([]byte, 64<<10)
	var carry []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := append(carry, buf[:n]...)
			if bytes.Contains(chunk, needle) {
				return true, nil
			}
			if len(chunk) > len(needle) {
				carry = append([]byte(nil), chunk[len(chunk)-len(needle):]...)
			} else {
				carry = chunk
			}
		}
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}

// countElements 流式数 zip 条目里名为 local（space 非空时还要求命名空间）的开始标签。
func countElements(ctx context.Context, f *zip.File, local, space string) (int, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	dec := xml.NewDecoder(bufio.NewReader(io.LimitReader(rc, maxEntryBytes)))
	dec.Strict = false
	n := 0
	for i := 0; ; i++ {
		if i%4096 == 0 && ctx.Err() != nil {
			return 0, ctx.Err()
		}
		tok, err := dec.RawToken()
		if err == io.EOF {
			return n, nil
		}
		if err != nil {
			return 0, err
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == local {
			// RawToken 不解析命名空间：前缀是文档里写的（ODF 惯例是 table:）
			if space == "" || se.Name.Space == "table" {
				n++
			}
		}
	}
}

// oleHasStreams 判断 OLE 复合文档是否同时含有所有指定的流。
func oleHasStreams(f *os.File, names ...string) (bool, error) {
	doc, err := mscfb.New(f)
	if err != nil {
		return false, errCorrupt("ole")
	}
	found := map[string]bool{}
	for e, err := doc.Next(); err == nil; e, err = doc.Next() {
		found[e.Name] = true
	}
	for _, n := range names {
		if !found[n] {
			return false, nil
		}
	}
	return true, nil
}

func inspectOLE(ctx context.Context, f *os.File, ext string) (int, error) {
	doc, err := mscfb.New(f)
	if err != nil {
		return 0, errCorrupt("ole")
	}
	streams := map[string]*mscfb.File{}
	for e, err := doc.Next(); err == nil; e, err = doc.Next() {
		if len(e.Path) == 0 { // 只看根目录下的流
			if _, dup := streams[e.Name]; !dup {
				streams[e.Name] = e
			}
		}
	}
	switch ext {
	case "doc":
		wd := streams["WordDocument"]
		if wd == nil {
			return 0, errCorrupt("missing=WordDocument")
		}
		fib := make([]byte, 12)
		if n, err := wd.ReadAt(fib, 0); n < len(fib) && err != nil {
			return 0, errCorrupt("fib")
		}
		if binary.LittleEndian.Uint16(fib[0x0A:])&0x0100 != 0 {
			return 0, errEncrypted()
		}
		return 0, nil
	case "xls":
		wb := streams["Workbook"]
		if wb == nil {
			wb = streams["Book"]
		}
		if wb == nil {
			return 0, errCorrupt("missing=Workbook")
		}
		return scanXLSGlobals(ctx, wb)
	case "ppt":
		if streams["EncryptedSummary"] != nil {
			return 0, errEncrypted()
		}
		if streams["PowerPoint Document"] == nil {
			return 0, errCorrupt("missing=PowerPoint Document")
		}
		if cu := streams["Current User"]; cu != nil {
			b := make([]byte, 16)
			if n, _ := cu.ReadAt(b, 0); n >= 16 && binary.LittleEndian.Uint32(b[12:]) == 0xF3D1C4DF {
				return 0, errEncrypted()
			}
		}
		return 0, nil
	}
	return 0, nil
}

// scanXLSGlobals 读 BIFF 全局子流：BOF 之后出现 FILEPASS（0x002F）= 加密；数 BOUNDSHEET（0x0085）= 工作表数；到 EOF（0x000A）为止。
func scanXLSGlobals(ctx context.Context, wb io.Reader) (int, error) {
	r := bufio.NewReader(wb)
	hdr := make([]byte, 4)
	sheets, sawBOF := 0, false
	for i := 0; i < 1_000_000; i++ {
		if i%4096 == 0 && ctx.Err() != nil {
			return -1, nil
		}
		if _, err := io.ReadFull(r, hdr); err != nil {
			if sawBOF {
				return sheetsOrUnknown(sheets), nil
			}
			return 0, errCorrupt("biff")
		}
		typ, size := binary.LittleEndian.Uint16(hdr), int(binary.LittleEndian.Uint16(hdr[2:]))
		if !sawBOF {
			if typ != 0x0809 && typ != 0x0209 && typ != 0x0409 && typ != 0x0009 {
				return 0, errCorrupt("biff_bof")
			}
			sawBOF = true
		} else {
			switch typ {
			case 0x002F:
				return 0, errEncrypted()
			case 0x0085:
				sheets++
			case 0x000A:
				return sheetsOrUnknown(sheets), nil
			}
		}
		if _, err := r.Discard(size); err != nil {
			return sheetsOrUnknown(sheets), nil
		}
	}
	return sheetsOrUnknown(sheets), nil
}

func sheetsOrUnknown(n int) int {
	if n == 0 {
		return -1
	}
	return n
}

var errInspectTimeout = errors.New("检查超时")

// isDocErr 判断是不是“不可重试”的文档错误（6.12.20）。
func notRetryable(code apperr.Code) bool {
	switch code {
	case apperr.DocEncrypted, apperr.DocCorrupt, apperr.DocFormatUnsupported, apperr.DocPDFInputUnsupported, apperr.DocPDFNoText:
		return true
	}
	return false
}

// notRetryableErr 在 notRetryable 之外，把 PDF 太大 / 页数太多（INVALID_ARGUMENT reason=too_large|too_many_pages）也算不可重试（v0.28）。
func notRetryableErr(e *apperr.AppError) bool {
	if e == nil {
		return false
	}
	if notRetryable(e.Code) {
		return true
	}
	if e.Code == apperr.InvalidArgument {
		first, _, _ := strings.Cut(e.Detail, "\n")
		return first == "reason="+reasonTooLarge || first == "reason="+reasonTooManyPages
	}
	return false
}

var _ = strings.TrimSpace
