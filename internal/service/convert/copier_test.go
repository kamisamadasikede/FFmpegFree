package convert

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/media"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// ---------- 源文件副本（契约 v0.24，6.15.3~6.15.8） ----------

type copyRec struct {
	mu   sync.Mutex
	evts []CopyEvent
}

func (r *copyRec) Emit(name string, p any) {
	if ce, ok := p.(CopyEvent); ok && name == EventCopy {
		r.mu.Lock()
		r.evts = append(r.evts, ce)
		r.mu.Unlock()
	}
}
func (r *copyRec) of(sid string) []CopyEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []CopyEvent
	for _, e := range r.evts {
		if e.SourceID == sid {
			out = append(out, e)
		}
	}
	return out
}

type copyEnv struct {
	svc     *Service
	st      *store.Store
	tm      *task.Manager
	dir     string
	uploads string
	em      *copyRec
	free    func(string) (int64, error)
	freeMu  sync.Mutex
	freeVal int64
}

func newCopyEnv(t *testing.T) *copyEnv {
	t.Helper()
	dir := t.TempDir()
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		dir = r
	}
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &copyEnv{st: st, dir: dir, uploads: filepath.Join(dir, "uploads"), em: &copyRec{}, freeVal: 1 << 50}
	e.tm = task.NewManager(task.Config{Store: st, LogDir: filepath.Join(dir, "logs"), BatchConcurrency: 2, ProgressInterval: -1, Logf: func(string, ...any) {}})
	t.Cleanup(func() { e.tm.Shutdown(3 * time.Second) })
	e.svc = e.newSvc(t)
	return e
}

// newSvc 在同一个库上造一个 Service（模拟重启时用第二次调用）。不需要 ffmpeg。
func (e *copyEnv) newSvc(t *testing.T) *Service {
	t.Helper()
	noFF := func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{}, apperr.New(apperr.FFmpegNotFound, "x") }
	med := media.New(media.Config{Require: noFF, ThumbsDir: filepath.Join(e.dir, "thumbs")})
	svc, err := New(context.Background(), Config{Presets: e.st, Media: med, Tasks: e.tm, Require: noFF,
		UploadsDir: func(context.Context) string { return e.uploads }, Emitter: e.em, CopyProgressInterval: -1,
		FreeSpace: func(string) (int64, error) { e.freeMu.Lock(); defer e.freeMu.Unlock(); return e.freeVal, nil },
		Logf:      func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close(3 * time.Second) })
	return svc
}

func (e *copyEnv) setFree(n int64) { e.freeMu.Lock(); e.freeVal = n; e.freeMu.Unlock() }

func (e *copyEnv) file(t *testing.T, name string, size int) string {
	t.Helper()
	p := filepath.Join(e.dir, "src", name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, bytes.Repeat([]byte{'a' + byte(len(name)%26)}, size), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func (e *copyEnv) add(t *testing.T, p string) AddSourceResult {
	t.Helper()
	res, err := e.svc.AddSources(context.Background(), []string{p})
	if err != nil || len(res) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	return res[0]
}

func (e *copyEnv) waitState(t *testing.T, sid, state string) ConvertSource {
	t.Helper()
	var src ConvertSource
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ent, err := e.svc.GetSource(context.Background(), sid)
		if err == nil && ent.Source.CopyState == state {
			return ent.Source
		}
		src = ent.Source
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等 %s 超时，现在是 %+v", state, src)
	return src
}

func detailOf2(err *apperr.AppError) string {
	if err == nil {
		return ""
	}
	return err.Detail
}

// 拦住复制：每写完一块等 gate（或 ctx 取消）。返回放行函数。
func holdCopies(t *testing.T) (started chan struct{}, release func()) {
	gate := make(chan struct{})
	started = make(chan struct{}, 64)
	copyChunkHook = func(ctx context.Context) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-gate:
		case <-ctx.Done():
		}
	}
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(func() { release(); copyChunkHook = nil })
	return started, release
}

