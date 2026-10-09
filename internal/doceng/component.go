package doceng

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doccomp"
)

// ComponentConverter 用 LibreOffice（文档组件）做转换 / 预览 PDF。
type ComponentConverter struct {
	// ConvertToArg / InFilterFor / FamilyOf 由调用方注入，避免 doceng 依赖 service/doc。
	ConvertToArg func(family, target string) string
	InFilterFor  func(src string) string
	FamilyOf     func(ext string) string
	// Markdown pipeline（md → html 交给组件；html → md 从组件输出再转）。
	MarkdownToHTML func(md []byte, title, mode, origDir string) ([]byte, error)
	HTMLToMarkdown func(html []byte) ([]byte, error)
	ReadTextFile   func(path string) ([]byte, error)
	WriteUTF8Temp  func(src, dst string) error
}

func (c *ComponentConverter) Convert(ctx context.Context, d Detected, req ConvertRequest) error {
	if d.ComponentExe == "" {
		return apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。")
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = TimeoutComponentConvert
	}
	work := req.WorkDir
	inDir := filepath.Join(work, "in")
	if err := os.MkdirAll(inDir, 0o755); err != nil {
		return apperr.Wrap(apperr.IOError, "无法创建临时文件夹", err)
	}
	stem := "input"
	input := req.InputPath
	src := req.SrcExt
	switch src {
	case "txt", "csv":
		input = filepath.Join(inDir, stem+"."+src)
		if c.WriteUTF8Temp != nil {
			if err := c.WriteUTF8Temp(req.InputPath, input); err != nil {
				return err
			}
		}
	case "md":
		if c.ReadTextFile == nil || c.MarkdownToHTML == nil {
			return apperr.New(apperr.Internal, "文档服务尚未初始化")
		}
		md, err := c.ReadTextFile(req.InputPath)
		if err != nil {
			return err
		}
		title := strings.TrimSuffix(req.Name, filepath.Ext(req.Name))
		h, err := c.MarkdownToHTML(md, title, "component", req.OrigDir)
		if err != nil {
			return apperr.New(apperr.DocCorrupt, "文件打不开，可能已损坏或不是有效的文档。").WithDetail("markdown")
		}
		input = filepath.Join(inDir, stem+".html")
		if err := os.WriteFile(input, h, 0o644); err != nil {
			return apperr.Wrap(apperr.IOError, "无法写入临时文件", err)
		}
		src = "html"
	default:
		input = filepath.Join(inDir, stem+"."+extOf(req.InputPath))
		if err := linkOrCopy(req.InputPath, input); err != nil {
			return err
		}
	}
	fam := c.FamilyOf(req.SrcExt)
	target, outExt := req.Target, req.Target
	if req.Target == "md" {
		target, outExt = "html", "html"
	}
	job := doccomp.Job{
		Exe: d.ComponentExe, Input: input, ConvertTo: c.ConvertToArg(fam, target), InFilter: c.InFilterFor(src),
		WorkDir: filepath.Join(work, "run"), OutExt: outExt, Timeout: timeout,
	}
	if req.Logf != nil {
		req.Logf("启动文档组件：--convert-to %s\n", job.ConvertTo)
	}
	t0 := time.Now()
	out, err := doccomp.Convert(ctx, job)
	if req.Logf != nil {
		req.Logf("文档组件用时 %s\n", time.Since(t0).Round(time.Millisecond))
	}
	if err != nil {
		return err
	}
	if req.Target == "md" {
		h, err := os.ReadFile(out)
		if err != nil {
			return apperr.Wrap(apperr.IOError, "读取临时文件失败", err)
		}
		md, err := c.HTMLToMarkdown(h)
		if err != nil {
			return apperr.New(apperr.DocComponentCrashed, "文档组件意外退出，请重试。").WithDetail("exit=0\nhtml_to_md")
		}
		return writeFile(req.OutputPath, md)
	}
	return copyFile(out, req.OutputPath)
}

func (c *ComponentConverter) PreviewPDF(ctx context.Context, d Detected, req PreviewPDFRequest) error {
	return c.Convert(ctx, d, ConvertRequest{
		SrcExt: req.SrcExt, Target: "pdf", InputPath: req.InputPath, OutputPath: req.OutputPath,
		WorkDir: req.WorkDir, Timeout: req.Timeout, HasMacro: req.HasMacro, Logf: req.Logf,
	})
}

func extOf(p string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(p), "."))
}

func linkOrCopy(src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return apperr.Wrap(apperr.IOError, "读取文件失败", err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return apperr.Wrap(apperr.IOError, "无法创建输出文件夹", err)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return apperr.Wrap(apperr.IOError, "写入文件失败", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return apperr.Wrap(apperr.IOError, "写入文件失败", err)
	}
	return out.Close()
}

func writeFile(dst string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return apperr.Wrap(apperr.IOError, "无法创建输出文件夹", err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return apperr.Wrap(apperr.IOError, "写入文件失败", err)
	}
	return nil
}
