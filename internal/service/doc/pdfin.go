package doc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/ledongthuc/pdf"

	"FFmpegFree/internal/apperr"
)

// ---------- PDF 作为输入（契约 v0.28，6.12.58~6.12.64） ----------

const (
	// MaxPDFInputBytes：PDF 源单独的大小上限 200 MiB（其他文档仍是 MaxInputBytes）。
	MaxPDFInputBytes int64 = 200 << 20
	// MaxPDFInputPages：页数上限 500。
	MaxPDFInputPages = 500

	pdfOpenTimeout    = 5 * time.Second // 添加 / 提交 / 运行前读页数和加密（和其他轻量检查同一个超时）
	pdfExtractTimeout = 2 * time.Minute // 纯 Go 提取单次超时（6.12.61）
	pdfHeaderScan     = 1024
	pdfGarbledPercent = 30

	// WarningSimpleFallback：简易转换（只保留文字）的结果警告（复用 v0.27）。
	WarningSimpleFallback = "simple_fallback"

	qualityEmpty   = "empty"
	qualityGarbled = "garbled"
)

func errPDFTooLarge() *apperr.AppError {
	return reasonErr(apperr.InvalidArgument, "PDF 太大了，最多支持 200 MB。", reasonTooLarge, "")
}

func errPDFTooManyPages(n int) *apperr.AppError {
	return reasonErr(apperr.InvalidArgument, "PDF 页数太多，最多支持 500 页。", reasonTooManyPages, fmt.Sprintf("pages=%d", n))
}

// errPDFNoText：detail 第一行 reason=no_text，第二行 quality=empty|garbled，之后可有 engine=component（6.12.63）。
func errPDFNoText(quality string, extra ...string) *apperr.AppError {
	d := "reason=no_text\nquality=" + quality
	for _, e := range extra {
		d += "\n" + e
	}
	return apperr.New(apperr.DocPDFNoText, "这个 PDF 里没有能提取的文字，可能是扫描件。").WithDetail(d)
}

// pdfParseErr：纯 Go 解析失败（报错 / panic / 超时）且没有组件时（6.12.62）。
func pdfParseErr(why string) *apperr.AppError {
	return apperr.New(apperr.DocCorrupt, "文件打不开，可能已损坏或不是有效的文档。").WithDetail("engine=go\nparse=" + shortReason(why))
}

func shortReason(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if r := []rune(s); len(r) > 120 {
		s = string(r[:120])
	}
	return s
}

// errPDFParse 是纯 Go 库解析不了（不算质量问题）。
type errPDFParse struct{ why string }

func (e *errPDFParse) Error() string { return "pdf parse: " + e.why }

// pdfEncryptRe 匹配 trailer / 交叉引用流字典里的 /Encrypt 引用或内联字典。
var pdfEncryptRe = regexp.MustCompile(`/Encrypt\s*(?:\d+\s+\d+\s+R|<<)`)

// rawHasEncrypt 在文件头尾找 /Encrypt（库解析不了时的保守判断；线性化 PDF 的第一个 trailer 在开头）。
func rawHasEncrypt(f io.ReaderAt, size int64) bool {
	const head, tail = 64 << 10, 256 << 10
	read := func(off, n int64) []byte {
		if off < 0 {
			n += off
			off = 0
		}
		if off+n > size {
			n = size - off
		}
		if n <= 0 {
			return nil
		}
		b := make([]byte, n)
		k, _ := f.ReadAt(b, off)
		return b[:k]
	}
	return pdfEncryptRe.Match(read(0, head)) || pdfEncryptRe.Match(read(size-tail, tail))
}

// openPDFLib 用纯 Go 库打开（recover 库的 panic）。返回的 Reader 只在 f 打开期间可用。
func openPDFLib(f io.ReaderAt, size int64) (r *pdf.Reader, err error) {
	defer func() {
		if x := recover(); x != nil {
			r, err = nil, fmt.Errorf("panic: %v", x)
		}
	}()
	return pdf.NewReader(f, size)
}

// pdfNumPage 读页数（recover）；读不出返回 -1。
func pdfNumPage(r *pdf.Reader) (n int) {
	defer func() {
		if recover() != nil {
			n = -1
		}
	}()
	n = r.NumPage()
	if n <= 0 {
		return -1
	}
	return n
}

// pdfOpenResult 是一次“打开 + 加密 + 页数”检查的结果。
type pdfOpenResult struct {
	pages int   // -1 = 读不出
	err   error // DOC_ENCRYPTED 等；nil = 可以继续（含读不出页数）
	parse string
}

