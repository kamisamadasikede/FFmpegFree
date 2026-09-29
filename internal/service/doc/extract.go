package doc

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"FFmpegFree/internal/apperr"

	"github.com/xuri/excelize/v2"
)

// 文本提取（契约 6.12.1）：只取文字，按顺序重排；逻辑参照 v1 的 office_controller（v1 后端已随 backend/ 删除，见 master 分支）。

type unitKind uint8

const (
	unitPara  unitKind = iota // 正文一段 / 一行
	unitHead                  // 标题（Sheet: 名称、Slide n）
	unitBlank                 // xlsx 空行
	unitBreak                 // 换页
)

type unit struct {
	kind unitKind
	text string
	tick bool // 是否计入进度（docx 段落、xlsx 行、pptx 幻灯片）
}

type docModel struct {
	units    []unit
	bodySize float64
	runes    map[rune]int // 清理后文本里各字符的出现次数（选字体、统计缺字）
	ticks    int
	textSize int
}

// maxTextBytes 是一份文档提取出的文字总量上限。5000 页放不下这么多字，超过按“超过 5000 页”处理，避免超大 XML 吃光内存。
const maxTextBytes = 64 << 20

var errTooLong = reasonErr(apperr.Unsupported, "超过 5000 页", reasonTooManyPages, "文档文字量超过上限")

func newModel(bodySize float64) *docModel {
	return &docModel{bodySize: bodySize, runes: map[rune]int{}}
}

// zeroWidth 是不该画出来（也不该算缺字）的不可见格式字符。
func zeroWidth(r rune) bool {
	switch {
	case r >= 0x200B && r <= 0x200F, r >= 0x202A && r <= 0x202E, r >= 0x2060 && r <= 0x206F:
		return true
	case r == 0x00AD, r == 0xFEFF, r == 0x2028, r == 0x2029:
		return true
	}
	return false
}

// clean 规范化一段文本：去掉不可见格式字符和控制字符（\n 保留，制表符变 4 个空格），
// 超出 BMP 的字符换成 U+FFFD（fpdf 只支持 U+0000~U+FFFF，字体也不会有）。同时统计字符。
func (m *docModel) clean(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteRune(r)
		case r == '\t':
			b.WriteString("    ")
			m.runes[' '] += 4
			continue
		case r < 0x20 || (r >= 0x7F && r <= 0x9F) || zeroWidth(r):
			continue
		case r > 0xFFFF || r == unicode.ReplacementChar || !isValidRune(r):
			r = unicode.ReplacementChar
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
		m.runes[r]++
	}
	return b.String()
}

func isValidRune(r rune) bool { return r >= 0 && r <= unicode.MaxRune && !(r >= 0xD800 && r <= 0xDFFF) }

func (m *docModel) add(u unit) error {
	m.textSize += len(u.text)
	if m.textSize > maxTextBytes {
		return errTooLong
	}
	if u.tick {
		m.ticks++
	}
	m.units = append(m.units, u)
	return nil
}

func readEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxEntryBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxEntryBytes {
		return nil, reasonErr(apperr.InvalidArgument, "不是有效的 OOXML 文件", reasonTooLarge, f.Name+" 解压后超过 256 MiB")
	}
	return data, nil
}

func invalidOOXML(err error) error {
	return reasonErr(apperr.InvalidArgument, "不是有效的 OOXML 文件", reasonInvalidOOXML, err.Error())
}

// extractParagraphs 按 v1 的 extractTextRuns：每个 <t> 的文本拼进当前段，遇到结束标签 </p> 输出一段（去首尾空白，空段跳过）。
func extractParagraphs(ctx context.Context, data []byte) ([]string, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var lines []string
	var cur strings.Builder
	for i := 0; ; i++ {
		if i%2000 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, invalidOOXML(err)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			if v.Name.Local == "t" {
				var text string
				if err := dec.DecodeElement(&text, &v); err != nil {
					return nil, invalidOOXML(err)
				}
				cur.WriteString(text)
			}
		case xml.EndElement:
			if v.Name.Local == "p" {
				if t := strings.TrimSpace(cur.String()); t != "" {
					lines = append(lines, t)
				}
				cur.Reset()
			}
		}
	}
	if t := strings.TrimSpace(cur.String()); t != "" {
		lines = append(lines, t)
	}
	return lines, nil
}

