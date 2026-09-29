package edit

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
)

var (
	clipIDRe     = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	videoTrackRe = regexp.MustCompile(`^V[1-8]$`)
	audioTrackRe = regexp.MustCompile(`^A[1-8]$`)
)

var (
	effectPresets = map[string]bool{"": true, "none": true, "grayscale": true, "sepia": true, "vintage": true, "cinematic": true}
	transitions   = map[string]bool{"": true, "none": true, "fade": true, "wipeleft": true, "wiperight": true, "slideleft": true,
		"slideright": true, "circleopen": true, "circleclose": true, "dissolve": true}
	formats = map[string]bool{"": true, "mp4": true, "mov": true, "mkv": true, "webm": true}
)

// ---------- 错误辅助 ----------

// cleanLine 把换行等控制字符替换成 ?（契约 6.11.2 B），避免用户可控的路径 / id 伪造 detail 的第一行。
func cleanLine(s string) string {
	return strings.Map(func(r rune) rune {
		if isCtrl(r) {
			return '?'
		}
		return r
	}, s)
}

func invalid(msg string) *apperr.AppError { return apperr.New(apperr.InvalidArgument, msg) }

// projectErr 是没有具体 clip 的错误：detail 第一行固定 `project`。
func projectErr(code apperr.Code, msg, reason string) *apperr.AppError {
	d := "project"
	if reason != "" {
		d += "\n" + reason
	}
	return apperr.New(code, msg).WithDetail(d)
}

// clipErr 生成 clip 错误：detail 第一行固定 `clip=<id> path=<path>`，之后是原因。
func clipErr(code apperr.Code, id, path, msg, reason string) *apperr.AppError {
	d := "clip=" + cleanLine(id) + " path=" + cleanLine(path)
	if reason != "" {
		d += "\n" + reason
	}
	return apperr.New(code, msg).WithDetail(d)
}

// withClip 把底层错误（探测失败等）包上 clip 定位行，原 detail 放在后面。
func withClip(err error, id, path string) error {
	ae := apperr.From(err)
	cp := *ae
	d := "clip=" + cleanLine(id) + " path=" + cleanLine(path)
	if ae.Detail != "" {
		d += "\n" + ae.Detail
	}
	cp.Detail = d
	return &cp
}

func finite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// ---------- 规范化后的计划 ----------

type rclip struct {
	kind       byte // 'v' | 'a'
	idx        int  // ffmpeg 输入序号
	id, path   string
	track      int // 轨道编号 1~8
	start      float64
	in, out    float64 // out 已按素材时长取整 / 截断
	speed      float64
	dur        float64 // (out-in)/speed
	effect     string
	transition string
	transReq   float64 // 请求的转场时长（0 = 默认 0.5）
	transDur   float64 // 已解析的实际转场时长；transition 为 none 时无意义
	blur       float64
	volume     float64
}

func (c rclip) end() float64 { return c.start + c.dur }

type plan struct {
	filterOpt string // 本次导出用的 ffmpeg 选项（OptFilterFile / OptFilterScript）
	format    string
	w, h      int
	fps       float64
	effects   GlobalEffects
	videos    []rclip // 与 project.VideoTrack 同序
	audios    []rclip // 与 project.AudioTrack 同序
	duration  float64
	inputs    []string
	hasAudio  bool
	warnings  []EditWarning
}

func (p *plan) toPlan() EditPlan {
	w := p.warnings
	if w == nil {
		w = []EditWarning{}
	}
	return EditPlan{DurationSec: p.duration, ClipCount: len(p.videos) + len(p.audios), Inputs: p.inputs, HasAudio: p.hasAudio, Warnings: w}
}

// ---------- 结构 / 范围校验（不碰磁盘） ----------

