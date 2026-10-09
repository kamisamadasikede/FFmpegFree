// Package lang 实现语音工具 · 转字幕一期（契约 6.18）：组件状态、提交识别、导出字幕。
package lang

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/langasr"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// Config 依赖。
type Config struct {
	Asr      *langasr.Manager
	Tasks    TaskAPI
	TempRoot string // <data>/tmp/lang
	Require  func() (ffmpeg.Binaries, error)
	Tier     func(ctx context.Context) string
	Emit     func(event string, payload any)
	Logf     func(format string, args ...any)
	// ActualOutputDir 导出默认目录。
	ActualOutputDir func(ctx context.Context) string
}

// TaskAPI 是任务管理器能力。
type TaskAPI interface {
	Submit(spec task.Spec, r task.Runner) (task.Task, error)
	RegisterFactory(t task.Type, f task.Factory)
	Get(id string) (task.Task, error)
}

// Service 是 LangService 实现。
type Service struct {
	cfg Config
}

// New 创建服务并注册 speech_to_subtitle 重试工厂；清理 tmp/lang 残留。
func New(cfg Config) *Service {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.Require == nil {
		cfg.Require = ffmpeg.Require
	}
	if cfg.Tier == nil {
		cfg.Tier = func(context.Context) string { return langasr.TierStandard }
	}
	s := &Service{cfg: cfg}
	if cfg.TempRoot != "" {
		langasr.CleanupResidues(cfg.TempRoot)
	}
	if cfg.Tasks != nil {
		cfg.Tasks.RegisterFactory(task.TypeSpeechToSubtitle, s.retryFactory)
	}
	return s
}

// ---------- 组件 ----------

func (s *Service) GetLangAsrStatus(ctx context.Context) (langasr.Status, error) {
	_ = ctx
	if s.cfg.Asr == nil {
		return langasr.Status{}, apperr.New(apperr.Internal, "语音识别服务尚未初始化")
	}
	return s.cfg.Asr.Status(), nil
}

func (s *Service) InstallLangAsr(ctx context.Context, tier string) (langasr.Status, error) {
	_ = ctx
	if s.cfg.Asr == nil {
		return langasr.Status{}, apperr.New(apperr.Internal, "语音识别服务尚未初始化")
	}
	return s.cfg.Asr.Install(tier)
}

func (s *Service) CancelLangAsrInstall(ctx context.Context) error {
	_ = ctx
	if s.cfg.Asr == nil {
		return apperr.New(apperr.Internal, "语音识别服务尚未初始化")
	}
	s.cfg.Asr.CancelInstall()
	return nil
}

func (s *Service) RecheckLangAsr(ctx context.Context) (langasr.Status, error) {
	_ = ctx
	if s.cfg.Asr == nil {
		return langasr.Status{}, apperr.New(apperr.Internal, "语音识别服务尚未初始化")
	}
	return s.cfg.Asr.Recheck(), nil
}

// ComponentDir 返回当前档已安装目录（OpenStorageFolder("lang_asr")）。
func (s *Service) ComponentDir() (string, error) {
	if s.cfg.Asr == nil {
		return "", apperr.New(apperr.NotFound, "语音识别组件还没有就绪。").WithDetail("reason=component")
	}
	st := s.cfg.Asr.Status()
	if st.State != langasr.StateReady || st.Path == "" {
		return "", apperr.New(apperr.NotFound, "语音识别组件还没有就绪。").WithDetail("reason=component")
	}
	return st.Path, nil
}

// OnAsrTierChanged 设置档切换后刷新引导体积（不自动下载）。
func (s *Service) OnAsrTierChanged() {
	if s.cfg.Asr != nil {
		s.cfg.Asr.RefreshTierGuide()
	}
}

// HWAcquire 供转换页在硬件编码前调用。
func (s *Service) HWAcquire(ctx context.Context) (func(), error) {
	if s.cfg.Asr == nil {
		return func() {}, nil
	}
	return s.cfg.Asr.Gate().AcquireHWEncode(ctx)
}

// ---------- 请求类型 ----------

// SpeechToSubtitleRequest 见契约 6.18.5。
type SpeechToSubtitleRequest struct {
	Paths     []string `json:"paths"`
	Language  string   `json:"language,omitempty"`
	Format    string   `json:"format"`
	OutputDir string   `json:"outputDir,omitempty"`
}

// speechParams 落库在 task.Params。
type speechParams struct {
	Path      string `json:"path"`
	Language  string `json:"language"`
	Format    string `json:"format"`
	OutputDir string `json:"outputDir,omitempty"`
	Tier      string `json:"tier"`
}

// ExportSubtitleRequest / Result。
type ExportSubtitleRequest struct {
	TaskID     string                `json:"taskId"`
	Cues       []langasr.SubtitleCue `json:"cues"`
	Format     string                `json:"format"`
	TargetPath string                `json:"targetPath,omitempty"`
}

