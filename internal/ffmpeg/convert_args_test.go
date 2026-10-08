package ffmpeg

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
)

var vsrc = ConvertSource{DurationSec: 10, HasVideo: true, VideoIndex: 0, HasAudio: true}

func plan(t *testing.T, o ConvertOptions, src ConvertSource) ConvertPlan {
	t.Helper()
	p, err := PlanConvert("/in/a.mp4", "/out/a.part.x", o, src)
	if err != nil {
		t.Fatalf("PlanConvert(%+v): %v", o, err)
	}
	return p
}

func TestPlanConvertTable(t *testing.T) {
	cases := []struct {
		name string
		o    ConvertOptions
		src  ConvertSource
		want []string
	}{
		{"mp4 默认 h264+aac", ConvertOptions{Container: "mp4", VideoCodec: "h264"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-map", "0:a?", "-sn", "-dn",
			"-vf", "crop=trunc(iw/2)*2:trunc(ih/2)*2", "-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "192000", "-map_metadata", "0", "-movflags", "+faststart", "file:/out/a.part.x"}},
		{"h265 mp4 带 hvc1", ConvertOptions{Container: "mp4", VideoCodec: "h265", Crf: 30, AudioCodec: "copy"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-map", "0:a?", "-sn", "-dn",
			"-vf", "crop=trunc(iw/2)*2:trunc(ih/2)*2", "-c:v", "libx265", "-preset", "medium", "-crf", "30", "-pix_fmt", "yuv420p", "-x265-params", "log-level=error", "-tag:v", "hvc1",
			"-c:a", "copy", "-map_metadata", "0", "-movflags", "+faststart", "file:/out/a.part.x"}},
		{"h265 mkv 不加 hvc1", ConvertOptions{Container: "mkv", VideoCodec: "h265"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-map", "0:a?", "-sn", "-dn",
			"-vf", "crop=trunc(iw/2)*2:trunc(ih/2)*2", "-c:v", "libx265", "-preset", "medium", "-crf", "28", "-pix_fmt", "yuv420p", "-x265-params", "log-level=error",
			"-c:a", "aac", "-b:a", "192000", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"webm vp9 默认恒定质量", ConvertOptions{Container: "webm", VideoCodec: "vp9"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-map", "0:a?", "-sn", "-dn",
			"-c:v", "libvpx-vp9", "-row-mt", "1", "-deadline", "good", "-cpu-used", "4", "-crf", "32", "-b:v", "0", "-pix_fmt", "yuv420p",
			"-c:a", "libopus", "-b:a", "128000", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"缩放宽度 + fps + 视频码率", ConvertOptions{Container: "mp4", VideoCodec: "h264", Width: 853, Fps: 24, VideoBitrate: 2_000_000, AudioBitrate: 96_000}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-map", "0:a?", "-sn", "-dn",
			"-vf", "fps=24,scale=852:-2", "-c:v", "libx264", "-preset", "medium", "-b:v", "2000000", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "96000", "-map_metadata", "0", "-movflags", "+faststart", "file:/out/a.part.x"}},
		{"宽高都给：等比缩进并补边", ConvertOptions{Container: "mkv", VideoCodec: "h264", Width: 1280, Height: 720}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-map", "0:a?", "-sn", "-dn",
			"-vf", "scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2", "-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "192000", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"只给高度", ConvertOptions{Container: "mkv", VideoCodec: "vp9", Height: 480}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-map", "0:a?", "-sn", "-dn",
			"-vf", "scale=-2:480", "-c:v", "libvpx-vp9", "-row-mt", "1", "-deadline", "good", "-cpu-used", "4", "-crf", "32", "-b:v", "0", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "192000", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"复制视频 + 裁剪", ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy", TrimStart: 1.5, TrimEnd: 4}, vsrc, []string{
			"-y", "-ss", "1.5", "-i", "file:/in/a.mp4", "-t", "2.5", "-map", "0:0", "-map", "0:a?", "-sn", "-dn",
			"-c:v", "copy", "-c:a", "copy", "-map_metadata", "0", "-avoid_negative_ts", "make_zero", "file:/out/a.part.x"}},
		{"去掉音轨", ConvertOptions{Container: "mp4", VideoCodec: "copy", AudioCodec: "none"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-sn", "-dn", "-an",
			"-c:v", "copy", "-map_metadata", "0", "-movflags", "+faststart", "file:/out/a.part.x"}},
		{"去掉视频（音轨留在视频容器里）", ConvertOptions{Container: "mkv", AudioCodec: "opus"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:a?", "-sn", "-dn", "-vn",
			"-c:a", "libopus", "-b:a", "128000", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"输入无音轨：视频容器不带音频", ConvertOptions{Container: "mp4", VideoCodec: "h264"}, ConvertSource{DurationSec: 5, HasVideo: true}, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-sn", "-dn", "-an",
			"-vf", "crop=trunc(iw/2)*2:trunc(ih/2)*2", "-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p",
			"-map_metadata", "0", "-movflags", "+faststart", "file:/out/a.part.x"}},
		{"主视频流不是第 0 条", ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}, ConvertSource{DurationSec: 5, HasVideo: true, VideoIndex: 2, HasAudio: true}, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:2", "-map", "0:a?", "-sn", "-dn",
			"-c:v", "copy", "-c:a", "copy", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"mp3 提取（默认 VBR）", ConvertOptions{Container: "mp3"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:a:0", "-sn", "-dn", "-vn",
			"-c:a", "libmp3lame", "-q:a", "2", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"mp3 指定码率 + 裁剪", ConvertOptions{Container: "mp3", AudioBitrate: 128_000, TrimStart: 2}, vsrc, []string{
			"-y", "-ss", "2", "-i", "file:/in/a.mp4", "-map", "0:a:0", "-sn", "-dn", "-vn",
			"-c:a", "libmp3lame", "-b:a", "128000", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"aac / m4a", ConvertOptions{Container: "m4a"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:a:0", "-sn", "-dn", "-vn",
			"-c:a", "aac", "-b:a", "192000", "-map_metadata", "0", "-movflags", "+faststart", "file:/out/a.part.x"}},
		{"flac", ConvertOptions{Container: "flac"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:a:0", "-sn", "-dn", "-vn",
			"-c:a", "flac", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"wav", ConvertOptions{Container: "wav"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:a:0", "-sn", "-dn", "-vn",
			"-c:a", "pcm_s16le", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"opus", ConvertOptions{Container: "opus"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:a:0", "-sn", "-dn", "-vn",
			"-c:a", "libopus", "-b:a", "128000", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"ogg vorbis", ConvertOptions{Container: "ogg"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:a:0", "-sn", "-dn", "-vn",
			"-c:a", "libvorbis", "-q:a", "5", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"gif 默认", ConvertOptions{Container: "gif"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-sn", "-dn", "-an",
			"-vf", "fps=12,scale='min(480,iw)':-2:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse",
			"-loop", "0", "-f", "gif", "file:/out/a.part.x"}},
		{"gif 指定宽度 fps 裁剪", ConvertOptions{Container: "gif", Width: 320, Fps: 8, TrimStart: 1, TrimEnd: 3}, vsrc, []string{
			"-y", "-ss", "1", "-i", "file:/in/a.mp4", "-t", "2", "-map", "0:0", "-sn", "-dn", "-an",
			"-vf", "fps=8,scale=320:-2,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse",
			"-loop", "0", "-f", "gif", "file:/out/a.part.x"}},
		{"avi h264+mp3", ConvertOptions{Container: "avi", VideoCodec: "h264"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-map", "0:a?", "-sn", "-dn",
			"-vf", "crop=trunc(iw/2)*2:trunc(ih/2)*2", "-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p",
			"-c:a", "libmp3lame", "-q:a", "2", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"flv", ConvertOptions{Container: "flv", VideoCodec: "h264"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-map", "0:a?", "-sn", "-dn",
			"-vf", "crop=trunc(iw/2)*2:trunc(ih/2)*2", "-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "192000", "-map_metadata", "0", "file:/out/a.part.x"}},
		{"mov + pcm", ConvertOptions{Container: "mov", VideoCodec: "h264", AudioCodec: "pcm"}, vsrc, []string{
			"-y", "-i", "file:/in/a.mp4", "-map", "0:0", "-map", "0:a?", "-sn", "-dn",
			"-vf", "crop=trunc(iw/2)*2:trunc(ih/2)*2", "-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p",
			"-c:a", "pcm_s16le", "-map_metadata", "0", "-movflags", "+faststart", "file:/out/a.part.x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := plan(t, c.o, c.src)
			if c.o.Container != "gif" { // v0.24：所有输出都显式加 -f <muxer>，放在输出路径前面
				n := len(c.want)
				c.want = append(append(append([]string{}, c.want[:n-1]...), "-f", ContainerMuxer(c.o.Container)), c.want[n-1])
			}
			if !reflect.DeepEqual(p.Final, c.want) {
				t.Fatalf("\n got: %q\nwant: %q", p.Final, c.want)
			}
		})
	}
}

func TestPlanConvertRejectsTargetSize(t *testing.T) {
	// 两遍编码 / 目标大小暂缓（契约 v0.7.2）：选项校验直接拒绝
	if _, err := PlanConvert("/a", "/b", ConvertOptions{Container: "mp4", VideoCodec: "h264", TargetSizeMB: 5}, vsrc); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("目标大小应 INVALID_ARGUMENT: %v", err)
	}
}

func tr(x float64) int64  { return int64(x) }
func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func TestPlanConvertOutputDuration(t *testing.T) {
	for name, c := range map[string]struct {
		o    ConvertOptions
		src  ConvertSource
		want float64
	}{
		"无裁剪":       {ConvertOptions{Container: "mp4", VideoCodec: "h264"}, vsrc, 10},
		"只裁开头":      {ConvertOptions{Container: "mp4", VideoCodec: "h264", TrimStart: 3}, vsrc, 7},
		"裁结尾":       {ConvertOptions{Container: "mp4", VideoCodec: "h264", TrimEnd: 4}, vsrc, 4},
		"结尾超出时长":    {ConvertOptions{Container: "mp4", VideoCodec: "h264", TrimStart: 2, TrimEnd: 99}, vsrc, 8},
		"未知时长有 end": {ConvertOptions{Container: "mp4", VideoCodec: "h264", TrimStart: 1, TrimEnd: 4}, ConvertSource{HasVideo: true}, 3},
		"未知时长无 end": {ConvertOptions{Container: "mp4", VideoCodec: "h264"}, ConvertSource{HasVideo: true}, 0},
	} {
		p, err := PlanConvert("/a", "/b", c.o, c.src)
		if err != nil || p.OutDurationSec != c.want {
			t.Errorf("%s: %v dur=%v want %v", name, err, p.OutDurationSec, c.want)
		}
	}
	// -t 只在有 trimEnd 时出现（否则让 ffmpeg 自然读到结尾）
	p := plan(t, ConvertOptions{Container: "mp4", VideoCodec: "h264", TrimStart: 3}, vsrc)
	if strings.Contains(strings.Join(p.Final, " "), "-t ") {
		t.Fatalf("只裁开头不应有 -t: %q", p.Final)
	}
}

func TestPlanConvertPathsAreSafe(t *testing.T) {
	for _, in := range []string{"-evil.mp4", `C:\视频 库\a b.mp4`, "/a/b:c.mp4", "/x/it's [1] (2).mp4"} {
		p, err := PlanConvert(in, "-out.part.mp4", ConvertOptions{Container: "mp4", VideoCodec: "h264"}, vsrc)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for i, a := range p.Final {
			if a == "-i" {
				found = p.Final[i+1] == "file:"+in
			}
		}
		if !found || p.Final[len(p.Final)-1] != "file:-out.part.mp4" {
			t.Fatalf("路径必须带 file: 前缀: %q", p.Final)
		}
		for _, a := range p.Final {
			if a == in || a == "-out.part.mp4" {
				t.Fatalf("出现了裸路径参数: %q", a)
			}
		}
	}
}

func TestPlanConvertSourceMismatch(t *testing.T) {
	audioOnly := ConvertSource{DurationSec: 5, HasAudio: true}
	videoOnly := ConvertSource{DurationSec: 5, HasVideo: true}
	for name, c := range map[string]struct {
		o   ConvertOptions
		src ConvertSource
	}{
		"音频文件转视频":   {ConvertOptions{Container: "mp4", VideoCodec: "h264"}, audioOnly},
		"音频文件转 gif": {ConvertOptions{Container: "gif"}, audioOnly},
		"无声视频提取音频":  {ConvertOptions{Container: "mp3"}, videoOnly},
		"裁剪开头超出时长":  {ConvertOptions{Container: "mp4", VideoCodec: "h264", TrimStart: 5}, vsrc2(5)},
		"什么都没有":     {ConvertOptions{Container: "mkv", AudioCodec: "opus"}, ConvertSource{}},
	} {
		if _, err := PlanConvert("/a", "/b", c.o, c.src); !apperr.Is(err, apperr.InvalidArgument) {
			t.Errorf("%s: 期望 INVALID_ARGUMENT, got %v", name, err)
		}
	}
	// 音频文件转 mkv 只要音频（VideoCodec 为空）是合法的
	if _, err := PlanConvert("/a", "/b", ConvertOptions{Container: "mkv", AudioCodec: "opus"}, audioOnly); err != nil {
		t.Fatal(err)
	}
	// 无声视频转视频容器合法
	if _, err := PlanConvert("/a", "/b", ConvertOptions{Container: "mp4", VideoCodec: "copy"}, videoOnly); err != nil {
		t.Fatal(err)
	}
}

func vsrc2(d float64) ConvertSource {
	return ConvertSource{DurationSec: d, HasVideo: true, HasAudio: true}
}

func TestValidateConvertOptions(t *testing.T) {
	ok := []ConvertOptions{
		{Container: "mp4", VideoCodec: "h264"},
		{Container: "mp4", VideoCodec: "copy", AudioCodec: "copy"},
		{Container: "webm", VideoCodec: "vp9", AudioCodec: "vorbis"},
		{Container: "gif"},
		{Container: "gif", AudioCodec: "none"},
		{Container: "mp3", AudioBitrate: 320000},
		{Container: "wav", AudioCodec: "pcm"},
		{Container: "ogg", AudioCodec: "opus"},
		{Container: "mp4", VideoCodec: "h264", TrimStart: 1},
		{Container: "mkv", VideoCodec: "", AudioCodec: "flac"},
	}
	for _, o := range ok {
		if err := ValidateConvertOptions(o); err != nil {
			t.Errorf("%+v 应合法: %v", o, err)
		}
	}
	bad := map[string]ConvertOptions{
		"未知容器":          {Container: "xyz"},
		"空容器":           {},
		"未知视频编码":        {Container: "mp4", VideoCodec: "av1"},
		"未知音频编码":        {Container: "mp4", VideoCodec: "h264", AudioCodec: "wma"},
		"webm 不支持 h264": {Container: "webm", VideoCodec: "h264"},
		"webm 不支持 aac":  {Container: "webm", VideoCodec: "vp9", AudioCodec: "aac"},
		"mov 不支持 vp9":   {Container: "mov", VideoCodec: "vp9"},
		"avi 不支持 h265":  {Container: "avi", VideoCodec: "h265"},
		"mp3 不能设分辨率":    {Container: "mp3", Width: 100},
		"mp3 不能去音轨":     {Container: "mp3", AudioCodec: "none"},
		"wav 只能 pcm":    {Container: "wav", AudioCodec: "aac"},
		"gif 带音频":       {Container: "gif", AudioCodec: "aac"},
		"gif 带码率":       {Container: "gif", VideoBitrate: 1000},
		"视频音频全去掉":       {Container: "mkv", AudioCodec: "none"},
		"负宽度":           {Container: "mp4", VideoCodec: "h264", Width: -1},
		"巨大宽度":          {Container: "mp4", VideoCodec: "h264", Width: 99999},
		"帧率过大":          {Container: "mp4", VideoCodec: "h264", Fps: 1000},
		"负码率":           {Container: "mp4", VideoCodec: "h264", VideoBitrate: -1},
		"crf 越界":        {Container: "mp4", VideoCodec: "h264", Crf: 64},
		"h264 crf 52":   {Container: "mp4", VideoCodec: "h264", Crf: 52},
		"trim 反了":       {Container: "mp4", VideoCodec: "h264", TrimStart: 5, TrimEnd: 3},
		"trim 负数":       {Container: "mp4", VideoCodec: "h264", TrimStart: -1},
		"copy 不能缩放":     {Container: "mp4", VideoCodec: "copy", Width: 100},
		"copy 不能 crf":   {Container: "mp4", VideoCodec: "copy", Crf: 20},
		"目标大小无视频编码":     {Container: "mp4", TargetSizeMB: 5, AudioCodec: "aac"},
		"flac 不能按大小":    {Container: "flac", TargetSizeMB: 5},
	}
	for name, o := range bad {
		err := ValidateConvertOptions(o)
		if !apperr.Is(err, apperr.InvalidArgument) {
			t.Errorf("%s: 期望 INVALID_ARGUMENT, got %v", name, err)
		}
	}
}

func TestClassifyConvertError(t *testing.T) {
	for tail, want := range map[string]apperr.Code{
		"av_interleaved_write_frame(): No space left on device":                    apperr.ConvertDiskFull,
		"Error writing trailer: ENOSPC":                                            apperr.ConvertDiskFull,
		"av_write_frame(): There is not enough space on the disk.":                 apperr.ConvertDiskFull,
		"write error: Disk quota exceeded":                                         apperr.ConvertDiskFull,
		"Error opening output file /x/a.mp4: Permission denied":                    apperr.IOError,
		"/x/a.mp4: Read-only file system":                                          apperr.IOError,
		"Error opening output /no/dir/a.mp4: No such file or directory":            apperr.IOError,
		"Unknown encoder 'libx265'":                                                apperr.ProcessFailed,
		"No such filter: 'palettegen'":                                             apperr.ProcessFailed,
		"x.mp4: Invalid data found when processing input":                          apperr.ProbeFailed,
		"moov atom not found":                                                      apperr.ProbeFailed,
		"Could not write header for output file #0 (incorrect codec parameters ?)": apperr.ProcessFailed,
		"访问被拒绝。 拒绝访问":                                                              apperr.IOError,
	} {
		e := ClassifyConvertError(tail, nil)
		if e == nil || e.Code != want {
			t.Errorf("%q → %+v, want %s", tail, e, want)
		}
	}
	if ClassifyConvertError("some random failure", nil) != nil || ClassifyConvertError("", nil) != nil {
		t.Error("不认识的应返回 nil")
	}
}

// H1：文件名 / 标题 / 元数据里出现关键词，但失败原因无关，不得误分类。
func TestClassifyIgnoresUserDataInStderr(t *testing.T) {
	stderr := func(tail ...string) string { return strings.Join(tail, "\n") }
	header := []string{
		"Input #0, mov,mp4,m4a,3gp,3g2,mj2, from '/videos/ENOSPC 磁盘空间不足 end of file permission denied no such file.mp4':",
		"  Metadata:",
		"    title           : No space left on device / Permission denied / end of file",
		"    artist          : Invalid data found when processing input",
		"  Duration: 00:01:00.00, start: 0.000000, bitrate: 1000 kb/s",
		"  Stream #0:0[0x1](und): Video: h264, yuv420p, 1280x720",
		"    Metadata:",
		"      handler_name    : ENOSPC no such file or directory",
		"Stream mapping:",
		"  Stream #0:0 -> #0:0 (h264 (native) -> hevc (libx265))",
		"Output #0, mp4, to '/out/enospc.mp4':",
		"  Metadata:",
		"    title           : Error writing trailer: No space left on device",
	}
	cases := map[string][]string{
		// 输出编码中途出错（不是磁盘满、也不是输入损坏）→ 不能因为 end of file 变成 PROBE_FAILED
		"encode error":   {"[libx265 @ 0x1] Error while encoding frame: end of file", "Conversion failed!"},
		"unrelated exit": {"Conversion failed!"},
		"filter error":   {"[Parsed_scale_0 @ 0x2] Unable to parse option value \"abc\"", "Error initializing filters"},
	}
	for name, body := range cases {
		if e := ClassifyConvertError(stderr(append(append([]string{}, header...), body...)...), nil); e != nil && name != "encode error" {
			t.Errorf("%s: 不应被分类, got %+v", name, e)
		} else if name == "encode error" && e != nil && e.Code == apperr.ProbeFailed {
			t.Errorf("%s: end of file 不应误判为 PROBE_FAILED: %+v", name, e)
		}
	}
	// 只有头部（文件名 / 标题里有关键词）+ 失败原因无关 → nil
	if e := ClassifyConvertError(stderr(header...), nil); e != nil {
		t.Errorf("头部里的关键词不应触发分类: %+v", e)
	}
	// 文件名里的 ENOSPC 出现在写入句式之外 → 不是磁盘满
	if e := ClassifyConvertError("Error opening output file /x/ENOSPC.mp4: Permission denied", nil); e == nil || e.Code != apperr.IOError {
		t.Errorf("应为 IO_ERROR: %+v", e)
	}
	if e := ClassifyConvertError("/x/No space left on device.mp4: Invalid data found when processing input", nil); e == nil || e.Code != apperr.ProbeFailed {
		t.Errorf("文件名带磁盘满字样不应误报 CONVERT_DISK_FULL: %+v", e)
	}
	// 磁盘满必须限定在写入句式上：孤立一行 ENOSPC 文件名不算
	if e := ClassifyConvertError("Could not open /x/ENOSPC", nil); e != nil && e.Code == apperr.ConvertDiskFull {
		t.Errorf("不应误报磁盘满: %+v", e)
	}
	// 真实磁盘满：完整 stderr（含头部）仍能识别
	real := stderr(append(append([]string{}, header...),
		"frame= 1200 fps= 60 q=28.0 size=  102400kB time=00:00:40.00",
		"[out#0/mp4 @ 0x3] Error muxing a packet",
		"av_interleaved_write_frame(): No space left on device",
		"Error writing trailer of /out/enospc.mp4: No space left on device",
		"Conversion failed!")...)
	if e := ClassifyConvertError(real, nil); e == nil || e.Code != apperr.ConvertDiskFull {
		t.Errorf("真实磁盘满应识别: %+v", e)
	}
	// Windows 措辞
	if e := ClassifyConvertError("[out#0 @ 0x1] Error muxing a packet\nav_interleaved_write_frame(): There is not enough space on the disk.", nil); e == nil || e.Code != apperr.ConvertDiskFull {
		t.Errorf("Windows 磁盘满应识别: %+v", e)
	}
	// tail 从元数据块中间开始（只保留最后 50 行）
	if e := ClassifyConvertError("    title : ENOSPC\n    artist : Permission denied\nConversion failed!", nil); e != nil {
		t.Errorf("块中间开头的元数据行应被忽略: %+v", e)
	}
}

// M2：选项上下限
func TestValidateConvertOptionsLimits(t *testing.T) {
	base := ConvertOptions{Container: "mp4", VideoCodec: "h264"}
	with := func(f func(*ConvertOptions)) ConvertOptions { o := base; f(&o); return o }
	bad := map[string]ConvertOptions{
		"trimStart 超上限":   with(func(o *ConvertOptions) { o.TrimStart = 1e6 + 1 }),
		"trimEnd 超上限":     with(func(o *ConvertOptions) { o.TrimEnd = 2e6 }),
		"fps 低于 0.1":      with(func(o *ConvertOptions) { o.Fps = 0.05 }),
		"videoBitrate 超限": with(func(o *ConvertOptions) { o.VideoBitrate = 1e9 + 1 }),
		"audioBitrate 太低": with(func(o *ConvertOptions) { o.AudioBitrate = 7999 }),
		"audioBitrate 太高": with(func(o *ConvertOptions) { o.AudioBitrate = 1_000_001 }),
		"width 8193":      with(func(o *ConvertOptions) { o.Width = 8193 }),
		"height 8193":     with(func(o *ConvertOptions) { o.Height = 8193 }),
	}
	for name, o := range bad {
		if err := ValidateConvertOptions(o); !apperr.Is(err, apperr.InvalidArgument) {
			t.Errorf("%s: 期望 INVALID_ARGUMENT, got %v", name, err)
		}
	}
	good := map[string]ConvertOptions{
		"边界值": with(func(o *ConvertOptions) {
			o.TrimStart, o.TrimEnd, o.Fps = 999_999, 1e6, 0.1
			o.VideoBitrate, o.AudioBitrate, o.Width, o.Height = 1e9, 8000, 8192, 8192
		}),
		"音频码率 1M": with(func(o *ConvertOptions) { o.AudioBitrate = 1_000_000 }),
		"全默认":     base,
	}
	for name, o := range good {
		if err := ValidateConvertOptions(o); err != nil {
			t.Errorf("%s 应通过: %v", name, err)
		}
	}
}

// M3：PlanConvert 对文件名带 % 的图片输入加 -pattern_type none（在 -i 之前）；其它输入不加。
func TestPlanConvertPatternTypeForPercentImages(t *testing.T) {
	img := ConvertSource{HasVideo: true}
	o := ConvertOptions{Container: "mp4", VideoCodec: "h264"}
	p, err := PlanConvert("/x/a%03d.jpg", "/o/a.part.mp4", o, img)
	if err != nil {
		t.Fatal(err)
	}
	j := strings.Join(p.Final, " ")
	if !strings.Contains(j, "-pattern_type none -i file:/x/a%03d.jpg") {
		t.Fatalf("图片文件名带 %% 时应有 -pattern_type none 且在 -i 之前: %s", j)
	}
	for _, in := range []string{"/x/a.jpg", "/x/a%03d.mp4", "/x/100%.gif", "/x/a.png"} {
		p, _ := PlanConvert(in, "/o/a.part.mp4", o, img)
		if strings.Contains(strings.Join(p.Final, " "), "pattern_type") {
			t.Errorf("%s 不应加 -pattern_type", in)
		}
	}
}
