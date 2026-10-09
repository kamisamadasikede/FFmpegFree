package doc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doccomp"
	"FFmpegFree/internal/doceng"
)

// slowComp 模拟冷启动时检测很慢的文档组件：一直 checking，直到 finish；Wait 真的会等。
type slowComp struct {
	mu    sync.Mutex
	st    doccomp.Status
	done  chan struct{}
	waits int
}

func newSlowComp() *slowComp {
	return &slowComp{st: doccomp.Status{State: doccomp.StateChecking, ComponentState: doccomp.StateChecking}, done: make(chan struct{})}
}

func (f *slowComp) Status() doccomp.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.st
}

func (f *slowComp) Wait(ctx context.Context, d time.Duration) doccomp.Status {
	f.mu.Lock()
	f.waits++
	f.mu.Unlock()
	if f.Status().State == doccomp.StateChecking {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-f.done:
		case <-t.C:
		case <-ctx.Done():
		}
	}
	return f.Status()
}

func (f *slowComp) waitCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.waits
}

func (f *slowComp) finish(state string) {
	f.mu.Lock()
	f.st = doccomp.Status{State: state, ComponentState: state}
	f.mu.Unlock()
	close(f.done)
}

func (f *slowComp) ExePath() string {
	if f.Status().State == doccomp.StateReady {
		return filepath.Join(os.TempDir(), "no-such-soffice")
	}
	return ""
}

func (f *slowComp) NotReadyError() *apperr.AppError {
	return apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。").WithDetail("reason=" + f.Status().State)
}

func findTarget(t *testing.T, m DocFormatMatrix, src, target string) DocTarget {
	t.Helper()
	for _, s := range m.Sources {
		if s.Ext != src {
			continue
		}
		for _, tg := range s.Targets {
			if tg.Ext == target {
				return tg
			}
		}
	}
	t.Fatalf("格式表里没有 %s → %s", src, target)
	return DocTarget{}
}

// 组件 checking 时格式表立即返回（不再等 6 秒），要组件的目标按未就绪返回。
func TestMatrixNoWaitWhileChecking(t *testing.T) {
	comp := newSlowComp()
	s := New(Config{Component: comp})
	start := time.Now()
	m := s.GetFormatMatrix(context.Background())
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("格式表等了 %s", d)
	}
	if comp.waitCount() != 0 {
		t.Fatal("格式表不应调用 Wait")
	}
	if m.ComponentReady {
		t.Fatal("checking 时 componentReady=false")
	}
	if tg := findTarget(t, m, "txt", "docx"); !tg.NeedsComponent || tg.Available || tg.DisabledReason == "" {
		t.Fatalf("txt→docx %+v", tg)
	}
	if tg := findTarget(t, m, "md", "html"); tg.NeedsComponent || !tg.Available {
		t.Fatalf("md→html %+v", tg)
	}
	if tg := findTarget(t, m, "txt", "pdf"); !tg.Simple || !tg.Available {
		t.Fatalf("txt→pdf 简易 %+v", tg)
	}
	if tg := findTarget(t, m, "pdf", "txt"); !tg.Available || tg.HintKey != HintPDFText {
		t.Fatalf("pdf→txt %+v", tg)
	}
	if tg := findTarget(t, m, "pdf", "docx"); tg.Available || !tg.NeedsComponent {
		t.Fatalf("pdf→docx %+v", tg)
	}
	if tg := findTarget(t, m, "pdf", "html"); !tg.Available || !tg.Simple {
		t.Fatalf("pdf→html %+v", tg)
	}
	// 检测完成后重拉：要组件的目标变可用
	comp.finish(doccomp.StateReady)
	m = s.GetFormatMatrix(context.Background())
	if tg := findTarget(t, m, "txt", "docx"); !m.ComponentReady || !tg.Available {
		t.Fatalf("ready 后 %+v", tg)
	}
}

