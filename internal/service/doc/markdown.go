package doc

import (
	"bytes"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	htmltomd "github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// ---------- Markdown（纯 Go，契约 6.12.11） ----------

// imageMode 决定 md 里图片的处理。
type imageMode int

const (
	// imagesForHTML：md → html 的最终输出。本地图片原样（相对路径相对输出文件仍可能有效），网络图片原样保留（只是文本，不下载）。
	imagesForHTML imageMode = iota
	// imagesForComponent：md → 其他格式的临时 HTML。本地图片转成 file:/// 绝对 URL，不存在的换成 alt；网络图片换成 alt（没有就去掉），组件不发网络请求。
	imagesForComponent
)

func isRemote(src string) bool {
	s := strings.ToLower(strings.TrimSpace(src))
	return strings.HasPrefix(s, "http:") || strings.HasPrefix(s, "https:") || strings.HasPrefix(s, "//")
}

func fileURL(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// imageRewriter 在 AST 上改写图片节点。
type imageRewriter struct {
	mode    imageMode
	baseDir string // md 文件所在目录（原文件所在目录；原文件不在时用副本目录）
	// local 收集本地图片：goldmark 不开 unsafe 时会把 file: 地址当危险地址清空，
	// 所以先写占位符 ffimg-<n>，渲染完再换成 file:/// URL（只有我们自己解析出的、确实存在的文件）。
	local *[]string
}

const imgPlaceholder = "ffmpegfree-img-"

func (r imageRewriter) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	if r.mode != imagesForComponent {
		return
	}
	src := reader.Source()
	var imgs []*ast.Image
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if img, ok := n.(*ast.Image); ok && entering {
			imgs = append(imgs, img)
		}
		return ast.WalkContinue, nil
	})
	for _, img := range imgs {
		dest := string(img.Destination)
		keep := false
		if !isRemote(dest) && !strings.HasPrefix(strings.ToLower(dest), "data:") {
			p := dest
			if u, err := url.PathUnescape(p); err == nil {
				p = u
			}
			if strings.HasPrefix(strings.ToLower(p), "file://") {
				if u, err := url.Parse(p); err == nil {
					p = u.Path
				}
			}
			if !filepath.IsAbs(p) {
				p = filepath.Join(r.baseDir, filepath.FromSlash(p))
			}
			if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
				*r.local = append(*r.local, fileURL(p))
				img.Destination = []byte(imgPlaceholder + strconv.Itoa(len(*r.local)-1))
				keep = true
			}
		}
		if keep {
			continue
		}
		// 换成 alt 文字（没有 alt 就去掉）
		alt := string(img.Text(src)) //nolint:staticcheck // goldmark 1.x 仍提供
		parent := img.Parent()
		if parent == nil {
			continue
		}
		if alt != "" {
			t := ast.NewString([]byte(alt))
			parent.InsertBefore(parent, img, t)
		}
		parent.RemoveChild(parent, img)
	}
}

// markdownToHTML 把 md（已是 UTF-8）转成完整的 HTML 文档。不开 WithUnsafe：md 里的原始 HTML 被丢弃。
func markdownToHTML(md []byte, title string, mode imageMode, baseDir string) ([]byte, error) {
	var local []string
	gm := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithASTTransformers(util.Prioritized(imageRewriter{mode: mode, baseDir: baseDir, local: &local}, 100))),
	)
	var body bytes.Buffer
	if err := gm.Convert(md, &body); err != nil {
		return nil, err
	}
	b := body.Bytes()
	for i := len(local) - 1; i >= 0; i-- { // 倒序：ffmpegfree-img-1 不会误换 ffmpegfree-img-10 的前缀
		b = bytes.ReplaceAll(b, []byte(`src="`+imgPlaceholder+strconv.Itoa(i)+`"`), []byte(`src="`+html.EscapeString(local[i])+`"`))
	}
	body.Reset()
	body.Write(b)
	var out bytes.Buffer
	out.WriteString("<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><title>")
	out.WriteString(html.EscapeString(title))
	out.WriteString("</title></head><body>\n")
	out.Write(body.Bytes())
	out.WriteString("</body></html>\n")
	return out.Bytes(), nil
}

// htmlToMarkdown 把 HTML 转成 md：UTF-8、\n 换行、无 BOM；<script> / <style> 丢弃；加表格插件。
func htmlToMarkdown(h []byte) ([]byte, error) {
	conv := htmltomd.NewConverter(htmltomd.WithPlugins(base.NewBasePlugin(), commonmark.NewCommonmarkPlugin(), table.NewTablePlugin()))
	md, err := conv.ConvertReader(bytes.NewReader(h))
	if err != nil {
		return nil, err
	}
	md = bytes.ReplaceAll(md, []byte("\r\n"), []byte("\n"))
	md = bytes.TrimPrefix(md, []byte{0xEF, 0xBB, 0xBF})
	if len(md) > 0 && md[len(md)-1] != '\n' {
		md = append(md, '\n')
	}
	return md, nil
}