type ExportSubtitleResult struct {
	Path string `json:"path"`
}

// SubmitSpeechToSubtitle 为每个输入提交一个 speech_to_subtitle 任务。
func (s *Service) SubmitSpeechToSubtitle(ctx context.Context, req SpeechToSubtitleRequest) ([]store.Task, error) {
	if s.cfg.Tasks == nil || s.cfg.Asr == nil {
		return nil, apperr.New(apperr.Internal, "语音识别服务尚未初始化")
	}
	if len(req.Paths) == 0 {
		return nil, apperr.New(apperr.InvalidArgument, "请选择要转成字幕的文件")
	}
	if len(req.Paths) > 50 {
		return nil, apperr.New(apperr.InvalidArgument, "一次最多选择 50 个文件")
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "srt"
	}
	if format != "srt" && format != "vtt" {
		return nil, apperr.New(apperr.InvalidArgument, "字幕格式不正确").WithDetail("format=" + format)
	}
	lang := req.Language
	if lang == "" {
		lang = "auto"
	}
	if _, err := s.cfg.Require(); err != nil {
		return nil, err
	}
	st := s.cfg.Asr.Status()
	if st.State != langasr.StateReady {
		return nil, s.cfg.Asr.NotReadyError()
	}
	tier := s.cfg.Tier(ctx)
	out := make([]store.Task, 0, len(req.Paths))
	for _, p := range req.Paths {
		if !filepath.IsAbs(p) {
			return nil, apperr.New(apperr.InvalidArgument, "文件路径必须是绝对路径").WithDetail(p)
		}
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			return nil, apperr.New(apperr.NotFound, "找不到这个文件").WithDetail(p)
		}
		base := filepath.Base(p)
		title := strings.TrimSuffix(base, filepath.Ext(base)) + " → 字幕"
		sp := speechParams{Path: p, Language: lang, Format: format, OutputDir: req.OutputDir, Tier: tier}
		pj, _ := json.Marshal(sp)
		r := s.newRunner(sp)
		tk, err := s.cfg.Tasks.Submit(task.Spec{
			Type: task.TypeSpeechToSubtitle, Title: title,
			InputPaths: []string{p}, Params: string(pj),
		}, r)
		if err != nil {
			return nil, err
		}
		out = append(out, tk)
	}
	return out, nil
}

func (s *Service) retryFactory(old task.Task) (task.Runner, error) {
	if old.Error != nil && old.Error.Code == apperr.LangAsrEmpty {
		return nil, apperr.New(apperr.Unsupported, "这段音频里没有识别到有效内容。").WithDetail("reason=not_retryable")
	}
	var p speechParams
	if err := json.Unmarshal([]byte(old.Params), &p); err != nil || p.Path == "" {
		return nil, apperr.New(apperr.InvalidArgument, "任务参数无效，无法重试")
	}
	if s.cfg.Asr.Status().State != langasr.StateReady {
		return nil, s.cfg.Asr.NotReadyError()
	}
	if _, err := s.cfg.Require(); err != nil {
		return nil, err
	}
	return s.newRunner(p), nil
}

// ExportSubtitleCues 硬校验后写 SRT/VTT。
func (s *Service) ExportSubtitleCues(ctx context.Context, req ExportSubtitleRequest) (ExportSubtitleResult, error) {
	if err := langasr.ValidateCues(req.Cues); err != nil {
		return ExportSubtitleResult{}, err
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format != "srt" && format != "vtt" {
		return ExportSubtitleResult{}, apperr.New(apperr.InvalidArgument, "字幕格式不正确").WithDetail("format=" + format)
	}
	path := req.TargetPath
	if path == "" {
		base := "字幕"
		if req.TaskID != "" && s.cfg.Tasks != nil {
			if tk, err := s.cfg.Tasks.Get(req.TaskID); err == nil && len(tk.InputPaths) > 0 {
				b := filepath.Base(tk.InputPaths[0])
				base = strings.TrimSuffix(b, filepath.Ext(b))
			}
		}
		dir := ""
		if s.cfg.ActualOutputDir != nil {
			dir = s.cfg.ActualOutputDir(ctx)
		}
		if dir == "" {
			return ExportSubtitleResult{}, apperr.New(apperr.InvalidArgument, "请指定保存位置")
		}
		path = filepath.Join(dir, base+"."+format)
	}
	if !filepath.IsAbs(path) {
		return ExportSubtitleResult{}, apperr.New(apperr.InvalidArgument, "保存路径必须是绝对路径").WithDetail(path)
	}
	if err := langasr.WriteSubtitleFile(path, format, req.Cues); err != nil {
		return ExportSubtitleResult{}, err
	}
	return ExportSubtitleResult{Path: path}, nil
}
