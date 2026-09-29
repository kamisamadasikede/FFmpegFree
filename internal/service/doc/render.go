package doc

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/task"

	"github.com/go-pdf/fpdf"
)

// errWriter 记录第一个写入错误，保证磁盘满等系统错误能被原样识别（fpdf 会把写错误包成自己的错误）。
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	n, err := e.w.Write(p)
	if err != nil {
		e.err = err
	}
	return n, err
}

// writeErr 把写 PDF 时的错误转成契约错误：磁盘满 CONVERT_DISK_FULL，其他 IO_ERROR。
func writeErr(err error) error {
	if isDiskFull(err) {
		return apperr.Wrap(apperr.ConvertDiskFull, "磁盘空间不足，无法写入输出文件", err)
	}
	return apperr.Wrap(apperr.IOError, "写入输出文件失败", err)
}

const checkEvery = 100 // 每处理约 100 个单元检查一次 ctx

// renderPDF 把 docModel 排成 PDF 写到 part。返回缺字统计写进任务日志（logw）。
func renderPDF(ctx context.Context, m *docModel, choice fontChoice, part string, report func(task.Progress), logw io.Writer) error {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 18, 15)
	pdf.SetAutoPageBreak(true, 18)
	tr := func(s string) string { return s }
	family := "helvetica"
	if choice.face != nil {
		family = "doc"
		pdf.AddUTF8FontFromBytes(family, "", choice.face.data)
	} else {
		tr = pdf.UnicodeTranslatorFromDescriptor("")
	}
	pdf.AddPage()
	pdf.SetFont(family, "", m.bodySize)
	if err := pdf.Error(); err != nil {
		return apperr.Wrap(apperr.Internal, "加载字体失败", err)
	}

	kinds, total, sample := missingRunes(choice.face, m.runes)
	fmt.Fprintf(logw, "font=%s\n", choice.name())
	if kinds > 0 { // 只记码点，不记文档文字内容
		fmt.Fprintf(logw, "missing_glyphs=%d total=%d sample=%s\n", kinds, total, sampleCodepoints(sample))
	}

	done := 0
	for i, u := range m.units {
		if i%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		switch u.kind {
		case unitHead:
			pdf.SetFontSize(14)
			if err := writeWrapped(ctx, pdf, tr(u.text), 8, choice.face != nil); err != nil {
				return err
			}
			pdf.Ln(2)
			pdf.SetFontSize(m.bodySize)
		case unitPara:
			if err := writeWrapped(ctx, pdf, tr(u.text), 6, choice.face != nil); err != nil {
				return err
			}
		case unitBlank:
			pdf.Ln(4)
		case unitBreak:
			pdf.AddPage()
		}
		if pdf.PageNo() > MaxPages {
			return apperr.New(apperr.Unsupported, "超过 5000 页").WithDetail(fmt.Sprintf("已排到第 %d 页仍未结束", pdf.PageNo()))
		}
		if u.tick {
			done++
			if m.ticks > 0 && (done%20 == 0 || done == m.ticks) {
				report(task.Progress{Fraction: float64(done) / float64(m.ticks)})
			}
		}
	}
	if err := pdf.Error(); err != nil {
		return apperr.Wrap(apperr.Internal, "生成 PDF 失败", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	f, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return writeErr(err)
	}
	ew := &errWriter{w: f}
	bw := bufio.NewWriterSize(ew, 1<<20)
	perr := pdf.Output(bw)
	if ferr := bw.Flush(); perr == nil {
		perr = ferr
	}
	cerr := f.Close()
	switch {
	case ew.err != nil:
		return writeErr(ew.err)
	case perr != nil:
		return apperr.Wrap(apperr.Internal, "生成 PDF 失败", perr)
	case cerr != nil:
		return writeErr(cerr)
	}
	report(task.Progress{Fraction: 1})
	return nil
}

func sampleCodepoints(s []rune) string {
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	parts := make([]string, len(s))
	for i, r := range s {
		parts[i] = fmt.Sprintf("U+%04X", r)
	}
	return strings.Join(parts, ",")
}

// writeWrapped 用 wrapText 折行后逐行输出（替代 fpdf.MultiCell，见 wrap.go 的说明）。
// 没有 Unicode 字体时（helvetica + cp1252 转码，字符串是单字节编码，不能按 rune 处理）仍用 MultiCell：
// 单字节模式下它只在空格处断，不会丢字。
func writeWrapped(ctx context.Context, pdf *fpdf.Fpdf, text string, lineH float64, utf8Font bool) error {
	if !utf8Font {
		pdf.MultiCell(0, lineH, text, "", "L", false)
		return ctx.Err()
	}
	pw, _ := pdf.GetPageSize()
	l, _, r, _ := pdf.GetMargins()
	maxW := pw - l - r - 2*pdf.GetCellMargin()
	lines, err := wrapText(ctx, text, maxW, pdf.GetStringWidth)
	if err != nil {
		return err
	}
	for i, line := range lines {
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		pdf.CellFormat(0, lineH, line, "", 2, "L", false, 0, "")
		if pdf.PageNo() > MaxPages { // 一个超长段落也不能排出无限页
			return nil // 由调用方的页数检查报 UNSUPPORTED
		}
	}
	return nil
}
