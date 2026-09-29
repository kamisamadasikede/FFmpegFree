package edit

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/store"
)

// fakeMedia 让校验测试不依赖 ffmpeg。
type fakeMedia struct{ m map[string]store.MediaInfo }

func (f fakeMedia) Inspect(_ context.Context, p string) (store.MediaInfo, error) {
	if mi, ok := f.m[p]; ok {
		return mi, nil
	}
	return store.MediaInfo{}, apperr.New(apperr.NotFound, "文件不存在").WithDetail(p)
}

func fakeSvc(m map[string]store.MediaInfo) *Service {
	return New(Config{Media: fakeMedia{m}, Require: func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{FFmpeg: "x", FFprobe: "y"}, nil },
		SupportsScript: func(context.Context, string) (string, error) { return OptFilterFile, nil }})
}

var (
	pV   = filepath.Join(string(filepath.Separator), "m", "v.mp4")
	pV2  = filepath.Join(string(filepath.Separator), "m", "v2.mp4")
	pA   = filepath.Join(string(filepath.Separator), "m", "a.mp3")
	pAV  = filepath.Join(string(filepath.Separator), "m", "silent.mp4")
	base = map[string]store.MediaInfo{
		pV:  {Path: pV, Duration: 10, HasVideo: true, HasAudio: true},
		pV2: {Path: pV2, Duration: 5, HasVideo: true, HasAudio: true},
		pA:  {Path: pA, Duration: 8, HasAudio: true},
		pAV: {Path: pAV, Duration: 4, HasVideo: true},
	}
)

func proj(vs ...VideoClip) EditProject { return EditProject{Name: "p", VideoTrack: vs} }

