package lang

import (
	"context"
	"path/filepath"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/langasr"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

type speechRunner struct {
	s    *Service
	p    speechParams
	res  *store.TaskResult
	task string // set on Submitted
}

func (s *Service) newRunner(p speechParams) *speechRunner {
	return &speechRunner{s: s, p: p}
}

func (r *speechRunner) Pool() task.Pool { return task.PoolASR }

func (r *speechRunner) Submitted(id string) { r.task = id }

func (r *speechRunner) Abandoned() {}

func (r *speechRunner) Result() *store.TaskResult { return r.res }

func (r *speechRunner) OnFinish(t task.Task) {
	if r.s.cfg.TempRoot != "" && t.ID != "" {
		langasr.CleanupTaskDir(r.s.cfg.TempRoot, t.ID)
	}
}

func (r *speechRunner) Run(ctx context.Context, report func(task.Progress)) (string, error) {
	_ = report
	taskID := r.task
	if taskID == "" {
		if info, ok := task.InfoFrom(ctx); ok {
			taskID = info.ID
		}
	}
	if taskID == "" {
		return "", apperr.New(apperr.Internal, "内部错误").WithDetail("missing task id")
	}
	tmp := filepath.Join(r.s.cfg.TempRoot, taskID)
	wav := filepath.Join(tmp, "audio.wav")
	asrDir := filepath.Join(tmp, "asr")
	defer langasr.CleanupTaskDir(r.s.cfg.TempRoot, taskID)

	bin, err := r.s.cfg.Require()
	if err != nil {
		return "", err
	}
	root := r.s.cfg.Asr.ComponentRoot()
	if root == "" {
		return "", r.s.cfg.Asr.NotReadyError()
	}
	tier := r.p.Tier
	if tier == "" {
		tier = r.s.cfg.Tier(ctx)
	}
	hd := tier == langasr.TierHD
	if hd {
		r.s.cfg.Asr.Gate().EnterHD()
		defer r.s.cfg.Asr.Gate().LeaveHD()
	}
	if err := langasr.ExtractMono16kWAV(ctx, bin.FFmpeg, r.p.Path, wav); err != nil {
		return "", err
	}
	cues, err := langasr.RunASR(ctx, langasr.RunOptions{
		ComponentRoot: root,
		WorkDir:       asrDir,
		AudioPath:     wav,
		Language:      r.p.Language,
		Tier:          tier,
		Logf:          r.s.cfg.Logf,
	})
	if err != nil {
		return "", err
	}
	// 把 cues 塞进 TaskResult（无迁移）
	r.res = &store.TaskResult{Cues: toStoreCues(cues)}
	return "", nil
}

func toStoreCues(in []langasr.SubtitleCue) []store.SubtitleCue {
	out := make([]store.SubtitleCue, len(in))
	for i, c := range in {
		out[i] = store.SubtitleCue{ID: c.ID, Text: c.Text, StartMs: c.StartMs, EndMs: c.EndMs}
	}
	return out
}
