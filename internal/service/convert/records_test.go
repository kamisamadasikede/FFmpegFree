package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/localassets"
	"FFmpegFree/internal/task"
)

func detailOf(err error) string {
	var ae *apperr.AppError
	if errors.As(err, &ae) {
		return ae.Detail
	}
	return ""
}

func wantReason(t *testing.T, err error, code apperr.Code, detail string) {
	t.Helper()
	if !apperr.Is(err, code) || detailOf(err) != detail {
		t.Fatalf("want %s %q, got %v (detail %q)", code, detail, err, detailOf(err))
	}
}

func (e *env) addSource(t *testing.T, p string) ConvertSource {
	t.Helper()
	res, err := e.svc.AddSources(context.Background(), []string{p})
	if err != nil || res[0].Error != nil || res[0].Source == nil {
		t.Fatalf("%+v %v", res, err)
	}
	return *res[0].Source
}

func (e *env) submitSources(t *testing.T, req ConvertSubmitRequest) []task.Task {
	t.Helper()
	ts, err := e.svc.SubmitSources(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for i := range ts {
		ts[i] = e.wait(t, ts[i].ID)
	}
	return ts
}

func snapshot(t *testing.T, tk task.Task) params {
	t.Helper()
	var p params
	if err := json.Unmarshal([]byte(tk.Params), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// 契约 v0.23.1：内置预设记录 presetId / presetName 非空、paramsSummary 有值；自定义参数 presetId / presetName 为 ""、paramsSummary 有值；
// v0.23 之前的旧记录三个键都没有（前端退回显示 title）。
func TestPresetSnapshotRules(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := e.genVideo(t, filepath.Join(e.dir, "a.mp4"), 1)
	src := e.addSource(t, in)
	out := filepath.Join(e.dir, "o")
	ps, _ := e.svc.ListPresets(ctx)
	var p720 Preset
	for _, p := range ps {
		if p.ID == "builtin-mp4-h264-720p" {
			p720 = p
		}
	}
	got := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{src.SourceID}, Options: p720.Options, OutputDir: out, PresetID: p720.ID})
	s := snapshot(t, got[0])
	if s.PresetID != p720.ID || s.PresetName != p720.Name || s.ParamsSummary != "H.264 · 720p" || got[0].SourceID != src.SourceID {
		t.Fatalf("预设记录: %+v", s)
	}
	custom := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{src.SourceID},
		Options: ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}, OutputDir: out})
	s = snapshot(t, custom[0])
	if s.PresetID != "" || s.PresetName != "" || s.ParamsSummary != "原画质" {
		t.Fatalf("自定义参数记录: %+v", s)
	}
	if !strings.Contains(custom[0].Params, `"presetId":""`) || !strings.Contains(custom[0].Params, `"presetName":""`) {
		t.Fatalf("自定义参数也写这两个键（值为空串）: %s", custom[0].Params)
	}
	// 兼容的 Submit：同样写快照，presetId / presetName 为空
	ts := mustSubmit(t, e, []string{in}, ffmpeg.ConvertOptions{Container: "mp3", AudioCodec: "mp3", AudioBitrate: 192000}, out)
	s = snapshot(t, e.wait(t, ts[0].ID))
	if s.PresetID != "" || s.PresetName != "" || s.ParamsSummary != "192 kbps" || ts[0].SourceID != src.SourceID {
		t.Fatalf("Submit: %+v %s", s, ts[0].SourceID)
	}
	// 不存在的预设
	_, err := e.svc.SubmitSources(ctx, ConvertSubmitRequest{SourceIDs: []string{src.SourceID}, Options: p720.Options, PresetID: "nope"})
	wantReason(t, err, apperr.NotFound, "reason=record")
	// 旧记录：params 里没有这三个键
	var legacy params
	json.Unmarshal([]byte(`{"input":"/a.mp4","options":{"container":"mp4"},"outputDir":""}`), &legacy)
	if legacy.PresetID != "" || legacy.PresetName != "" || legacy.ParamsSummary != "" {
		t.Fatalf("%+v", legacy)
	}
}

