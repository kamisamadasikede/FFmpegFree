// Package ffmpeg 负责 ffmpeg / ffprobe 的定位与校验（契约第 9.1 节）。
//
// 检测顺序：用户指定路径 → <数据目录>/bin → 系统 PATH → 程序同级 ffmpeg/ 目录（兼容 v1）。
// 每个候选都要求 ffmpeg 与 ffprobe 能运行、主版本不低于 6、-encoders 含 libx264 和 aac，
// 第一个通过的即为结果。
package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// 状态取值，见契约第 9.4 节。
const (
	StateChecking   = "checking"
	StateReady      = "ready"
	StateMissing    = "missing"
	StateOutdated   = "outdated"
	StateInstalling = "installing"
	StateFailed     = "failed"
)

// 来源取值，见契约第 9.4 节。
const (
	SourceCustom  = "custom"
	SourceBundled = "bundled"
	SourceSystem  = "system"
	SourceLegacy  = "legacy"
)

// Binaries 是一对可执行文件的绝对路径。
type Binaries struct {
	FFmpeg  string
	FFprobe string
}

// Info 是一个通过（或部分通过）校验的候选。
type Info struct {
	Binaries
	Source  string
	Version string // ffmpeg 的原始版本串，如 "6.1.1-3ubuntu5"
	// FFprobeMissing 为 true 表示 ffmpeg 可用但没有可用的 ffprobe（此时 FFprobe 为空）。
	// 只会出现在 v1 兼容目录（SourceLegacy）：v1 随包只带了 ffmpeg.exe，不能因此判整个候选失败。
	// 依赖 ffprobe 的功能（媒体探测）用 RequireProbe 门控，安装流程会把 ffprobe 补进 <数据目录>/bin。
	FFprobeMissing bool
	Major          int
	Known          bool // 主版本号是否解析成功
}

// Attempt 记录一个被尝试过的候选及其失败原因，用于排查"为什么没找到"。
type Attempt struct {
	Source string
	Path   string
	Reason string
}

// Result 是 Locate 的结果。
type Result struct {
	State    string // ready | outdated | missing
	Info     Info   // State 为 ready / outdated 时有效
	Attempts []Attempt
}

// Locator 定位并校验 ffmpeg。所有外部依赖都可注入，便于测试。
type Locator struct {
	Run      Runner                       // 运行命令，默认 ExecRunner
	LookPath func(string) (string, error) // 默认 exec.LookPath
	ExeDir   func() (string, error)       // 当前程序所在目录，默认 os.Executable 所在目录
	BinDir   string                       // <数据目录>/bin
	GOOS     string                       // 默认 runtime.GOOS，决定可执行文件后缀
}

// NewLocator 用真实依赖创建 Locator。
func NewLocator(binDir string) *Locator {
	return &Locator{
		Run:      ExecRunner(0),
		LookPath: exec.LookPath,
		ExeDir:   executableDir,
		BinDir:   binDir,
		GOOS:     runtime.GOOS,
	}
}

func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Dir(exe), nil
}

func (l *Locator) exeName(base string) string {
	goos := l.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos == "windows" {
		return base + ".exe"
	}
	return base
}

func (l *Locator) run() Runner {
	if l.Run != nil {
		return l.Run
	}
	return ExecRunner(0)
}

func (l *Locator) lookPath(name string) (string, error) {
	if l.LookPath != nil {
		return l.LookPath(name)
	}
	return exec.LookPath(name)
}