// checkProjectHeader 校验与素材无关的工程级限制。Save 只用这里的数量 / 大小 / 名称 / schema 部分。
func checkCounts(p EditProject) error {
	if p.SchemaVersion > 1 || p.SchemaVersion < 0 {
		return apperr.New(apperr.Unsupported, "工程版本过新，请升级应用后再打开").WithDetail(fmt.Sprintf("schemaVersion=%d", p.SchemaVersion))
	}
	if n := len(p.VideoTrack) + len(p.AudioTrack); n > MaxClips {
		return projectErr(apperr.InvalidArgument, fmt.Sprintf("片段总数最多 %d 个", MaxClips), fmt.Sprintf("clips=%d", n))
	}
	if len(p.Sources) > MaxSources {
		return projectErr(apperr.InvalidArgument, fmt.Sprintf("素材库最多 %d 个文件", MaxSources), fmt.Sprintf("sources=%d", len(p.Sources)))
	}
	b, err := json.Marshal(p)
	if err != nil {
		return apperr.Wrap(apperr.InvalidArgument, "工程无法序列化", err)
	}
	if len(b) > MaxProjectBytes {
		return projectErr(apperr.InvalidArgument, "工程太大（序列化后超过 1 MiB）", fmt.Sprintf("bytes=%d", len(b)))
	}
	return nil
}

func checkName(name string) error {
	n := utf8.RuneCountInString(strings.TrimSpace(name))
	if n < 1 || n > MaxNameRunes {
		return projectErr(apperr.InvalidArgument, "工程名称不能为空，最多 80 个字", "")
	}
	return nil
}