func TestGetSourceMatchesListSourcesItem(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "a.mp4"), 1))
	b := e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "b.mp4"), 1))
	o := ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}
	for i := 0; i < 3; i++ {
		e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{a.SourceID}, Options: o, OutputDir: filepath.Join(e.dir, "o")})
	}
	page, err := e.svc.ListSources(ctx, ConvertSourceFilter{})
	if err != nil || page.Total != 2 || page.Items[0].Source.SourceID != a.SourceID {
		t.Fatalf("%+v %v", page, err)
	}
	got, err := e.svc.GetSource(ctx, a.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	want := page.Items[0]
	gj, _ := json.Marshal(got)
	wj, _ := json.Marshal(want)
	if string(gj) != string(wj) {
		t.Fatalf("GetSource 应与 ListSources 的一项完全相同:\n%s\n%s", gj, wj)
	}
	if got.RecordCount != 3 || len(got.Records) != 3 || got.Records[0].CreatedAt < got.Records[2].CreatedAt {
		t.Fatalf("%+v", got)
	}
	if eb, _ := e.svc.GetSource(ctx, b.SourceID); eb.RecordCount != 0 || len(eb.Records) != 0 || eb.Records == nil {
		t.Fatalf("没有记录的行: %+v", eb)
	}
	_, err = e.svc.GetSource(ctx, "nope")
	wantReason(t, err, apperr.NotFound, "reason=record")
}

func TestSearchSources(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	hol := e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "Holiday Trip.mp4"), 1))
	work := e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "work.mp4"), 1))
	e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "misc.mp4"), 1))
	o := ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}
	recs := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{work.SourceID}, Options: o, OutputDir: filepath.Join(e.dir, "TRIP-out")})
	// 输出目录名不算，只看输出文件名：work.mkv 不含 trip
	page, err := e.svc.SearchSources(ctx, ConvertSearchFilter{Keyword: "  TRIP "})
	if err != nil || page.Total != 1 || page.Items[0].Source.SourceID != hol.SourceID || !page.Items[0].NameMatched || len(page.Items[0].MatchedTaskIDs) != 0 {
		t.Fatalf("%+v %v", page, err)
	}
	page, _ = e.svc.SearchSources(ctx, ConvertSearchFilter{Keyword: "WORK.MKV"})
	if page.Total != 1 || page.Items[0].Source.SourceID != work.SourceID || page.Items[0].NameMatched ||
		len(page.Items[0].MatchedTaskIDs) != 1 || page.Items[0].MatchedTaskIDs[0] != recs[0].ID {
		t.Fatalf("输出文件名命中: %+v", page)
	}
	page, _ = e.svc.SearchSources(ctx, ConvertSearchFilter{Keyword: ".mp4"})
	if page.Total != 3 {
		t.Fatalf("%+v", page)
	}
	if page, _ := e.svc.SearchSources(ctx, ConvertSearchFilter{Keyword: "zzz"}); page.Total != 0 || page.Items == nil {
		t.Fatalf("%+v", page)
	}
	for _, kw := range []string{"", "   ", strings.Repeat("字", 101)} {
		if _, err := e.svc.SearchSources(ctx, ConvertSearchFilter{Keyword: kw}); !apperr.Is(err, apperr.InvalidArgument) {
			t.Fatalf("%q: %v", kw, err)
		}
	}
}

func TestRevealRecord(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	var revealed []string
	e.svc.cfg.Reveal = func(p string) error { revealed = append(revealed, p); return nil }
	src := e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "a.mp4"), 1))
	rec := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{src.SourceID},
		Options: ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}, OutputDir: filepath.Join(e.dir, "o")})[0]
	if err := e.svc.RevealRecord(ctx, rec.ID); err != nil || len(revealed) != 1 || revealed[0] != rec.OutputPath {
		t.Fatalf("%v %v", err, revealed)
	}
	wantReason(t, e.svc.RevealRecord(ctx, "nope"), apperr.NotFound, "reason=record")
	os.Remove(rec.OutputPath)
	wantReason(t, e.svc.RevealRecord(ctx, rec.ID), apperr.NotFound, "reason=file")
	// 非转换任务
	other, _ := e.tm.Submit(task.Spec{Type: task.TypeEditExport}, task.RunnerFunc(func(context.Context, func(task.Progress)) (string, error) {
		return rec.OutputPath, nil
	}))
	e.wait(t, other.ID)
	if err := e.svc.RevealRecord(ctx, other.ID); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
	if len(revealed) != 1 {
		t.Fatal("出错时不应调用文件管理器")
	}
	// RevealSource：源文件
	if err := e.svc.RevealSource(ctx, src.SourceID); err != nil || revealed[1] != src.Path {
		t.Fatalf("%v %v", err, revealed)
	}
}

