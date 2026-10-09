package apperr

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 契约 1.1：面向用户的文字不出现 "ffmpeg" / "ffprobe"，统一叫“转换组件”。
// 这里扫描仓库里的非测试 Go 源码，检查三类面向用户的字面量：
//   - apperr.New / apperr.Wrap / projectErr 的 message 参数（AppError.message）；
//   - 结构体字面量里的 Title 字段（任务标题）；
//   - 名字里带 reason 的常量（显卡编码等状态说明）。
//
// WithDetail、日志、标识符不在检查范围内（契约允许保留 ffmpeg）。
func TestUserFacingTextHasNoFFmpeg(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	var bad []string
	check := func(pos token.Pos, e ast.Expr) {
		ast.Inspect(e, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			low := strings.ToLower(s)
			if strings.Contains(low, "ffmpeg") && !strings.Contains(low, "ffmpegfree") || strings.Contains(low, "ffprobe") {
				bad = append(bad, fset.Position(lit.Pos()).String()+": "+s)
			}
			return true
		})
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "frontend", "build", ".git", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				name := ""
				switch fn := x.Fun.(type) {
				case *ast.SelectorExpr:
					if id, ok := fn.X.(*ast.Ident); ok && id.Name == "apperr" {
						name = fn.Sel.Name
					}
				case *ast.Ident:
					if fn.Name == "projectErr" || fn.Name == "New" || fn.Name == "Wrap" {
						name = fn.Name
					}
				}
				if name == "New" || name == "Wrap" || name == "projectErr" {
					if len(x.Args) >= 2 {
						check(x.Pos(), x.Args[1])
					}
				}
			case *ast.KeyValueExpr:
				if k, ok := x.Key.(*ast.Ident); ok && k.Name == "Title" {
					check(x.Pos(), x.Value)
				}
			case *ast.ValueSpec:
				for i, id := range x.Names {
					if strings.Contains(strings.ToLower(id.Name), "reason") && i < len(x.Values) {
						if lit, ok := x.Values[i].(*ast.BasicLit); ok {
							// 只看中文说明，跳过 "encoder_unavailable" 这类稳定枚举
							if s, _ := strconv.Unquote(lit.Value); strings.ContainsFunc(s, func(r rune) bool { return r >= 0x4e00 && r <= 0x9fff }) {
								check(lit.Pos(), lit)
							}
						}
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Fatalf("面向用户的文字里出现了 ffmpeg / ffprobe（契约 1.1，应改称“转换组件”）：\n%s", strings.Join(bad, "\n"))
	}
}

// 契约 v0.25.3（架构师定）：错误码和 reason 只给程序用，界面上永远不显示；message 是给用户看的中文。
// 这里检查 apperr.New / Wrap 的 message 字面量：不含错误码（INTERNAL 等）、不夹英文单词（参数名、英文原句），
// 格式化的参数不是 %v（%v 常常是英文的错误原文，应放进 detail）。
func TestUserFacingMessageIsChinese(t *testing.T) {
	codes := []string{}
	for _, c := range allCodesForTest {
		codes = append(codes, string(c))
	}
	allowed := map[string]bool{ // 产品名、格式名、平台名：界面上本来就这样写
		"FFmpegFree": true, "PDF": true, "JSON": true, "Windows": true, "macOS": true, "X11": true, "Office": true, "ID": true,
		"Word": true, "ODT": true, "TXT": true, "CSV": true, "Markdown": true, "MD": true, "HTML": true, "DOC": true, "DOCX": true,
		"XLS": true, "XLSX": true, "PPT": true, "PPTX": true, "ODS": true, "ODP": true, "RTF": true, "GB": true, "MiB": true,
	}
	linuxLOHints := map[string]bool{ // 契约 6.12.24（v0.27）：只有 Linux 的这两句可以出现 LibreOffice
		"请先在系统里安装 LibreOffice，然后重启应用。":                 true,
		"系统里的 LibreOffice 版本太旧，请升级到 7.2 或更高版本，然后重启应用。": true,
	}
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	var bad []string
	check := func(e ast.Expr) {
		ast.Inspect(e, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			pos := fset.Position(lit.Pos()).String()
			if linuxLOHints[s] {
				return true
			}
			if strings.Contains(strings.ToLower(s), "libreoffice") {
				bad = append(bad, pos+": 含 LibreOffice（只允许 Linux 的两句提示）: "+s)
			}
			for _, c := range codes {
				if strings.Contains(s, c) {
					bad = append(bad, pos+": 含错误码 "+c+": "+s)
				}
			}
			if strings.Contains(s, "剪辑") { // 契约 v0.23.5 / v0.25.3：剪辑功能已移除，旧记录叫“旧版导出”
				bad = append(bad, pos+": 出现了“剪辑”: "+s)
			}
			if strings.Contains(s, "%v") {
				bad = append(bad, pos+": message 里用了 %v（英文原文应放进 detail）: "+s)
			}
			for _, w := range strings.FieldsFunc(s, func(r rune) bool {
				return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
			}) {
				letters := strings.IndexFunc(w, func(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }) >= 0
				if letters && len(w) >= 2 && !allowed[w] && !strings.HasPrefix(w, "0") {
					bad = append(bad, pos+": 夹了英文 "+strconv.Quote(w)+": "+s)
					break
				}
			}
			return true
		})
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "frontend", "build", ".git", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			x, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if fn, ok := x.Fun.(*ast.SelectorExpr); ok {
				if id, ok := fn.X.(*ast.Ident); ok && id.Name == "apperr" && (fn.Sel.Name == "New" || fn.Sel.Name == "Wrap") && len(x.Args) >= 2 {
					check(x.Args[1])
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Fatalf("面向用户的 message 应是中文，不含错误码和英文（契约 v0.25.3）：\n%s", strings.Join(bad, "\n"))
	}
}

var allCodesForTest = []Code{InvalidArgument, NotFound, FFmpegNotFound, TaskConflict, IOError, ProcessFailed, UnsupportedPlatform, Internal,
	ProbeFailed, ConvertDiskFull, Canceled, Unsupported, LiveURLInvalid, LiveConnectFailed, LivePushRejected, LivePushInterrupted,
	ScreenPermissionDenied, LiveSourceGone,
	DocEncrypted, DocCorrupt, DocTimeout, DocComponentCrashed, DocComponentNotReady, DocDownloadFailed, DocChecksumFailed,
	DocComponentInstallFailed, DocFormatUnsupported, DocPDFInputUnsupported,
	DocPresentationBusy, DocEngineBusy}
