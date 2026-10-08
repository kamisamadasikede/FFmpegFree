package convert

import (
	"encoding/json"

	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/store"
)

// Preset 是契约第 3 节的 Preset。
type Preset struct {
	ID      string                `json:"id"`
	Name    string                `json:"name"`
	BuiltIn bool                  `json:"builtIn"`
	Options ffmpeg.ConvertOptions `json:"options"`
}

// builtinPresets 是内置预设（常见格式）。id 固定，升级时会按这里的内容更新库里的内置行。
// 分辨率预设只给宽度，高度按比例；不设码率，画质用编码器默认 CRF。
func builtinPresets() []Preset {
	mk := func(id, name string, o ffmpeg.ConvertOptions) Preset {
		return Preset{ID: id, Name: name, BuiltIn: true, Options: o}
	}
	return []Preset{
		mk("builtin-mp4-h264", "MP4（H.264 + AAC，通用）", ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}),
		mk("builtin-mp4-h264-1080p", "MP4 1080p（H.264 + AAC）", ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", Width: 1920}),
		mk("builtin-mp4-h264-720p", "MP4 720p（H.264 + AAC）", ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", Width: 1280}),
		mk("builtin-mp4-h265", "MP4（H.265 + AAC，体积更小）", ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h265", AudioCodec: "aac"}),
		mk("builtin-mkv-h264", "MKV（H.264 + AAC）", ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "h264", AudioCodec: "aac"}),
		mk("builtin-mkv-copy", "MKV（不重新编码，只换封装）", ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}),
		mk("builtin-mov-h264", "MOV（H.264 + AAC）", ffmpeg.ConvertOptions{Container: "mov", VideoCodec: "h264", AudioCodec: "aac"}),
		mk("builtin-webm-vp9", "WebM（VP9 + Opus）", ffmpeg.ConvertOptions{Container: "webm", VideoCodec: "vp9", AudioCodec: "opus"}),
		mk("builtin-gif", "GIF 动图（宽 480，12 帧/秒）", ffmpeg.ConvertOptions{Container: "gif", Width: 480, Fps: 12}),
		mk("builtin-mp3", "MP3（192 kbps）", ffmpeg.ConvertOptions{Container: "mp3", AudioCodec: "mp3", AudioBitrate: 192_000}),
		mk("builtin-m4a", "M4A（AAC 192 kbps）", ffmpeg.ConvertOptions{Container: "m4a", AudioCodec: "aac", AudioBitrate: 192_000}),
		mk("builtin-wav", "WAV（无损 PCM）", ffmpeg.ConvertOptions{Container: "wav", AudioCodec: "pcm"}),
		mk("builtin-flac", "FLAC（无损压缩）", ffmpeg.ConvertOptions{Container: "flac", AudioCodec: "flac"}),
		// v0.24（契约 6.16.2）：每个格式一个内置默认预设，id 为 builtin-<extension>（MKV 的默认是上面的 builtin-mkv-h264）。
		mk("builtin-avi", "AVI（Xvid + MP3）", ffmpeg.ConvertOptions{Container: "avi", VideoCodec: "mpeg4", AudioCodec: "mp3"}),
		mk("builtin-flv", "FLV（H.264 + AAC）", ffmpeg.ConvertOptions{Container: "flv", VideoCodec: "h264", AudioCodec: "aac"}),
		mk("builtin-wmv", "WMV（WMV2 + WMA）", ffmpeg.ConvertOptions{Container: "wmv", VideoCodec: "wmv2", AudioCodec: "wma"}),
		mk("builtin-mpg", "MPG（MPEG-2 + MP2）", ffmpeg.ConvertOptions{Container: "mpg", VideoCodec: "mpeg2", AudioCodec: "mp2"}),
		mk("builtin-vob", "VOB（MPEG-2 + AC-3）", ffmpeg.ConvertOptions{Container: "vob", VideoCodec: "mpeg2", AudioCodec: "ac3"}),
		mk("builtin-3gp", "3GP（H.264 + AAC）", ffmpeg.ConvertOptions{Container: "3gp", VideoCodec: "h264", AudioCodec: "aac", AudioBitrate: 96_000}),
		mk("builtin-swf", "SWF（Flash 视频）", ffmpeg.ConvertOptions{Container: "swf", VideoCodec: "flv1", AudioCodec: "mp3"}),
		mk("builtin-ogv", "OGV（Theora + Vorbis）", ffmpeg.ConvertOptions{Container: "ogv", VideoCodec: "theora", AudioCodec: "vorbis"}),
		mk("builtin-aac", "AAC（192 kbps）", ffmpeg.ConvertOptions{Container: "aac", AudioCodec: "aac"}),
		mk("builtin-ogg", "OGG（Vorbis）", ffmpeg.ConvertOptions{Container: "ogg", AudioCodec: "vorbis"}),
		mk("builtin-opus", "Opus（128 kbps）", ffmpeg.ConvertOptions{Container: "opus", AudioCodec: "opus", AudioBitrate: 128_000}),
		mk("builtin-wma", "WMA（192 kbps）", ffmpeg.ConvertOptions{Container: "wma", AudioCodec: "wma", AudioBitrate: 192_000}),
		mk("builtin-amr", "AMR（语音 12.2 kbps）", ffmpeg.ConvertOptions{Container: "amr", AudioCodec: "amr_nb"}),
		mk("builtin-m4r", "M4R 铃声（AAC 192 kbps）", ffmpeg.ConvertOptions{Container: "m4r", AudioCodec: "aac", AudioBitrate: 192_000}),
		mk("builtin-mp2", "MP2（224 kbps）", ffmpeg.ConvertOptions{Container: "mp2", AudioCodec: "mp2", AudioBitrate: 224_000}),
		mk("builtin-ape", "APE（无损）", ffmpeg.ConvertOptions{Container: "ape", AudioCodec: "ape"}),
		mk("builtin-wv", "WV（WavPack 无损）", ffmpeg.ConvertOptions{Container: "wv", AudioCodec: "wavpack"}),
		mk("builtin-mmf", "MMF（手机铃声）", ffmpeg.ConvertOptions{Container: "mmf", AudioCodec: "adpcm_yamaha"}),
		mk("builtin-jpg", "JPG 图片", ffmpeg.ConvertOptions{Container: "jpg"}),
		mk("builtin-png", "PNG 图片", ffmpeg.ConvertOptions{Container: "png"}),
		mk("builtin-webp", "WebP 图片", ffmpeg.ConvertOptions{Container: "webp"}),
		mk("builtin-ico", "ICO 图标（最大 256×256）", ffmpeg.ConvertOptions{Container: "ico"}),
		mk("builtin-bmp", "BMP 图片", ffmpeg.ConvertOptions{Container: "bmp"}),
		mk("builtin-tif", "TIF 图片", ffmpeg.ConvertOptions{Container: "tif"}),
		mk("builtin-tga", "TGA 图片", ffmpeg.ConvertOptions{Container: "tga"}),
	}
}

func toRow(p Preset, sort int) store.PresetRow {
	b, _ := json.Marshal(p.Options)
	return store.PresetRow{ID: p.ID, Name: p.Name, BuiltIn: p.BuiltIn, Options: string(b), Sort: sort}
}

func fromRow(r store.PresetRow) Preset {
	p := Preset{ID: r.ID, Name: r.Name, BuiltIn: r.BuiltIn}
	if err := json.Unmarshal([]byte(r.Options), &p.Options); err != nil {
		p.Options = ffmpeg.ConvertOptions{} // 损坏的行：返回空参数，用户重新保存即可
	}
	return p
}
