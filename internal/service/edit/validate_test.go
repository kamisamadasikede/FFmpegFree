package edit

import (
	"context"
	"path/filepath"
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
	return New(Config{Media: fakeMedia{m}, Require: func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{FFmpeg: "x", FFprobe: "y"}, nil }})
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
	// 大工程：sources 超过 200
	p = proj(vclip("c1", pV, "V1", 0, 0, 1))
	for i := 0; i < 201; i++ {
		p.Sources = append(p.Sources, filepath.Join(string(filepath.Separator), "m", "s"+itoa(i)+".mp4"))
	}
	if _, err := s.ValidateProject(context.Background(), p); code(t, err) != apperr.InvalidArgument {
		t.Fatal("sources > 200")
	}
	// 工程 > 1 MiB
	p = proj(vclip("c1", pV, "V1", 0, 0, 1))
	p.Name = strings.Repeat("字", 80)
	for i := 0; i < 200; i++ {
		p.Sources = append(p.Sources, filepath.Join(string(filepath.Separator), "m", strings.Repeat("x", 6000)+itoa(i)+".mp4"))
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
		t.Fatalf("缺 OUT_TRUNCATED 警告: %+v", pl.Warnings)
	}
}

func TestOverlapAndGap(t *testing.T) {
	s := fakeSvc(base)
	ctx := context.Background()
	// 同轨重叠：detail 指后一个
	_, err := s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 5), vclip("c2", pV2, "V1", 4.9, 0, 2)))
	if code(t, err) != apperr.InvalidArgument || firstLine(err) != "clip=c2 path="+pV2 {
		t.Fatalf("%v", err)
	}
	// 顺序颠倒（数组里后面的 clip 时间更早）：仍然是时间上后一个被指
	_, err = s.ValidateProject(ctx, proj(vclip("late", pV2, "V1", 4.9, 0, 2), vclip("early", pV, "V1", 0, 0, 5)))
	if firstLine(err) != "clip=late path="+pV2 {
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
	// 间隙 0.12 秒：视为首尾相接（不报 VIDEO_GAP）；0.13 秒：空隙
	pl, err := s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 2), vclip("c2", pV2, "V1", 2.12, 0, 1)))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range pl.Warnings {
		if w.Code == WarnVideoGap {
			t.Fatalf("0.12 秒不算空隙: %+v", w)
		}
	}
	pl, _ = s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 0, 0, 2), vclip("c2", pV2, "V1", 2.13, 0, 1)))
	gap := false
	for _, w := range pl.Warnings {
		if w.Code == WarnVideoGap && w.ClipID == "c2" {
			gap = true
		}
	}
	if !gap {
		t.Fatalf("0.13 秒应报 VIDEO_GAP: %+v", pl.Warnings)
	}
	// 开头空隙
	pl, _ = s.ValidateProject(ctx, proj(vclip("c1", pV, "V1", 1, 0, 2)))
	if len(pl.Warnings) == 0 || pl.Warnings[0].Code != WarnVideoGap || pl.Warnings[0].ClipID != "c1" {
		t.Fatalf("开头空隙: %+v", pl.Warnings)
	}
	// 音频比视频长：末尾黑场警告；无音轨警告
	p = proj(vclip("c1", pV, "V1", 0, 0, 2))
	p.AudioTrack = []AudioClip{aclip("a1", pA, "A1", 0, 0, 6)}
	pl, _ = s.ValidateProject(ctx, p)
	if pl.DurationSec != 6 || !hasWarn(pl, WarnVideoGap) || hasWarn(pl, WarnNoAudioTrack) {
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
	// 默认时长 0.5 但相邻片段只有 0.6 秒（一半 0.3）→ 缩短并警告
	a := vclip("c1", pV, "V1", 0, 0, 0.6)
	a.TransitionToNext = "fade"
	pl, err = s.ValidateProject(ctx, proj(a, vclip("c2", pV2, "V1", 0.6, 0, 3)))
	if err != nil || !hasWarn(pl, WarnTransitionClamped) {
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