func TestValidateOK(t *testing.T) {
	s := fakeSvc(base)
	p := proj(vclip("c1", pV, "V1", 0, 1, 4), vclip("c2", pV2, "V1", 3, 0, 2))
	p.AudioTrack = []AudioClip{aclip("a1", pA, "A1", 1, 0, 6)}
	pl, err := s.ValidateProject(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if pl.DurationSec != 7 || pl.ClipCount != 3 || !pl.HasAudio || len(pl.Inputs) != 3 || pl.Warnings == nil {
		t.Fatalf("%+v", pl)
	}
}

func TestValidateStructuralErrors(t *testing.T) {
	s := fakeSvc(base)
	ok := func() EditProject { return proj(vclip("c1", pV, "V1", 0, 0, 3)) }
	type tc struct {
		name string
		mut  func(p *EditProject)
		want string // detail 第一行
	}
	cases := []tc{
		{"空视频轨", func(p *EditProject) { p.VideoTrack = nil }, "project"},
		{"trackId 非 V", func(p *EditProject) { p.VideoTrack[0].TrackID = "A1" }, "clip=c1 path=" + pV},
		{"V9", func(p *EditProject) { p.VideoTrack[0].TrackID = "V9" }, "clip=c1 path=" + pV},
		{"V0", func(p *EditProject) { p.VideoTrack[0].TrackID = "V0" }, "clip=c1 path=" + pV},
		{"v1 小写", func(p *EditProject) { p.VideoTrack[0].TrackID = "v1" }, "clip=c1 path=" + pV},
		{"A 轨 id 不合法", func(p *EditProject) { p.AudioTrack = []AudioClip{aclip("a1", pA, "V1", 0, 0, 1)} }, "clip=a1 path=" + pA},
		{"A9", func(p *EditProject) { p.AudioTrack = []AudioClip{aclip("a1", pA, "A9", 0, 0, 1)} }, "clip=a1 path=" + pA},
		{"outSec=0", func(p *EditProject) { p.VideoTrack[0].OutSec = 0 }, "clip=c1 path=" + pV},
		{"outSec=inSec", func(p *EditProject) { p.VideoTrack[0].InSec, p.VideoTrack[0].OutSec = 2, 2 }, "clip=c1 path=" + pV},
		{"outSec<inSec", func(p *EditProject) { p.VideoTrack[0].InSec, p.VideoTrack[0].OutSec = 3, 2 }, "clip=c1 path=" + pV},
		{"音频 outSec=0", func(p *EditProject) { p.AudioTrack = []AudioClip{aclip("a1", pA, "A1", 0, 0, 0)} }, "clip=a1 path=" + pA},
		{"startSec<0", func(p *EditProject) { p.VideoTrack[0].StartSec = -1 }, "clip=c1 path=" + pV},
		{"inSec<0", func(p *EditProject) { p.VideoTrack[0].InSec = -1 }, "clip=c1 path=" + pV},
		{"speed 过小", func(p *EditProject) { p.VideoTrack[0].Speed = 0.2 }, "clip=c1 path=" + pV},
		{"speed 过大", func(p *EditProject) { p.VideoTrack[0].Speed = 4.5 }, "clip=c1 path=" + pV},
		{"speed 负", func(p *EditProject) { p.VideoTrack[0].Speed = -1 }, "clip=c1 path=" + pV},
		{"volume 负", func(p *EditProject) {
			p.AudioTrack = []AudioClip{{ID: "a1", Path: pA, TrackID: "A1", OutSec: 1, Volume: -1}}
		}, "clip=a1 path=" + pA},
		{"volume>4", func(p *EditProject) {
			p.AudioTrack = []AudioClip{{ID: "a1", Path: pA, TrackID: "A1", OutSec: 1, Volume: 4.1}}
		}, "clip=a1 path=" + pA},
		{"相对路径", func(p *EditProject) { p.VideoTrack[0].Path = "rel/v.mp4" }, "clip=c1 path=rel/v.mp4"},
		{"转场名", func(p *EditProject) { p.VideoTrack[0].TransitionToNext = "spin" }, "clip=c1 path=" + pV},
		{"转场时长过大", func(p *EditProject) { p.VideoTrack[0].TransitionDurationSec = 2.5 }, "clip=c1 path=" + pV},
		{"转场时长过小", func(p *EditProject) { p.VideoTrack[0].TransitionDurationSec = 0.05 }, "clip=c1 path=" + pV},
		{"模糊", func(p *EditProject) { p.VideoTrack[0].Blur = 5 }, "clip=c1 path=" + pV},
		{"预设", func(p *EditProject) { p.VideoTrack[0].EffectPreset = "neon" }, "clip=c1 path=" + pV},
		{"id 含空格", func(p *EditProject) { p.VideoTrack[0].ID = "c 1" }, "project"},
		{"id 含中文", func(p *EditProject) { p.VideoTrack[0].ID = "片段" }, "project"},
		{"id 空", func(p *EditProject) { p.VideoTrack[0].ID = "" }, "project"},
		{"id 65 字符", func(p *EditProject) { p.VideoTrack[0].ID = strings.Repeat("a", 65) }, "project"},
		{"id 重复", func(p *EditProject) { p.VideoTrack = append(p.VideoTrack, vclip("c1", pV, "V2", 0, 0, 1)) }, "project"},
		{"格式", func(p *EditProject) { p.Output.Format = "avi" }, "project"},
		{"宽度", func(p *EditProject) { p.Output.Width = 8000 }, "project"},
		{"宽度过小", func(p *EditProject) { p.Output.Width = 8 }, "project"},
		{"高度", func(p *EditProject) { p.Output.Height = 5000 }, "project"},
		{"fps", func(p *EditProject) { p.Output.Fps = 121 }, "project"},
		{"fps 负", func(p *EditProject) { p.Output.Fps = -1 }, "project"},
		{"亮度", func(p *EditProject) { p.Effects.Brightness = 0.6 }, "project"},
		{"对比度", func(p *EditProject) { p.Effects.Contrast = 3 }, "project"},
		{"饱和度", func(p *EditProject) { p.Effects.Saturation = 2.1 }, "project"},
		{"锐化", func(p *EditProject) { p.Effects.Sharpen = -1 }, "project"},
		{"sources 相对路径", func(p *EditProject) { p.Sources = []string{"x.mp4"} }, "project"},
	}
	for _, c := range cases {
		p := ok()
		c.mut(&p)
		_, err := s.ValidateProject(context.Background(), p)
		if err == nil {
			t.Errorf("%s: 期望失败", c.name)
			continue
		}
		if got := code(t, err); got != apperr.InvalidArgument {
			t.Errorf("%s: code=%s", c.name, got)
		}
		if got := firstLine(err); got != c.want {
			t.Errorf("%s: detail 第一行 %q，期望 %q", c.name, got, c.want)
		}
	}
}

func TestValidateNaNInf(t *testing.T) {
	s := fakeSvc(base)
	nan := func() float64 { z := 0.0; return z / z }()
	p := proj(vclip("c1", pV, "V1", 0, 0, 3))
	p.VideoTrack[0].StartSec = nan
	if _, err := s.ValidateProject(context.Background(), p); code(t, err) != apperr.InvalidArgument {
		t.Fatal("NaN")
	}
}

func TestValidateLimits(t *testing.T) {
	s := fakeSvc(base)
	// 100 个 clip 可以，101 个不行；限制包括视频 + 音频合计
	mk := func(nv, na int) EditProject {
		p := EditProject{Name: "p"}
		for i := 0; i < nv; i++ {
			p.VideoTrack = append(p.VideoTrack, vclip("v"+itoa(i), pV, "V"+itoa(i%8+1), float64(i/8)*0, 0, 0.5))
		}
		for i := 0; i < na; i++ {
			p.AudioTrack = append(p.AudioTrack, aclip("a"+itoa(i), pA, "A"+itoa(i%8+1), 0, 0, 0.5))
		}
		return p
	}
	// 让同轨 clip 依次排开避免重叠
	spread := func(p EditProject) EditProject {
		for i := range p.VideoTrack {
			p.VideoTrack[i].StartSec = float64(i/8) * 1
		}
		for i := range p.AudioTrack {
			p.AudioTrack[i].StartSec = float64(i/8) * 1
		}
		return p
	}
	if _, err := s.ValidateProject(context.Background(), spread(mk(60, 40))); err != nil {
		t.Fatalf("100 个应通过: %v", err)
	}
	_, err := s.ValidateProject(context.Background(), spread(mk(60, 41)))
	if code(t, err) != apperr.InvalidArgument || firstLine(err) != "project" {
		t.Fatalf("101 个应 INVALID_ARGUMENT: %v", err)
	}
	// 时间线 ≤ 6 小时
	big := map[string]store.MediaInfo{pV: {Path: pV, Duration: 30000, HasVideo: true}}
	s2 := fakeSvc(big)
	p := proj(vclip("c1", pV, "V1", 0, 0, 21600))
	if _, err := s2.ValidateProject(context.Background(), p); err != nil {
		t.Fatalf("恰好 6 小时应通过: %v", err)
	}
	p = proj(vclip("c1", pV, "V1", 0, 0, 21600.5))
	if _, err := s2.ValidateProject(context.Background(), p); code(t, err) != apperr.InvalidArgument {
		t.Fatalf("超过 6 小时: %v", err)
	}
	// 轨道：V1~V8、A1~A8 全部可用
	p = EditProject{Name: "p"}
	for i := 1; i <= 8; i++ {
		p.VideoTrack = append(p.VideoTrack, vclip("v"+itoa(i), pV, "V"+itoa(i), 0, 0, 1))
		p.AudioTrack = append(p.AudioTrack, aclip("a"+itoa(i), pA, "A"+itoa(i), 0, 0, 1))
	}
	if _, err := s.ValidateProject(context.Background(), p); err != nil {
		t.Fatalf("8 轨: %v", err)
	}
	// 素材库上限 100：恰好 100 通过，101 → INVALID_ARGUMENT（detail sources=101）；旧上限 200 已废
	if MaxSources != 100 {
		t.Fatalf("MaxSources = %d，产品经理定稿是 100", MaxSources)
	}
	srcs := func(n int) []string {
		out := make([]string, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, filepath.Join(string(filepath.Separator), "m", "s"+itoa(i)+".mp4"))
		}
		return out
	}
	p = proj(vclip("c1", pV, "V1", 0, 0, 1))
	p.Sources = srcs(100)
	if _, err := s.ValidateProject(context.Background(), p); err != nil {
		t.Fatalf("恰好 100 个素材应通过: %v", err)
	}
	p.Sources = srcs(101)
	_, err = s.ValidateProject(context.Background(), p)
	if code(t, err) != apperr.InvalidArgument || firstLine(err) != "project" || !strings.Contains(apperr.From(err).Message, "100") || !strings.Contains(apperr.From(err).Detail, "sources=101") {
		t.Fatalf("sources=101: %v", err)
	}
	p.Sources = srcs(150) // 旧上限 200 以内也不再允许
	if _, err := s.ValidateProject(context.Background(), p); code(t, err) != apperr.InvalidArgument {
		t.Fatal("sources=150 应被拒绝")
	}
	// 片段总数上限仍是 100（与素材上限相互独立）
	if MaxClips != 100 {
		t.Fatalf("MaxClips = %d", MaxClips)
	}
	// 工程 > 1 MiB
	p = proj(vclip("c1", pV, "V1", 0, 0, 1))
	p.Name = strings.Repeat("字", 80)
	for i := 0; i < MaxSources; i++ { // 100 × 12000 字符 > 1 MiB
		p.Sources = append(p.Sources, filepath.Join(string(filepath.Separator), "m", strings.Repeat("x", 12000)+itoa(i)+".mp4"))
	}
	if _, err := s.ValidateProject(context.Background(), p); code(t, err) != apperr.InvalidArgument || !strings.Contains(apperr.From(err).Message, "1 MiB") {
		t.Fatalf("1 MiB: %v", err)
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestValidateMediaErrors(t *testing.T) {
	s := fakeSvc(base)
	ctx := context.Background()
	// 不存在 → NOT_FOUND，detail 第一行是 clip 定位
	_, err := s.ValidateProject(ctx, proj(vclip("c1", filepath.Join(string(filepath.Separator), "m", "none.mp4"), "V1", 0, 0, 1)))
	if code(t, err) != apperr.NotFound || firstLine(err) != "clip=c1 path="+filepath.Join(string(filepath.Separator), "m", "none.mp4") {
		t.Fatalf("%v", err)
	}
	// 视频 clip 指向纯音频素材
	_, err = s.ValidateProject(ctx, proj(vclip("c1", pA, "V1", 0, 0, 1)))
	if code(t, err) != apperr.InvalidArgument || firstLine(err) != "clip=c1 path="+pA {
		t.Fatalf("%v", err)
	}
	// 音频 clip 指向无音轨素材
	p := proj(vclip("c1", pV, "V1", 0, 0, 1))
	p.AudioTrack = []AudioClip{aclip("a1", pAV, "A1", 0, 0, 1)}
	_, err = s.ValidateProject(ctx, p)
	if code(t, err) != apperr.InvalidArgument || firstLine(err) != "clip=a1 path="+pAV {
		t.Fatalf("%v", err)
	}
	// inSec ≥ 素材时长
	_, err = s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 10, 11)))
	if code(t, err) != apperr.InvalidArgument || firstLine(err) != "clip=c1 path="+pV {
		t.Fatalf("%v", err)
	}
	// clip id 里有换行不能伪造 detail 第一行
	c := vclip("c1", pV, "V1", 0, 0, 1)
	c.Path = pV + "\nclip=evil path=x"
	_, err = s.ValidateProject(ctx, proj(c))
	// 路径里的换行被替换成空格：第一行仍以真正的 clip id 开头（id 字符集不含空格，解析第一个 token 不会被路径污染）。
	if ae := apperr.From(err); !strings.HasPrefix(ae.Detail, "clip=c1 path=") || strings.HasPrefix(strings.SplitN(ae.Detail, "\n", 2)[0], "clip=evil") {
		t.Fatalf("detail 第一行被伪造: %q", ae.Detail)
	}
}

