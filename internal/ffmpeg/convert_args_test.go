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
	p, err := PlanConvert("/in/a.mp4", "/out/a.part.x", "/tmp/pass", o, src)
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
			if p.Pass1 != nil {
				t.Fatalf("不应是两遍编码")
			}
			if !reflect.DeepEqual(p.Final, c.want) {
				t.Fatalf("\n got: %q\nwant: %q", p.Final, c.want)
			}
		})
	}
}

func TestPlanConvertTwoPass(t *testing.T) {
	// 10 秒，目标 5 MB：总位数 5*1024*1024*8*0.95，减去 192 kbps 音频
	p := plan(t, ConvertOptions{Container: "mp4", VideoCodec: "h264", TargetSizeMB: 5}, vsrc)
	if p.Pass1 == nil {
		t.Fatal("应是两遍编码")
	}
	want := tr((5*1024*1024*8*0.95 - 192000*10) / 10)
	if p.VideoBitrate != want {
		t.Fatalf("bitrate = %d, want %d", p.VideoBitrate, want)
	}
	j1, j2 := strings.Join(p.Pass1, " "), strings.Join(p.Final, " ")
	for _, s := range []string{"-pass 1", "-passlogfile /tmp/pass", "-an", "-f null -", "-b:v " + itoa(want)} {
		if !strings.Contains(j1, s) {
			t.Errorf("第一遍缺 %q: %s", s, j1)
		}
	}
	if strings.Contains(j1, "0:a") || strings.Contains(j1, "file:/out") || strings.Contains(j1, "-c:a") {
		t.Errorf("第一遍不应带音频或输出文件: %s", j1)
	}
	for _, s := range []string{"-pass 2", "-passlogfile /tmp/pass", "-b:v " + itoa(want), "-c:a aac", "file:/out/a.part.x"} {
		if !strings.Contains(j2, s) {
			t.Errorf("第二遍缺 %q: %s", s, j2)
		}
	}
	if strings.Contains(j1, "-crf") || strings.Contains(j2, "-crf") {
		t.Error("两遍编码不应带 -crf")
	}
	if p.OutDurationSec != 10 {
		t.Fatalf("dur = %v", p.OutDurationSec)
	}
}

func TestPlanConvertTwoPassVariants(t *testing.T) {
	// 无音轨：不扣音频码率
	p := plan(t, ConvertOptions{Container: "mkv", VideoCodec: "vp9", TargetSizeMB: 1}, ConvertSource{DurationSec: 8, HasVideo: true})
	if want := tr(1 * 1024 * 1024 * 8 * 0.95 / 8); p.VideoBitrate != want {
		t.Fatalf("%d != %d", p.VideoBitrate, want)
	}
	// 裁剪后按裁剪时长算
	p = plan(t, ConvertOptions{Container: "mp4", VideoCodec: "h265", TargetSizeMB: 2, TrimStart: 2, TrimEnd: 6, AudioBitrate: 64_000}, vsrc)
	if want := tr((2*1024*1024*8*0.95 - 64000*4) / 4); p.VideoBitrate != want || p.OutDurationSec != 4 {
		t.Fatalf("%d != %d dur=%v", p.VideoBitrate, want, p.OutDurationSec)
	}
	// 音频容器按目标大小：单遍 -b:a
	p = plan(t, ConvertOptions{Container: "mp3", TargetSizeMB: 1}, vsrc)
	if p.Pass1 != nil || !strings.Contains(strings.Join(p.Final, " "), "-b:a "+itoa(tr(1*1024*1024*8*0.95/10))) {
		t.Fatalf("%q", p.Final)
	}
	// 太小
	if _, err := PlanConvert("/a", "/b", "/p", ConvertOptions{Container: "mp4", VideoCodec: "h264", TargetSizeMB: 0.01}, vsrc); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("目标太小应报错: %v", err)
	}
	// 未知时长
	if _, err := PlanConvert("/a", "/b", "/p", ConvertOptions{Container: "mp4", VideoCodec: "h264", TargetSizeMB: 5}, ConvertSource{HasVideo: true, HasAudio: true}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("未知时长应报错: %v", err)
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
		p, err := PlanConvert("/a", "/b", "/p", c.o, c.src)
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
		p, err := PlanConvert(in, "-out.part.mp4", "/tmp/p", ConvertOptions{Container: "mp4", VideoCodec: "h264"}, vsrc)
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
		if _, err := PlanConvert("/a", "/b", "/p", c.o, c.src); !apperr.Is(err, apperr.InvalidArgument) {
			t.Errorf("%s: 期望 INVALID_ARGUMENT, got %v", name, err)
		}
	}
	// 音频文件转 mkv 只要音频（VideoCodec 为空）是合法的
	if _, err := PlanConvert("/a", "/b", "/p", ConvertOptions{Container: "mkv", AudioCodec: "opus"}, audioOnly); err != nil {
		t.Fatal(err)
	}
	// 无声视频转视频容器合法
	if _, err := PlanConvert("/a", "/b", "/p", ConvertOptions{Container: "mp4", VideoCodec: "copy"}, videoOnly); err != nil {
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
		"av_interleaved_write_frame(): No space left on device":                    apperr.IOError,
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
