package edit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/localassets"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

func storeSvc(t *testing.T) (*Service, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(Config{Projects: st, Lister: st, Media: fakeMedia{base}}), st, dir
}

func TestSaveLoadListDelete(t *testing.T) {
	s, _, dir := storeSvc(t)
	ctx := context.Background()
	exist := filepath.Join(dir, "real.mp4")
	os.WriteFile(exist, []byte("x"), 0o644)
	gone := filepath.Join(dir, "gone.mp4")
	p := EditProject{Name: "  草稿  ", Sources: []string{exist, gone}, VideoTrack: []VideoClip{
		vclip("c1", exist, "V1", 0, 0, 2), vclip("c2", gone, "V1", 2, 0, 3)},
		AudioTrack: []AudioClip{aclip("a1", gone, "A1", 0, 0, 1)}}
	m, err := s.SaveProject(ctx, p)
	if err != nil || m.ID == "" || m.Name != "草稿" || m.ClipCount != 3 || m.DurationSec != 5 || m.UpdatedAt == 0 {
		t.Fatalf("%+v %v", m, err)
	}
	// 载入：素材丢失不报错；missingPaths 去重、按首次出现顺序
	lp, err := s.LoadProject(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(lp.MissingPaths) != 1 || lp.MissingPaths[0] != gone || lp.Project.ID != m.ID || lp.Project.SchemaVersion != 1 ||
		len(lp.Project.VideoTrack) != 2 || lp.Project.VideoTrack[1].ID != "c2" || lp.Project.Name != "草稿" {
		t.Fatalf("%+v", lp)
	}
	// 更新：id 不变，updatedAt 变
	p2 := lp.Project
	p2.Name = "改名"
	p2.VideoTrack = p2.VideoTrack[:1]
	m2, err := s.SaveProject(ctx, p2)
	if err != nil || m2.ID != m.ID || m2.Name != "改名" || m2.ClipCount == 3 {
		t.Fatalf("%+v %v", m2, err)
	}
	// 另一个工程；列表按 updatedAt 倒序
	m3, _ := s.SaveProject(ctx, EditProject{Name: "第二个"})
	list, err := s.ListProjects(ctx, 0)
	if err != nil || len(list) != 2 || list[0].ID != m3.ID && list[0].UpdatedAt < list[1].UpdatedAt {
		t.Fatalf("%+v %v", list, err)
	}
	// 更新不存在的 → NOT_FOUND；载入 / 删除不存在 → NOT_FOUND
	px := p2
	px.ID = "nope"
	if _, err := s.SaveProject(ctx, px); code(t, err) != apperr.NotFound {
		t.Fatal(err)
	}
	if _, err := s.LoadProject(ctx, "nope"); code(t, err) != apperr.NotFound {
		t.Fatal(err)
	}
	if err := s.DeleteProject(ctx, "nope"); code(t, err) != apperr.NotFound {
		t.Fatal(err)
	}
	if err := s.DeleteProject(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadProject(ctx, m.ID); code(t, err) != apperr.NotFound {
		t.Fatal(err)
	}
	// 删工程不删素材
	if _, err := os.Stat(exist); err != nil {
		t.Fatal("素材被删")
	}
}

// 架构师决定：SaveProject 只校验数量上限，不校验同轨重叠 / outSec / 素材（草稿可保存）；重叠只在 Validate / Export 报。
func TestSaveAllowsDraftsButValidateRejects(t *testing.T) {
	s := fakeSvc(base)
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s.cfg.Projects = st
	ctx := context.Background()
	draft := EditProject{Name: "草稿", VideoTrack: []VideoClip{
		vclip("c1", pV, "V1", 0, 0, 5),
		vclip("c2", pV2, "V1", 3, 0, 2),                // 与 c1 重叠
		{ID: "c3", Path: pV, TrackID: "V1", OutSec: 0}, // outSec=0
		{ID: "c4", Path: filepath.Join(string(filepath.Separator), "nowhere.mp4"), TrackID: "V1", OutSec: 1}, // 素材不存在
	}}
	m, err := s.SaveProject(ctx, draft)
	if err != nil {
		t.Fatalf("草稿应可保存: %v", err)
	}
	if m.ClipCount != 4 {
		t.Fatalf("%+v", m)
	}
	// 同一份数据 Validate 报错
	if _, err := s.ValidateProject(ctx, draft); code(t, err) != apperr.InvalidArgument {
		t.Fatal(err)
	}
	// 只有重叠的草稿：Validate 报 clip=后一个
	d2 := proj(vclip("c1", pV, "V1", 0, 0, 5), vclip("c2", pV2, "V1", 3, 0, 2))
	if _, err := s.SaveProject(ctx, d2); err != nil {
		t.Fatalf("重叠草稿可保存: %v", err)
	}
	if _, err := s.ValidateProject(ctx, d2); firstLine(err) != "clip=c2 path="+pV2 {
		t.Fatalf("%v", err)
	}
	// 数量上限仍然生效
	big := EditProject{Name: "big"}
	for i := 0; i < 101; i++ {
		big.VideoTrack = append(big.VideoTrack, vclip("c"+itoa(i), pV, "V1", 0, 0, 1))
	}
	if _, err := s.SaveProject(ctx, big); code(t, err) != apperr.InvalidArgument {
		t.Fatalf("101 个 clip 应拒绝: %v", err)
	}
	// 名称
	for _, n := range []string{"", "   ", strings.Repeat("字", 81)} {
		if _, err := s.SaveProject(ctx, EditProject{Name: n}); code(t, err) != apperr.InvalidArgument {
			t.Fatalf("名称 %q: %v", n, err)
		}
	}
	if _, err := s.SaveProject(ctx, EditProject{Name: strings.Repeat("字", 80)}); err != nil {
		t.Fatalf("80 字应可以: %v", err)
	}
	// schemaVersion > 1：Save 与 Load 都 UNSUPPORTED
	sv := EditProject{Name: "v2", SchemaVersion: 2}
	if _, err := s.SaveProject(ctx, sv); code(t, err) != apperr.Unsupported {
		t.Fatalf("%v", err)
	}
}

func TestLoadSchemaTooNew(t *testing.T) {
	s, st, _ := storeSvc(t)
	ctx := context.Background()
	if err := st.SaveEditProject(ctx, store.EditProjectRow{ID: "new1", Name: "n", Project: `{"schemaVersion":2,"name":"n"}`, UpdatedAt: 1}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadProject(ctx, "new1"); code(t, err) != apperr.Unsupported {
		t.Fatalf("%v", err)
	}
	if err := st.SaveEditProject(ctx, store.EditProjectRow{ID: "bad1", Name: "n", Project: `{not json`, UpdatedAt: 1}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadProject(ctx, "bad1"); code(t, err) != apperr.IOError {
		t.Fatalf("损坏的工程: %v", err)
	}
	// 旧版没写 schemaVersion（0）当 1
	st.SaveEditProject(ctx, store.EditProjectRow{ID: "old1", Name: "n", Project: `{"name":"n"}`, UpdatedAt: 1}, true)
	lp, err := s.LoadProject(ctx, "old1")
	if err != nil || lp.Project.SchemaVersion != 1 || lp.Project.VideoTrack == nil || lp.MissingPaths == nil {
		t.Fatalf("%+v %v", lp, err)
	}
}

func TestListProjectsLimit(t *testing.T) {
	s, _, _ := storeSvc(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		s.SaveProject(ctx, EditProject{Name: "p" + itoa(i)})
	}
	l, _ := s.ListProjects(ctx, 2)
	if len(l) != 2 {
		t.Fatal(len(l))
	}
	l, _ = s.ListProjects(ctx, 0)
	if len(l) != 3 {
		t.Fatal(len(l))
	}
}

func TestGetPreviewURL(t *testing.T) {
	dir := t.TempDir()
	reg := localassets.New(localassets.Config{})
	s := New(Config{Preview: reg})
	f := filepath.Join(dir, "a.MP4")
	os.WriteFile(f, make([]byte, 100), 0o644)
	u, err := s.GetPreviewURL(f)
	if err != nil || !strings.HasPrefix(u.URL, "/local/") || u.Mime != "video/mp4" || u.Size != 100 {
		t.Fatalf("%+v %v", u, err)
	}
	if u2, _ := s.GetPreviewURL(f); u2.URL != u.URL {
		t.Fatal("同一文件应复用 token")
	}
	if _, err := s.GetPreviewURL("rel.mp4"); code(t, err) != apperr.InvalidArgument {
		t.Fatal(err)
	}
	txt := filepath.Join(dir, "a.txt")
	os.WriteFile(txt, []byte("x"), 0o644)
	if _, err := s.GetPreviewURL(txt); code(t, err) != apperr.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := s.GetPreviewURL(filepath.Join(dir, "none.mp4")); code(t, err) != apperr.NotFound {
		t.Fatal(err)
	}
	d := filepath.Join(dir, "dir.mp4")
	os.Mkdir(d, 0o755)
	if _, err := s.GetPreviewURL(d); code(t, err) != apperr.InvalidArgument {
		t.Fatal(err)
	}
	// 与 Handler 打通：HEAD 200，删除文件后 404
	h := reg.Handler()
	rec := headReq(h, u.URL)
	if rec != 200 {
		t.Fatalf("HEAD %d", rec)
	}
	os.Remove(f)
	if rec := headReq(h, u.URL); rec != 404 {
		t.Fatalf("删除后 HEAD %d", rec)
	}
	// 没有 Preview 依赖：INTERNAL
	if _, err := New(Config{}).GetPreviewURL(f); code(t, err) != apperr.Internal {
		t.Fatal(err)
	}
}

func TestCleanupInterruptedParts(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	out := filepath.Join(dir, "o")
	os.MkdirAll(out, 0o755)
	final := filepath.Join(out, "cut.mp4")
	part := filepath.Join(out, "cut.part.mp4")
	part2 := filepath.Join(out, "cut(2).part.mp4")
	keep := filepath.Join(out, "cut.mp4") // 已完成的最终文件不能被删
	other := filepath.Join(out, "other.part.mp4")
	for _, f := range []string{part, part2, keep, other} {
		os.WriteFile(f, []byte("x"), 0o644)
	}
	mk := func(id string, typ store.TaskType, status store.TaskStatus, output string) {
		if err := st.InsertTask(ctx, store.Task{ID: id, Type: typ, Status: status, OutputPath: output, CreatedAt: 1, Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
	mk("t1", store.TypeEditExport, store.StatusInterrupted, final)
	mk("t2", store.TypeConvert, store.StatusInterrupted, filepath.Join(out, "other.mp4")) // 转换任务不归我管
	s := New(Config{Lister: st})
	n := s.CleanupInterruptedParts(ctx)
	if n != 2 {
		t.Fatalf("应删 2 个（part、part2），实际 %d", n)
	}
	for _, f := range []string{part, part2} {
		if _, err := os.Stat(f); err == nil {
			t.Fatalf("%s 应被删", f)
		}
	}
	for _, f := range []string{keep, other} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("%s 不应被删", f)
		}
	}
	// 只删普通文件：.part 位置是目录 / 符号链接时不动
	if runtime.GOOS != "windows" {
		os.Symlink(keep, part)
		if s.CleanupInterruptedParts(ctx) != 0 {
			t.Fatal("符号链接不应被删")
		}
		if _, err := os.Lstat(part); err != nil {
			t.Fatal("符号链接被删了")
		}
	}
}

func TestClassifyExportError(t *testing.T) {
	dep := "-filter_complex_script is deprecated, use -/filter_complex instead"
	// deprecated 行不参与分类；其他内容照旧
	if e := classifyExportError(dep+"\nError writing trailer: No space left on device\nError while writing", nil); e == nil || e.Code != apperr.ConvertDiskFull {
		t.Fatalf("%+v", e)
	}
	if e := classifyExportError(dep+"\nsome random failure", nil); e != nil {
		t.Fatalf("deprecated 行不应导致分类: %+v", e)
	}
	if e := classifyExportError("Unrecognized option 'filter_complex_script'.\nError splitting the argument list: Option not found", nil); e == nil || e.Code != apperr.Unsupported {
		t.Fatalf("%+v", e)
	}
	if e := classifyExportError("[AVFilterGraph @ 0x1] No such filter: 'foo'", nil); e == nil || e.Code != apperr.ProcessFailed {
		t.Fatalf("%+v", e)
	}
	if e := classifyExportError("/x/a.mp4: Invalid data found when processing input", nil); e == nil || e.Code != apperr.ProbeFailed {
		t.Fatalf("%+v", e)
	}
	// 文件名里带关键词不误判
	if e := classifyExportError("Input #0, mov, from '/x/Permission denied ENOSPC.mp4':\n  Metadata:\n    title : No space left on device\nsomething odd", nil); e != nil {
		t.Fatalf("%+v", e)
	}
}

// ffmpeg 不支持 -filter_complex_script：Export 提交前探测，返回 UNSUPPORTED（不落 PROCESS_FAILED），不产生任务。
func TestUnsupportedFilterScriptProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("用 shell 脚本伪造 ffmpeg")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "ffmpeg")
	os.WriteFile(fake, []byte("#!/bin/sh\necho \"Unrecognized option 'filter_complex_script'.\" >&2\necho 'Error splitting the argument list: Option not found' >&2\nexit 1\n"), 0o755)
	ft := &fakeTasks{}
	s := New(Config{Media: fakeMedia{base}, Tasks: ft, TempDir: dir,
		Require: func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{FFmpeg: fake, FFprobe: "y"}, nil }})
	out := t.TempDir()
	_, err := s.Export(context.Background(), proj(vclip("c1", pV, "V1", 0, 0, 1)), EditExportOptions{OutputDir: out})
	if code(t, err) != apperr.Unsupported || ft.n != 0 {
		t.Fatalf("%v n=%d", err, ft.n)
	}
	// 别的失败（如脚本语法错）不是 UNSUPPORTED
	os.WriteFile(fake, []byte("#!/bin/sh\necho 'boom' >&2\nexit 1\n"), 0o755)
	s2 := New(Config{Media: fakeMedia{base}, Tasks: ft, TempDir: dir,
		Require: func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{FFmpeg: fake, FFprobe: "y"}, nil }})
	_, err = s2.Export(context.Background(), proj(vclip("c1", pV, "V1", 0, 0, 1)), EditExportOptions{OutputDir: out})
	if code(t, err) != apperr.ProcessFailed {
		t.Fatalf("%v", err)
	}
	// 探测用的临时目录已清理
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "edit-probe-") {
			t.Fatal("探测临时目录未清理")
		}
	}
}