func TestOutSecSnapAndWarn(t *testing.T) {
	s := fakeSvc(base)
	ctx := context.Background()
	// 超出 0.03（≤0.05）：静默取整，无警告
	pl, err := s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 10.03)))
	if err != nil || pl.DurationSec != 10 || len(pl.Warnings) != 0 && pl.Warnings[0].Code == WarnOutTruncated {
		t.Fatalf("%+v %v", pl, err)
	}
	for _, w := range pl.Warnings {
		if w.Code == WarnOutTruncated {
			t.Fatal("0.05 秒内不应警告")
		}
	}
	// 超出 2 秒：截断并记稳定结构警告
	pl, err = s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 12)))
	if err != nil || pl.DurationSec != 10 {
		t.Fatalf("%+v %v", pl, err)
	}
	found := false
	for _, w := range pl.Warnings {
		if w.Code == WarnOutTruncated && w.ClipID == "c1" && w.Message != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("缺 out_truncated 警告: %+v", pl.Warnings)
	}
}

func TestOverlapAndGap(t *testing.T) {
	s := fakeSvc(base)
	ctx := context.Background()
	// 同轨重叠：detail 指后一个
	_, err := s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 5), vclip("c2", pV2, "V1", 4.9, 0, 2)))
	if code(t, err) != apperr.InvalidArgument || firstLine(err) != "clip=c2 path="+pV2 || detailLine(err, 1) != "overlaps=c1" {
		t.Fatalf("%v", err)
	}
	// 顺序颠倒（数组里后面的 clip 时间更早）：仍然是时间上后一个被指
	_, err = s.ValidateProject(ctx, proj(vclip("late", pV2, "V1", 4.9, 0, 2), vclip("early", pV, "V1", 0, 0, 5)))
	if firstLine(err) != "clip=late path="+pV2 || detailLine(err, 1) != "overlaps=early" {
		t.Fatalf("%v", firstLine(err))
	}
	// 不同轨可以重叠
	if _, err := s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 5), vclip("c2", pV2, "V2", 1, 0, 2))); err != nil {
		t.Fatal(err)
	}
	// 音频同轨重叠
	p := proj(vclip("c1", pV, "V1", 0, 0, 5))
	p.AudioTrack = []AudioClip{aclip("a1", pA, "A1", 0, 0, 4), aclip("a2", pA, "A1", 3, 0, 2)}
	if _, err := s.ValidateProject(ctx, p); firstLine(err) != "clip=a2 path="+pA {
		t.Fatalf("%v", err)
	}
	// 恰好首尾相接（含浮点误差）不算重叠
	if _, err := s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0.1, 0, 3.3), vclip("c2", pV2, "V1", 3.4000000000000004, 0, 1))); err != nil {
		t.Fatalf("首尾相接: %v", err)
	}
	// 毫秒取整：4.9996 秒开始 ≈ 5.000（不重叠）；4.9994 ≈ 4.999（重叠）
	if _, err := s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 5), vclip("c2", pV2, "V1", 4.9996, 0, 1))); err != nil {
		t.Fatalf("取整到 5.000 应不算重叠: %v", err)
	}
	if _, err := s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 5), vclip("c2", pV2, "V1", 4.9994, 0, 1))); firstLine(err) != "clip=c2 path="+pV2 {
		t.Fatalf("取整到 4.999 应重叠: %v", err)
	}
	// 间隙 0.12 秒：视为首尾相接（不报 clip_gap）；0.13 秒：空隙
	pl, err := s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 2), vclip("c2", pV2, "V1", 2.12, 0, 1)))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range pl.Warnings {
		if w.Code == WarnClipGap {
			t.Fatalf("0.12 秒不算空隙: %+v", w)
		}
	}
	pl, _ = s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 2), vclip("c2", pV2, "V1", 2.13, 0, 1)))
	gap := false
	for _, w := range pl.Warnings {
		if w.Code == WarnClipGap && w.ClipID == "c2" {
			gap = true
		}
	}
	if !gap {
		t.Fatalf("0.13 秒应报 clip_gap: %+v", pl.Warnings)
	}
	// 片头空隙：视频轨第一个 clip startSec>0.12 → leading_gap（clipId=该 clip）；0.12 以内不报
	pl, _ = s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 1, 0, 2)))
	if len(pl.Warnings) == 0 || pl.Warnings[0].Code != WarnLeadingGap || pl.Warnings[0].ClipID != "c1" {
		t.Fatalf("开头空隙: %+v", pl.Warnings)
	}
	pl, _ = s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0.12, 0, 2)))
	if hasWarn(pl, WarnLeadingGap) {
		t.Fatalf("0.12 秒不算片头空隙: %+v", pl.Warnings)
	}
	// 音频轨：leading_gap 与同轨 clip_gap（clipId=后一个）；有音轨则没有 no_audio_track
	p = proj(vclip("c1", pV, "V1", 0, 0, 6))
	p.AudioTrack = []AudioClip{aclip("a1", pA, "A1", 1, 0, 1), aclip("a2", pA, "A1", 3, 0, 1)}
	pl, _ = s.ValidateProject(ctx, p)
	var got []string
	for _, w := range pl.Warnings {
		got = append(got, w.Code+":"+w.ClipID)
	}
	if strings.Join(got, ",") != "leading_gap:a1,clip_gap:a2" || pl.DurationSec != 6 || hasWarn(pl, WarnNoAudioTrack) {
		t.Fatalf("%v %+v", got, pl)
	}
	// 音频比视频长：不再有"末尾黑场"警告（契约只有同轨相邻 / 片头）
	p = proj(vclip("c1", pV, "V1", 0, 0, 2))
	p.AudioTrack = []AudioClip{aclip("a1", pA, "A1", 0, 0, 6)}
	pl, _ = s.ValidateProject(ctx, p)
	if pl.DurationSec != 6 || len(pl.Warnings) != 0 {
		t.Fatalf("%+v", pl)
	}
	pl, _ = s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 2)))
	if pl.HasAudio || !hasWarn(pl, WarnNoAudioTrack) {
		t.Fatalf("%+v", pl)
	}
}

