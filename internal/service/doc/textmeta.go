package doc

import (
	"bytes"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"FFmpegFree/internal/apperr"
)

// 文本类编码 / 换行（契约 6.12.40）。
const (
	encUTF8    = "utf8"
	encUTF8BOM = "utf8_bom"
	encGBK     = "gbk"

	lineCRLF = "crlf"
	lineLF   = "lf"

	maxEditTextBytes = 2 << 20 // 2 MiB
	maxEditCSVRows   = 1000
	maxCSVCellRunes  = 32767
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

var htmlCharsetRe = regexp.MustCompile(`(?is)<meta[^>]+charset\s*=\s*["']?\s*([a-z0-9_\-]+)`)

// detectEncoding 识别原始字节的编码标签（读取：非 UTF-8 按 GB18030 解码，标签叫 gbk）。
func detectEncoding(raw []byte) string {
	if bytes.HasPrefix(raw, utf8BOM) {
		body := raw[len(utf8BOM):]
		if utf8.Valid(body) {
			return encUTF8BOM
		}
	}
	if utf8.Valid(raw) {
		return encUTF8
	}
	return encGBK
}

// detectLineEnding 多数行风格；一个换行都没有时 Windows=crlf，其他=lf。
func detectLineEnding(raw []byte) string {
	crlf, lf := 0, 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\n' {
			if i > 0 && raw[i-1] == '\r' {
				crlf++
			} else {
				lf++
			}
		}
	}
	if crlf == 0 && lf == 0 {
		if runtime.GOOS == "windows" {
			return lineCRLF
		}
		return lineLF
	}
	if crlf >= lf {
		return lineCRLF
	}
	return lineLF
}

// decodeRawText 按检测结果解码成 UTF-8 字符串（不含 BOM）。
func decodeRawText(raw []byte) (text string, enc string) {
	enc = detectEncoding(raw)
	switch enc {
	case encUTF8BOM:
		return string(raw[len(utf8BOM):]), enc
	case encUTF8:
		return string(raw), enc
	default:
		out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw)
		if err != nil {
			return string(bytes.ToValidUTF8(raw, []byte("\uFFFD"))), enc
		}
		return string(out), enc
	}
}

// htmlMetaCharset 返回 html 里 <meta charset> 的规范化名；没有返回 ""。
func htmlMetaCharset(text string) string {
	m := htmlCharsetRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(m[1], "_", "-"))
}

// htmlEncodingSupported 是否允许编辑（UTF-8 / GBK / GB2312 / GB18030）。
func htmlEncodingSupported(charset string) bool {
	if charset == "" {
		return true // 没有声明：按字节检测
	}
	switch charset {
	case "utf-8", "utf8", "gbk", "gb2312", "gb-2312", "gb18030", "gb-18030":
		return true
	}
	return false
}

// htmlDeclaredGBK meta 声明了 GB 系编码。
func htmlDeclaredGBK(charset string) bool {
	switch charset {
	case "gbk", "gb2312", "gb-2312", "gb18030", "gb-18030":
		return true
	}
	return false
}

// encodeTextBytes 按目标编码和换行把 UTF-8 文本编成要写入的字节。
// encoding=gbk 时严格 GBK（CP936），编不进去返回 INVALID_ARGUMENT reason=encoding。
func encodeTextBytes(text, encoding, lineEnding string) ([]byte, error) {
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	norm = strings.ReplaceAll(norm, "\r", "\n")
	if lineEnding == lineCRLF {
		norm = strings.ReplaceAll(norm, "\n", "\r\n")
	}
	switch encoding {
	case encUTF8:
		return []byte(norm), nil
	case encUTF8BOM:
		return append(append([]byte{}, utf8BOM...), []byte(norm)...), nil
	case encGBK:
		return encodeStrictGBK(norm)
	default:
		return nil, apperr.New(apperr.InvalidArgument, "不支持的编码")
	}
}

// encodeStrictGBK 严格按 CP936 编码；失败时 detail 带 char= / line=。
func encodeStrictGBK(text string) ([]byte, error) {
	var out bytes.Buffer
	line := 1
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			return nil, encodingRefuseErr(r, line)
		}
		if r == '\n' {
			line++
		}
		if r < 0x80 {
			out.WriteByte(byte(r))
			i += size
			continue
		}
		b, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(string(r)))
		if err != nil || len(b) == 0 {
			return nil, encodingRefuseErr(r, line)
		}
		back, err := simplifiedchinese.GBK.NewDecoder().Bytes(b)
		if err != nil {
			return nil, encodingRefuseErr(r, line)
		}
		br, _ := utf8.DecodeRune(back)
		if br != r {
			return nil, encodingRefuseErr(r, line)
		}
		out.Write(b)
		i += size
	}
	return out.Bytes(), nil
}

func encodingRefuseErr(r rune, line int) *apperr.AppError {
	return apperr.New(apperr.InvalidArgument, "有些字符没法按原编码保存，请另存为 UTF-8。").
		WithDetail(fmt.Sprintf("reason=encoding\nchar=U+%04X\nline=%d", r, line))
}

// applyLineEnding 只换换行（已是 \n 规范化文本）。
func applyLineEnding(text, lineEnding string) string {
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	norm = strings.ReplaceAll(norm, "\r", "\n")
	if lineEnding == lineCRLF {
		return strings.ReplaceAll(norm, "\n", "\r\n")
	}
	return norm
}