// 有 Registry、Office 已检测到、组件还在慢慢检测：Office 能做的照常可用，只有组件能做的未就绪。
func TestMatrixNoWaitWithOffice(t *testing.T) {
	release := make(chan struct{})
	var comp *doccomp.Manager
	defer func() {
		close(release)
		comp.Wait(context.Background(), 3*time.Second) // 等检测收尾再删临时目录
	}()
	exe := filepath.Join(t.TempDir(), "soffice")
	os.WriteFile(exe, []byte("x"), 0o755)
	comp = doccomp.New(doccomp.Config{Dir: t.TempDir(), Candidates: func() []string { return []string{exe} },
		Validate: func(ctx context.Context, e, tmp string) (string, error) {
			<-release
			return "", apperr.New(apperr.DocComponentInstallFailed, "x").WithDetail("check=version")
		}})
	comp.Start()
	reg := doceng.NewRegistry(doceng.Config{Component: comp})
	reg.SetDetected(&doceng.Detected{ID: doceng.IDOffice, Name: doceng.NameOffice, Installed: true, Available: true,
		Families: []string{doceng.FamilyText, doceng.FamilyPDF}, WordMajor: 16}, nil)
	s := New(Config{Component: comp, Engines: reg})
	start := time.Now()
	m := s.GetFormatMatrix(context.Background())
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("格式表等了 %s", d)
	}
	if comp.Status().State != doccomp.StateChecking {
		t.Fatal("组件应仍在 checking")
	}
	if !m.ComponentReady {
		t.Fatal("有 Office 时整体 ready")
	}
	if tg := findTarget(t, m, "docx", "pdf"); !tg.Available || len(tg.Engines) == 0 || tg.Engines[0] != doceng.IDOffice {
		t.Fatalf("docx→pdf %+v", tg)
	}
	if tg := findTarget(t, m, "pdf", "docx"); !tg.Available || len(tg.Engines) == 0 || tg.Engines[0] != doceng.IDOffice {
		t.Fatalf("pdf→docx %+v", tg)
	}
	if tg := findTarget(t, m, "xlsx", "ods"); tg.Available || !tg.NeedsComponent {
		t.Fatalf("xlsx→ods 只有组件能做，检测中不可用 %+v", tg)
	}
	// 重试 / 重转：只有 Office 的电脑不再被误拒（以前只看组件）
	if err := s.engineReadyFor(context.Background(), docJob{src: "docx", target: "pdf"}); err != nil {
		t.Fatal(err)
	}
}

// 提交时组件还在检测：最多等 submitWait；检测期间变 ready 就照常提交（以前会被 reason=checking 拒绝）。
func TestSubmitWhileCheckingWaitsForDetection(t *testing.T) {
	comp := newSlowComp()
	e := newDocEnv(t, comp)
	src := filepath.Join(e.dir, "src")
	os.MkdirAll(src, 0o755)
	txt := writeFile(t, filepath.Join(src, "a.txt"), []byte("hello\n"))
	md := writeFile(t, filepath.Join(src, "b.md"), []byte("# hi\n"))
	res := e.add(t, txt, md)
	for _, r := range res {
		if r.Error != nil {
			t.Fatalf("%s: %+v", r.Path, r.Error)
		}
	}
	// 不要组件的目标：不等
	start := time.Now()
	if ts := e.submit(t, "html", res[1].Source.SourceID); len(ts) != 1 {
		t.Fatal(ts)
	}
	if d := time.Since(start); d > 500*time.Millisecond || comp.waitCount() != 0 {
		t.Fatalf("md→html 不该等组件：%s waits=%d", d, comp.waitCount())
	}
	// 要组件的目标：检测 200ms 后完成 → 提交成功
	go func() { time.Sleep(200 * time.Millisecond); comp.finish(doccomp.StateReady) }()
	ts := e.submit(t, "docx", res[0].Source.SourceID)
	if len(ts) != 1 || comp.waitCount() != 1 {
		t.Fatalf("%+v waits=%d", ts, comp.waitCount())
	}
	e.wait(t, ts[0].ID) // 假组件跑不起来，失败无所谓，只验证没被拒
}

// 检测一直没完成：等满 submitWait 后照旧拒绝（DOC_COMPONENT_NOT_READY，reason=checking），整个提交只等一次。
func TestSubmitWhileCheckingTimesOut(t *testing.T) {
	old := submitWait
	submitWait = 150 * time.Millisecond
	defer func() { submitWait = old }()
	comp := newSlowComp()
	e := newDocEnv(t, comp)
	src := filepath.Join(e.dir, "src")
	os.MkdirAll(src, 0o755)
	a := writeFile(t, filepath.Join(src, "a.txt"), []byte("a\n"))
	b := writeFile(t, filepath.Join(src, "b.txt"), []byte("b\n"))
	res := e.add(t, a, b)
	start := time.Now()
	_, err := e.svc.SubmitDocConvert(context.Background(), DocSubmitRequest{SourceIDs: []string{res[0].Source.SourceID, res[1].Source.SourceID}, Target: "docx"})
	if !apperr.Is(err, apperr.DocComponentNotReady) || !strings.HasPrefix(apperr.From(err).Detail, "reason=checking") {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second || comp.waitCount() != 1 {
		t.Fatalf("%s waits=%d", d, comp.waitCount())
	}
}
