package cat_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

func keyFor(goos string) func(string) string {
	return func(p string) string { return paths.KeyFor(goos, p) }
}

// 两个只差大小写的真实文件夹（Linux 测试机区分大小写，两个都能建出来）。
func caseDirs(t *testing.T) (lower, upper, other string) {
	base := t.TempDir()
	lower, upper, other = filepath.Join(base, "Proj"), filepath.Join(base, "PROJ"), filepath.Join(base, "other")
	for _, d := range []string{lower, upper, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if fi, err := os.Stat(filepath.Join(base, "proj")); err == nil && fi.IsDir() {
		t.Skip("文件系统不区分大小写")
	}
	return
}

func TestProjectPathKeyCaseRules(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		goos    string
		dupCase bool // 只差大小写算同一个项目
	}{{"windows", true}, {"darwin", true}, {"linux", false}} {
		t.Run(tc.goos, func(t *testing.T) {
			a, b, other := caseDirs(t)
			svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{PathKey: keyFor(tc.goos)})
			first, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: a})
			if err != nil {
				t.Fatal(err)
			}
			second, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: b})
			if err != nil {
				t.Fatal(err)
			}
			if second.Existed != tc.dupCase || (tc.dupCase && second.Project.ID != first.Project.ID) {
				t.Fatalf("Create 去重不对: %+v", second)
			}

			// Relocate：把另一个项目换到只差大小写的文件夹
			o, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: other})
			_, err = svc.RelocateCatProject(ctx, cat.RelocateProjectRequest{ID: o.Project.ID, Path: b})
			if tc.dupCase {
				if !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Detail != "reason=project_duplicate\nprojectId="+first.Project.ID {
					t.Fatalf("Relocate 应判重复: %v", err)
				}
				// 换到自己只差大小写的写法：算同一路径，原样返回
				same, err := svc.RelocateCatProject(ctx, cat.RelocateProjectRequest{ID: first.Project.ID, Path: b})
				if err != nil || same.Path != a || same.UpdatedAt != first.Project.UpdatedAt {
					t.Fatalf("同一路径应原样返回: %+v %v", same, err)
				}
			} else if !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Detail != "reason=project_duplicate\nprojectId="+second.Project.ID {
				// linux：b 已是 second 的项目
				t.Fatalf("linux 下 b 属于第二个项目: %v", err)
			}
		})
	}
}

func TestProjectPathKeyRekeyOldRows(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	base := t.TempDir()
	mk := func(n string) string {
		d := filepath.Join(base, n)
		_ = os.MkdirAll(d, 0o755)
		return d
	}
	foo1, foo2, bar, fooUp, barUp := mk("Foo"), mk("foo"), mk("Bar"), mk("FOO"), mk("BAR")
	if fi, err := os.Stat(filepath.Join(base, "fOO")); err == nil && fi.IsDir() {
		t.Skip("文件系统不区分大小写")
	}
	// 模拟 v0.31 首版在 macOS 上按“不转小写”写入的行
	ins := func(path string, created int64) store.CatProjectRow {
		r, _, err := st.InsertOrGetCatProject(ctx, store.CatProjectRow{Name: filepath.Base(path), Path: path, PathKey: path, CreatedAt: created})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	p1, p2, p3 := ins(foo1, 100), ins(foo2, 200), ins(bar, 300)

	logs := 0
	svc, _ := newProjSvc(t, st, nil, cat.Config{PathKey: keyFor("darwin"), Logf: func(string, ...any) { logs++ }})
	list, err := svc.ListCatProjects(ctx)
	if err != nil || len(list) != 3 {
		t.Fatalf("不删数据: %v %v", list, err)
	}
	g1, _ := st.GetCatProject(ctx, p1.ID)
	g2, _ := st.GetCatProject(ctx, p2.ID)
	g3, _ := st.GetCatProject(ctx, p3.ID)
	if g1.PathKey != foo1 || g2.PathKey != foo2 {
		t.Fatalf("冲突的两条应保持不动: %q %q", g1.PathKey, g2.PathKey)
	}
	if g3.PathKey != paths.KeyFor("darwin", bar) {
		t.Fatalf("无冲突的应重算为小写: %q", g3.PathKey)
	}
	if logs == 0 {
		t.Fatal("冲突应记日志")
	}
	// Create 走正常重复判断：冲突组命中较早创建的，重算过的直接命中
	r, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: fooUp})
	if err != nil || !r.Existed || r.Project.ID != p1.ID {
		t.Fatalf("应命中较早创建的 Foo: %+v %v", r, err)
	}
	r, err = svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: barUp})
	if err != nil || !r.Existed || r.Project.ID != p3.ID {
		t.Fatalf("%+v %v", r, err)
	}

	// Linux 口径：同样的旧数据不做任何改动
	st2 := openStore(t)
	q, _, _ := st2.InsertOrGetCatProject(ctx, store.CatProjectRow{Name: "Bar", Path: bar, PathKey: bar})
	svc2, _ := newProjSvc(t, st2, nil, cat.Config{PathKey: keyFor("linux")})
	if _, err := svc2.ListCatProjects(ctx); err != nil {
		t.Fatal(err)
	}
	if g, _ := st2.GetCatProject(ctx, q.ID); g.PathKey != bar {
		t.Fatalf("linux 不应改: %q", g.PathKey)
	}
	if r, err := svc2.CreateCatProject(ctx, cat.CreateProjectRequest{Path: barUp}); err != nil || r.Existed {
		t.Fatalf("linux 区分大小写: %+v %v", r, err)
	}
}
