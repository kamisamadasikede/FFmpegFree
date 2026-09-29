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
		mk("builtin-mkv-copy", "MKV（不重新编码，只换封装）", ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}),
		mk("builtin-mov-h264", "MOV（H.264 + AAC）", ffmpeg.ConvertOptions{Container: "mov", VideoCodec: "h264", AudioCodec: "aac"}),
		mk("builtin-webm-vp9", "WebM（VP9 + Opus）", ffmpeg.ConvertOptions{Container: "webm", VideoCodec: "vp9", AudioCodec: "opus"}),
		mk("builtin-gif", "GIF 动图（宽 480，12 帧/秒）", ffmpeg.ConvertOptions{Container: "gif", Width: 480, Fps: 12}),
		mk("builtin-mp3", "MP3（192 kbps）", ffmpeg.ConvertOptions{Container: "mp3", AudioCodec: "mp3", AudioBitrate: 192_000}),
		mk("builtin-m4a", "M4A（AAC 192 kbps）", ffmpeg.ConvertOptions{Container: "m4a", AudioCodec: "aac", AudioBitrate: 192_000}),
		mk("builtin-wav", "WAV（无损 PCM）", ffmpeg.ConvertOptions{Container: "wav", AudioCodec: "pcm"}),
		mk("builtin-flac", "FLAC（无损压缩）", ffmpeg.ConvertOptions{Container: "flac", AudioCodec: "flac"}),
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