func hasWarn(pl EditPlan, c string) bool {
	for _, w := range pl.Warnings {
		if w.Code == c {
			return true
		}
	}
	return false
}

func TestTransitionRules(t *testing.T) {
	s := fakeSvc(base)
	ctx := context.Background()
	mk := func(tr string, d float64, gap float64) EditProject {
		a := vclip("c1", pV, "V1", 0, 0, 4) // 4 秒
		a.TransitionToNext, a.TransitionDurationSec = tr, d
		return proj(a, vclip("c2", pV2, "V1", 4+gap, 0, 3)) // 3 秒
	}
	// 显式 1.5 = 较短(3)的一半，允许
	if _, err := s.ValidateProject(ctx, mk("fade", 1.5, 0)); err != nil {
		t.Fatal(err)
	}
	// 显式 1.6 超过一半 → INVALID_ARGUMENT，指向设了转场的 clip
	_, err := s.ValidateProject(ctx, mk("fade", 1.6, 0))
	if code(t, err) != apperr.InvalidArgument || firstLine(err) != "clip=c1 path="+pV {
		t.Fatalf("%v", err)
	}
	// 不首尾相接：转场被忽略，警告
	pl, err := s.ValidateProject(ctx, mk("fade", 0.5, 1))
	if err != nil || !hasWarn(pl, WarnTransitionIgnored) {
		t.Fatalf("%+v %v", pl, err)
	}
	// 最后一个 clip 设转场：忽略
	last := vclip("c1", pV, "V1", 0, 0, 2)
	last.TransitionToNext = "dissolve"
	pl, _ = s.ValidateProject(ctx, proj(last))
	if !hasWarn(pl, WarnTransitionIgnored) {
		t.Fatalf("%+v", pl)
	}
	// 默认时长 0.5 但相邻片段只有 0.6 秒（一半 0.3）→ 静默缩短（契约没有对应警告），不报错
	a := vclip("c1", pV, "V1", 0, 0, 0.6)
	a.TransitionToNext = "fade"
	pl, err = s.ValidateProject(ctx, proj(a, vclip("c2", pV2, "V1", 0.6, 0, 3)))
	if err != nil || hasWarn(pl, WarnTransitionIgnored) {
		t.Fatalf("%+v %v", pl, err)
	}
}