func TestPreviewAndOpenWhitelist(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.svc.cfg.Preview = localassets.New(localassets.Config{})
	var opened []string
	e.svc.cfg.Open = func(p string) error { opened = append(opened, p); return nil }
	in := e.genVideo(t, filepath.Join(e.dir, "a.mp4"), 1)
	src := e.addSource(t, in)
	gif := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{src.SourceID},
		Options: ffmpeg.ConvertOptions{Container: "gif", Width: 80, Fps: 5}, OutputDir: filepath.Join(e.dir, "o")})[0]
	u, err := e.svc.TaskPreviewURL(ctx, gif.ID, "output")
	if err != nil || !strings.HasPrefix(u.URL, "/local/") || u.Mime != "image/gif" || u.Size <= 0 {
		t.Fatalf("gif 可以预览: %+v %v", u, err)
	}
	if u, err := e.svc.GetSourcePreviewURL(ctx, src.SourceID); err != nil || !strings.HasPrefix(u.URL, "/local/") {
		t.Fatalf("%+v %v", u, err)
	}
	opus := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{src.SourceID},
		Options: ffmpeg.ConvertOptions{Container: "opus", AudioCodec: "opus"}, OutputDir: filepath.Join(e.dir, "o")})
	if opus[0].Status == task.StatusSucceeded {
		_, err := e.svc.TaskPreviewURL(ctx, opus[0].ID, "output")
		wantReason(t, err, apperr.Unsupported, "reason=format")
		if err := e.svc.TaskOpenWithSystem(ctx, opus[0].ID, "output"); err != nil || opened[len(opened)-1] != opus[0].OutputPath {
			t.Fatalf("opus 可以用系统程序打开: %v", err)
		}
	}
	// 白名单本身
	for _, c := range []struct {
		ext           string
		preview, open bool
	}{{".gif", true, true}, {".MP4", true, true}, {".opus", false, true}, {".wmv", false, true}, {".txt", false, false}, {".exe", false, false}} {
		if hasExt(previewExts, "x"+c.ext) != c.preview || hasExt(openExts, "x"+c.ext) != c.open {
			t.Errorf("%s", c.ext)
		}
	}
	// 不在系统打开白名单里：UNSUPPORTED reason=format，不调用 Open
	n := len(opened)
	txt := filepath.Join(e.dir, "notes.txt")
	os.WriteFile(txt, []byte("x"), 0o644)
	tsrc := e.addSource(t, txt)
	wantReason(t, e.svc.OpenSourceWithSystem(ctx, tsrc.SourceID), apperr.Unsupported, "reason=format")
	if len(opened) != n {
		t.Fatal("不应调用 Open")
	}
	_, err = e.svc.GetSourcePreviewURL(ctx, tsrc.SourceID)
	wantReason(t, err, apperr.Unsupported, "reason=format")
	// 缩略图：不是音视频扩展名 UNSUPPORTED；文件不在 reason=file；不存在的行 reason=record
	_, err = e.svc.GetSourceThumbnail(ctx, tsrc.SourceID)
	wantReason(t, err, apperr.Unsupported, "reason=format")
	if u, err := e.svc.GetSourceThumbnail(ctx, src.SourceID); err != nil || !strings.HasPrefix(u, "data:image/jpeg;base64,") {
		t.Fatalf("%v", err)
	}
	if u, err := e.svc.GetRecordThumbnail(ctx, gif.ID); err != nil || !strings.HasPrefix(u, "data:image/jpeg;base64,") {
		t.Fatalf("gif 输出可以出缩略图: %v", err)
	}
	// 失败写应用日志（包 19 Windows：之前失败一行记录都没有）：哪种缩略图、id、路径、错误码
	var logBuf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(oldOut)
	os.Remove(in)
	_, err = e.svc.GetSourceThumbnail(ctx, src.SourceID)
	wantReason(t, err, apperr.NotFound, "reason=file")
	_, err = e.svc.GetSourceThumbnail(ctx, "nope")
	wantReason(t, err, apperr.NotFound, "reason=record")
	for _, want := range []string{"缩略图: 转换页 source=" + src.SourceID + ` path="` + in + `" code=NOT_FOUND`, `detail="reason=file"`, "source=nope", `detail="reason=record"`} {
		if !strings.Contains(logBuf.String(), want) {
			t.Errorf("日志缺少 %q:\n%s", want, logBuf.String())
		}
	}
	wantReason(t, e.svc.OpenSourceWithSystem(ctx, src.SourceID), apperr.NotFound, "reason=file")
}

