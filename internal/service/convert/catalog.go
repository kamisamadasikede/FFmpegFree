package convert

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
)

// ---------- 格式目录（契约 v0.24，6.16） ----------

// FormatEntry 是 GetFormatCatalog 的一项（契约 6.16.1）。
type FormatEntry struct {
	Category        string         `json:"category"`             // "video" | "audio" | "image"
	Extension       string         `json:"extension"`            // 输出扩展名，也是 ConvertOptions.container 的取值
	DisplayName     string         `json:"displayName"`          // 给人看的名字
	Aliases         []string       `json:"aliases"`              // 搜索用的别名，没有时 []
	Encodable       bool           `json:"encodable"`            // 当前转换组件能否输出（按默认预设检测）
	Reason          string         `json:"reason,omitempty"`     // encodable=false 时给用户看的一句话
	ReasonCode      string         `json:"reasonCode,omitempty"` // converter_not_ready | missing_muxer | missing_encoder | check_failed
	DefaultPresetID string         `json:"defaultPresetId"`      // 默认预设（内置）
	Presets         []FormatPreset `json:"presets"`              // 这个格式可用的预设：先内置，后用户预设
}

// FormatPreset 是 FormatEntry.presets 的一项。
type FormatPreset struct {
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	BuiltIn       bool                  `json:"builtIn"`
	ParamsSummary string                `json:"paramsSummary"`
	Options       ffmpeg.ConvertOptions `json:"options"`
}

// reasonCode 的取值与文案（契约 6.16.1 / 6.16.3，面向用户的文字不出现 “ffmpeg”）。
const (
	ReasonConverterNotReady = "converter_not_ready"
	ReasonMissingMuxer      = "missing_muxer"
	ReasonMissingEncoder    = "missing_encoder"
	ReasonCheckFailed       = "check_failed"

	msgNotReady    = "转换组件尚未就绪"
	msgUnsupported = "当前转换组件不支持输出这个格式"
	msgCheckFailed = "暂时无法确认转换组件是否支持这个格式"
	msgNoEncoder   = "当前转换组件不支持所选的编码"
)

type formatDef struct {
	category, ext, name string
	aliases             []string
	defaultPreset       string
}

// formatDefs 是目录内容，顺序即返回顺序（契约 6.16.2 的表格）。别名以后只追加。
var formatDefs = []formatDef{
	{"video", "mp4", "MP4", []string{"通用视频", "手机视频", "MPEG-4"}, "builtin-mp4-h264"},
	{"video", "mkv", "MKV", []string{"高清", "蓝光", "Matroska"}, "builtin-mkv-h264"},
	{"video", "mov", "MOV", []string{"苹果", "苹果视频", "QuickTime"}, "builtin-mov-h264"},
	{"video", "webm", "WebM", []string{"网页视频"}, "builtin-webm-vp9"},
	{"video", "avi", "AVI", []string{"老式视频", "Xvid", "DivX"}, "builtin-avi"},
	{"video", "flv", "FLV", []string{"Flash 视频", "网络视频"}, "builtin-flv"},
	{"video", "gif", "GIF", []string{"动图", "表情包", "动画", "图片"}, "builtin-gif"},
	{"video", "wmv", "WMV", []string{"Windows 视频", "微软视频"}, "builtin-wmv"},
	{"video", "mpg", "MPG", []string{"MPEG", "MPEG-2", "VCD"}, "builtin-mpg"},
	{"video", "vob", "VOB", []string{"DVD"}, "builtin-vob"},
	{"video", "3gp", "3GP", []string{"老手机", "手机视频"}, "builtin-3gp"},
	{"video", "swf", "SWF", []string{"Flash", "Flash 动画"}, "builtin-swf"},
	{"video", "ogv", "OGV", []string{"Ogg 视频", "Theora"}, "builtin-ogv"},
	{"audio", "mp3", "MP3", []string{"音乐", "歌曲", "通用音频"}, "builtin-mp3"},
	{"audio", "m4a", "M4A", []string{"苹果", "苹果音频", "AAC"}, "builtin-m4a"},
	{"audio", "aac", "AAC", []string{"高级音频编码"}, "builtin-aac"},
	{"audio", "wav", "WAV", []string{"无损", "波形", "CD 音质"}, "builtin-wav"},
	{"audio", "flac", "FLAC", []string{"无损", "无损压缩"}, "builtin-flac"},
	{"audio", "ogg", "OGG", []string{"Vorbis", "Ogg 音频"}, "builtin-ogg"},
	{"audio", "opus", "Opus", []string{"语音", "网络音频"}, "builtin-opus"},
	{"audio", "wma", "WMA", []string{"Windows 音频", "微软音频"}, "builtin-wma"},
	{"audio", "amr", "AMR", []string{"录音", "手机录音", "语音"}, "builtin-amr"},
	{"audio", "m4r", "M4R", []string{"苹果铃声", "铃声", "iPhone 铃声"}, "builtin-m4r"},
	{"audio", "mp2", "MP2", []string{"MPEG 音频", "广播"}, "builtin-mp2"},
	{"audio", "ape", "APE", []string{"无损", "Monkey's Audio"}, "builtin-ape"},
	{"audio", "wv", "WV", []string{"无损", "WavPack"}, "builtin-wv"},
	{"audio", "mmf", "MMF", []string{"手机铃声", "彩铃"}, "builtin-mmf"},
	{"image", "jpg", "JPG", []string{"照片", "图片", "JPEG"}, "builtin-jpg"},
	{"image", "png", "PNG", []string{"透明图片", "截图", "图片"}, "builtin-png"},
	{"image", "webp", "WebP", []string{"网页图片", "图片"}, "builtin-webp"},
	{"image", "ico", "ICO", []string{"图标"}, "builtin-ico"},
	{"image", "bmp", "BMP", []string{"位图", "图片"}, "builtin-bmp"},
	{"image", "tif", "TIF", []string{"TIFF", "印刷", "图片"}, "builtin-tif"},
	{"image", "tga", "TGA", []string{"Targa", "游戏贴图"}, "builtin-tga"},
}

