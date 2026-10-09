package catagent

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 源码检查：本包非测试代码里，exec.Command / exec.CommandContext 的调用和 cfg.Exec 的调用
// 只允许出现在 runCmd 里（它统一做 proc.Configure：Windows 隐藏窗口）。
// 把 exec.CommandContext 当函数值传来传去（绕过 helper）也算违规。
func TestAllCommandsGoThroughRunCmd(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var bad []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			allowed := ok && fn.Name.Name == "runCmd"
			ast.Inspect(decl, func(n ast.Node) bool {
				if allowed {
					return false
				}
				switch v := n.(type) {
				case *ast.SelectorExpr:
					// exec.Command / exec.CommandContext：调用或当函数值引用都不行。
					if x, _ := v.X.(*ast.Ident); x != nil && x.Name == "exec" && (v.Sel.Name == "Command" || v.Sel.Name == "CommandContext") {
						bad = append(bad, fset.Position(v.Pos()).String())
					}
				case *ast.CallExpr:
					// a.cfg.Exec(...) 调用（判空比较可以）。
					if sel, ok := v.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Exec" && isCfgSelector(sel.X) {
						bad = append(bad, fset.Position(v.Pos()).String())
					}
				}
				return true
			})
		}
	}
	if len(bad) > 0 {
		t.Fatalf("catagent 里有不经 runCmd 构造的子进程（Windows 会弹黑窗口）: %v", bad)
	}
}

func isCfgSelector(e ast.Expr) bool {
	s, ok := e.(*ast.SelectorExpr)
	return ok && s.Sel.Name == "cfg"
}

// runCmd 必须调 proc.Configure 并设置进程树取消（各平台都有 SysProcAttr）。
func TestRunCmdConfiguresProcess(t *testing.T) {
	a := NewBuildAdapter(BuildConfig{DataTemp: t.TempDir()})
	cmd := a.runCmd(context.Background(), "grok", "-v")
	if cmd.SysProcAttr == nil {
		t.Fatal("runCmd 必须调用 proc.Configure（SysProcAttr 为空）")
	}
	if cmd.Cancel == nil {
		t.Fatal("runCmd 必须设置 Cancel 结束进程树")
	}
}