func TestSchemaVersionInValidate(t *testing.T) {
	s := fakeSvc(base)
	p := proj(vclip("c1", pV, "V1", 0, 0, 1))
	p.SchemaVersion = 2
	_, err := s.ValidateProject(context.Background(), p)
	if code(t, err) != apperr.Unsupported {
		t.Fatalf("%v", err)
	}
}

func TestFFmpegMissing(t *testing.T) {
	s := New(Config{Media: fakeMedia{base}, Require: func() (ffmpeg.Binaries, error) {
		return ffmpeg.Binaries{}, apperr.New(apperr.FFmpegNotFound, "no")
	}})
	_, err := s.ValidateProject(context.Background(), proj(vclip("c1", pV, "V1", 0, 0, 1)))
	if code(t, err) != apperr.FFmpegNotFound {
		t.Fatal(err)
	}
}

func TestValidateCanceled(t *testing.T) {
	s := fakeSvc(base)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 1)))
	if code(t, err) != apperr.Canceled {
		t.Fatalf("%v", err)
	}
}

// detailLine 返回 AppError.Detail 的第 n 行（0 起）。
func detailLine(err error, n int) string {
	lines := strings.Split(apperr.From(err).Detail, "\n")
	if n < len(lines) {
		return lines[n]
	}
	return ""
}

