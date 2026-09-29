package edit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/localassets"
	"FFmpegFree/internal/service/media"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

type rec struct {
	mu   sync.Mutex
	prog map[string][]task.ProgressEvent
}

func (r *rec) Emit(name string, p any) {
	if pe, ok := p.(task.ProgressEvent); ok {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.prog[pe.ID] = append(r.prog[pe.ID], pe)
	}
}
func (r *rec) get(id string) []task.ProgressEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]task.ProgressEvent(nil), r.prog[id]...)
}

type env struct {
	svc  *Service
	tm   *task.Manager
	st   *store.Store
	bin  ffmpeg.Binaries
	dir  string // 测试根目录（素材、输出都在里面）
	tmp  string // 任务临时目录
	em   *rec
	reg  *localassets.Registry
	defA string
}

func realBins(t *testing.T) ffmpeg.Binaries {
	t.Helper()
	// EDIT_TEST_FFMPEG_DIR：指向放着 ffmpeg / ffprobe 的目录，用来在别的 ffmpeg 版本（如 9.x）上跑整套真实导出测试。
	if d := os.Getenv("EDIT_TEST_FFMPEG_DIR"); d != "" {
		return ffmpeg.Binaries{FFmpeg: filepath.Join(d, "ffmpeg"), FFprobe: filepath.Join(d, "ffprobe")}
	}
	fm, err1 := exec.LookPath("ffmpeg")
	fp, err2 := exec.LookPath("ffprobe")
	if err1 != nil || err2 != nil {
		t.Skip("没有 ffmpeg / ffprobe，跳过真实 ffmpeg 测试")
	}
	return ffmpeg.Binaries{FFmpeg: fm, FFprobe: fp}
}

func newEnvWith(t *testing.T, bin ffmpeg.Binaries, conc int) *env {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &env{st: st, bin: bin, dir: dir, tmp: filepath.Join(dir, "tmp"), em: &rec{prog: map[string][]task.ProgressEvent{}}, reg: localassets.New(localassets.Config{})}
	os.MkdirAll(e.tmp, 0o755)
	e.tm = task.NewManager(task.Config{Store: st, Emitter: e.em, LogDir: filepath.Join(dir, "logs"), BatchConcurrency: conc,
		ProgressInterval: -1, Logf: func(string, ...any) {}})
	t.Cleanup(func() { e.tm.Shutdown(3 * time.Second) })
	req := func() (ffmpeg.Binaries, error) { return bin, nil }
	med := media.New(media.Config{Require: req, ThumbsDir: filepath.Join(dir, "thumbs")})
	e.svc = New(Config{Projects: st, Lister: st, Tasks: e.tm, Media: med, Preview: e.reg, Require: req,
		DefaultOutputDir: func(context.Context) string { return e.defA }, TempDir: e.tmp})
	return e
}

func newEnv(t *testing.T) *env { return newEnvWith(t, realBins(t), 2) }

func (e *env) gen(t *testing.T, out string, args ...string) string {
	t.Helper()
	full := append([]string{"-v", "error", "-y"}, args...)
	full = append(full, "file:"+out)
	if b, err := exec.Command(e.bin.FFmpeg, full...).CombinedOutput(); err != nil {
		t.Skipf("生成测试媒体失败（编码器缺失？）: %v\n%s", err, b)
	}
	return out
}

// 带音频的测试视频。
func (e *env) genVideo(t *testing.T, name string, secs int, size string) string {
	s := strconv.Itoa(secs)
	return e.gen(t, filepath.Join(e.dir, name), "-f", "lavfi", "-i", "testsrc=size="+size+":rate=25:duration="+s,
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+s, "-c:v", "mpeg4", "-c:a", "aac", "-shortest")
}

// 无音轨的测试视频。
func (e *env) genSilentVideo(t *testing.T, name string, secs int) string {
	return e.gen(t, filepath.Join(e.dir, name), "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration="+strconv.Itoa(secs), "-c:v", "mpeg4", "-an")
}

func (e *env) genAudio(t *testing.T, name string, secs int) string {
	return e.gen(t, filepath.Join(e.dir, name), "-f", "lavfi", "-i", "sine=frequency=880:duration="+strconv.Itoa(secs), "-c:a", "libmp3lame")
}

func (e *env) wait(t *testing.T, id string) task.Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tk, err := e.tm.Wait(ctx, id)
	if err != nil {
		t.Fatalf("等待任务: %v", err)
	}
	return tk
}

type probeResult struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Duration  string `json:"duration"`
	} `json:"streams"`
	Format struct {
		Duration   string `json:"duration"`
		FormatName string `json:"format_name"`
	} `json:"format"`
}

func (e *env) probe(t *testing.T, path string) probeResult {
	t.Helper()
	b, err := exec.Command(e.bin.FFprobe, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", "file:"+path).Output()
	if err != nil {
		t.Fatalf("输出文件无法被 ffprobe 解析: %v", err)
	}
	var r probeResult
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r probeResult) dur() float64 {
	f, _ := strconv.ParseFloat(r.Format.Duration, 64)
	return f
}
func (r probeResult) count(kind string) int {
	n := 0
	for _, s := range r.Streams {
		if s.CodecType == kind {
			n++
		}
	}
	return n
}

// vclip / aclip 生成默认合法的 clip（speed=1, volume=1）。
func vclip(id, path, track string, start, in, out float64) VideoClip {
	return VideoClip{ID: id, Path: path, TrackID: track, StartSec: start, InSec: in, OutSec: out, Speed: 1}
}
func aclip(id, path, track string, start, in, out float64) AudioClip {
	return AudioClip{ID: id, Path: path, TrackID: track, StartSec: start, InSec: in, OutSec: out, Speed: 1, Volume: 1}
}

func code(t *testing.T, err error) apperr.Code {
	t.Helper()
	ae := apperr.From(err)
	if ae == nil {
		t.Fatal("期望有错误")
	}
	return ae.Code
}

func firstLine(err error) string {
	ae := apperr.From(err)
	d := ae.Detail
	for i := 0; i < len(d); i++ {
		if d[i] == '\n' {
			return d[:i]
		}
	}
	return d
}

func headReq(h http.Handler, url string) int {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("HEAD", url, nil))
	return rec.Code
}