func TestDeleteSourceAndReconvert(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := e.genVideo(t, filepath.Join(e.dir, "a.mp4"), 1)
	src := e.addSource(t, in)
	o := ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}
	out := filepath.Join(e.dir, "o")
	r1 := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{src.SourceID}, Options: o, OutputDir: out})[0]
	if r1.OutputPath != filepath.Join(out, "a.mkv") || r1.Result == nil || r1.Result.SizeBytes <= 0 || r1.Result.DurationSec <= 0 {
		t.Fatalf("%+v %+v", r1, r1.Result)
	}
	// 又转一次：新 id、带空格的序号、快照照抄
	r2, err := e.svc.Reconvert(ctx, r1.ID)
	if err != nil || r2.ID == r1.ID || r2.OutputPath != filepath.Join(out, "a (1).mkv") || r2.SourceID != src.SourceID {
		t.Fatalf("%+v %v", r2, err)
	}
	if s1, s2 := snapshot(t, r1), snapshot(t, r2); s1.ParamsSummary != s2.ParamsSummary || s1.PresetID != s2.PresetID {
		t.Fatalf("%+v %+v", s1, s2)
	}
	r2 = e.wait(t, r2.ID)
	// 非 succeeded 不能再转一次
	e.tm.Cancel(r2.ID)
	failedOut := filepath.Join(e.dir, "blocker")
	os.WriteFile(failedOut, []byte("x"), 0o644)
	bad := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{src.SourceID}, Options: o, OutputDir: filepath.Join(failedOut, "sub")})[0]
	if bad.Status != task.StatusFailed {
		t.Fatalf("%+v", bad)
	}
	if _, err := e.svc.Reconvert(ctx, bad.ID); !apperr.Is(err, apperr.TaskConflict) {
		t.Fatalf("%v", err)
	}
	if _, err := e.svc.Reconvert(ctx, "nope"); detailOf(err) != "reason=record" {
		t.Fatalf("%v", err)
	}
	// DeleteRecords：只删一条
	res, err := e.svc.DeleteRecords(ctx, []string{r1.ID}, true)
	if err != nil || len(res.DeletedTaskIDs) != 1 || res.DeletedFiles != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := e.svc.DeleteRecords(ctx, nil, true); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
	// DeleteSource：删掉剩下的记录后删行；源文件不碰
	res, err = e.svc.DeleteSource(ctx, src.SourceID, false)
	if err != nil || len(res.DeletedTaskIDs) != 2 || len(res.DeletedSourceIDs) != 1 || res.DeletedSourceIDs[0] != src.SourceID {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := os.Stat(in); err != nil {
		t.Fatal("源文件永远不删")
	}
	if _, err := os.Stat(r2.OutputPath); err != nil {
		t.Fatal("deleteOutputs=false 不删输出")
	}
	_, err = e.svc.GetSource(ctx, src.SourceID)
	wantReason(t, err, apperr.NotFound, "reason=record")
	_, err = e.svc.DeleteSource(ctx, src.SourceID, false)
	wantReason(t, err, apperr.NotFound, "reason=record")
}