// checkStructure 是 Validate / Export 的第一步：不碰磁盘的全部结构与范围校验（顺序固定，先报先出现的问题）。
func checkStructure(p EditProject) error {
	if err := checkCounts(p); err != nil {
		return err
	}
	if err := checkName(p.Name); err != nil {
		return err
	}
	if len(p.VideoTrack) == 0 {
		return projectErr(apperr.InvalidArgument, "视频轨道不能为空", "videoTrack is empty")
	}
	for i, s := range p.Sources {
		if !filepath.IsAbs(s) {
			return projectErr(apperr.InvalidArgument, "素材路径必须是绝对路径", fmt.Sprintf("sources[%d]=%s", i, cleanLine(s)))
		}
	}
	o := p.Output
	if !finite(o.Fps) || !formats[o.Format] {
		return projectErr(apperr.InvalidArgument, "输出格式必须是 mp4、mov、mkv 或 webm", "output.format="+cleanLine(o.Format))
	}
	if o.Width != 0 && (o.Width < 16 || o.Width > 7680) {
		return projectErr(apperr.InvalidArgument, "输出宽度必须在 16~7680", fmt.Sprintf("output.width=%d", o.Width))
	}
	if o.Height != 0 && (o.Height < 16 || o.Height > 4320) {
		return projectErr(apperr.InvalidArgument, "输出高度必须在 16~4320", fmt.Sprintf("output.height=%d", o.Height))
	}
	if o.Fps != 0 && (o.Fps < 0 || o.Fps > 120) {
		return projectErr(apperr.InvalidArgument, "输出帧率必须在 (0, 120]", fmt.Sprintf("output.fps=%v", o.Fps))
	}
	e := p.Effects
	if !finite(e.Brightness, e.Contrast, e.Saturation, e.Sharpen) ||
		e.Brightness < -0.5 || e.Brightness > 0.5 ||
		(e.Contrast != 0 && (e.Contrast < 0.5 || e.Contrast > 2)) ||
		e.Saturation < 0 || e.Saturation > 2 || e.Sharpen < 0 || e.Sharpen > 2 {
		return projectErr(apperr.InvalidArgument, "全局效果参数越界", fmt.Sprintf("effects=%+v", e))
	}

	if d := declaredDuration(p); d > MaxTimelineSec {
		return projectErr(apperr.InvalidArgument, "时间线总长不能超过 6 小时", fmt.Sprintf("durationSec=%.1f", d))
	}

	seen := map[string]bool{}
	checkID := func(kind string, i int, id string) error {
		if !clipIDRe.MatchString(id) {
			return projectErr(apperr.InvalidArgument, "片段 id 只能包含字母、数字、下划线和连字符，长度 1~64", fmt.Sprintf("%s[%d].id 不合法", kind, i))
		}
		if seen[id] {
			return projectErr(apperr.InvalidArgument, "片段 id 重复", fmt.Sprintf("%s[%d].id=%s", kind, i, id))
		}
		seen[id] = true
		return nil
	}
	common := func(id, path string, start, in, out, speed float64) error {
		if !finite(start, in, out, speed) {
			return clipErr(apperr.InvalidArgument, id, path, "片段的数值不合法", "存在 NaN 或无穷大")
		}
		if start < 0 {
			return clipErr(apperr.InvalidArgument, id, path, "startSec 不能为负", fmt.Sprintf("startSec=%v", start))
		}
		if in < 0 {
			return clipErr(apperr.InvalidArgument, id, path, "inSec 不能为负", fmt.Sprintf("inSec=%v", in))
		}
		if out <= in { // outSec=0 不表示"到结尾"，也是错误
			return clipErr(apperr.InvalidArgument, id, path, "outSec 必须大于 inSec（0 不表示到素材结尾）", fmt.Sprintf("inSec=%v outSec=%v", in, out))
		}
		if speed != 0 && (speed < 0.25 || speed > 4) {
			return clipErr(apperr.InvalidArgument, id, path, "速度必须在 0.25~4", fmt.Sprintf("speed=%v", speed))
		}
		return nil
	}
	for i, c := range p.VideoTrack {
		if err := checkID("videoTrack", i, c.ID); err != nil {
			return err
		}
		if !videoTrackRe.MatchString(c.TrackID) {
			return clipErr(apperr.InvalidArgument, c.ID, c.Path, "视频轨道必须是 V1~V8", "trackId="+cleanLine(c.TrackID))
		}
		if err := common(c.ID, c.Path, c.StartSec, c.InSec, c.OutSec, c.Speed); err != nil {
			return err
		}
		if !effectPresets[c.EffectPreset] {
			return clipErr(apperr.InvalidArgument, c.ID, c.Path, "不支持的滤镜预设", "effectPreset="+cleanLine(c.EffectPreset))
		}
		if !transitions[c.TransitionToNext] {
			return clipErr(apperr.InvalidArgument, c.ID, c.Path, "不支持的转场", "transitionToNext="+cleanLine(c.TransitionToNext))
		}
		if !finite(c.TransitionDurationSec, c.Blur) || (c.TransitionDurationSec != 0 && (c.TransitionDurationSec < 0.1 || c.TransitionDurationSec > 2)) {
			return clipErr(apperr.InvalidArgument, c.ID, c.Path, "转场时长必须在 0.1~2 秒", fmt.Sprintf("transitionDurationSec=%v", c.TransitionDurationSec))
		}
		if c.Blur < 0 || c.Blur > 4 {
			return clipErr(apperr.InvalidArgument, c.ID, c.Path, "模糊必须在 0~4", fmt.Sprintf("blur=%v", c.Blur))
		}
	}
	for i, c := range p.AudioTrack {
		if err := checkID("audioTrack", i, c.ID); err != nil {
			return err
		}
		if !audioTrackRe.MatchString(c.TrackID) {
			return clipErr(apperr.InvalidArgument, c.ID, c.Path, "音频轨道必须是 A1~A8", "trackId="+cleanLine(c.TrackID))
		}
		if err := common(c.ID, c.Path, c.StartSec, c.InSec, c.OutSec, c.Speed); err != nil {
			return err
		}
		if !finite(c.Volume) || c.Volume < 0 || c.Volume > 4 {
			return clipErr(apperr.InvalidArgument, c.ID, c.Path, "音量必须在 0~4", fmt.Sprintf("volume=%v", c.Volume))
		}
	}
	return nil
}