// probePDF 检查加密和页数（6.12.60）：
//   - 没有 /Encrypt，或空用户密码能打开（只有所有者密码）→ 不算加密；
//   - 要用户密码 → DOC_ENCRYPTED；库不支持这种加密但空用户密码能通过 → DOC_ENCRYPTED reason=owner_only；
//   - 库解析不了（没有 /Encrypt）→ 不拒绝，pages=-1。
func probePDF(f io.ReaderAt, size int64) pdfOpenResult {
	r, err := openPDFLib(f, size)
	if err != nil {
		if errors.Is(err, pdf.ErrInvalidPassword) {
			return pdfOpenResult{pages: -1, err: errEncrypted()} // 库认得这种加密，空密码打不开
		}
		if strings.Contains(err.Error(), "encrypt") || rawHasEncrypt(f, size) {
			// 库不支持的加密（如 AES-256）：空用户密码能打开 → owner_only，否则要密码
			return pdfOpenResult{pages: -1, err: pdfEncryptErr(f, size)}
		}
		return pdfOpenResult{pages: -1, parse: err.Error()}
	}
	return pdfOpenResult{pages: pdfNumPage(r)}
}

// inspectPDF 是 PDF 源的添加 / 提交 / 运行前检查（6.12.60）：空 → DOC_CORRUPT；开头 1024 字节里没有 %PDF- → DOC_CORRUPT；
// > 200 MiB → too_large；加密 → DOC_ENCRYPTED；> 500 页 → too_many_pages。读不出页数不拒绝。
// 返回读到的页数（-1 = 读不出）。
func inspectPDF(ctx context.Context, path string) (int, error) {
	f, fi, err := openRegular(path)
	if err != nil {
		return -1, err
	}
	defer f.Close()
	size := fi.Size()
	if size == 0 {
		return -1, errCorrupt("empty")
	}
	if size > MaxPDFInputBytes {
		return -1, errPDFTooLarge()
	}
	head := make([]byte, pdfHeaderScan)
	n, _ := io.ReadFull(f, head)
	if !bytes.Contains(head[:n], []byte("%PDF-")) {
		return -1, errCorrupt("pdf_header")
	}
	ch := make(chan pdfOpenResult, 1)
	go func() { ch <- probePDF(f, size) }()
	t := time.NewTimer(pdfOpenTimeout)
	defer t.Stop()
	var res pdfOpenResult
	select {
	case res = <-ch:
	case <-t.C:
		// 库太慢：页数按读不出处理（交给引擎，引擎的单次超时兜底）；加密在库外再保守查一次。
		// 返回后 f 被关闭，库那边的读取会出错结束（panic 已 recover），结果丢进有缓冲的 ch。
		if rawHasEncrypt(f, size) {
			return -1, pdfEncryptErr(f, size)
		}
		return -1, nil
	case <-ctx.Done():
		return -1, nil
	}
	if res.err != nil {
		return -1, res.err
	}
	if res.pages > MaxPDFInputPages {
		return res.pages, errPDFTooManyPages(res.pages)
	}
	return res.pages, nil
}

// ---------- 纯 Go 提取文字（6.12.62） ----------

// pdfPage 是一页：段落（每段若干行）。
type pdfPage struct {
	paras [][]string
}

// extractPDFText 逐页取文字（最多 500 页），单次最多 2 分钟；超时、报错、panic 都返回 *errPDFParse。
func extractPDFText(ctx context.Context, path string) ([]pdfPage, error) {
	return extractPDFTextTimeout(ctx, path, pdfExtractTimeout)
}

func extractPDFTextTimeout(ctx context.Context, path string, timeout time.Duration) ([]pdfPage, error) {
	f, fi, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	type res struct {
		pages []pdfPage
		err   error
	}
	stop := make(chan struct{})
	ch := make(chan res, 1)
	go func() {
		defer f.Close()
		p, err := extractPDFFrom(f, fi.Size(), stop)
		ch <- res{p, err}
	}()
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case r := <-ch:
		return r.pages, r.err
	case <-t.C:
		close(stop)
		return nil, &errPDFParse{why: "timeout"}
	case <-ctx.Done():
		close(stop)
		return nil, ctx.Err()
	}
}

