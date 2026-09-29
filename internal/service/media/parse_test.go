package media

import (
	"os"
	"testing"

	"FFmpegFree/internal/apperr"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseH264AAC(t *testing.T) {
	m, err := ParseProbe(fixture(t, "h264_aac.mp4.json"), "/媒体 库/-视频 1.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "-视频 1.mp4" || !m.HasVideo || !m.HasAudio {
		t.Fatalf("%+v", m)
	}
	if m.VideoCodec != "h264" || m.AudioCodec != "aac" || m.Width != 320 || m.Height != 240 || m.Fps != 25 {
		t.Fatalf("%+v", m)
	}
	if m.Duration < 1.9 || m.Duration > 2.2 || m.Bitrate <= 0 || m.Size <= 0 {
		t.Fatalf("duration/bitrate/size: %+v", m)
	}
	if m.SampleRate != 44100 || m.Channels != 1 || m.Rotation != 0 {
		t.Fatalf("audio/rotation: %+v", m)
	}
	if len(m.Streams) != 2 || m.Streams[0].Type != "video" || m.Streams[0].Profile != "High" || m.Streams[0].PixFmt != "yuv420p" ||
		m.Streams[1].Type != "audio" || m.Streams[1].Bitrate <= 0 {
		t.Fatalf("streams: %+v", m.Streams)
	}
	if m.Streams[0].Language != "" { // und 不算语言
		t.Fatalf("und 应被忽略: %q", m.Streams[0].Language)
	}
	if m.Container == "" {
		t.Fatal("container 为空")
	}
}

func TestParseRotationSwapsDisplaySize(t *testing.T) {
	m, err := ParseProbe(fixture(t, "rotated90.mp4.json"), "r.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if m.Rotation != 90 {
		t.Fatalf("rotation = %d", m.Rotation)
	}
	if m.Width != 240 || m.Height != 320 {
		t.Fatalf("显示尺寸应交换为 240x320: %dx%d", m.Width, m.Height)
	}
	if m.Streams[0].Width != 320 || m.Streams[0].Height != 240 || m.Streams[0].Rotation != 90 {
		t.Fatalf("流里保留编码尺寸: %+v", m.Streams[0])
	}
}

func TestParseAudioOnly(t *testing.T) {
	m, err := ParseProbe(fixture(t, "audio.mp3.json"), "a.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if m.HasVideo || !m.HasAudio || m.VideoCodec != "" || m.AudioCodec != "mp3" || m.Width != 0 || m.Height != 0 {
		t.Fatalf("%+v", m)
	}
	if m.SampleRate != 48000 || m.Channels != 2 || m.Bitrate < 100000 {
		t.Fatalf("%+v", m)
	}
}

func TestParseWebmDurationFallsBackToStream(t *testing.T) {
	m, err := ParseProbe(fixture(t, "vp9.webm.json"), "v.webm")
	if err != nil {
		t.Fatal(err)
	}
	if m.VideoCodec != "vp9" || m.Width != 160 || m.Height != 120 || m.Duration <= 0 || m.Fps != 10 {
		t.Fatalf("%+v", m)
	}
}

func TestParseFailures(t *testing.T) {
	for name, in := range map[string]string{
		"空输出":     "",
		"空对象":     "{\n\n}",
		"无流":      `{"streams":[],"format":{"duration":"1.0"}}`,
		"不是 json": "garbage",
	} {
		_, err := ParseProbe([]byte(in), "x")
		if !apperr.Is(err, apperr.ProbeFailed) {
			t.Errorf("%s: 应返回 PROBE_FAILED, got %v", name, err)
		}
	}
}

func TestParseNAAndOddValues(t *testing.T) {
	js := `{"streams":[
	 {"index":0,"codec_name":"h264","codec_type":"video","width":100,"height":50,"avg_frame_rate":"0/0","r_frame_rate":"30000/1001","bit_rate":"N/A","duration":"N/A"},
	 {"index":1,"codec_name":"mjpeg","codec_type":"video","width":10,"height":10,"disposition":{"attached_pic":1}},
	 {"index":2,"codec_name":"aac","codec_type":"audio","sample_rate":"44100","channels":2,"bit_rate":"1000","duration":"5.5","tags":{"language":"chi"}}],
	 "format":{"format_name":"matroska,webm","duration":"N/A","size":"1234","bit_rate":"N/A"}}`
	m, err := ParseProbe([]byte(js), "x.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if m.Fps != 29.97 { // avg 0/0 退回 r_frame_rate，保留 3 位小数
		t.Fatalf("fps = %v", m.Fps)
	}
	if m.Duration != 5.5 || m.Bitrate != 1000 || m.Size != 1234 {
		t.Fatalf("N/A 应回退到流: %+v", m)
	}
	if m.Width != 100 || m.VideoCodec != "h264" { // 封面图不算主视频流
		t.Fatalf("封面图不应被当作视频: %+v", m)
	}
	if !m.Streams[1].AttachedPic || m.Streams[2].Language != "chi" {
		t.Fatalf("%+v", m.Streams)
	}
}

func TestRotationNormalization(t *testing.T) {
	for in, want := range map[float64]int{0: 0, 90: 90, -90: 270, 270: 270, 180: 180, -180: 180, 360: 0, 450: 90, 89.9: 90} {
		if got := normalizeRotation(in); got != want {
			t.Errorf("normalizeRotation(%v) = %d, want %d", in, got, want)
		}
	}
	// 老式 rotate 标签是顺时针：rotate=90 相当于逆时针 270
	s := &probeStream{Tags: map[string]string{"rotate": "90"}}
	if streamRotation(s) != 270 {
		t.Fatalf("rotate=90 标签 = %d", streamRotation(s))
	}
}

func TestParseFrameRate(t *testing.T) {
	for in, want := range map[string]float64{"25/1": 25, "30000/1001": 29.97, "0/0": 0, "": 0, "abc": 0, "24": 24, "1/0": 0, "-5/1": 0} {
		if got := parseFrameRate(in); got != want {
			t.Errorf("parseFrameRate(%q) = %v, want %v", in, got, want)
		}
	}
}