// declaredDuration 是按 clip 自填值算的时间线总长 max(startSec + (outSec-inSec)/speed)（契约 6.11.2 第 1 条），
// 只用于"总长 ≤ 6 小时"的工程级检查；数值不合法（NaN、speed 越界等）的 clip 在这里跳过，留给逐 clip 字段校验去报。
func declaredDuration(p EditProject) float64 {
	d := 0.0
	add := func(start, in, out, speed float64) {
		if speed == 0 {
			speed = 1
		}
		if !finite(start, in, out, speed) || speed < 0.25 || speed > 4 {
			return
		}
		d = math.Max(d, start+(out-in)/speed)
	}
	for _, c := range p.VideoTrack {
		add(c.StartSec, c.InSec, c.OutSec, c.Speed)
	}
	for _, c := range p.AudioTrack {
		add(c.StartSec, c.InSec, c.OutSec, c.Speed)
	}
	return d
}

// ---------- 素材探测 + 计划 ----------

// build 是 ValidateProject / Export 共用的完整校验：ffmpeg 就绪 → 结构 → 探测素材 → 素材匹配与时长 → 同轨重叠 / 转场 → 总时长。
func (s *Service) build(ctx context.Context, p EditProject) (*plan, error) {
	// 0 环境：ffmpeg / ffprobe 就绪，从文件读 filtergraph 的选项可用（-/filter_complex 或 -filter_complex_script，功能探测择一）。
	bin, err := s.cfg.Require()
	if err != nil {
		return nil, err
	}
	opt, err := s.cfg.SupportsScript(ctx, bin.FFmpeg)
	if err != nil {
		return nil, err
	}
	if err := checkStructure(p); err != nil {
		return nil, err
	}
	infos, probeErrs, err := s.probeAll(ctx, p)
	if err != nil {
		return nil, err
	}

	pl := &plan{filterOpt: opt, format: p.Output.Format, w: p.Output.Width, h: p.Output.Height, fps: p.Output.Fps, effects: p.Effects}
	if pl.format == "" {
		pl.format = "mp4"
	}
	if pl.w == 0 {
		pl.w = 1920
	}
	if pl.h == 0 {
		pl.h = 1080
	}
	if pl.fps == 0 {
		pl.fps = 30
	}
	pl.w &^= 1
	pl.h &^= 1
	if pl.effects.Contrast == 0 {
		pl.effects.Contrast = 1
	}
	if pl.effects.Saturation == 0 {
		pl.effects.Saturation = 1
	}

	// 素材匹配：视频 clip 要有视频流，音频 clip 要有音频流；inSec 不能超出素材；outSec 超出素材按 0.05 秒规则取整 / 截断。
	resolve := func(kind byte, id, path string, in, out float64) (float64, error) {
		if err := checkClipPath(id, path); err != nil {
			return 0, err
		}
		if err := probeErrs[path]; err != nil {
			return 0, withClip(err, id, path)
		}
		mi := infos[path]
		if kind == 'v' && !mi.HasVideo {
			return 0, clipErr(apperr.InvalidArgument, id, path, "视频片段的素材没有视频画面", "")
		}
		if kind == 'a' && !mi.HasAudio {
			return 0, clipErr(apperr.InvalidArgument, id, path, "音频片段的素材没有音轨", "")
		}
		dur := mi.Duration
		if in >= dur {
			return 0, clipErr(apperr.InvalidArgument, id, path, "inSec 超出了素材时长", fmt.Sprintf("inSec=%v duration=%.3f", in, dur))
		}
		if out > dur {
			if out-dur > snapSec {
				pl.warnings = append(pl.warnings, EditWarning{Code: WarnOutTruncated, ClipID: id,
					Message: fmt.Sprintf("片段 %s 的 outSec（%.3f）超过素材时长（%.3f），已截断", id, out, dur)})
			}
			out = dur
		}
		return out, nil
	}
	seenPath := map[string]bool{}
	addInput := func(path string) {
		if !seenPath[path] {
			seenPath[path] = true
			pl.inputs = append(pl.inputs, path)
		}
	}
	for i, c := range p.VideoTrack {
		out, err := resolve('v', c.ID, c.Path, c.InSec, c.OutSec)
		if err != nil {
			return nil, err
		}
		sp := c.Speed
		if sp == 0 {
			sp = 1
		}
		preset := c.EffectPreset
		if preset == "" {
			preset = "none"
		}
		tr := c.TransitionToNext
		if tr == "" {
			tr = "none"
		}
		rc := rclip{kind: 'v', idx: i, id: c.ID, path: c.Path, track: int(c.TrackID[1] - '0'), start: c.StartSec, in: c.InSec, out: out,
			speed: sp, dur: (out - c.InSec) / sp, effect: preset, transition: tr, transReq: c.TransitionDurationSec, blur: c.Blur}
		if rc.dur < MinClipDurSec {
			return nil, clipErr(apperr.InvalidArgument, c.ID, c.Path, "片段太短（按速度折算后不足 0.04 秒）", fmt.Sprintf("duration=%.4f", rc.dur))
		}
		pl.videos = append(pl.videos, rc)
		addInput(c.Path)
	}
	for i, c := range p.AudioTrack {
		out, err := resolve('a', c.ID, c.Path, c.InSec, c.OutSec)
		if err != nil {
			return nil, err
		}
		sp := c.Speed
		if sp == 0 {
			sp = 1
		}
		rc := rclip{kind: 'a', idx: len(p.VideoTrack) + i, id: c.ID, path: c.Path, track: int(c.TrackID[1] - '0'), start: c.StartSec, in: c.InSec, out: out,
			speed: sp, dur: (out - c.InSec) / sp, volume: c.Volume}
		if rc.dur < MinClipDurSec {
			return nil, clipErr(apperr.InvalidArgument, c.ID, c.Path, "片段太短（按速度折算后不足 0.04 秒）", fmt.Sprintf("duration=%.4f", rc.dur))
		}
		pl.audios = append(pl.audios, rc)
		addInput(c.Path)
	}
	pl.hasAudio = len(pl.audios) > 0

	// 同一轨道上后一个开始早于前一个结束 → INVALID_ARGUMENT，detail 的 clip= 指后一个。
	if err := checkOverlap(pl.videos); err != nil {
		return nil, err
	}
	if err := checkOverlap(pl.audios); err != nil {
		return nil, err
	}
	if err := resolveTransitions(pl); err != nil {
		return nil, err
	}

	// 时间线总长：视频取"实际画面段"的末尾（有转场时后一个 clip 会和前一个重叠，整段变短），音频取声明的结束。
	// （工程级的 6 小时上限已在 checkStructure 里按 clip 自填值检查过。）
	for _, sg := range videoSegments(pl) {
		pl.duration = math.Max(pl.duration, sg.end)
	}
	for _, c := range pl.audios {
		pl.duration = math.Max(pl.duration, c.end())
	}
	pl.warnings = append(pl.warnings, gapWarnings(pl)...)
	if !pl.hasAudio {
		pl.warnings = append(pl.warnings, EditWarning{Code: WarnNoAudioTrack, Message: "音轨为空，导出的视频是静音的（不会使用视频素材自带的声音）"})
	}
	return pl, nil
}

