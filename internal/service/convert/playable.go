package convert

import (
	"strings"

	"FFmpegFree/internal/store"
)

// ---------- 应用内预览的编码门控（契约 v0.23.4，6.14.7；走查 G4） ----------
//
// 扩展名白名单（previewExts）只看容器：ProRes 的 .mov 能拿到地址，WebView 里却是黑屏、时间照走、不报错。
// 所以在扩展名之后再按探测出的编码挡一层：下面这些编码 WebView（WebView2 / WKWebView）解不了，
// 一律 UNSUPPORTED（reason=format），前端显示“无法在应用内播放”，引导“用系统播放器打开”。
// 名单只放这一处；键是 ffprobe 的 codec_name（小写）。

// unplayableVideoCodecs：有画面的文件（封面图不算画面）按第一条视频流的编码判断。
var unplayableVideoCodecs = codecSet(
	// 剪辑 / 中间编码
	"prores prores_ks prores_aw dnxhd dnxhr cfhd hap dxv aic avrp " +
		// 无损 / 无压缩
		"ffv1 ffvhuff huffyuv utvideo magicyuv rawvideo v210 v410 r210 r10k qtrle png tiff jpeg2000 " +
		// 老的 MPEG / 微软 / 其他
		"mpeg1video mpeg2video msmpeg4v1 msmpeg4v2 msmpeg4v3 msmpeg4 wmv1 wmv2 wmv3 vc1 " +
		"h263 h263p h261 flv1 mjpeg mjpegb dvvideo cinepak svq1 svq3 rv10 rv20 rv30 rv40 indeo2 indeo3 indeo4 indeo5 vp6 vp6f vp6a")

// unplayableAudioCodecs：只有声音的文件（没有画面，含只带封面图的音频）按第一条音频流的编码判断。
// 有画面的文件不看音频编码（画面能播就让它播）。
var unplayableAudioCodecs = codecSet(
	"wmav1 wmav2 wmapro wmalossless wmavoice ape amr_nb amr_wb ac3 eac3 dts truehd mlp mp1 mp2 " +
		"wavpack tta cook ra_144 ra_288 sipr ralf atrac1 atrac3 atrac3p qdm2 qdmc nellymoser speex gsm gsm_ms " +
		"adpcm_ima_wav adpcm_ms adpcm_ima_qt adpcm_yamaha dsd_lsbf dsd_msbf dsd_lsbf_planar dsd_msbf_planar")

func codecSet(list string) map[string]bool {
	m := map[string]bool{}
	for _, c := range strings.Fields(list) {
		m[c] = true
	}
	return m
}

// unplayableCodec 返回 WebView 解不了的编码名（空 = 可以试着播）。m 为 nil（没探测到）时不挡，交给前端兜底。
func unplayableCodec(m *store.MediaInfo) string {
	if m == nil {
		return ""
	}
	if m.HasVideo {
		if c := strings.ToLower(m.VideoCodec); unplayableVideoCodecs[c] {
			return c
		}
		return ""
	}
	if m.HasAudio {
		if c := strings.ToLower(m.AudioCodec); unplayableAudioCodecs[c] {
			return c
		}
	}
	return ""
}
