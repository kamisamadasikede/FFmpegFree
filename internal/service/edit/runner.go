package edit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/task"
)

// exportRunner 写 filtergraph 临时文件、运行 ffmpeg（走 task.FFmpegRunner：.part + 原子提交、进度、取消、进程树回收），
// 结束后删除临时目录。
type exportRunner struct {
	s   *Service
	bin ffmpeg.Binaries
	pl  *plan
	out string
}

func (s *Service) newRunner(bin ffmpeg.Binaries, pl *plan, out string) task.Runner {
	return &exportRunner{s: s, bin: bin, pl: pl, out: out}
}

func (r *exportRunner) Run(ctx context.Context, report func(task.Progress)) (string, error) {
	tmp, err := os.MkdirTemp(r.s.cfg.TempDir, "edit-")
	if err != nil {
		return "", apperr.Wrap(apperr.IOError, "创建临时目录失败", err)
	}
	defer os.RemoveAll(tmp)
	script := filepath.Join(tmp, "graph.txt")
	if err := os.WriteFile(script, []byte(buildFilterGraph(r.pl)), 0o600); err != nil {
		return "", outputErr("写入滤镜脚本失败", err)
	}
	fr := &task.FFmpegRunner{
		Exe:         r.bin.FFmpeg,
		Output:      r.out,
		DurationSec: r.pl.duration,
		Classify:    classifyExportError,
		BuildArgs:   func(part string) []string { return exportArgs(r.pl, script, part) },
	}
	return fr.Run(ctx, report)
}

func outputErr(msg string, err error) error {
	return apperr.Wrap(apperr.IOError, msg, err)
}

// exportArgs 生成 ffmpeg 参数（不含 -y / -progress 等，由 ffmpeg.Run 添加）。filtergraph 只走 -filter_complex_script。
func exportArgs(pl *plan, script, part string) []string {
	var a []string
	// 每个 clip 一个输入，序号 = clip.idx（视频 clip 在前，音频 clip 在后）。
	all := make([]rclip, 0, len(pl.videos)+len(pl.audios))
	all = append(all, pl.videos...)
	all = append(all, pl.audios...)
	for _, c := range all {
		a = append(a, ffmpeg.ImagePatternArgs(c.path)...)
		a = append(a, "-i", "file:"+c.path)
	}
	a = append(a, "-filter_complex_script", script, "-map", "[vout]", "-map", "[aout]", "-t", num(pl.duration))
	if pl.format == "webm" {
		a = append(a, "-c:v", "libvpx-vp9", "-b:v", "2M", "-pix_fmt", "yuv420p", "-c:a", "libopus", "-b:a", "128k")
	} else {
		a = append(a, "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k")
		if pl.format == "mp4" {
			a = append(a, "-movflags", "+faststart")
		}
	}
	return append(a, "file:"+part)
}

// classifyExportError 在转换的分类上加两条：ffmpeg 不认识 -filter_complex_script → UNSUPPORTED；
// `-filter_complex_script is deprecated` 那一行（7.1.5 会打）不参与分类，但仍在 detail / 日志里。
func classifyExportError(tail string, exitErr error) *apperr.AppError {
	var kept []string
	for _, l := range strings.Split(tail, "\n") {
		if strings.Contains(strings.ToLower(l), "-filter_complex_script is deprecated") {
			continue
		}
		kept = append(kept, l)
	}
	filtered := strings.Join(kept, "\n")
	low := strings.ToLower(filtered)
	if strings.Contains(low, "unrecognized option 'filter_complex_script'") ||
		(strings.Contains(low, "option filter_complex_script not found")) {
		return apperr.New(apperr.Unsupported, "当前 ffmpeg 版本不支持 -filter_complex_script，无法导出多轨剪辑")
	}
	if strings.Contains(low, "invalid data found when processing input") || strings.Contains(low, "moov atom not found") {
		return apperr.New(apperr.ProbeFailed, "某个素材文件已损坏或不是有效的音视频文件")
	}
	return ffmpeg.ClassifyConvertError(filtered, exitErr)
}

var _ = fmt.Sprintf