// byTrack 按轨道编号分组（指向 cs 里的元素），组内按 startSec 升序（相同保持原顺序）；返回的组按轨道编号升序。
func byTrack(cs []rclip) [][]*rclip {
	m := map[int][]*rclip{}
	for i := range cs {
		m[cs[i].track] = append(m[cs[i].track], &cs[i])
	}
	var keys []int
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	out := make([][]*rclip, 0, len(keys))
	for _, k := range keys {
		g := m[k]
		sort.SliceStable(g, func(i, j int) bool { return ms(g[i].start) < ms(g[j].start) })
		out = append(out, g)
	}
	return out
}

// ms 把秒四舍五入到毫秒整数（契约 6.11.2 A：重叠 / 相接比较前先取整，避免浮点误差）。
func ms(sec float64) int64 { return int64(math.Round(sec * 1000)) }

const contiguousGapMs = 120 // ContiguousGapSec 的毫秒数

// checkOverlap：同一轨道上相邻两个 clip（按 startSec 升序，相同按数组下标），后一个 startSec 早于前一个 end
// （都先取整到毫秒）即重叠。detail 第一行 clip= 指后一个，第二行 overlaps=<前一个 id>。
func checkOverlap(cs []rclip) error {
	for _, g := range byTrack(cs) {
		for i := 1; i < len(g); i++ {
			prev, cur := g[i-1], g[i]
			if ms(cur.start) < ms(prev.end()) {
				return clipErr(apperr.InvalidArgument, cur.id, cur.path, "同一轨道上的片段重叠", "overlaps="+prev.id)
			}
		}
	}
	return nil
}