// pdfLibHook 只给测试用：在每页提取前调用（可以 panic 模拟库崩溃）。
// 用原子指针：超时后提取 goroutine 仍在跑，测试此时清空钩子，普通变量会形成 data race。
var pdfLibHook atomic.Pointer[func(page int)]

func extractPDFFrom(f io.ReaderAt, size int64, stop <-chan struct{}) (pages []pdfPage, err error) {
	defer func() {
		if x := recover(); x != nil {
			pages, err = nil, &errPDFParse{why: fmt.Sprintf("panic: %v", x)}
		}
	}()
	r, err := openPDFLib(f, size)
	if err != nil {
		if errors.Is(err, pdf.ErrInvalidPassword) {
			return nil, errEncrypted()
		}
		if strings.Contains(err.Error(), "encrypt") {
			return nil, pdfEncryptErr(f, size)
		}
		return nil, &errPDFParse{why: err.Error()}
	}
	n := pdfNumPage(r)
	if n < 0 {
		return nil, &errPDFParse{why: "page_count"}
	}
	if n > MaxPDFInputPages {
		n = MaxPDFInputPages
	}
	for i := 1; i <= n; i++ {
		select {
		case <-stop:
			return nil, &errPDFParse{why: "timeout"}
		default:
		}
		if h := pdfLibHook.Load(); h != nil {
			(*h)(i)
		}
		p := r.Page(i)
		if p.V.IsNull() {
			pages = append(pages, pdfPage{})
			continue
		}
		pages = append(pages, pageText(p))
	}
	return pages, nil
}

// pdfGlyph 是库给的一段文字（通常一个字）和它的位置。
type pdfGlyph struct {
	x, y, w, size float64
	s             string
}

// pageText 取一页文字：先按库给的字形位置排成行（Y 相近算同一行，行内按 X），取不到字形时退回库的纯文本（按库给的行）。
func pageText(p pdf.Page) pdfPage {
	var gs []pdfGlyph
	for _, t := range p.Content().Text {
		if t.S == "" {
			continue
		}
		gs = append(gs, pdfGlyph{x: t.X, y: t.Y, w: t.W, size: t.FontSize, s: t.S})
	}
	if len(gs) == 0 {
		s, err := p.GetPlainText(nil)
		if err != nil {
			panic(err) // 外层 recover → 解析失败
		}
		var para []string
		for _, l := range strings.Split(s, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				para = append(para, l)
			}
		}
		if len(para) == 0 {
			return pdfPage{}
		}
		return pdfPage{paras: [][]string{para}}
	}
	return glyphsToPage(gs)
}