// inputExts 是 AddSources 接受的输入扩展名（契约 6.16.4）：目录里全部扩展名 + jpeg tiff m4v mpeg ts mts m2ts aif aiff。
var inputExts = func() map[string]bool {
	m := map[string]bool{}
	for _, d := range formatDefs {
		m["."+d.ext] = true
	}
	for _, e := range strings.Fields("jpeg tiff m4v mpeg ts mts m2ts aif aiff") {
		m["."+e] = true
	}
	return m
}()

// capabilities 是一次 -muxers / -encoders 检测的结果。
type capabilities struct {
	muxers, encoders map[string]bool
}

// capsProbe 运行检测命令（测试里替换）。返回 -muxers 和 -encoders 的标准输出。
type capsProbe func(ctx context.Context, exe string) (muxers, encoders string, err error)

func realCapsProbe(ctx context.Context, exe string) (string, string, error) {
	run := ffmpeg.ExecRunner(10 * time.Second)
	mux, err := run(ctx, exe, "-hide_banner", "-muxers")
	if err != nil {
		return "", "", err
	}
	enc, err := run(ctx, exe, "-hide_banner", "-encoders")
	if err != nil {
		return "", "", err
	}
	return mux, enc, nil
}

// parseMuxers 解析 `ffmpeg -muxers`：分隔线之后每行 “<标志> <名字[,名字]> <说明>”，标志含 E 的才算。
func parseMuxers(out string) map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(strings.TrimRight(line, "\r"))
		if len(f) < 2 || f[1] == "=" || len(f[0]) > 3 || !strings.Contains(f[0], "E") || strings.Trim(f[0], "DEd.") != "" {
			continue
		}
		for _, n := range strings.Split(f[1], ",") {
			set[n] = true
		}
	}
	return set
}

// parseAllEncoders 解析 `ffmpeg -encoders`：每行 “<6 位标志> <名字> <说明>”，视频、音频、字幕编码器都算。
func parseAllEncoders(out string) map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(strings.TrimRight(line, "\r"))
		if len(f) < 2 || f[1] == "=" || len(f[0]) != 6 || !strings.ContainsAny(f[0][:1], "VAS") {
			continue
		}
		set[f[1]] = true
	}
	return set
}

// catalogCache 按转换组件“路径 + 大小 + 修改时间”缓存检测结果（换了转换组件即失效）；同时只检测一次。
type catalogCache struct {
	mu    sync.Mutex
	key   string
	caps  *capabilities
	probe capsProbe
	// inflight 让并发调用等同一次检测。
	inflight chan struct{}
}

func binKey(exe string) string {
	fi, err := os.Stat(exe)
	if err != nil {
		return exe
	}
	return fmt.Sprintf("%s|%d|%d", exe, fi.Size(), fi.ModTime().UnixNano())
}

