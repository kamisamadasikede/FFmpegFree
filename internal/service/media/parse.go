// Package media 是 MediaService 的实现：用 ffprobe 探测媒体信息、用 ffmpeg 生成缩略图（契约第 4 节）。
//
// 本文件是纯解析逻辑（ffprobe JSON → store.MediaInfo），不启动任何进程，
// 测试用真实 ffprobe 输出（testdata/*.json）覆盖。
package media

import (
	"bytes"
	"encoding/json"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
)

// flex 兼容 ffprobe 里同一个字段时而是数字、时而是字符串（甚至 "N/A"）的情况。
type flex string

func (f *flex) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = flex(v)
		return nil
	}
	*f = flex(s)
	return nil
}

func (f flex) float() float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(string(f)), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0 // "N/A" 等
	}
	return v
}

func (f flex) int64() int64 { return int64(math.Round(f.float())) }

type probeJSON struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

type probeFormat struct {
	FormatName string `json:"format_name"`
	Duration   flex   `json:"duration"`
	Size       flex   `json:"size"`
	BitRate    flex   `json:"bit_rate"`
}

type probeStream struct {
	Index         int               `json:"index"`
	CodecName     string            `json:"codec_name"`
	CodecType     string            `json:"codec_type"`
	Profile       string            `json:"profile"`
	Width         int               `json:"width"`
	Height        int               `json:"height"`
	PixFmt        string            `json:"pix_fmt"`
	AvgFrameRate  string            `json:"avg_frame_rate"`
	RFrameRate    string            `json:"r_frame_rate"`
	BitRate       flex              `json:"bit_rate"`
	Duration      flex              `json:"duration"`
	SampleRate    flex              `json:"sample_rate"`
	Channels      int               `json:"channels"`
	ChannelLayout string            `json:"channel_layout"`
	Disposition   map[string]int    `json:"disposition"`
	Tags          map[string]string `json:"tags"`
	SideData      []struct {
		Rotation flex `json:"rotation"`
	} `json:"side_data_list"`
}

// ParseProbe 把 `ffprobe -print_format json -show_format -show_streams` 的输出转成 MediaInfo。
// path 只用来填 Name，不读文件；调用方补 ID、Path、Size（以文件系统为准）。
// 没有任何可识别的流时返回 PROBE_FAILED。
func ParseProbe(data []byte, path string) (store.MediaInfo, error) {
	var pj probeJSON
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&pj); err != nil {
		return store.MediaInfo{}, apperr.Wrap(apperr.ProbeFailed, "无法解析媒体信息", err)
	}
	if len(pj.Streams) == 0 {
		return store.MediaInfo{}, apperr.New(apperr.ProbeFailed, "没有找到可识别的音视频流")
	}

	m := store.MediaInfo{
		Name:      filepath.Base(path),
		Container: pj.Format.FormatName,
		Size:      pj.Format.Size.int64(),
		Duration:  pj.Format.Duration.float(),
		Bitrate:   pj.Format.BitRate.int64(),
	}

	var streamDur float64
	var streamBits int64
	var video, audio *probeStream
	for i := range pj.Streams {
		s := &pj.Streams[i]
		si := toStreamInfo(s)
		m.Streams = append(m.Streams, si)
		if si.Duration > streamDur {
			streamDur = si.Duration
		}
		streamBits += si.Bitrate
		switch s.CodecType {
		case "video":
			// 封面图（mp3 的专辑图等）不算视频画面。
			if !si.AttachedPic && video == nil {
				video = s
			}
		case "audio":
			if audio == nil {
				audio = s
			}
		}
	}
	// 只有字幕 / 数据流（.srt 等）或只有封面图的文件不是可用的媒体文件。
	if video == nil && audio == nil {
		return store.MediaInfo{}, apperr.New(apperr.ProbeFailed, "没有找到音频或视频流（只有字幕、数据流或封面图）")
	}
	if m.Duration <= 0 {
		m.Duration = streamDur
	}
	if m.Bitrate <= 0 {
		m.Bitrate = streamBits
	}
	if video != nil {
		si := toStreamInfo(video)
		m.HasVideo = true
		m.VideoCodec = si.Codec
		m.Fps = si.Fps
		m.Rotation = si.Rotation
		m.Width, m.Height = si.Width, si.Height
		if si.Rotation == 90 || si.Rotation == 270 {
			m.Width, m.Height = si.Height, si.Width // 显示尺寸
		}
	}
	if audio != nil {
		si := toStreamInfo(audio)
		m.HasAudio = true
		m.AudioCodec = si.Codec
		m.SampleRate = si.SampleRate
		m.Channels = si.Channels
	}
	m.FillCodecNames()
	return m, nil
}

func toStreamInfo(s *probeStream) store.StreamInfo {
	si := store.StreamInfo{
		Index:         s.Index,
		Type:          s.CodecType,
		Codec:         s.CodecName,
		Profile:       s.Profile,
		Width:         s.Width,
		Height:        s.Height,
		PixFmt:        s.PixFmt,
		Bitrate:       s.BitRate.int64(),
		Duration:      s.Duration.float(),
		SampleRate:    int(s.SampleRate.int64()),
		Channels:      s.Channels,
		ChannelLayout: s.ChannelLayout,
		Language:      s.Tags["language"],
		AttachedPic:   s.Disposition["attached_pic"] == 1,
	}
	if si.Language == "und" {
		si.Language = ""
	}
	if s.CodecType == "video" {
		si.Fps = parseFrameRate(s.AvgFrameRate)
		if si.Fps == 0 {
			si.Fps = parseFrameRate(s.RFrameRate)
		}
		si.Rotation = streamRotation(s)
	}
	return si
}

// parseFrameRate 解析 "30000/1001"，"0/0" 或非法值返回 0，结果保留 3 位小数。
func parseFrameRate(s string) float64 {
	num, den, ok := strings.Cut(strings.TrimSpace(s), "/")
	if !ok {
		v, _ := strconv.ParseFloat(num, 64)
		return round3(v)
	}
	n, err1 := strconv.ParseFloat(num, 64)
	d, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || d == 0 || n <= 0 {
		return 0
	}
	return round3(n / d)
}

func round3(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return math.Round(v*1000) / 1000
}

// streamRotation 返回视频流的旋转角度，归一到 0/90/180/270，**逆时针**（与 ffprobe 的 display matrix 同向）：
// 播放时画面需要逆时针转这么多度才是正的。优先取 display matrix，其次是老式 rotate 标签（顺时针，取反换算）。
func streamRotation(s *probeStream) int {
	for _, sd := range s.SideData {
		if sd.Rotation != "" {
			return normalizeRotation(sd.Rotation.float())
		}
	}
	if r, ok := s.Tags["rotate"]; ok {
		if v, err := strconv.ParseFloat(strings.TrimSpace(r), 64); err == nil {
			return normalizeRotation(-v)
		}
	}
	return 0
}

func normalizeRotation(deg float64) int {
	q := int(math.Round(deg/90)) % 4
	if q < 0 {
		q += 4
	}
	return q * 90
}