// glyphsToPage：Y 差小于半个字号算同一行；行内相邻两字的空隙大于 0.3 个字号时补一个空格；
// 行距大于 1.6 个字号（大约空了一行）时分段。
func glyphsToPage(gs []pdfGlyph) pdfPage {
	type line struct {
		y, size float64
		gs      []pdfGlyph
	}
	sort.SliceStable(gs, func(i, j int) bool { return gs[i].y > gs[j].y })
	var lines []*line
	for _, g := range gs {
		size := g.size
		if size <= 0 {
			size = 10
		}
		if n := len(lines); n > 0 {
			l := lines[n-1]
			if d := l.y - g.y; d < size*0.5 && d > -size*0.5 {
				l.gs = append(l.gs, g)
				if size > l.size {
					l.size = size
				}
				continue
			}
		}
		lines = append(lines, &line{y: g.y, size: size, gs: []pdfGlyph{g}})
	}
	var pg pdfPage
	var cur []string
	prevY, prevSize := 0.0, 0.0
	for i, l := range lines {
		sort.SliceStable(l.gs, func(a, b int) bool { return l.gs[a].x < l.gs[b].x })
		var b strings.Builder
		end := 0.0
		for k, g := range l.gs {
			if k > 0 && g.x-end > l.size*0.3 {
				s := b.String()
				if !strings.HasSuffix(s, " ") && !strings.HasPrefix(g.s, " ") {
					b.WriteByte(' ')
				}
			}
			b.WriteString(g.s)
			end = g.x + g.w
		}
		text := strings.TrimFunc(b.String(), unicode.IsSpace)
		if text == "" {
			continue
		}
		if i > 0 && len(cur) > 0 && prevY-l.y > 1.6*maxf(prevSize, l.size) {
			pg.paras = append(pg.paras, cur)
			cur = nil
		}
		cur = append(cur, text)
		prevY, prevSize = l.y, l.size
	}
	if len(cur) > 0 {
		pg.paras = append(pg.paras, cur)
	}
	return pg
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// pdfQuality 质量判定（6.12.62）：返回 "" 表示通过，否则 empty / garbled。
func pdfQuality(s string) string {
	total, bad := 0, 0
	for _, r := range s {
		if unicode.Is(unicode.White_Space, r) {
			continue
		}
		total++
		if r == unicode.ReplacementChar || isPrivateUse(r) || unicode.Is(unicode.Cc, r) {
			bad++
		}
	}
	if total == 0 {
		return qualityEmpty
	}
	if bad*100 >= total*pdfGarbledPercent {
		return qualityGarbled
	}
	return ""
}

func isPrivateUse(r rune) bool {
	return r >= 0xE000 && r <= 0xF8FF || r >= 0xF0000 && r <= 0xFFFFD || r >= 0x100000 && r <= 0x10FFFD
}

// pagesPlain 是用于质量判定的全部文字。
func pagesPlain(pages []pdfPage) string {
	var b strings.Builder
	for _, p := range pages {
		for _, para := range p.paras {
			for _, l := range para {
				b.WriteString(l)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

// txtNewline：Windows 写 \r\n，macOS / Linux 写 \n（6.12.62）。
func txtNewline() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

// renderPDFTxt：页内按行；段与段、页与页之间空一行。UTF-8 不带 BOM。
func renderPDFTxt(pages []pdfPage, nl string) []byte {
	var blocks []string
	for _, p := range pages {
		for _, para := range p.paras {
			blocks = append(blocks, strings.Join(para, nl))
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	return []byte(strings.Join(blocks, nl+nl) + nl)
}

// joinLines 把一段里的行合成一行：两边都是 ASCII 字母数字时补一个空格（英文断行），中文直接相连。
func joinLines(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			prev := b.String()
			last, _ := lastRune(prev)
			first := []rune(l)[0]
			if last < 0x80 && first < 0x80 && !strings.HasSuffix(prev, "-") {
				b.WriteByte(' ')
			}
		}
		b.WriteString(l)
	}
	return b.String()
}

func lastRune(s string) (rune, bool) {
	r := []rune(s)
	if len(r) == 0 {
		return 0, false
	}
	return r[len(r)-1], true
}

var mdLineStart = regexp.MustCompile(`^(\s*)(?:([#>\-+|])|(\d+)\.)`)

// mdEscape：行内 \ ` * _ [ ] < 加反斜杠；行首 # > - + * | 和“数字.”也转义（6.12.62）。
func mdEscape(s string) string {
	var b strings.Builder
	if m := mdLineStart.FindStringSubmatchIndex(s); m != nil {
		b.WriteString(s[:m[3]])
		if m[4] >= 0 { // # > - + |
			b.WriteByte('\\')
			b.WriteString(s[m[4]:m[5]])
		} else { // 数字.
			b.WriteString(s[m[6]:m[7]])
			b.WriteString("\\.")
		}
		s = s[m[1]:]
	}
	for _, r := range s {
		switch r {
		case '\\', '`', '*', '_', '[', ']', '<':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// renderPDFMarkdown：每段一段，段与段、页与页之间空一行；换行 \n，不带 BOM。
func renderPDFMarkdown(pages []pdfPage) []byte {
	var blocks []string
	for _, p := range pages {
		for _, para := range p.paras {
			blocks = append(blocks, mdEscape(joinLines(para)))
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	return []byte(strings.Join(blocks, "\n\n") + "\n")
}

// renderPDFSimpleHTML：简易网页（6.12.62），每段一个 <p>，页与页之间 <hr>。
func renderPDFSimpleHTML(pages []pdfPage, title string) []byte {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString("</title></head><body>\n")
	first := true
	for _, p := range pages {
		if len(p.paras) == 0 {
			continue
		}
		if !first {
			b.WriteString("<hr>\n")
		}
		first = false
		for _, para := range p.paras {
			b.WriteString("<p>")
			b.WriteString(html.EscapeString(joinLines(para)))
			b.WriteString("</p>\n")
		}
	}
	b.WriteString("</body></html>\n")
	return []byte(b.String())
}

// ensureNoBOM 去掉 UTF-8 BOM（组件导出的 txt / md 也一样，6.12.62）。
func ensureNoBOM(p string) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return apperr.Wrap(apperr.IOError, "读取文件失败", err)
	}
	if bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
		return writeOut(p, b[3:])
	}
	return nil
}
