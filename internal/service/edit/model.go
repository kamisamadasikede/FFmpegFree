// Package edit 是 EditService 的实现（契约 6.11）：多轨时间线校验、导出任务、工程存取、预览 URL。
package edit

// 限制（契约 6.11.2 与架构师确认的决定）。越界一律 INVALID_ARGUMENT，不再静默截断。
const (
	MaxClips        = 100      // 视频 + 音频 clip 总数
	MaxSources      = 200      // 素材库条目
	MaxProjectBytes = 1 << 20  // 序列化后的工程
	MaxTimelineSec  = 6 * 3600 // 时间线总长
	MaxNameRunes    = 80       // 工程名
	MaxIDLen        = 64       // clip id
	MaxTracks       = 8        // V1~V8、A1~A8

	// ContiguousGapSec：同轨后一个 clip 与前一个的间隙 ≤ 此值视为首尾相接，> 此值是空隙（导出时补黑场 / 静音）。
	ContiguousGapSec = 0.12
	// MinClipDurSec：clip 按速度折算后的最短时长，更短的导出会得到 0 帧。
	MinClipDurSec = 0.04
	// snapSec：outSec 超过素材时长在此范围内静默取整，更大则截断并记 warning。
	snapSec = 0.05
	// overlapEpsSec：判断同轨重叠时的浮点容差（前端拖拽产生的 3.3000000000000003 之类不算重叠）。
	overlapEpsSec = 1e-6
)

// 警告码：只追加，不改名、不删除（前端按 code 做本地化 / 高亮）。
const (
	WarnOutTruncated      = "OUT_TRUNCATED"      // clip.outSec 超过素材时长，已截断（clipId 有值）
	WarnVideoGap          = "VIDEO_GAP"          // 时间线上一段没有任何画面，导出时补黑场（clipId 是空隙后的第一个 clip，末尾空隙为空）
	WarnTransitionIgnored = "TRANSITION_IGNORED" // clip 设了转场但与下一个 clip 不首尾相接（或没有下一个），转场被忽略（clipId 有值）
	WarnTransitionClamped = "TRANSITION_CLAMPED" // 默认转场时长（0.5 秒）超过相邻片段较短者的一半，已缩短（clipId 有值）
	WarnNoAudioTrack      = "NO_AUDIO_TRACK"     // 音轨为空，导出静音音轨
)

// EditProject 是工程（契约 6.11.1）。
type EditProject struct {
	SchemaVersion int           `json:"schemaVersion"` // 当前 1；大于 1 → UNSUPPORTED；0 视为 1
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Sources       []string      `json:"sources"`
	Output        EditOutput    `json:"output"`
	VideoTrack    []VideoClip   `json:"videoTrack"`
	AudioTrack    []AudioClip   `json:"audioTrack"`
	Effects       GlobalEffects `json:"effects"`
	UpdatedAt     int64         `json:"updatedAt"`
}

type EditOutput struct {
	Format string  `json:"format"` // mp4 | mov | mkv | webm，空 = mp4
	Width  int     `json:"width"`  // 16~7680，0 = 1280；导出时向下取偶数
	Height int     `json:"height"` // 16~4320，0 = 720
	Fps    float64 `json:"fps"`    // (0,120]，0 = 30
}

type VideoClip struct {
	ID                    string  `json:"id"`
	Path                  string  `json:"path"`
	TrackID               string  `json:"trackId"` // V1~V8，编号大的盖在上面
	StartSec              float64 `json:"startSec"`
	InSec                 float64 `json:"inSec"`
	OutSec                float64 `json:"outSec"` // 必须 > inSec；0 不表示"到结尾"，一律 INVALID_ARGUMENT
	Speed                 float64 `json:"speed"`  // 0.25~4，0 = 1
	EffectPreset          string  `json:"effectPreset"`
	TransitionToNext      string  `json:"transitionToNext"`
	TransitionDurationSec float64 `json:"transitionDurationSec"` // 0 = 0.5；否则 0.1~2，且不超过相邻两个 clip 中较短者的一半
	Blur                  float64 `json:"blur"`                  // 0~4
}

type AudioClip struct {
	ID       string  `json:"id"`
	Path     string  `json:"path"`
	TrackID  string  `json:"trackId"` // A1~A8
	StartSec float64 `json:"startSec"`
	InSec    float64 `json:"inSec"`
	OutSec   float64 `json:"outSec"`
	Speed    float64 `json:"speed"`  // 0.25~4，0 = 1
	Volume   float64 `json:"volume"` // 0~4，0 = 静音
}

type GlobalEffects struct {
	Brightness float64 `json:"brightness"` // -0.5~0.5
	Contrast   float64 `json:"contrast"`   // 0.5~2，0 视为 1
	Saturation float64 `json:"saturation"` // 0~2，0 视为 1
	Sharpen    float64 `json:"sharpen"`    // 0~2
}

type EditExportOptions struct {
	OutputName string `json:"outputName"` // 不含扩展名；空 = 工程名；按 fsutil.SanitizeFileName 净化，空则 "edit"
	OutputDir  string `json:"outputDir"`  // 空 = Settings.defaultOutputDir，仍空 = 第一个视频 clip 所在文件夹；必须是绝对路径
}

// EditWarning 是 EditPlan.Warnings 的稳定结构。Code 见 Warn* 常量（只追加）。
type EditWarning struct {
	Code    string `json:"code"`
	ClipID  string `json:"clipId,omitempty"`
	Message string `json:"message"`
}

type EditPlan struct {
	DurationSec float64       `json:"durationSec"`
	ClipCount   int           `json:"clipCount"`
	Inputs      []string      `json:"inputs"`
	HasAudio    bool          `json:"hasAudio"`
	Warnings    []EditWarning `json:"warnings"`
}

type EditProjectMeta struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	DurationSec float64 `json:"durationSec"`
	ClipCount   int     `json:"clipCount"`
	UpdatedAt   int64   `json:"updatedAt"`
}

type LoadedProject struct {
	Project      EditProject `json:"project"`
	MissingPaths []string    `json:"missingPaths"`
}

type PreviewURL struct {
	URL  string `json:"url"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
}