func findEntry(zr *zip.ReadCloser, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func extractDocx(ctx context.Context, path string) (*docModel, error) {
	if err := checkZipEntries(path); err != nil {
		return nil, err
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, openErr(err)
	}
	defer zr.Close()
	f := findEntry(zr, "word/document.xml")
	if f == nil {
		return nil, invalidOOXML(errors.New("缺少 word/document.xml"))
	}
	data, err := readEntry(f)
	if err != nil {
		return nil, entryErr(err)
	}
	lines, err := extractParagraphs(ctx, data)
	if err != nil {
		return nil, err
	}
	m := newModel(12)
	for _, l := range lines {
		if err := m.add(unit{kind: unitPara, text: m.clean(l), tick: true}); err != nil {
			return nil, err
		}
	}
	return m, nil
}

var slideRe = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

func extractPptx(ctx context.Context, path string) (*docModel, error) {
	if err := checkZipEntries(path); err != nil {
		return nil, err
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, openErr(err)
	}
	defer zr.Close()
	type slide struct {
		n int
		f *zip.File
	}
	var slides []slide
	for _, f := range zr.File {
		if mm := slideRe.FindStringSubmatch(f.Name); mm != nil {
			n, err := strconv.Atoi(mm[1])
			if err != nil {
				continue
			}
			slides = append(slides, slide{n, f})
		}
	}
	if len(slides) == 0 {
		return nil, invalidOOXML(errors.New("缺少 ppt/slides/slide<n>.xml"))
	}
	// 按数字顺序（v1 按字符串排序，slide10 会排在 slide2 前面）。
	sort.Slice(slides, func(i, j int) bool { return slides[i].n < slides[j].n })
	m := newModel(11)
	for idx, sl := range slides {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := readEntry(sl.f)
		if err != nil {
			return nil, entryErr(err)
		}
		lines, err := extractParagraphs(ctx, data)
		if err != nil {
			return nil, err
		}
		if idx > 0 {
			if err := m.add(unit{kind: unitBreak}); err != nil {
				return nil, err
			}
		}
		if err := m.add(unit{kind: unitHead, text: fmt.Sprintf("Slide %d", idx+1), tick: true}); err != nil {
			return nil, err
		}
		for _, l := range lines {
			if err := m.add(unit{kind: unitPara, text: m.clean(l)}); err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}

func extractXlsx(ctx context.Context, path string) (*docModel, error) {
	if err := checkZipEntries(path); err != nil {
		return nil, err
	}
	book, err := excelize.OpenFile(path, excelize.Options{UnzipXMLSizeLimit: maxEntryBytes, UnzipSizeLimit: 4 * maxEntryBytes})
	if err != nil {
		return nil, invalidOOXML(err)
	}
	defer func() { _ = book.Close() }()
	m := newModel(11)
	sheets := book.GetSheetList()
	for si, sheet := range sheets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if si > 0 {
			if err := m.add(unit{kind: unitBreak}); err != nil {
				return nil, err
			}
		}
		if err := m.add(unit{kind: unitHead, text: m.clean("Sheet: " + sheet)}); err != nil {
			return nil, err
		}
		rows, err := book.Rows(sheet) // 流式读取，不一次性载入整张表
		if err != nil {
			return nil, invalidOOXML(fmt.Errorf("读取工作表 %q 失败: %w", sheet, err))
		}
		for ri := 0; rows.Next(); ri++ {
			if ri%500 == 0 {
				if err := ctx.Err(); err != nil {
					rows.Close()
					return nil, err
				}
			}
			row, err := rows.Columns()
			if err != nil {
				rows.Close()
				return nil, invalidOOXML(fmt.Errorf("读取工作表 %q 失败: %w", sheet, err))
			}
			for ci, c := range row {
				if r := []rune(c); len(r) > maxCellRunes {
					row[ci] = string(r[:maxCellRunes])
				}
			}
			line := strings.Join(row, "    ")
			if strings.TrimSpace(line) == "" {
				if err := m.add(unit{kind: unitBlank, tick: true}); err != nil {
					rows.Close()
					return nil, err
				}
				continue
			}
			if err := m.add(unit{kind: unitPara, text: m.clean(line), tick: true}); err != nil {
				rows.Close()
				return nil, err
			}
		}
		if err := rows.Close(); err != nil {
			return nil, invalidOOXML(err)
		}
	}
	return m, nil
}

func (s *Service) extract(ctx context.Context, path, ext string) (*docModel, error) {
	switch ext {
	case "docx":
		return extractDocx(ctx, path)
	case "xlsx":
		return extractXlsx(ctx, path)
	case "pptx":
		return extractPptx(ctx, path)
	}
	return nil, reasonErr(apperr.Unsupported, "暂不支持这种格式", reasonFormat, "."+ext)
}