func TestAddSourcesAndCheckSources(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := e.genVideo(t, filepath.Join(e.dir, "a.mp4"), 1)
	res, err := e.svc.AddSources(ctx, []string{in, in, "rel/path.mp4", filepath.Join(e.dir, "nope.mp4"), e.dir})
	if err != nil || len(res) != 5 {
		t.Fatalf("%+v %v", res, err)
	}
	if res[0].Source == nil || res[0].Existed || res[1].Source == nil || !res[1].Existed || res[1].Source.SourceID != res[0].Source.SourceID {
		t.Fatalf("同一路径只建一行: %+v %+v", res[0], res[1])
	}
	if res[2].Error == nil || res[2].Error.Code != apperr.InvalidArgument {
		t.Fatalf("%+v", res[2])
	}
	if res[3].Error == nil || res[3].Error.Code != apperr.NotFound || res[3].Error.Detail != "reason=file" {
		t.Fatalf("%+v", res[3])
	}
	if res[4].Error == nil || res[4].Error.Code != apperr.InvalidArgument {
		t.Fatalf("%+v", res[4])
	}
	id := res[0].Source.SourceID
	chk, err := e.svc.CheckSources(ctx, []string{id, "nope"})
	if err != nil || !chk[0].Found || !chk[0].Exists || chk[1].Found || chk[1].Exists {
		t.Fatalf("%+v %v", chk, err)
	}
	// 将保存为：此刻会用的名字（不占位）
	o := ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264"}
	if p, err := e.svc.PreviewOutputName(ctx, id, o, ""); err != nil || p != filepath.Join(e.dir, "a (1).mp4") {
		t.Fatalf("源文件同目录且与源同名 → 顺延: %s %v", p, err)
	}
	os.Remove(in)
	if chk, _ := e.svc.CheckSources(ctx, []string{id}); !chk[0].Found || chk[0].Exists {
		t.Fatalf("%+v", chk)
	}
}

// 契约 v0.23.1：ListSources 的 status 只筛行，每行内嵌的记录和 recordCount 不受影响。
func TestListSourcesStatusFilter(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "a.mp4"), 1))
	b := e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "b.mp4"), 1))
	c := e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "c.mp4"), 1))
	o := ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}
	out := filepath.Join(e.dir, "o")
	ra := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{a.SourceID, a.SourceID}, Options: o, OutputDir: out})
	markFailed(t, e.st, ra[0].ID) // a：一条失败 + 一条成功
	rb := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{b.SourceID}, Options: o, OutputDir: out})
	tk, _ := e.st.GetTask(ctx, rb[0].ID) // b：只有一条已取消（不算 failed）
	tk.Status, tk.Version = task.StatusCanceled, tk.Version+1
	e.st.UpdateTask(ctx, tk)
	// c：一条排队中的记录（直接落库模拟）
	if err := e.st.InsertTask(ctx, task.Task{ID: "q1", Type: task.TypeConvert, Status: task.StatusQueued, SourceID: c.SourceID, CreatedAt: 1, Version: 1}); err != nil {
		t.Fatal(err)
	}
	ids := func(p ConvertSourcePage) []string {
		var out []string
		for _, it := range p.Items {
			out = append(out, it.Source.SourceID)
		}
		return out
	}
	all, err := e.svc.ListSources(ctx, ConvertSourceFilter{})
	if err != nil || all.Total != 3 {
		t.Fatalf("%+v %v", all, err)
	}
	failed, err := e.svc.ListSources(ctx, ConvertSourceFilter{Status: "failed"})
	if err != nil || failed.Total != 1 || ids(failed)[0] != a.SourceID {
		t.Fatalf("failed: %v %v", ids(failed), err)
	}
	if it := failed.Items[0]; it.RecordCount != 2 || len(it.Records) != 2 {
		t.Fatalf("内嵌记录不按状态筛: %+v", it)
	}
	active, _ := e.svc.ListSources(ctx, ConvertSourceFilter{Status: "active"})
	if active.Total != 1 || ids(active)[0] != c.SourceID {
		t.Fatalf("active: %v", ids(active))
	}
	paged, _ := e.svc.ListSources(ctx, ConvertSourceFilter{Status: "", Limit: 1, Offset: 1})
	if paged.Total != 3 || len(paged.Items) != 1 || paged.Items[0].Source.SourceID != all.Items[1].Source.SourceID {
		t.Fatalf("%+v", paged)
	}
	for _, bad := range []string{"canceled", "ACTIVE", "all"} {
		if _, err := e.svc.ListSources(ctx, ConvertSourceFilter{Status: bad}); !apperr.Is(err, apperr.InvalidArgument) {
			t.Fatalf("%q: %v", bad, err)
		}
	}
}

