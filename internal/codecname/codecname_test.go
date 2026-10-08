package codecname

import "testing"

// 走查 X3：所有给人看的编码名都走这一张表，常见编码不再被兜底规则写成 “Ffv1” 这样。
func TestVideo(t *testing.T) {
	cases := map[string]string{
		"ffv1": "FFV1", "FFV1": "FFV1", "dnxhd": "DNxHD", "mjpeg": "MJPEG", "prores": "ProRes", "prores_ks": "ProRes",
		"h264": "H.264", "hevc": "H.265", "hevc_nvenc": "H.265", "h264_qsv": "H.264", "av1": "AV1", "vp9": "VP9", "vp8": "VP8",
		"mpeg4": "MPEG-4", "msmpeg4v3": "MPEG-4", "mpeg2video": "MPEG-2", "wmv3": "WMV", "vc1": "VC-1", "cfhd": "CineForm",
		"hap": "HAP", "utvideo": "Ut Video", "huffyuv": "HuffYUV", "qtrle": "QuickTime RLE", "rawvideo": "无压缩",
		"dvvideo": "DV", "flv1": "FLV", "theora": "Theora", "gif": "GIF", "png": "PNG", "jpeg2000": "JPEG 2000",
		"copy": "原画质", "": "无画面",
		// 不在表里：去掉 lib 后全部大写（契约 v0.24.1 的兜底）
		"foocodec": "FOOCODEC", "libfoo": "FOO", "LibBar": "BAR",
	}
	for in, want := range cases {
		if got := Video(in); got != want {
			t.Errorf("Video(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAudio(t *testing.T) {
	cases := map[string]string{
		"aac": "AAC", "mp3": "MP3", "mp3float": "MP3", "mp2": "MP2", "opus": "Opus", "vorbis": "Vorbis", "flac": "FLAC",
		"alac": "ALAC", "ac3": "AC-3", "eac3": "E-AC-3", "dts": "DTS", "truehd": "TrueHD", "wmav2": "WMA", "wmapro": "WMA Pro",
		"amr_nb": "AMR", "amr_wb": "AMR-WB", "ape": "APE", "wavpack": "WavPack",
		"pcm_s16le": "PCM", "pcm_f32le": "PCM", "pcm_s24be": "PCM", "adpcm_ms": "ADPCM", "dsd_lsbf": "DSD", "aac_at": "AAC",
		"foo": "FOO", "libfoo": "FOO",
	}
	for in, want := range cases {
		if got := Audio(in); got != want {
			t.Errorf("Audio(%q) = %q, want %q", in, got, want)
		}
	}
}