// get 返回当前转换组件的能力；检测失败返回 err（结果不缓存）。
func (c *catalogCache) get(ctx context.Context, exe string) (*capabilities, error) {
	key := binKey(exe)
	for {
		c.mu.Lock()
		if c.caps != nil && c.key == key {
			caps := c.caps
			c.mu.Unlock()
			return caps, nil
		}
		if c.inflight != nil {
			ch := c.inflight
			c.mu.Unlock()
			select {
			case <-ch:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		ch := make(chan struct{})
		c.inflight = ch
		probe := c.probe
		c.mu.Unlock()

		if probe == nil {
			probe = realCapsProbe
		}
		mux, enc, err := probe(ctx, exe)
		c.mu.Lock()
		c.inflight = nil
		close(ch)
		if err != nil {
			c.mu.Unlock()
			return nil, err
		}
		caps := &capabilities{muxers: parseMuxers(mux), encoders: parseAllEncoders(enc)}
		c.key, c.caps = key, caps
		c.mu.Unlock()
		return caps, nil
	}
}

// encodability 判断一组参数在这份能力下能不能输出：返回 reasonCode（"" 表示可以）。
func (caps *capabilities) encodability(o ffmpeg.ConvertOptions) string {
	mux, encs := ffmpeg.RequiredEncoders(o)
	if mux == "" || !caps.muxers[mux] {
		return ReasonMissingMuxer
	}
	for _, e := range encs {
		if !caps.encoders[e] {
			return ReasonMissingEncoder
		}
	}
	return ""
}

// capabilities 返回当前转换组件的能力：未就绪返回 reasonCode converter_not_ready，检测失败返回 check_failed。
func (s *Service) capabilities(ctx context.Context) (*capabilities, string) {
	bin, err := s.cfg.Require()
	if err != nil {
		return nil, ReasonConverterNotReady
	}
	caps, err := s.catalog.get(ctx, bin.FFmpeg)
	if err != nil {
		return nil, ReasonCheckFailed
	}
	return caps, ""
}

// GetFormatCatalog 返回格式目录（契约 6.16）。转换组件没就绪、检测失败时照样返回完整目录，只是全部不可输出，不报错。
func (s *Service) GetFormatCatalog(ctx context.Context) ([]FormatEntry, error) {
	presets, err := s.ListPresets(ctx)
	if err != nil {
		return nil, err
	}
	byFormat := map[string][]FormatPreset{}
	defaults := map[string]ffmpeg.ConvertOptions{}
	for _, p := range presets {
		byFormat[p.Options.Container] = append(byFormat[p.Options.Container], FormatPreset{
			ID: p.ID, Name: p.Name, BuiltIn: p.BuiltIn, ParamsSummary: ParamsSummary(p.Options), Options: p.Options,
		})
	}
	for _, p := range builtinPresets() {
		defaults[p.ID] = p.Options
	}
	caps, failCode := s.capabilities(ctx)
	out := make([]FormatEntry, 0, len(formatDefs))
	for _, d := range formatDefs {
		e := FormatEntry{Category: d.category, Extension: d.ext, DisplayName: d.name, Aliases: append([]string{}, d.aliases...),
			DefaultPresetID: d.defaultPreset, Presets: byFormat[d.ext]}
		if e.Presets == nil {
			e.Presets = []FormatPreset{}
		}
		code := failCode
		if caps != nil {
			code = caps.encodability(defaults[d.defaultPreset])
		}
		if code != "" {
			e.ReasonCode = code
			switch code {
			case ReasonConverterNotReady:
				e.Reason = msgNotReady
			case ReasonCheckFailed:
				e.Reason = msgCheckFailed
			default:
				e.Reason = msgUnsupported
			}
		} else {
			e.Encodable = true
		}
		out = append(out, e)
	}
	return out, nil
}

// checkFormat 是提交时的格式检查（契约 6.16.6）：格式不可输出 UNSUPPORTED reason=format；
// 格式可输出但所选编码没有编码器 UNSUPPORTED reason=encoder。转换组件没就绪由 Require 先报；
// 检测命令本身失败时不拦（v0.24.1 实现取舍：检测失败不代表不能转，交给 ffmpeg 自己报错）。
func (s *Service) checkFormat(ctx context.Context, o ffmpeg.ConvertOptions) error {
	caps, code := s.capabilities(ctx)
	if caps == nil {
		_ = code
		return nil
	}
	def := ""
	for _, d := range formatDefs {
		if d.ext == o.Container {
			def = d.defaultPreset
		}
	}
	for _, p := range builtinPresets() {
		if p.ID == def {
			if caps.encodability(p.Options) != "" {
				return apperr.New(apperr.Unsupported, msgUnsupported).WithDetail("reason=format")
			}
		}
	}
	if caps.encodability(o) != "" {
		return apperr.New(apperr.Unsupported, msgNoEncoder).WithDetail("reason=encoder")
	}
	return nil
}