// isContiguous 判断同轨相邻片段是否首尾相接（间隙 ≤ 0.12 秒，按毫秒取整比较）。
func isContiguous(prev, cur *rclip) bool { return ms(cur.start)-ms(prev.end()) <= contiguousGapMs }

// resolveTransitions 解析每个视频 clip 的转场（写回 transition / transDur）。转场只对"与同轨下一个片段首尾相接"的片段有效：
//   - 没有下一个片段或有空隙：忽略转场，记 transition_ignored；
//   - 显式时长 > 相邻两个片段中较短者的一半：INVALID_ARGUMENT（detail 的 clip= 指设了转场的片段）；
//   - 默认时长（0 = 0.5）超过一半：静默取一半（契约没有为此定义警告）；一半不足 0.1 秒则转场不生效，记 transition_ignored。
func resolveTransitions(pl *plan) error {
	for _, g := range byTrack(pl.videos) {
		for i, cur := range g {
			if cur.transition == "none" {
				continue
			}
			if i+1 >= len(g) || !isContiguous(cur, g[i+1]) {
				cur.transition = "none"
				pl.warnings = append(pl.warnings, EditWarning{Code: WarnTransitionIgnored, ClipID: cur.id,
					Message: fmt.Sprintf("片段 %s 设置了转场，但它与同轨的下一个片段不是首尾相接，转场已忽略", cur.id)})
				continue
			}
			half := math.Min(cur.dur, g[i+1].dur) / 2
			if cur.transReq != 0 {
				if cur.transReq > half+1e-9 {
					return clipErr(apperr.InvalidArgument, cur.id, cur.path, "转场时长不能超过相邻两个片段中较短者的一半",
						fmt.Sprintf("transitionDurationSec=%v，允许的最大值 %.3f", cur.transReq, half))
				}
				cur.transDur = cur.transReq
				continue
			}
			d := 0.5
			if d > half {
				d = half
				if d < 0.1 {
					cur.transition = "none"
					pl.warnings = append(pl.warnings, EditWarning{Code: WarnTransitionIgnored, ClipID: cur.id,
						Message: fmt.Sprintf("片段 %s 与相邻片段太短，放不下转场，转场已忽略", cur.id)})
					continue
				}
			}
			cur.transDur = d
		}
	}
	return nil
}

// vseg 是同一轨道上首尾相接、拼成一段的画面。
type vseg struct {
	firstID    string
	start, end float64
}

// videoSegments 计算实际画面段：同轨首尾相接（间隙 ≤ 0.12 秒）的 clip 连成一段；
// 段内每个转场让后一个 clip 与前一个重叠 transDur 秒，所以段长 = 各 clip 时长之和 - 各转场时长之和（与 filtergraph 一致）。
func videoSegments(pl *plan) []vseg {
	var out []vseg
	for _, g := range byTrack(pl.videos) {
		cur := vseg{firstID: g[0].id, start: g[0].start, end: g[0].start + g[0].dur}
		for i := 1; i < len(g); i++ {
			prev, c := g[i-1], g[i]
			if !isContiguous(prev, c) {
				out = append(out, cur)
				cur = vseg{firstID: c.id, start: c.start, end: c.start + c.dur}
				continue
			}
			d := c.dur
			if prev.transition != "none" && prev.transDur > 0 {
				d -= prev.transDur
			}
			cur.end += d
		}
		out = append(out, cur)
	}
	return out
}

