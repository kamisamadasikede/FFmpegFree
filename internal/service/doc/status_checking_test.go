package doc

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doccomp"
	"FFmpegFree/internal/doceng"
)

type evRec struct {
	mu   sync.Mutex
	list []any
}

func (e *evRec) emit(n string, p any) {
	if n != doccomp.EventComponent {
		return
	}
	e.mu.Lock()
	e.list = append(e.list, p)
	e.mu.Unlock()
}

func (e *evRec) lastView() (doceng.StatusView, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := len(e.list) - 1; i >= 0; i-- {
		if v, ok := e.list[i].(doceng.StatusView); ok {
			return v, len(e.list)
		}
	}
	return doceng.StatusView{}, len(e.list)
}

// 按 app.go 的接法建组件 + Registry：组件的 doc:component 改由 Registry 发合并状态。
func wiredComp(t *testing.T, validate func(ctx context.Context, e, tmp string) (string, error), office *doceng.Detected) (*doccomp.Manager, *doceng.Registry, *evRec) {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "soffice")
	os.WriteFile(exe, []byte("x"), 0o755)
	ev := &evRec{}
	var reg *doceng.Registry
	var mu sync.Mutex
	get := func() *doceng.Registry { mu.Lock(); defer mu.Unlock(); return reg }
	comp := doccomp.New(doccomp.Config{Dir: t.TempDir(), Emit: doceng.MergedComponentEmit(ev.emit, get),
		Candidates: func() []string { return []string{exe} }, Validate: validate})
	r := doceng.NewRegistry(doceng.Config{Component: comp, Emit: ev.emit})
	r.SetDetected(office, nil)
	mu.Lock()
	reg = r
	mu.Unlock()
	return comp, r, ev
}

// (a) 检测中 GetDocComponentStatus 立即返回 checking（有 Office 时 state=ready、componentState=checking）；
// (b) 检测结束（各种结果）都发 doc:component（合并状态），之后 GetDocComponentStatus 返回最终状态。
func TestComponentStatusWhileCheckingAndEvent(t *testing.T) {
	office := &doceng.Detected{ID: doceng.IDOffice, Name: doceng.NameOffice, Installed: true, Available: true, Families: []string{doceng.FamilyText}}
	cases := []struct {
		name      string
		office    *doceng.Detected
		ver       string
		err       error
		wantComp  string
		wantState string
	}{
		{"ready", nil, "7.6.4.1", nil, doccomp.StateReady, doccomp.StateReady},
		{"missing", nil, "", apperr.New(apperr.DocComponentInstallFailed, "x").WithDetail("check=smoke"), doccomp.StateMissing, doccomp.StateMissing},
		{"outdated", nil, "7.1.0.3", apperr.New(apperr.DocComponentInstallFailed, "x").WithDetail("check=outdated"), doccomp.StateOutdated, doccomp.StateOutdated},
		{"office+missing", office, "", apperr.New(apperr.DocComponentInstallFailed, "x").WithDetail("check=version"), doccomp.StateMissing, doccomp.StateReady},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			release := make(chan struct{})
			comp, reg, ev := wiredComp(t, func(ctx context.Context, e, tmp string) (string, error) {
				<-release
				return c.ver, c.err
			}, c.office)
			s := New(Config{Component: comp, Engines: reg})
			comp.Start()
			start := time.Now()
			st, err := s.GetDocComponentStatus(context.Background())
			if err != nil || time.Since(start) > 100*time.Millisecond {
				t.Fatalf("%v %s", err, time.Since(start))
			}
			wantNow := doccomp.StateChecking
			if c.office != nil {
				wantNow = doccomp.StateReady // 整体可用靠 Office；组件自己仍在检测
			}
			if st.State != wantNow || st.ComponentState != doccomp.StateChecking {
				t.Fatalf("检测中 %+v", st)
			}
			close(release)
			deadline := time.Now().Add(3 * time.Second)
			for {
				v, n := ev.lastView()
				if n > 0 && v.ComponentState == c.wantComp {
					if v.State != c.wantState {
						t.Fatalf("事件 %+v", v)
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("没有收到检测结束的 doc:component（%d 个事件，最后 %+v）", n, v)
				}
				time.Sleep(10 * time.Millisecond)
			}
			// 监听晚于检测结束：直接拉也拿到最终状态，不是 checking
			st, _ = s.GetDocComponentStatus(context.Background())
			if st.State != c.wantState || st.ComponentState != c.wantComp {
				t.Fatalf("结束后 %+v", st)
			}
		})
	}
}

// 安装失败（failed）与取消安装后的重新检测，同样发合并状态的 doc:component。
func TestComponentEventOnRecheck(t *testing.T) {
	calls := 0
	var mu sync.Mutex
	comp, reg, ev := wiredComp(t, func(ctx context.Context, e, tmp string) (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return "7.6.4.1", nil
	}, nil)
	comp.Start()
	comp.Wait(context.Background(), 3*time.Second)
	if st := reg.Status(context.Background()); st.State != doccomp.StateReady {
		t.Fatalf("%+v", st)
	}
	_, n0 := ev.lastView()
	st := comp.Recheck()
	if st.State != doccomp.StateChecking {
		t.Fatalf("%+v", st)
	}
	comp.Wait(context.Background(), 3*time.Second)
	time.Sleep(20 * time.Millisecond)
	v, n := ev.lastView()
	if n < n0+2 || v.State != doccomp.StateReady || v.ComponentState != doccomp.StateReady {
		t.Fatalf("重新检测应发 checking 和 ready：%d→%d %+v", n0, n, v)
	}
}
