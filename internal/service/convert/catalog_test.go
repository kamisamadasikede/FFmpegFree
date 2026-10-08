package convert

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// ---------- 格式目录（契约 v0.24，6.16） ----------

// fakeCaps 按内置预设生成 -muxers / -encoders 的输出，去掉 dropMux / dropEnc 里的名字。
func fakeCaps(dropMux, dropEnc []string) (string, string) {
	mux, enc := map[string]bool{}, map[string]bool{}
	for _, p := range builtinPresets() {
		m, es := ffmpeg.RequiredEncoders(p.Options)
		mux[m] = true
		for _, e := range es {
			enc[e] = true
		}
	}
	for _, e := range []string{"libx264", "libx265", "aac", "libmp3lame", "libopus", "libvpx-vp9"} {
		enc[e] = true
	}
	for _, d := range dropMux {
		delete(mux, d)
	}
	for _, d := range dropEnc {
		delete(enc, d)
	}
	var m, e strings.Builder
	m.WriteString("File formats:\n D. = Demuxing supported\n .E = Muxing supported\n --\n D  aa              demux only\n")
	for k := range mux {
		fmt.Fprintf(&m, "  E %-15s some format\n", k)
	}
	e.WriteString("Encoders:\n V..... = Video\n A..... = Audio\n ------\n")
	for k := range enc {
		fmt.Fprintf(&e, " A..... %-20s some encoder\n", k)
	}
	return m.String(), e.String()
}

func catalogSvc(t *testing.T, probe capsProbe, ready bool) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	exe := filepath.Join(dir, "ffmpeg")
	os.WriteFile(exe, []byte("bin"), 0o755)
	tm := task.NewManager(task.Config{Store: st, LogDir: filepath.Join(dir, "logs"), ProgressInterval: -1, Logf: func(string, ...any) {}})
	t.Cleanup(func() { tm.Shutdown(time.Second) })
	req := func() (ffmpeg.Binaries, error) {
		if !ready {
			return ffmpeg.Binaries{}, apperr.New(apperr.FFmpegNotFound, "x")
		}
		return ffmpeg.Binaries{FFmpeg: exe, FFprobe: exe}, nil
	}
	svc, err := New(context.Background(), Config{Presets: st, Tasks: tm, Require: req, CapsProbe: probe})
	if err != nil {
		t.Fatal(err)
	}
	return svc, exe
}

func entryOf(cat []FormatEntry, ext string) FormatEntry {
	for _, e := range cat {
		if e.Extension == ext {
			return e
		}
	}
	return FormatEntry{}
}

func TestFormatCatalogProbed(t *testing.T) {
	var calls atomic.Int32
	mux, enc := fakeCaps([]string{"amr"}, []string{"wmav2"})
	probe := func(ctx context.Context, exe string) (string, string, error) { calls.Add(1); return mux, enc, nil }
	svc, exe := catalogSvc(t, probe, true)
	ctx := context.Background()
	cat, err := svc.GetFormatCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat) != 34 {
		t.Fatalf("34 种格式，实际 %d", len(cat))
	}
	n := map[string]int{}
	for _, e := range cat {
		n[e.Category]++
		if e.DefaultPresetID == "" || e.Aliases == nil || e.Presets == nil || e.DisplayName == "" {
			t.Fatalf("%+v", e)
		}
		found := false
		for _, p := range e.Presets {
			if p.ID == e.DefaultPresetID && p.BuiltIn && p.ParamsSummary != "" {
				found = true
			}
			if p.Options.Container != e.Extension {
				t.Fatalf("预设归错格式: %+v", p)
			}
		}
		if !found {
			t.Fatalf("%s 的默认预设不在 presets 里", e.Extension)
		}
		if strings.Contains(strings.ToLower(e.Reason), "ffmpeg") {
			t.Fatal("面向用户的文字不出现 ffmpeg")
		}
	}
	if n["video"]+n["audio"]+n["image"] != 34 || n["image"] == 0 {
		t.Fatalf("%v", n)
	}
	if mp4 := entryOf(cat, "mp4"); !mp4.Encodable || mp4.Reason != "" || mp4.ReasonCode != "" {
		t.Fatalf("%+v", mp4)
	}
	if amr := entryOf(cat, "amr"); amr.Encodable || amr.ReasonCode != ReasonMissingMuxer || amr.Reason != msgUnsupported {
		t.Fatalf("%+v", amr)
	}
	if wma := entryOf(cat, "wma"); wma.Encodable || wma.ReasonCode != ReasonMissingEncoder {
		t.Fatalf("%+v", wma)
	}
	gif := entryOf(cat, "gif")
	if gif.Category != "video" || !strings.Contains(strings.Join(gif.Aliases, ","), "动图") || !strings.Contains(strings.Join(gif.Aliases, ","), "图片") {
		t.Fatalf("GIF 留在视频类，别名含 动图 / 图片: %+v", gif)
	}
	for _, ext := range []string{"jpg", "png", "webp", "ico", "bmp", "tif", "tga"} {
		if e := entryOf(cat, ext); e.Category != "image" {
			t.Fatalf("%s: %+v", ext, e)
		}
	}
	// 缓存：同一个转换组件只检测一次；换了文件（大小 / 修改时间变了）重新检测
	svc.GetFormatCatalog(ctx)
	if calls.Load() != 1 {
		t.Fatalf("应缓存: %d", calls.Load())
	}
	os.WriteFile(exe, []byte("new binary"), 0o755)
	svc.GetFormatCatalog(ctx)
	if calls.Load() != 2 {
		t.Fatalf("换了转换组件应重新检测: %d", calls.Load())
	}
	// 用户预设出现在它的格式下，排在内置之后
	if _, err := svc.SavePreset(ctx, Preset{Name: "我的 MP4", Options: ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", Height: 720}}); err != nil {
		t.Fatal(err)
	}
	cat, _ = svc.GetFormatCatalog(ctx)
	ps := entryOf(cat, "mp4").Presets
	if last := ps[len(ps)-1]; last.BuiltIn || last.Name != "我的 MP4" || !ps[0].BuiltIn {
		t.Fatalf("%+v", ps)
	}
}