// gapWarnings 生成 clip_gap / leading_gap（契约 6.11.2 D）：
//   - clip_gap：同一轨道（V1~V8、A1~A8 各自）相邻两个 clip 之间的空隙 > 0.12 秒，clipId = 后一个 clip；
//   - leading_gap：视频轨（videoTrack）/ 音频轨（audioTrack）上最早的 clip 的 startSec > 0.12 秒，clipId = 该 clip。
//
// 顺序：先视频后音频，轨道编号升序，轨内按时间。
func gapWarnings(pl *plan) []EditWarning {
	var out []EditWarning
	for _, cs := range [][]rclip{pl.videos, pl.audios} {
		if len(cs) == 0 {
			continue
		}
		first := &cs[0]
		for i := range cs {
			if ms(cs[i].start) < ms(first.start) {
				first = &cs[i]
			}
		}
		if ms(first.start) > contiguousGapMs {
			out = append(out, EditWarning{Code: WarnLeadingGap, ClipID: first.id,
				Message: fmt.Sprintf("时间线开头 %.2f 秒没有内容，导出时补黑场 / 静音", first.start)})
		}
		for _, g := range byTrack(cs) {
			for i := 1; i < len(g); i++ {
				prev, cur := g[i-1], g[i]
				if ms(cur.start)-ms(prev.end()) > contiguousGapMs {
					out = append(out, EditWarning{Code: WarnClipGap, ClipID: cur.id,
						Message: fmt.Sprintf("片段 %s 与 %s 之间有 %.2f 秒空隙，导出时补黑场 / 静音", prev.id, cur.id, cur.start-prev.end())})
				}
			}
		}
	}
	return out
}

// checkClipPath 是逐 clip 路径检查的第一步：必须是绝对路径且不含控制字符（含换行，否则会破坏 detail 的行格式）。
func checkClipPath(id, path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return clipErr(apperr.InvalidArgument, id, path, "素材路径必须是绝对路径", "")
	}
	if strings.IndexFunc(path, isCtrl) >= 0 {
		return clipErr(apperr.InvalidArgument, id, path, "素材路径不能包含控制字符（含换行）", "")
	}
	if hasDevicePrefix(path) {
		return clipErr(apperr.InvalidArgument, id, path, `素材路径不能以 \\?\ 或 \\.\ 开头`, "")
	}
	return nil
}

// probeAll 并行探测工程里所有 clip 引用的素材（去重，最多 4 个并行），返回 path → 结果 / 错误。
// 只探测通过 checkClipPath 的路径；哪个 clip 报错由调用方按契约顺序（先视频后音频、数组顺序）逐个 clip 决定，
// 所以同一素材的失败记在按顺序第一个用到它的 clip 上。
func (s *Service) probeAll(ctx context.Context, p EditProject) (map[string]store.MediaInfo, map[string]error, error) {
	var order []string
	seen := map[string]bool{}
	add := func(id, path string) {
		if seen[path] || checkClipPath(id, path) != nil {
			return
		}
		seen[path] = true
		order = append(order, path)
	}
	for _, c := range p.VideoTrack {
		add(c.ID, c.Path)
	}
	for _, c := range p.AudioTrack {
		add(c.ID, c.Path)
	}
	infos := make([]store.MediaInfo, len(order))
	errs := make([]error, len(order))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, path := range order {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				errs[i] = err
				return
			}
			infos[i], errs[i] = s.cfg.Media.Inspect(ctx, filepath.Clean(path))
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, apperr.Wrap(apperr.Canceled, "操作已取消", err)
	}
	mi, me := make(map[string]store.MediaInfo, len(order)), map[string]error{}
	for i, path := range order {
		if errs[i] != nil {
			me[path] = errs[i]
			continue
		}
		mi[path] = infos[i]
	}
	return mi, me, nil
}

// hasDevicePrefix 判断 Windows 设备 / 扩展长度路径前缀（\\?\ 与 \\.\），这类路径绕过 Win32 规范化，一律拒绝。
func hasDevicePrefix(p string) bool {
	return strings.HasPrefix(p, `\\?\`) || strings.HasPrefix(p, `\\.\`)
}

func isCtrl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || r == 0x2028 || r == 0x2029
}