// Locate 按顺序尝试各候选，返回第一个完全通过的；都不通过时，如果有"能运行但版本过低"的
// 候选就返回 outdated，否则返回 missing。customPath 为空表示用户没有手动指定。
//
// 只有 ctx 被取消时才返回 error，其余失败原因都记在 Result.Attempts 里。
func (l *Locator) Locate(ctx context.Context, customPath string) (Result, error) {
	res := Result{State: StateMissing}
	var outdated *Info
	tried := map[string]bool{}

	try := func(source string, b Binaries) (bool, error) {
		key := filepath.Clean(b.FFmpeg)
		if tried[key] {
			return false, nil
		}
		tried[key] = true
		info, state, reason := l.Check(ctx, source, b)
		if err := ctx.Err(); err != nil {
			return false, err
		}
		switch state {
		case StateReady:
			res.State, res.Info = StateReady, info
			return true, nil
		case StateOutdated:
			if outdated == nil {
				c := info
				outdated = &c
			}
		}
		res.Attempts = append(res.Attempts, Attempt{Source: source, Path: b.FFmpeg, Reason: reason})
		return false, nil
	}
	note := func(source, path, reason string) {
		res.Attempts = append(res.Attempts, Attempt{Source: source, Path: path, Reason: reason})
	}

	// 1. 用户指定
	if strings.TrimSpace(customPath) != "" {
		cands, err := l.customCandidates(customPath)
		if err != nil {
			note(SourceCustom, customPath, err.Error())
		}
		for _, b := range cands {
			if ok, err := try(SourceCustom, b); err != nil || ok {
				return res, err
			}
		}
	}

	// 2. 应用自带目录
	if l.BinDir != "" {
		b := Binaries{
			FFmpeg:  filepath.Join(l.BinDir, l.exeName("ffmpeg")),
			FFprobe: filepath.Join(l.BinDir, l.exeName("ffprobe")),
		}
		if fileExists(b.FFmpeg) {
			if ok, err := try(SourceBundled, b); err != nil || ok {
				return res, err
			}
		}
	}

	// 3. 系统 PATH
	if p, err := l.lookPath("ffmpeg"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		b := Binaries{FFmpeg: p, FFprobe: l.probeBeside(p, true)}
		if ok, err := try(SourceSystem, b); err != nil || ok {
			return res, err
		}
	} else {
		note(SourceSystem, "PATH", "PATH 中没有 ffmpeg")
	}

	// 4. 兼容 v1：程序同级 ffmpeg/ 目录。
	// v1 只随包带了 ffmpeg，没有 ffprobe：先退回 PATH 里找，还没有就把 ffprobe 记为缺失，
	// ffmpeg 本身仍视为可用（见 Info.FFprobeMissing）。
	if l.ExeDir != nil {
		if dir, err := l.ExeDir(); err == nil {
			ff := filepath.Join(dir, "ffmpeg", l.exeName("ffmpeg"))
			if fileExists(ff) {
				b := Binaries{FFmpeg: ff, FFprobe: l.probeBeside(ff, true)}
				if ok, err := try(SourceLegacy, b); err != nil || ok {
					return res, err
				}
			}
		}
	}

	if outdated != nil {
		res.State, res.Info = StateOutdated, *outdated
	}
	return res, nil
}

// CheckCustom 只校验用户指定的路径（不回退到其他来源），供 SetFFmpegPath 使用。
// 返回的 state 为 ready 时 info 有效；否则 reason 说明原因。
func (l *Locator) CheckCustom(ctx context.Context, customPath string) (info Info, state string, reason string) {
	cands, err := l.customCandidates(customPath)
	if err != nil {
		return Info{}, StateMissing, err.Error()
	}
	var reasons []string
	var outdated *Info
	for _, b := range cands {
		i, st, r := l.Check(ctx, SourceCustom, b)
		if st == StateReady {
			return i, st, ""
		}
		if st == StateOutdated && outdated == nil {
			c := i
			outdated = &c
		}
		reasons = append(reasons, fmt.Sprintf("%s: %s", b.FFmpeg, r))
	}
	if outdated != nil {
		return *outdated, StateOutdated, strings.Join(reasons, "; ")
	}
	return Info{}, StateMissing, strings.Join(reasons, "; ")
}

// customCandidates 把用户给的路径展开成候选：
// 文件 → 该文件 + 同目录 ffprobe；目录 → 目录本身，以及目录下的 bin/（常见的压缩包解压结构）。
func (l *Locator) customCandidates(p string) ([]Binaries, error) {
	p = strings.TrimSpace(p)
	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, fmt.Errorf("路径无效: %w", err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("路径不存在: %s", abs)
		}
		return nil, fmt.Errorf("无法访问路径: %w", err)
	}
	if !fi.IsDir() {
		return []Binaries{{FFmpeg: abs, FFprobe: l.probeBeside(abs, false)}}, nil
	}
	var out []Binaries
	for _, d := range []string{abs, filepath.Join(abs, "bin")} {
		ff := filepath.Join(d, l.exeName("ffmpeg"))
		if fileExists(ff) {
			out = append(out, Binaries{FFmpeg: ff, FFprobe: filepath.Join(d, l.exeName("ffprobe"))})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("目录下没有 %s: %s", l.exeName("ffmpeg"), abs)
	}
	return out, nil
}