// 校验顺序（契约 6.11.2）：环境 → 工程级 → 逐 clip 字段（先视频后音频）→ 逐 clip 路径 / 探测 → 同轨重叠。第一个失败就返回。
func TestValidationOrder(t *testing.T) {
	s := fakeSvc(base)
	ctx := context.Background()
	missing := filepath.Join(string(filepath.Separator), "m", "nope.mp4")
	// 路径阶段（素材不存在）先于重叠阶段：c1 素材不存在，c3/c4 重叠 → 报 c1
	a, b := vclip("c1", missing, "V1", 0, 0, 1), vclip("c2", pV, "V2", 0, 0, 1)
	c3, c4 := vclip("c3", pV, "V3", 0, 0, 2), vclip("c4", pV2, "V3", 1, 0, 1)
	_, err := s.ValidateProject(ctx, proj(a, b, c3, c4))
	if code(t, err) != apperr.NotFound || firstLine(err) != "clip=c1 path="+missing {
		t.Fatalf("%v", err)
	}
	// 字段阶段先于路径阶段：c2 的 outSec=0，c1 的素材不存在 → 报 c2（字段）
	bad := vclip("c2", pV, "V1", 3, 0, 0)
	_, err = s.ValidateProject(ctx, proj(a, bad))
	if code(t, err) != apperr.InvalidArgument || firstLine(err) != "clip=c2 path="+pV {
		t.Fatalf("%v", err)
	}
	// 字段阶段先视频后音频：音频 a1 与视频 c2 都有字段错 → 报视频 c2
	p := proj(vclip("c1", pV, "V1", 0, 0, 1), bad)
	p.AudioTrack = []AudioClip{aclip("a1", pA, "A1", -1, 0, 1)}
	if _, err = s.ValidateProject(ctx, p); firstLine(err) != "clip=c2 path="+pV {
		t.Fatalf("%v", err)
	}
	// 工程级先于逐 clip：名称过长 + clip 字段错 → project
	p = proj(bad)
	p.Name = strings.Repeat("长", 81)
	if _, err = s.ValidateProject(ctx, p); firstLine(err) != "project" {
		t.Fatalf("%v", err)
	}
	// 同一素材探测失败记在按顺序第一个用到它的 clip 上（视频先于音频）
	p = proj(vclip("c1", missing, "V1", 0, 0, 1))
	p.AudioTrack = []AudioClip{aclip("a0", missing, "A1", 0, 0, 1)}
	if _, err = s.ValidateProject(ctx, p); firstLine(err) != "clip=c1 path="+missing {
		t.Fatalf("%v", err)
	}
	// 路径含换行 / 控制字符 → INVALID_ARGUMENT，detail 第一行仍然是一行（换行被替换），且不会探测
	nl := filepath.Join(string(filepath.Separator), "m", "a\nb.mp4")
	_, err = s.ValidateProject(ctx, proj(vclip("c1", nl, "V1", 0, 0, 1)))
	if code(t, err) != apperr.InvalidArgument || !detailRe.MatchString(firstLine(err)) {
		t.Fatalf("%v %q", err, firstLine(err))
	}
}