func TestCopyOnAdd(t *testing.T) {
	e := newCopyEnv(t)
	in := e.file(t, "clip.mov", 3<<20+7)
	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	os.Chtimes(in, old, old)
	r := e.add(t, in)
	if r.Error != nil || r.Source == nil {
		t.Fatalf("%+v", r)
	}
	if r.Source.CopyState != store.CopyCopying && r.Source.CopyState != store.CopyReady {
		t.Fatalf("%+v", r.Source)
	}
	if r.Source.OriginalPath != in || r.Source.TotalBytes != 3<<20+7 {
		t.Fatalf("%+v", r.Source)
	}
	src := e.waitState(t, r.Source.SourceID, store.CopyReady)
	want := filepath.Join(e.uploads, src.SourceID, "clip.mov")
	if src.StoredPath != want || src.CopiedBytes != src.TotalBytes || src.CopyError != nil {
		t.Fatalf("%+v", src)
	}
	a, _ := os.ReadFile(in)
	b, _ := os.ReadFile(want)
	if !bytes.Equal(a, b) {
		t.Fatal("副本内容不一致")
	}
	if fi, _ := os.Stat(want); !fi.ModTime().Equal(old) {
		t.Fatalf("副本修改时间应改回原文件的: %v vs %v", fi.ModTime(), old)
	}
	if _, err := os.Stat(task.PartPath(want)); !os.IsNotExist(err) {
		t.Fatal(".part 应已改名")
	}
	if readPath(src) != want || displayPath(src) != want {
		t.Fatal("ready 读副本")
	}
	// 事件：开始（copying, 0）…… 结束（ready），seq 递增
	ev := e.em.of(src.SourceID)
	if len(ev) < 2 || ev[0].CopyState != store.CopyCopying || ev[0].CopiedBytes != 0 || ev[len(ev)-1].CopyState != store.CopyReady ||
		ev[len(ev)-1].CopiedBytes != src.TotalBytes || ev[len(ev)-1].StoredPath != want {
		t.Fatalf("%+v", ev)
	}
	for i := 1; i < len(ev); i++ {
		if ev[i].Seq <= ev[i-1].Seq {
			t.Fatal("seq 应递增")
		}
	}
	c, _ := e.st.GetCopy(context.Background(), src.CopyID)
	if c.RefCount != 1 || c.OriginalMtimeNs != old.UnixNano() {
		t.Fatalf("%+v", c)
	}
	// 原文件不受影响；删掉原文件后预览 / 打开仍读副本
	os.Remove(in)
	if p, err := sourceFile(src); err != nil || p != want {
		t.Fatalf("%s %v", p, err)
	}
}

// v0.24 之前的旧行（copy_id 为空）：copyState=none，读原路径，不补做副本（前端按 none 显示移除对话框文案）。
func TestLegacyRowCopyStateNone(t *testing.T) {
	e := newCopyEnv(t)
	in := e.file(t, "old.mp4", 100)
	p, key, _ := paths.Normalize(in)
	src, _, err := e.st.UpsertConvertSource(context.Background(), p, key, 1)
	if err != nil {
		t.Fatal(err)
	}
	ent, err := e.svc.GetSource(context.Background(), src.SourceID)
	if err != nil || ent.Source.CopyState != "none" || ent.Source.StoredPath != "" || ent.Source.OriginalPath != in || readPath(ent.Source) != in {
		t.Fatalf("%+v %v", ent.Source, err)
	}
	_, err = e.svc.RetryCopy(context.Background(), src.SourceID)
	if !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("旧行 RetryCopy 应 INVALID_ARGUMENT: %v", err)
	}
	if err := e.svc.CancelCopy(context.Background(), src.SourceID); !apperr.Is(err, apperr.TaskConflict) {
		t.Fatalf("%v", err)
	}
	// 旧行被再次添加：新做一份副本
	r := e.add(t, in)
	if r.Source.SourceID != src.SourceID || !r.Existed {
		t.Fatalf("%+v", r)
	}
	e.waitState(t, src.SourceID, store.CopyReady)
}