// 契约 v0.23.2：SearchSources 的 status 与 ListSources 完全相同，与关键字是 AND，只筛行。
func TestSearchSourcesStatusFilter(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "trip a.mp4"), 1))
	b := e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "trip b.mp4"), 1))
	c := e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "trip c.mp4"), 1))
	d := e.addSource(t, e.genVideo(t, filepath.Join(e.dir, "other.mp4"), 1))
	o := ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}
	out := filepath.Join(e.dir, "o")
	ra := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{a.SourceID, a.SourceID}, Options: o, OutputDir: out})
	markFailed(t, e.st, ra[0].ID) // a：失败 + 成功
	rb := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{b.SourceID}, Options: o, OutputDir: out})
	tk, _ := e.st.GetTask(ctx, rb[0].ID) // b：只有已取消
	tk.Status, tk.Version = task.StatusCanceled, tk.Version+1
	e.st.UpdateTask(ctx, tk)
	for i, src := range []ConvertSource{c, d} { // c、d：各一条排队中
		if err := e.st.InsertTask(ctx, task.Task{ID: "q" + string(rune('0'+i)), Type: task.TypeConvert, Status: task.StatusQueued, SourceID: src.SourceID, CreatedAt: 1, Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(p ConvertSourcePage) string {
		var out []string
		for _, it := range p.Items {
			out = append(out, it.Source.Name)
		}
		return strings.Join(out, ",")
	}
	all, err := e.svc.SearchSources(ctx, ConvertSearchFilter{Keyword: "trip"})
	if err != nil || all.Total != 3 {
		t.Fatalf("%s %v", ids(all), err)
	}
	failed, err := e.svc.SearchSources(ctx, ConvertSearchFilter{Keyword: "trip", Status: "failed"})
	if err != nil || failed.Total != 1 || ids(failed) != "trip a.mp4" {
		t.Fatalf("failed（canceled 不算）: %s %v", ids(failed), err)
	}
	if it := failed.Items[0]; it.RecordCount != 2 || len(it.Records) != 2 || !it.NameMatched {
		t.Fatalf("内嵌记录不按状态筛: %+v", it)
	}
	active, _ := e.svc.SearchSources(ctx, ConvertSearchFilter{Keyword: "TRIP", Status: "active"})
	if active.Total != 1 || ids(active) != "trip c.mp4" {
		t.Fatalf("active 与关键字 AND（other.mp4 也在排队但不命中）: %s", ids(active))
	}
	// 分页 + 筛选：total 是筛选后的行数，顺序同不筛时
	p1, _ := e.svc.SearchSources(ctx, ConvertSearchFilter{Keyword: "trip", Limit: 2})
	p2, _ := e.svc.SearchSources(ctx, ConvertSearchFilter{Keyword: "trip", Limit: 2, Offset: 2, Status: ""})
	if p1.Total != 3 || p2.Total != 3 || ids(p1)+","+ids(p2) != ids(all) {
		t.Fatalf("%s | %s | %s", ids(p1), ids(p2), ids(all))
	}
	if pf, _ := e.svc.SearchSources(ctx, ConvertSearchFilter{Keyword: "trip", Status: "failed", Limit: 1, Offset: 1}); pf.Total != 1 || len(pf.Items) != 0 {
		t.Fatalf("%+v", pf)
	}
	for _, bad := range []string{"canceled", "Failed", "all"} {
		if _, err := e.svc.SearchSources(ctx, ConvertSearchFilter{Keyword: "trip", Status: bad}); !apperr.Is(err, apperr.InvalidArgument) {
			t.Fatalf("%q: %v", bad, err)
		}
	}
}
