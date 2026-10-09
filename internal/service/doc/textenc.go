package doc

import (
	"bytes"
	"os"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"FFmpegFree/internal/apperr"
)

// decodeText 把 CSV / TXT / MD 的内容转成 UTF-8（契约 6.12.17）：整份是合法 UTF-8（开头的 BOM 去掉）就按 UTF-8，
// 否则按 GB18030（兼容 GBK / GB2312）解码。不提示用户。
func decodeText(b []byte) []byte {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(b) {
		return b
	}
	out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(b)
	if err != nil {
		return bytes.ToValidUTF8(b, []byte("\uFFFD"))
	}
	return out
}

// readTextFile 读入并解码（输入已限制 ≤ 100 MiB）。
func readTextFile(p string) ([]byte, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, readErr("读取文件失败", err)
	}
	return decodeText(b), nil
}

// writeUTF8Temp 把解码后的内容写成临时文件（交给组件 / goldmark）。
func writeUTF8Temp(src, dst string) error {
	b, err := readTextFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return apperr.Wrap(apperr.IOError, "无法写入临时文件", err)
	}
	return nil
}
