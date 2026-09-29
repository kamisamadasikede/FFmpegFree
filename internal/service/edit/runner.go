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
	// 编码器在提交 / 重试时解析一次（契约 9.7）：hw 非空 = 用该硬件 H.264 编码器；info 是任务开始时的编码器信息，cpuInfo 是回退 CPU 后的。
	hw      string
	info    ffmpeg.EncoderInfo
	cpuInfo ffmpeg.EncoderInfo
}

func (s *Service) newRunner(bin ffmpeg.Binaries, pl *plan, out string) task.Runner {
	// mp4 / mov / mkv 等非 webm 导出是 libx264 重编码 → 可以用硬件；webm 是 VP9，走 CPU。
	cpuInfo := ffmpeg.EncoderInfo{Encoder: "libx264", Device: "cpu"}
	r := &exportRunner{s: s, bin: bin, pl: pl, out: out, cpuInfo: cpuInfo, info: cpuInfo}
	if pl.format == "webm" {
		r.info = ffmpeg.EncoderInfo{Encoder: "libvpx-vp9", Device: "cpu"}
		r.cpuInfo = r.info
		return r
	}
	r.hw, r.info = ffmpeg.DecideEncoding(context.Background(), s.cfg.Encoder, "h264", ffmpeg.HWDimsOK("h264", pl.w, pl.h))
	return r
}

// EncoderInfo 实现 task.EncoderReporter。
func (r *exportRunner) EncoderInfo() ffmpeg.EncoderInfo { return r.info }

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
		BuildArgs:   func(part string) []string { return exportArgsHW(r.pl, script, part, r.hw) },
		Encoding:    r.info,
	}
	if r.hw != "" {
		fr.HWEncoder, fr.CPUEncoding = r.hw, r.cpuInfo
		fr.BuildCPUArgs = func(part string) []string { return exportArgsHW(r.pl, script, part, "") }
	}
	return fr.Run(ctx, report)
}

func outputErr(msg string, err error) error {
	return apperr.Wrap(apperr.IOError, msg, err)
}

// exportArgs 生成 ffmpeg 参数（不含 -y / -progress 等，由 ffmpeg.Run 添加）。filtergraph 写文件，用探测选中的选项（pl.filterOpt：-/filter_complex 或 -filter_complex_script）传入，不走命令行。
//
// hw 非空时视频用该硬件 H.264 编码器（画质对应 x264 CRF 20，见 ffmpeg.HWRateArgs），否则 libx264；webm 恒用 CPU 的 VP9。
func exportArgs(pl *plan, script, part string) []string { return exportArgsHW(pl, script, part, "") }

// exportArgsHW 同 exportArgs，hw 非空时 H.264 视频用该硬件编码器。
func exportArgsHW(pl *plan, script, part, hw string) []string {
	var a []string
	// 每个 clip 一个输入，序号 = clip.idx（视频 clip 在前，音频 clip 在后）。
	all := make([]rclip, 0, len(pl.videos)+len(pl.audios))
	all = append(all, pl.videos...)
	all = append(all, pl.audios...)
	for _, c := range all {
		a = append(a, ffmpeg.ImagePatternArgs(c.path)...)
		a = append(a, "-i", "file:"+c.path)
	}
	opt := pl.filterOpt
	if opt == "" {
		opt = OptFilterFile
	}
	a = append(a, opt, script, "-map", "[vout]", "-map", "[aout]", "-t", num(pl.duration))
	if pl.format == "webm" {
		a = append(a, "-c:v", "libvpx-vp9", "-b:v", "2M", "-pix_fmt", "yuv420p", "-c:a", "libopus", "-b:a", "128k")
	} else {
		if hw != "" {
			a = append(a, ffmpeg.HWRateArgs(hw, "h264", 20, 0)...)
		} else {
			a = append(a, "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p")
		}
		a = append(a, "-c:a", "aac", "-b:a", "192k")
		if pl.format == "mp4" {
			a = append(a, "-movflags", "+faststart")
		}
	}
	return append(a, "file:"+part)
}

// classifyExportError 在转换的分类上加两条：ffmpeg 不认识 -/filter_complex 或 -filter_complex_script → UNSUPPORTED；
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
	if strings.Contains(low, "unrecognized option 'filter_complex_script'") || strings.Contains(low, "unrecognized option '/filter_complex'") ||
		strings.Contains(low, "option filter_complex_script not found") || strings.Contains(low, "option /filter_complex not found") {
		return apperr.New(apperr.Unsupported, "当前 ffmpeg 版本不支持从文件读取滤镜图，无法导出多轨剪辑").WithDetail("project\nmissing=filter_complex")
	}
	if strings.Contains(low, "invalid data found when processing input") || strings.Contains(low, "moov atom not found") {
		return apperr.New(apperr.ProbeFailed, "某个素材文件已损坏或不是有效的音视频文件")
	}
	return ffmpeg.ClassifyConvertError(filtered, exitErr)
}

var _ = fmt.Sprintf