var detailRe = regexp.MustCompile(`^(?:clip=([A-Za-z0-9_-]{1,64}) path=(.*)|project)$`)

// 每个错误的 detail 第一行都符合契约正则。
func TestDetailFirstLineRegex(t *testing.T) {
	s := fakeSvc(base)
	ctx := context.Background()
	cases := []EditProject{
		{Name: "x"},                           // 视频轨为空
		proj(vclip("c 1", pV, "V1", 0, 0, 1)), // id 非法
		proj(vclip("c1", pV, "V9", 0, 0, 1)),
		proj(vclip("c1", "rel.mp4", "V1", 0, 0, 1)),
		proj(vclip("c1", pV, "V1", 0, 0, 0)),
		proj(vclip("c1", pV, "V1", 0, 0, 5), vclip("c2", pV, "V1", 1, 0, 1)),
	}
	for i, p := range cases {
		_, err := s.ValidateProject(ctx, p)
		if err == nil || !detailRe.MatchString(firstLine(err)) {
			t.Errorf("case %d: %v / %q", i, err, firstLine(err))
		}
	}
}

// 时间线总长 ≤ 6 小时：按 clip 自填值 max(startSec+(outSec-inSec)/speed) 在工程级检查，先于素材探测。
func TestTimelineLimit(t *testing.T) {
	s := fakeSvc(base)
	c := vclip("c1", pV, "V1", 6*3600, 0, 1) // 结束于 6h+1s
	_, err := s.ValidateProject(context.Background(), proj(c))
	if code(t, err) != apperr.InvalidArgument || firstLine(err) != "project" {
		t.Fatalf("%v", err)
	}
	c = vclip("c1", pV, "V1", 6*3600-1, 0, 1) // 恰好 6h
	if _, err := s.ValidateProject(context.Background(), proj(c)); err != nil {
		t.Fatalf("6 小时整应通过: %v", err)
	}
}

// 控制字符在 detail 第一行里替换为 ?（契约 6.11.2 B），设备路径前缀被拒绝。
func TestCleanLineAndDevicePrefix(t *testing.T) {
	if got := cleanLine("a\nb\rc\x00d"); got != "a?b?c?d" {
		t.Fatalf("%q", got)
	}
	s := fakeSvc(base)
	nl := filepath.Join(string(filepath.Separator), "m", "a\nb.mp4")
	_, err := s.ValidateProject(context.Background(), proj(vclip("c1", nl, "V1", 0, 0, 1)))
	if firstLine(err) != "clip=c1 path="+filepath.Join(string(filepath.Separator), "m", "a?b.mp4") {
		t.Fatalf("%q", firstLine(err))
	}
	for _, p := range []string{`\\?\C:\a.mp4`, `\\.\C:\a.mp4`} {
		if err := checkClipPath("c1", p); err == nil || apperr.From(err).Code != apperr.InvalidArgument {
			t.Fatalf("%s: %v", p, err)
		}
	}
}