// 引用计数：重复添加不再复制；把副本本身拖进来 → 新行共享同一份副本（ref 2）；
// 删一行副本还在，删到归零才删副本文件和 uploads/<owner>/；永远不删原文件。
func TestCopyRefcounts(t *testing.T) {
	e := newCopyEnv(t)
	ctx := context.Background()
	in := e.file(t, "a.mp4", 2048)
	r := e.add(t, in)
	src := e.waitState(t, r.Source.SourceID, store.CopyReady)
	nEv := len(e.em.of(src.SourceID))

	again := e.add(t, in)
	if !again.Existed || again.Source.CopyID != src.CopyID || again.Source.CopyState != store.CopyReady {
		t.Fatalf("重复添加复用副本: %+v", again.Source)
	}
	if len(e.em.of(src.SourceID)) != nEv {
		t.Fatal("复用不发事件")
	}
	shared := e.add(t, src.StoredPath) // 2.1：路径本身是 ready 副本
	if shared.Error != nil || shared.Source.SourceID == src.SourceID || shared.Source.CopyID != src.CopyID || shared.Source.CopyState != store.CopyReady {
		t.Fatalf("%+v", shared)
	}
	if c, _ := e.st.GetCopy(ctx, src.CopyID); c.RefCount != 2 {
		t.Fatalf("ref=%d", c.RefCount)
	}
	if _, err := e.svc.DeleteSource(ctx, src.SourceID, false); err != nil {
		t.Fatal(err)
	}
	if c, err := e.st.GetCopy(ctx, src.CopyID); err != nil || c.RefCount != 1 {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := os.Stat(src.StoredPath); err != nil {
		t.Fatal("还有别的行共享：副本不删")
	}
	res, err := e.svc.DeleteSource(ctx, shared.Source.SourceID, false)
	if err != nil || len(res.Failures) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := os.Stat(src.StoredPath); !os.IsNotExist(err) {
		t.Fatal("归零后删副本")
	}
	if _, err := os.Stat(filepath.Dir(src.StoredPath)); !os.IsNotExist(err) {
		t.Fatal("空的 uploads/<owner>/ 一并删掉")
	}
	if _, err := e.st.GetCopy(ctx, src.CopyID); err == nil {
		t.Fatal("副本行应删除")
	}
	if _, err := os.Stat(in); err != nil {
		t.Fatal("永远不删原文件")
	}
}

// 原文件变了再添加：新做一份副本，旧副本归零删掉；副本被替换过时不碰文件（not_task_output）。
func TestCopyRenewAndForeignStoredFile(t *testing.T) {
	e := newCopyEnv(t)
	ctx := context.Background()
	in := e.file(t, "b.mp4", 1000)
	r := e.add(t, in)
	first := e.waitState(t, r.Source.SourceID, store.CopyReady)
	os.WriteFile(in, bytes.Repeat([]byte("z"), 1500), 0o644)
	e.add(t, in)
	var second ConvertSource
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ent, _ := e.svc.GetSource(ctx, first.SourceID)
		if second = ent.Source; second.CopyID != first.CopyID && second.CopyState == store.CopyReady {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if second.CopyID == first.CopyID || second.TotalBytes != 1500 {
		t.Fatalf("%+v", second)
	}
	if _, err := e.st.GetCopy(ctx, first.CopyID); err == nil {
		t.Fatal("旧副本行应删除")
	}
	// 用户把副本换成了自己的文件：从列表移除时不碰它
	os.WriteFile(second.StoredPath, []byte("mine"), 0o644)
	res, err := e.svc.DeleteSource(ctx, first.SourceID, false)
	if err != nil || len(res.Failures) != 1 || res.Failures[0].Reason != task.DeleteNotTaskOutput || res.Failures[0].SourceID != first.SourceID {
		t.Fatalf("%+v %v", res, err)
	}
	if b, _ := os.ReadFile(second.StoredPath); string(b) != "mine" {
		t.Fatal("别人的文件不能删")
	}
}

// 副本删不掉：failures 里一条 in_use，副本行标 pending_delete，下次启动再删。
func TestCopyDeleteFailurePendingDelete(t *testing.T) {
	e := newCopyEnv(t)
	ctx := context.Background()
	r := e.add(t, e.file(t, "c.mp4", 500))
	src := e.waitState(t, r.Source.SourceID, store.CopyReady)
	removeCopyFile = func(string) error { return &os.PathError{Op: "remove", Path: src.StoredPath, Err: errBusy} }
	res, err := e.svc.DeleteSource(ctx, src.SourceID, false)
	removeCopyFile = os.Remove
	if err != nil || len(res.Failures) != 1 || res.Failures[0].Path != src.StoredPath || res.Failures[0].TaskID != "" {
		t.Fatalf("%+v %v", res, err)
	}
	c, err := e.st.GetCopy(ctx, src.CopyID)
	if err != nil || !c.PendingDelete || c.RefCount != 0 {
		t.Fatalf("%+v %v", c, err)
	}
	e.newSvc(t) // 重启
	if _, err := e.st.GetCopy(ctx, src.CopyID); err == nil {
		t.Fatal("启动时应删掉 pending_delete 副本")
	}
	if _, err := os.Stat(src.StoredPath); !os.IsNotExist(err) {
		t.Fatal("副本文件应删除")
	}
}

// 磁盘空间：need = 大小 + 排队中的剩余 + 64 MiB；不够时副本直接 failed（不是调用错误），RetryCopy 重新检查。
func TestCopyDiskSpace(t *testing.T) {
	e := newCopyEnv(t)
	ctx := context.Background()
	in := e.file(t, "big.mov", 4096)
	e.setFree(10 << 20)
	r := e.add(t, in)
	if r.Error != nil || r.Source.CopyState != store.CopyFailed || r.Source.CopyError == nil {
		t.Fatalf("%+v", r)
	}
	ce := r.Source.CopyError
	if ce.Code != apperr.ConvertDiskFull || ce.Message != "磁盘空间不足，需要 64 MB，剩余 10 MB。" ||
		ce.Detail != "reason=no_space\nneedBytes=67112960\nfreeBytes=10485760" {
		t.Fatalf("%+v", ce)
	}
	if ev := e.em.of(r.Source.SourceID); len(ev) != 1 || ev[0].CopyState != store.CopyFailed || ev[0].Error == nil {
		t.Fatalf("%+v", ev)
	}
	// 复制失败的行：不能预览、提交时跳过（reason=copy_failed）
	if readPath(*r.Source) != "" {
		t.Fatal("failed 不能转换")
	}
	e.setFree(1 << 40)
	got, err := e.svc.RetryCopy(ctx, r.Source.SourceID)
	if err != nil || (got.CopyState != store.CopyCopying && got.CopyState != store.CopyReady) || got.CopyError != nil {
		t.Fatalf("%+v %v", got, err)
	}
	e.waitState(t, r.Source.SourceID, store.CopyReady)
	if _, err := e.svc.RetryCopy(ctx, r.Source.SourceID); !apperr.Is(err, apperr.TaskConflict) {
		t.Fatalf("完好的 ready 不能 RetryCopy: %v", err)
	}
	if humanBytes(1536) != "1.5 KB" || humanBytes(512) != "512 B" || humanBytes(3<<30) != "3 GB" {
		t.Fatal(humanBytes(1536))
	}
}

// 写满（ENOSPC）：CONVERT_DISK_FULL，detail 同样三行。
func TestCopyWriteDiskFull(t *testing.T) {
	e := newCopyEnv(t)
	err := e.svc.writeError(errNoSpace, store.ConvertCopy{ID: "x", StoredPath: filepath.Join(e.uploads, "s", "a.mp4"), TotalBytes: 100})
	if err.Code != apperr.ConvertDiskFull || !strings.HasPrefix(err.Detail, "reason=no_space\nneedBytes=") {
		t.Fatalf("%+v", err)
	}
}

func TestCopyCancelAndRetry(t *testing.T) {
	e := newCopyEnv(t)
	ctx := context.Background()
	started, release := holdCopies(t)
	in := e.file(t, "slow.mov", 3<<20)
	r := e.add(t, in)
	<-started
	src := e.waitState(t, r.Source.SourceID, store.CopyCopying)
	if src.CopiedBytes <= 0 {
		t.Fatalf("复制中应带实时 copiedBytes: %+v", src)
	}
	// 复制中：不能预览；提交时跳过
	_, err := e.svc.GetSourcePreviewURL(ctx, src.SourceID)
	if !apperr.Is(err, apperr.TaskConflict) || detailOf(err) != "reason=copying" {
		t.Fatalf("%v", err)
	}
	// 复制中缩略图照常出：用原文件（显示路径，6.15.6），不等副本
	th := &pathThumbs{}
	e.svc.cfg.Thumbs = th
	if u, err := e.svc.GetSourceThumbnail(ctx, src.SourceID); err != nil || u == "" || th.last() != in {
		t.Fatalf("复制中缩略图应取原文件: %q %v (取的是 %q)", u, err, th.last())
	}
	if err := e.svc.CancelCopy(ctx, src.SourceID); err != nil {
		t.Fatal(err)
	}
	got := e.waitState(t, src.SourceID, store.CopyCanceled)
	if got.CopyError != nil {
		t.Fatal("取消不是错误")
	}
	ev := e.em.of(src.SourceID)
	if ev[len(ev)-1].CopyState != store.CopyCanceled {
		t.Fatalf("%+v", ev[len(ev)-1])
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(task.PartPath(src.StoredPath)); !os.IsNotExist(err) {
		t.Fatal("取消后删 .part")
	}
	if err := e.svc.CancelCopy(ctx, src.SourceID); err != nil {
		t.Fatalf("已取消再取消不报错: %v", err)
	}
	release()
	copyChunkHook = nil
	if _, err := e.svc.RetryCopy(ctx, src.SourceID); err != nil {
		t.Fatal(err)
	}
	done := e.waitState(t, src.SourceID, store.CopyReady)
	if b, _ := os.ReadFile(done.StoredPath); len(b) != 3<<20 {
		t.Fatal("重试后副本完整")
	}
}

// 复制期间原文件被改：failed（reason=source_changed）。
func TestCopySourceChanged(t *testing.T) {
	e := newCopyEnv(t)
	in := e.file(t, "chg.mov", 3<<20)
	var once sync.Once
	copyChunkHook = func(context.Context) {
		once.Do(func() { os.WriteFile(in, []byte("changed"), 0o644) })
	}
	t.Cleanup(func() { copyChunkHook = nil })
	r := e.add(t, in)
	got := e.waitState(t, r.Source.SourceID, store.CopyFailed)
	if detailOf2(got.CopyError) != "reason=source_changed" {
		t.Fatalf("%+v", got.CopyError)
	}
}

// 退出时正在复制：库里保持 copying；下次启动标成 failed（reason=interrupted）、删 .part，不续传，可以重试。
func TestCopyInterruptedByExit(t *testing.T) {
	e := newCopyEnv(t)
	ctx := context.Background()
	started, _ := holdCopies(t)
	r := e.add(t, e.file(t, "exit.mov", 3<<20))
	<-started
	e.svc.Close(3 * time.Second)
	c, _ := e.st.GetCopy(ctx, r.Source.CopyID)
	if c.State != store.CopyCopying {
		t.Fatalf("退出时不改库: %+v", c)
	}
	copyChunkHook = nil
	os.WriteFile(task.PartPath(c.StoredPath), []byte("half"), 0o644) // 进程直接没了的情形
	e.svc = e.newSvc(t)
	ent, _ := e.svc.GetSource(ctx, r.Source.SourceID)
	if ent.Source.CopyState != store.CopyFailed || detailOf2(ent.Source.CopyError) != "reason=interrupted" {
		t.Fatalf("%+v", ent.Source)
	}
	if _, err := os.Stat(task.PartPath(c.StoredPath)); !os.IsNotExist(err) {
		t.Fatal(".part 应删除")
	}
	if _, err := e.svc.RetryCopy(ctx, r.Source.SourceID); err != nil {
		t.Fatal(err)
	}
	e.waitState(t, r.Source.SourceID, store.CopyReady)
}

// 删除复制中的行：先取消；但副本还被别的行共享时不取消（v0.24.1 实现取舍）。
func TestDeleteSourceWhileCopying(t *testing.T) {
	e := newCopyEnv(t)
	ctx := context.Background()
	started, release := holdCopies(t)
	r := e.add(t, e.file(t, "del.mov", 3<<20))
	<-started
	res, err := e.svc.DeleteSource(ctx, r.Source.SourceID, false)
	if err != nil || len(res.DeletedSourceIDs) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	release()
	time.Sleep(50 * time.Millisecond)
	if _, err := e.st.GetCopy(ctx, r.Source.CopyID); err == nil {
		t.Fatal("副本行应删除")
	}
	if ents, _ := os.ReadDir(e.uploads); len(ents) != 0 {
		t.Fatalf("uploads 应是空的: %v", ents)
	}
}

// AddSources 只收输入白名单里的扩展名；SubmitSources 没就绪的行全部跳过时 TASK_CONFLICT。
func TestSubmitSourcesAllSkipped(t *testing.T) {
	e := newCopyEnv(t)
	ctx := context.Background()
	started, _ := holdCopies(t)
	r := e.add(t, e.file(t, "wait.mov", 3<<20))
	<-started
	_, err := e.svc.SubmitSources(ctx, ConvertSubmitRequest{SourceIDs: []string{r.Source.SourceID},
		Options: ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}, OutputDir: filepath.Join(e.dir, "o")})
	if !apperr.Is(err, apperr.TaskConflict) || !strings.HasPrefix(detailOf(err), "reason=copying") {
		t.Fatalf("%v (%s)", err, detailOf(err))
	}
	// v0.24.4：复制中的提示说“准备中”（包 20 用词），reason 不变
	if m := apperr.From(err).Message; m != "文件还在准备中，准备好后再转换。" {
		t.Fatalf("message: %q", m)
	}
	_, err = e.svc.GetSourcePreviewURL(ctx, r.Source.SourceID)
	wantReason(t, err, apperr.TaskConflict, "reason=copying")
	if m := apperr.From(err).Message; m != "文件还在准备中，准备好后才能预览。" {
		t.Fatalf("preview message: %q", m)
	}
	_, err = e.svc.RetryCopy(ctx, r.Source.SourceID)
	if m := apperr.From(err).Message; !apperr.Is(err, apperr.TaskConflict) || m != "文件还在准备中，不需要重试。" {
		t.Fatalf("retry: %v", err)
	}
}

// pathThumbs 是假的 Thumbnailer：记下被要求截图的路径。
type pathThumbs struct {
	mu    sync.Mutex
	paths []string
}

func (p *pathThumbs) DefaultThumbnailDataURL(_ context.Context, path string, _ float64) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paths = append(p.paths, path)
	return "data:image/jpeg;base64,AA==", nil
}

func (p *pathThumbs) last() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.paths) == 0 {
		return ""
	}
	return p.paths[len(p.paths)-1]
}