// probeBeside 返回 ffmpeg 同目录的 ffprobe。同目录没有时：fallbackPath 为 false 返回
// 那个不存在的同目录路径（让校验报"无法运行 ffprobe"）；为 true 则到 PATH 里找，
// PATH 里也没有就返回空串，表示 ffprobe 缺失。
func (l *Locator) probeBeside(ffmpegPath string, fallbackPath bool) string {
	beside := filepath.Join(filepath.Dir(ffmpegPath), l.exeName("ffprobe"))
	if fileExists(beside) {
		return beside
	}
	if !fallbackPath {
		return beside
	}
	if p, err := l.lookPath("ffprobe"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	return ""
}

// probeOptional 判断这个候选的 ffprobe 是否允许缺失：仅 v1 兼容目录，且 ffprobe 不在
// ffmpeg 同目录（为空，或是从 PATH 借来的）。借来的 ffprobe 不可用时同样按缺失处理，
// 不因 PATH 里没有或有个坏的 ffprobe 就否掉 v1 自带的 ffmpeg。
func probeOptional(source string, b Binaries) bool {
	if source != SourceLegacy {
		return false
	}
	return b.FFprobe == "" || filepath.Dir(b.FFprobe) != filepath.Dir(b.FFmpeg)
}

// Check 校验一个候选，返回 ready / outdated / missing 三种状态之一（missing 表示不可用，
// reason 说明原因）：
//  1. ffmpeg、ffprobe 都能运行 -version 且输出可识别；
//  2. 主版本不低于 MinMajor，否则 outdated（不再检查编码器）；
//  3. -encoders 输出包含 libx264 和 aac。
func (l *Locator) Check(ctx context.Context, source string, b Binaries) (Info, string, string) {
	run := l.run()
	info := Info{Binaries: b, Source: source}

	out, err := run(ctx, b.FFmpeg, "-hide_banner", "-version")
	if err != nil {
		return info, StateMissing, fmt.Sprintf("无法运行 ffmpeg: %v", err)
	}
	fv, ok := ParseVersion(out)
	if !ok {
		return info, StateMissing, "ffmpeg -version 输出无法识别"
	}
	info.Version, info.Major, info.Known = fv.Display, fv.Major, fv.Known

	probeVer := fv
	if b.FFprobe == "" {
		if !probeOptional(source, b) {
			return info, StateMissing, "缺少 ffprobe"
		}
		info.FFprobe, info.FFprobeMissing = "", true
	} else {
		pv, perr := l.checkProbe(ctx, run, b.FFprobe)
		switch {
		case perr == nil:
			probeVer = pv
		case probeOptional(source, b):
			// 借来的 ffprobe 不可用：按缺失处理，ffmpeg 仍然可用。
			info.FFprobe, info.FFprobeMissing = "", true
		default:
			return info, StateMissing, perr.Error()
		}
	}

	if !fv.Acceptable() || !probeVer.Acceptable() {
		return info, StateOutdated, fmt.Sprintf("版本过低（ffmpeg %s，ffprobe %s），需要 %d 或更高", fv.Display, probeVer.Display, MinMajor)
	}

	out, err = run(ctx, b.FFmpeg, "-hide_banner", "-encoders")
	if err != nil {
		return info, StateMissing, fmt.Sprintf("读取编码器列表失败: %v", err)
	}
	if missing := missingEncoders(out, "libx264", "aac"); len(missing) > 0 {
		return info, StateMissing, "缺少编码器: " + strings.Join(missing, ", ")
	}
	return info, StateReady, ""
}

// checkProbe 运行 ffprobe -version 并解析版本。
func (l *Locator) checkProbe(ctx context.Context, run Runner, ffprobe string) (Version, error) {
	out, err := run(ctx, ffprobe, "-hide_banner", "-version")
	if err != nil {
		return Version{}, fmt.Errorf("无法运行 ffprobe: %v", err)
	}
	v, ok := ParseVersion(out)
	if !ok {
		return Version{}, errors.New("ffprobe -version 输出无法识别")
	}
	return v, nil
}

// missingEncoders 在 `ffmpeg -encoders` 输出里找编码器。每行格式为
// " V....D libx264  libx264 H.264 ..."，第二列才是编码器名，
// 按整词匹配以免把 libx264rgb 当成 libx264。
func missingEncoders(output string, want ...string) []string {
	have := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && len(f[0]) == 6 {
			have[f[1]] = true
		}
	}
	var missing []string
	for _, w := range want {
		if !have[w] {
			missing = append(missing, w)
		}
	}
	return missing
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
