package apperr

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// 契约 2.1（v0.31）：后端一共 39 个码，和契约里 AppErrorCode 清单逐项一致。
const contractCodeCount = 39

func declaredCodes(t *testing.T) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "apperr.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, sp := range g.Specs {
			vs := sp.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "Code" {
				continue
			}
			for _, v := range vs.Values {
				if lit, ok := v.(*ast.BasicLit); ok {
					s, _ := strconv.Unquote(lit.Value)
					out = append(out, s)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func contractCodes(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "architecture", "contract.md"))
	if err != nil {
		t.Skipf("读不到契约: %v", err)
	}
	s := string(b)
	i := strings.Index(s, "export type AppErrorCode =")
	if i < 0 {
		t.Fatal("契约里找不到 AppErrorCode 清单")
	}
	s = s[i:]
	if j := strings.Index(s, "```"); j > 0 {
		s = s[:j]
	}
	var out []string
	for _, m := range regexp.MustCompile(`'([A-Z_]+)'`).FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

func TestCodeCountMatchesContract(t *testing.T) {
	decl := declaredCodes(t)
	if len(decl) != contractCodeCount {
		t.Fatalf("apperr 声明了 %d 个码，契约 2.1 是 %d 个：%v", len(decl), contractCodeCount, decl)
	}
	seen := map[string]bool{}
	for _, c := range decl {
		if seen[c] {
			t.Fatalf("重复的码 %s", c)
		}
		seen[c] = true
	}
	if len(allCodesForTest) != contractCodeCount {
		t.Fatalf("allCodesForTest 有 %d 个，应为 %d", len(allCodesForTest), contractCodeCount)
	}
	if got := contractCodes(t); strings.Join(got, ",") != strings.Join(decl, ",") {
		t.Fatalf("契约 2.1 清单与 apperr 不一致：\n契约 %v\n后端 %v", got, decl)
	}
	if CatProjectMissing != "CAT_PROJECT_MISSING" {
		t.Fatal(CatProjectMissing)
	}
}