// 真 ffmpeg：探测成功且缓存。
func TestRealFilterScriptProbeCached(t *testing.T) {
	bin := realBins(t)
	s := New(Config{TempDir: t.TempDir()})
	if err := s.probeFilterScript(context.Background(), bin.FFmpeg); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.scriptOK.Load(bin.FFmpeg); !ok {
		t.Fatal("应缓存")
	}
	// 不存在的可执行文件 → 失败但不是 UNSUPPORTED
	err := s.probeFilterScript(context.Background(), filepath.Join(t.TempDir(), "nope"))
	if err == nil || apperr.Is(err, apperr.Unsupported) {
		t.Fatalf("%v", err)
	}
	_ = exec.ErrNotFound
	_ = errors.New
	_ = task.TypeEditExport
}

func TestFilterGraphContent(t *testing.T) {
	s := fakeSvc(base)
	a := vclip("c1", pV, "V1", 0, 0, 4)
	a.TransitionToNext, a.TransitionDurationSec, a.EffectPreset, a.Blur = "wipeleft", 0.7, "grayscale", 2
	b := vclip("c2", pV2, "V1", 4.1, 0, 3) // 间隙 0.1 → 首尾相接
	b.Speed = 2
	c := vclip("c3", pV, "V2", 6, 0, 2) // 覆盖层，有空隙
	p := proj(a, b, c)
	p.Output = EditOutput{Width: 641, Height: 361, Fps: 24}
	p.Effects = GlobalEffects{Brightness: 0.1, Sharpen: 1}
	p.AudioTrack = []AudioClip{{ID: "a1", Path: pA, TrackID: "A1", StartSec: 1.5, OutSec: 4, Speed: 4, Volume: 0.5}, aclip("a2", pA, "A2", 0, 1, 2)}
	pl, err := s.build(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	g := buildFilterGraph(pl)
	for _, want := range []string{
		"color=c=black:s=640x360:r=24:d=", // 向下取偶数
		"trim=start=0:end=4", "setpts=(PTS-STARTPTS)/2",
		"scale=640:360:force_original_aspect_ratio=decrease", "pad=640:360:(ow-iw)/2:(oh-ih)/2:black",
		"hue=s=0", "boxblur=2:1", "eq=brightness=0.1:contrast=1:saturation=1", "unsharp=5:5:1:5:5:0.0",
		"xfade=transition=wipeleft:duration=0.7:offset=3.3",
		"overlay=shortest=0:eof_action=pass:repeatlast=0", "[vout]",
		"atempo=2.0,atempo=2", "volume=0.5", "adelay=1500|1500", "amix=inputs=2:normalize=0:dropout_transition=0", "[aout]",
	} {
		if !strings.Contains(g, want) {
			t.Errorf("filtergraph 缺 %q\n%s", want, g)
		}
	}
	// 路径不出现在 filtergraph 里
	if strings.Contains(g, "/m/") {
		t.Errorf("filtergraph 不应含素材路径:\n%s", g)
	}
	// 无音轨 → anullsrc
	pl2, _ := s.build(context.Background(), proj(vclip("c1", pV, "V1", 0, 0, 2)))
	if g2 := buildFilterGraph(pl2); !strings.Contains(g2, "anullsrc") || strings.Contains(g2, "[0:a]") {
		t.Errorf("无音轨应只用 anullsrc:\n%s", g2)
	}
	// 首尾相接无转场 → concat
	pl3, _ := s.build(context.Background(), proj(vclip("c1", pV, "V1", 0, 0, 2), vclip("c2", pV2, "V1", 2.05, 0, 1)))
	if g3 := buildFilterGraph(pl3); !strings.Contains(g3, "concat=n=2:v=1:a=0") || strings.Contains(g3, "xfade") {
		t.Errorf("应 concat:\n%s", g3)
	}
	// 空隙 → 不 concat，各自 overlay 且平移
	pl4, _ := s.build(context.Background(), proj(vclip("c1", pV, "V1", 0, 0, 2), vclip("c2", pV2, "V1", 2.5, 0, 1)))
	if g4 := buildFilterGraph(pl4); strings.Contains(g4, "concat") || strings.Count(g4, "overlay=") != 2 || !strings.Contains(g4, "setpts=PTS+2.5/TB") {
		t.Errorf("空隙:\n%s", g4)
	}
}

func TestExportArgsUseScriptOnly(t *testing.T) {
	s := fakeSvc(base)
	pl, _ := s.build(context.Background(), proj(vclip("c1", pV, "V1", 0, 0, 2)))
	for _, f := range []string{"mp4", "mov", "mkv", "webm"} {
		pl.format = f
		args := exportArgs(pl, "/tmp/g.txt", "/o/x.part."+f)
		j := " " + strings.Join(args, " ") + " "
		if !strings.Contains(j, " -filter_complex_script /tmp/g.txt ") || strings.Contains(j, "-/filter_complex") || strings.Contains(j, " -filter_complex ") {
			t.Errorf("%s: %s", f, j)
		}
		if args[len(args)-1] != "file:/o/x.part."+f || !strings.Contains(j, " -i file:"+pV+" ") {
			t.Errorf("%s: 输入输出应带 file: 前缀: %s", f, j)
		}
		if f == "webm" && !strings.Contains(j, "libvpx-vp9") || f == "mp4" && !strings.Contains(j, "+faststart") || f == "mkv" && strings.Contains(j, "+faststart") {
			t.Errorf("%s: 编码参数: %s", f, j)
		}
	}
}

func TestFmtNum(t *testing.T) {
	for in, want := range map[float64]string{0: "0", 1: "1", 0.5: "0.5", 2.0000001: "2", 1e-9: "0", 1234.5678901: "1234.56789", -0.0000001: "0", 1e7: "10000000"} {
		if got := num(in); got != want {
			t.Errorf("num(%v)=%q want %q", in, got, want)
		}
	}
}