func TestFormatCatalogNotReadyAndProbeFailure(t *testing.T) {
	svc, _ := catalogSvc(t, func(context.Context, string) (string, string, error) {
		t.Fatal("没就绪不应检测")
		return "", "", nil
	}, false)
	cat, err := svc.GetFormatCatalog(context.Background())
	if err != nil || len(cat) != 34 {
		t.Fatalf("%v %d", err, len(cat))
	}
	for _, e := range cat {
		if e.Encodable || e.ReasonCode != ReasonConverterNotReady || e.Reason != msgNotReady {
			t.Fatalf("%+v", e)
		}
	}
	// 检测失败：全部 check_failed，结果不缓存（下次再试）
	var calls atomic.Int32
	svc2, _ := catalogSvc(t, func(context.Context, string) (string, string, error) {
		calls.Add(1)
		return "", "", errors.New("timeout")
	}, true)
	cat, _ = svc2.GetFormatCatalog(context.Background())
	for _, e := range cat {
		if e.Encodable || e.ReasonCode != ReasonCheckFailed || e.Reason != msgCheckFailed {
			t.Fatalf("%+v", e)
		}
	}
	svc2.GetFormatCatalog(context.Background())
	if calls.Load() != 2 {
		t.Fatalf("失败不缓存: %d", calls.Load())
	}
	// 检测失败时提交检查放行（v0.24.1 实现取舍）
	if err := svc2.checkFormat(context.Background(), ffmpeg.ConvertOptions{Container: "amr"}); err != nil {
		t.Fatal(err)
	}
}

// 6.16.6：格式不可输出 reason=format；格式可以但所选编码缺编码器 reason=encoder。
func TestCheckFormatOnSubmit(t *testing.T) {
	mux, enc := fakeCaps([]string{"amr"}, []string{"libx265"})
	svc, _ := catalogSvc(t, func(context.Context, string) (string, string, error) { return mux, enc, nil }, true)
	ctx := context.Background()
	err := svc.checkFormat(ctx, ffmpeg.ConvertOptions{Container: "amr"})
	if !apperr.Is(err, apperr.Unsupported) || detailOf(err) != "reason=format" || strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("%v", err)
	}
	err = svc.checkFormat(ctx, ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h265", AudioCodec: "aac"})
	if !apperr.Is(err, apperr.Unsupported) || detailOf(err) != "reason=encoder" {
		t.Fatalf("%v", err)
	}
	if err := svc.checkFormat(ctx, ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}); err != nil {
		t.Fatal(err)
	}
}

func TestParseCaps(t *testing.T) {
	m := parseMuxers("File formats:\n D. = Demuxing supported\n .E = Muxing supported\n --\n  E mp4             MP4\n D  aac             raw ADTS\n DE matroska,webm   Matroska\n")
	if !m["mp4"] || m["aac"] || !m["matroska"] || !m["webm"] || m["="] {
		t.Fatalf("%v", m)
	}
	e := parseAllEncoders("Encoders:\n V..... = Video\n ------\n V....D libx264              H.264\n A..... aac                  AAC\n")
	if !e["libx264"] || !e["aac"] || e["="] {
		t.Fatalf("%v", e)
	}
}
