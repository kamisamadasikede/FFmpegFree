package edit

import (
	"fmt"
	"math"
	"strings"
)

// buildFilterGraph 生成 -filter_complex_script 的内容，输出标签 [vout] / [aout]。
// 语义沿用 v1：黑色底画布 → 每个 clip trim + setpts + fps + scale/pad + 预设 / 全局效果 + boxblur →
// 同轨首尾相接（间隙 ≤ 0.12 秒）的 clip 用 xfade（有转场）或 concat（无转场）→ 按 startSec 平移后 overlay 到画布，轨道编号大的在上；
// 音频：atrim + atempo + volume + adelay → amix(normalize=0) → 补齐并截到时间线总长；音轨为空导出静音（anullsrc）。
// 每个 clip 一个 -i（输入序号 = clip 的 idx），素材路径不出现在 filtergraph 里。
func buildFilterGraph(pl *plan) string {
	var f []string
	add := func(format string, a ...any) { f = append(f, fmt.Sprintf(format, a...)) }

	add("color=c=black:s=%dx%d:r=%s:d=%s[base]", pl.w, pl.h, num(pl.fps), num(pl.duration))

	// 1) 每个视频 clip 的处理链。
	label := func(c *rclip) string { return fmt.Sprintf("c%d", c.idx) }
	for i := range pl.videos {
		c := &pl.videos[i]
		chain := []string{
			fmt.Sprintf("trim=start=%s:end=%s", num(c.in), num(c.out)),
			fmt.Sprintf("setpts=(PTS-STARTPTS)/%s", num(c.speed)),
			fmt.Sprintf("fps=%s", num(pl.fps)),
			fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", pl.w, pl.h),
			fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black", pl.w, pl.h),
			"setsar=1",
		}
		if e := presetFilter(c.effect); e != "" {
			chain = append(chain, e)
		}
		if e := globalFilter(pl.effects); e != "" {
			chain = append(chain, e)
		}
		if c.blur > 0 {
			chain = append(chain, fmt.Sprintf("boxblur=%s:1", num(c.blur)))
		}
		chain = append(chain, "format=yuv420p")
		add("[%d:v]%s[%s]", c.idx, strings.Join(chain, ","), label(c))
	}

	// 2) 同轨拼接：首尾相接的连成一段（segment），段之间有空隙。
	type seg struct {
		label string
		start float64
	}
	var segs []seg
	n := 0
	for _, g := range byTrack(pl.videos) {
		cur := label(g[0])
		curStart, curDur := g[0].start, g[0].dur
		for i := 1; i < len(g); i++ {
			prev, c := g[i-1], g[i]
			if !isContiguous(prev, c) {
				segs = append(segs, seg{cur, curStart})
				cur, curStart, curDur = label(c), c.start, c.dur
				continue
			}
			n++
			out := fmt.Sprintf("m%d", n)
			if prev.transition != "none" && prev.transDur > 0 {
				add("[%s][%s]xfade=transition=%s:duration=%s:offset=%s[%s]", cur, label(c), prev.transition,
					num(prev.transDur), num(math.Max(0, curDur-prev.transDur)), out)
				curDur += c.dur - prev.transDur
			} else {
				add("[%s][%s]concat=n=2:v=1:a=0[%s]", cur, label(c), out)
				curDur += c.dur
			}
			cur = out
		}
		segs = append(segs, seg{cur, curStart})
	}

	// 3) 依次叠到画布上（轨道编号小的在下，同轨按时间）。
	curV := "base"
	for i, s := range segs {
		src := s.label
		if s.start > 0 {
			src = fmt.Sprintf("s%d", i)
			add("[%s]setpts=PTS+%s/TB[%s]", s.label, num(s.start), src)
		}
		out := fmt.Sprintf("o%d", i)
		add("[%s][%s]overlay=shortest=0:eof_action=pass:repeatlast=0[%s]", curV, src, out)
		curV = out
	}
	add("[%s]format=yuv420p[vout]", curV)

	// 4) 音频。
	if len(pl.audios) == 0 {
		add("anullsrc=r=48000:cl=stereo,atrim=end=%s,asetpts=PTS-STARTPTS[aout]", num(pl.duration))
	} else {
		var ins []string
		for i := range pl.audios {
			c := &pl.audios[i]
			chain := []string{
				fmt.Sprintf("atrim=start=%s:end=%s", num(c.in), num(c.out)),
				"asetpts=PTS-STARTPTS",
			}
			chain = append(chain, atempoChain(c.speed)...)
			chain = append(chain, fmt.Sprintf("volume=%s", num(c.volume)),
				"aresample=48000", "aformat=sample_fmts=fltp:channel_layouts=stereo")
			if ms := int(math.Round(c.start * 1000)); ms > 0 {
				chain = append(chain, fmt.Sprintf("adelay=%d|%d", ms, ms))
			}
			add("[%d:a]%s[a%d]", c.idx, strings.Join(chain, ","), i)
			ins = append(ins, fmt.Sprintf("[a%d]", i))
		}
		if len(ins) == 1 {
			add("%sanull[amix]", ins[0])
		} else {
			add("%samix=inputs=%d:normalize=0:dropout_transition=0[amix]", strings.Join(ins, ""), len(ins))
		}
		add("[amix]apad=whole_dur=%s,atrim=end=%s,asetpts=PTS-STARTPTS[aout]", num(pl.duration), num(pl.duration))
	}
	return strings.Join(f, ";\n") + "\n"
}

// num 把数字写成 ffmpeg 表达式里安全的十进制（不用科学计数法、去掉多余的 0）。
func num(v float64) string {
	s := fmt.Sprintf("%.6f", v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

func presetFilter(p string) string {
	switch p {
	case "grayscale":
		return "hue=s=0"
	case "sepia":
		return "colorchannelmixer=.393:.769:.189:.349:.686:.168:.272:.534:.131"
	case "vintage":
		return "eq=saturation=0.75:contrast=1.10:brightness=-0.04"
	case "cinematic":
		return "eq=contrast=1.15:saturation=1.20:brightness=-0.02"
	}
	return ""
}

// globalFilter 全局亮度 / 对比度 / 饱和度 / 锐化；全是默认值时不加滤镜。
func globalFilter(e GlobalEffects) string {
	var parts []string
	if e.Brightness != 0 || e.Contrast != 1 || e.Saturation != 1 {
		parts = append(parts, fmt.Sprintf("eq=brightness=%s:contrast=%s:saturation=%s", num(e.Brightness), num(e.Contrast), num(e.Saturation)))
	}
	if e.Sharpen > 0 {
		parts = append(parts, fmt.Sprintf("unsharp=5:5:%s:5:5:0.0", num(e.Sharpen)))
	}
	return strings.Join(parts, ",")
}

// atempoChain 把速度拆成 atempo 链（单个 atempo 只接受 0.5~2，速度 > 2 或 < 0.5 时链式拆分）。
func atempoChain(speed float64) []string {
	if speed == 1 {
		return nil
	}
	var parts []string
	for speed > 2 {
		parts = append(parts, "atempo=2.0")
		speed /= 2
	}
	for speed < 0.5 {
		parts = append(parts, "atempo=0.5")
		speed *= 2
	}
	return append(parts, "atempo="+num(speed))
}
