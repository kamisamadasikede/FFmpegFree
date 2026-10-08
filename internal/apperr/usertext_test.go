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
