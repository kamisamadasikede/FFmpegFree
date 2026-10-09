package doc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	xhtml "golang.org/x/net/html"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doccomp"
	"FFmpegFree/internal/doceng"
	"FFmpegFree/internal/task"
)

// ---------- PDF 源的运行（契约 v0.28，6.12.61 / 6.12.62） ----------

// pdfComponentAvailable：现在有可用的文档组件能导入 PDF（应用下载的或系统 LibreOffice）。
func (s *Service) pdfComponentAvailable(ctx context.Context) bool {
	if s.cfg.Engines != nil {
		ids := doceng.EnginesForTarget(doceng.PrefOrder(doceng.IDComponent), s.cfg.Engines.Engines(ctx), s.cfg.Engines.Skips(), "pdf", "txt")
		for _, id := range ids {
			if id == doceng.IDComponent {
				return true
			}
		}
		return false
	}
	return s.componentStatus(ctx, 0).State == doccomp.StateReady && s.cfg.Component != nil && s.cfg.Component.ExePath() != ""
}

func (r *docRunner) producePDF(ctx context.Context, dst string, logw io.Writer) error {
	j := r.j
	if pdfLayoutTarget(j.target) {
		// Word（≥ 2013）→ 文档组件；不用 WPS（能力表里 wps 没有 pdf）。
		return r.runEngines(ctx, dst, logw)
	}
	if j.target == "html" && j.engine() == engineComponent && r.s.pdfComponentAvailable(ctx) {
		err := r.runEngines(ctx, dst, logw)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil || apperr.Is(err, apperr.DocEncrypted) || apperr.Is(err, apperr.DocCorrupt) || apperr.Is(err, apperr.ConvertDiskFull) {
			return err
		}
		// 组件导出失败：退回纯 Go 简易网页（result 带 simple_fallback）。
		fmt.Fprintf(logw, "[FFmpegFree] 文档组件导出网页失败，改用简易网页：%s\n", apperr.From(err).Code)
	}

	pages, err := extractPDFText(ctx, j.in)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var parseWhy, quality string
	if err != nil {
		var pe *errPDFParse
		if !errors.As(err, &pe) {
			return err // 加密、读文件失败等
		}
		parseWhy = pe.why
	} else {
		quality = pdfQuality(pagesPlain(pages))
	}
	if parseWhy == "" && quality == "" {
		r.usedEngine = engineGo
		var out []byte
		switch j.target {
		case "txt":
			out = renderPDFTxt(pages, txtNewline())
		case "md":
			out = renderPDFMarkdown(pages)
		default: // html
			out = renderPDFSimpleHTML(pages, strings.TrimSuffix(j.name, filepath.Ext(j.name)))
		}
		return writeOut(dst, out)
	}

	why := quality
	if why == "" {
		why = "parse"
		fmt.Fprintf(logw, "PDF 解析失败：%s\n", shortReason(parseWhy))
	}
	if r.s.pdfComponentAvailable(ctx) {
		fmt.Fprintf(logw, "[FFmpegFree] PDF 文字提取失败（%s），改用文档组件\n", why)
		release, err := task.AcquireDocSlot(ctx) // 插到文档组件池最前面，不另起任务
		if err != nil {
			return err
		}
		defer release()
		if j.target == "txt" {
			// 组件的 writer_pdf_import 文档直接导出 txt 拿不到文本框里的字：先导出 html，再取文字。
			tmp := dst + ".html"
			defer os.Remove(tmp)
			if err := r.runEnginesTo(ctx, "html", tmp, logw); err != nil {
				return err
			}
			h, err := os.ReadFile(tmp)
			if err != nil {
				return apperr.Wrap(apperr.IOError, "读取临时文件失败", err)
			}
			if err := writeOut(dst, htmlToPlainText(h, txtNewline())); err != nil {
				return err
			}
		} else if err := r.runEngines(ctx, dst, logw); err != nil {
			return err
		}
		if j.target == "txt" || j.target == "md" {
			if err := ensureNoBOM(dst); err != nil {
				return err
			}
			b, err := os.ReadFile(dst)
			if err != nil {
				return apperr.Wrap(apperr.IOError, "读取文件失败", err)
			}
			if strings.TrimFunc(string(b), unicode.IsSpace) == "" {
				return errPDFNoText(qualityEmpty, "engine=component")
			}
		}
		return nil
	}
	if parseWhy != "" {
		return pdfParseErr(parseWhy)
	}
	return errPDFNoText(quality)
}

// pdfEngineNow 是重试 / 重转时按现在的引擎重新定的 PDF 源 params.engine。
func (s *Service) pdfEngineNow(ctx context.Context, target string) string {
	if target == "html" && s.pdfComponentAvailable(ctx) {
		return engineComponent
	}
	return pdfJobEngine(target, nil)
}

// htmlToPlainText 取网页里的文字（组件导出的 html → txt）：跳过 head / script / style；块级元素和 <br> 换行，
// 连续空行合成一个；UTF-8 不带 BOM。
func htmlToPlainText(h []byte, nl string) []byte {
	doc, err := xhtml.Parse(bytes.NewReader(h))
	if err != nil {
		return nil
	}
	var b strings.Builder
	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			switch n.Data {
			case "head", "script", "style", "title":
				return
			case "br":
				b.WriteString("\n")
				return
			}
		}
		if n.Type == xhtml.TextNode {
			b.WriteString(strings.Join(strings.Fields(n.Data), " "))
			if strings.TrimSpace(n.Data) != "" && n.Data != strings.TrimRight(n.Data, " \t\r\n") {
				b.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == xhtml.ElementNode {
			switch n.Data {
			case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr", "table", "ul", "ol", "blockquote", "pre", "section", "article", "hr":
				b.WriteString("\n\n")
			case "td", "th":
				b.WriteByte('\t')
			}
		}
	}
	walk(doc)
	var lines []string
	blank := true
	for _, l := range strings.Split(b.String(), "\n") {
		l = strings.TrimRight(strings.TrimLeft(l, " "), " \t")
		if l == "" {
			if !blank {
				lines = append(lines, "")
			}
			blank = true
			continue
		}
		lines = append(lines, l)
		blank = false
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, nl) + nl)
}