// v0.24.3：打开所在文件夹 / 用系统程序打开都走原文件，副本 ready 也不打开 storedPath；原文件不在时不退回副本。
func TestOpenAndRevealOriginalNotCopy(t *testing.T) {
	e := newCopyEnv(t)
	in := e.file(t, "clip.mov", 64)
	r := e.add(t, in)
	if r.Error != nil || r.Source == nil {
		t.Fatalf("%+v", r)
	}
	src := e.waitState(t, r.Source.SourceID, store.CopyReady)
	if src.StoredPath == "" || src.StoredPath == in {
		t.Fatalf("应有一份不同于原文件的副本: %+v", src)
	}
	var opened, revealed []string
	e.svc.cfg.Open = func(p string) error { opened = append(opened, p); return nil }
	e.svc.cfg.Reveal = func(p string) error { revealed = append(revealed, p); return nil }
	ctx := context.Background()
	if err := e.svc.OpenSourceWithSystem(ctx, src.SourceID); err != nil || len(opened) != 1 || opened[0] != in {
		t.Fatalf("应打开原文件 %s: %v %v", in, err, opened)
	}
	if err := e.svc.RevealSource(ctx, src.SourceID); err != nil || len(revealed) != 1 || revealed[0] != in {
		t.Fatalf("应选中原文件: %v %v", err, revealed)
	}
	if strings.Contains(opened[0], e.uploads) || strings.Contains(revealed[0], e.uploads) {
		t.Fatal("不应打开 uploads 里的副本")
	}
	if err := os.Remove(in); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"open", "reveal"} {
		var err error
		if name == "open" {
			err = e.svc.OpenSourceWithSystem(ctx, src.SourceID)
		} else {
			err = e.svc.RevealSource(ctx, src.SourceID)
		}
		wantReason(t, err, apperr.NotFound, "reason=file")
		if apperr.From(err).Message != "原文件不存在，无法打开。" {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if len(opened) != 1 || len(revealed) != 1 {
		t.Fatalf("原文件不在时不应退回副本: open %v reveal %v", opened, revealed)
	}
	if _, err := os.Stat(src.StoredPath); err != nil {
		t.Fatal("副本应还在")
	}
	wantReason(t, e.svc.OpenSourceWithSystem(ctx, "nope"), apperr.NotFound, "reason=record")
	wantReason(t, e.svc.RevealSource(ctx, "nope"), apperr.NotFound, "reason=record")
}
