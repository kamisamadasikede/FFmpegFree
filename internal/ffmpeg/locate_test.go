package ffmpeg

import (
	"FFmpegFree/internal/apperr"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const encodersOK = "Encoders:\n ------\n V....D libx264  libx264 H.264\n A....D aac  AAC\n"

// fakeSpec 描述一个假的 ffmpeg / ffprobe：keyed by 可执行文件路径。
type fakeSpec struct {
	version  string // 版本行，如 "ffmpeg version 6.1.1"；空表示无法运行
	encoders string
}

// fakeRunner 按路径返回预设输出，并记录调用。
func fakeRunner(specs map[string]fakeSpec, calls *[]string) Runner {
	return func(ctx context.Context, exe string, args ...string) (string, error) {
		if calls != nil {
			*calls = append(*calls, exe+" "+strings.Join(args, " "))
		}
		s, ok := specs[filepath.Clean(exe)]
		if !ok || s.version == "" {
			return "", errors.New("无法执行")
		}
		for _, a := range args {
			switch a {
			case "-version":
				return s.version + " Copyright (c) the FFmpeg developers\n", nil
			case "-encoders":
				return s.encoders, nil
			}
		}
		return "", fmt.Errorf("未预期的参数 %v", args)
	}
}

// touch 创建空文件（fakeRunner 不真正执行，只需要文件存在）。
func touch(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

type env struct {
	root, binDir, exeDir string
	loc                  *Locator
	specs                map[string]fakeSpec
	calls                []string
	pathHit              map[string]string // LookPath 结果
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	e := &env{
		root:    root,
		binDir:  filepath.Join(root, "data", "bin"),
		exeDir:  filepath.Join(root, "app"),
		specs:   map[string]fakeSpec{},
		pathHit: map[string]string{},
	}
	e.loc = &Locator{
		Run: fakeRunner(e.specs, &e.calls),
		LookPath: func(n string) (string, error) {
			if p, ok := e.pathHit[n]; ok {
				return p, nil
			}
			return "", errors.New("not found")
		},
		ExeDir: func() (string, error) { return e.exeDir, nil },
		BinDir: e.binDir,
		GOOS:   "linux",
	}
	return e
}

// add 创建 ffmpeg + ffprobe 文件并登记假的行为。
func (e *env) add(t *testing.T, dir, ver string) (ffmpegPath, ffprobePath string) {
	t.Helper()
	ffmpegPath = touch(t, filepath.Join(dir, "ffmpeg"))
	ffprobePath = touch(t, filepath.Join(dir, "ffprobe"))
	e.specs[filepath.Clean(ffmpegPath)] = fakeSpec{version: "ffmpeg version " + ver, encoders: encodersOK}
	e.specs[filepath.Clean(ffprobePath)] = fakeSpec{version: "ffprobe version " + ver}
	return
}

func TestLocateOrder(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	custom := filepath.Join(e.root, "custom")
	cf, _ := e.add(t, custom, "6.0")
	bf, _ := e.add(t, e.binDir, "6.1")
	sysDir := filepath.Join(e.root, "sys")
	e.add(t, sysDir, "7.0")
	e.pathHit["ffmpeg"] = filepath.Join(sysDir, "ffmpeg")
	lf, _ := e.add(t, filepath.Join(e.exeDir, "ffmpeg"), "6.2")

	// 1. 指定了自定义目录：优先
	res, err := e.loc.Locate(ctx, custom)
	if err != nil || res.State != StateReady || res.Info.Source != SourceCustom || res.Info.FFmpeg != cf {
		t.Fatalf("custom: %+v err=%v", res, err)
	}
	// 自定义也可以直接指向文件
	res, _ = e.loc.Locate(ctx, cf)
	if res.State != StateReady || res.Info.Source != SourceCustom {
		t.Fatalf("custom file: %+v", res)
	}
	// 2. 不指定：自带 bin
	res, _ = e.loc.Locate(ctx, "")
	if res.State != StateReady || res.Info.Source != SourceBundled || res.Info.FFmpeg != bf || res.Info.Version != "6.1" {
		t.Fatalf("bundled: %+v", res)
	}
	// 3. 删掉自带：PATH
	os.Remove(bf)
	res, _ = e.loc.Locate(ctx, "")
	if res.State != StateReady || res.Info.Source != SourceSystem || res.Info.Major != 7 {
		t.Fatalf("system: %+v", res)
	}
	// 4. PATH 也没有：legacy
	delete(e.pathHit, "ffmpeg")
	res, _ = e.loc.Locate(ctx, "")
	if res.State != StateReady || res.Info.Source != SourceLegacy || res.Info.FFmpeg != lf {
		t.Fatalf("legacy: %+v", res)
	}
	// 5. 全没有
	os.Remove(lf)
	res, _ = e.loc.Locate(ctx, "")
	if res.State != StateMissing {
		t.Fatalf("missing: %+v", res)
	}
}

func TestLocateCustomInvalidFallsThrough(t *testing.T) {
	e := newEnv(t)
	e.add(t, e.binDir, "6.1")
	res, _ := e.loc.Locate(context.Background(), filepath.Join(e.root, "不存在"))
	if res.State != StateReady || res.Info.Source != SourceBundled {
		t.Fatalf("自定义路径失效时应回退到后续来源: %+v", res)
	}
	if len(res.Attempts) == 0 || res.Attempts[0].Source != SourceCustom {
		t.Fatalf("应记录自定义路径失败原因: %+v", res.Attempts)
	}
}

func TestLocateCustomDirWithBinSubdir(t *testing.T) {
	e := newEnv(t)
	top := filepath.Join(e.root, "ffmpeg-7.0-build")
	f, _ := e.add(t, filepath.Join(top, "bin"), "7.0")
	res, _ := e.loc.Locate(context.Background(), top)
	if res.State != StateReady || res.Info.FFmpeg != f {
		t.Fatalf("应识别解压目录下的 bin/: %+v", res)
	}
}

func TestLocateOutdated(t *testing.T) {
	e := newEnv(t)
	f, _ := e.add(t, e.binDir, "4.4.2")
	res, _ := e.loc.Locate(context.Background(), "")
	if res.State != StateOutdated || res.Info.FFmpeg != f || res.Info.Major != 4 {
		t.Fatalf("outdated: %+v", res)
	}
	// 低版本不应继续检查 -encoders
	for _, c := range e.calls {
		if strings.Contains(c, "-encoders") {
			t.Fatalf("版本过低不应再查编码器: %v", e.calls)
		}
	}
	// 后面有合格的来源则以合格的为准
	sysDir := filepath.Join(e.root, "sys")
	e.add(t, sysDir, "6.0")
	e.pathHit["ffmpeg"] = filepath.Join(sysDir, "ffmpeg")
	res, _ = e.loc.Locate(context.Background(), "")
	if res.State != StateReady || res.Info.Source != SourceSystem {
		t.Fatalf("应跳过过低版本选用合格的: %+v", res)
	}
}

func TestCheckFailures(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)

	// 缺 libx264
	f, p := e.add(t, e.binDir, "6.1")
	e.specs[f] = fakeSpec{version: "ffmpeg version 6.1", encoders: " A....D aac  AAC\n"}
	_, st, reason := e.loc.Check(ctx, SourceBundled, Binaries{f, p})
	if st != StateMissing || !strings.Contains(reason, "libx264") {
		t.Fatalf("缺 libx264: %s %s", st, reason)
	}
	// 缺 aac
	e.specs[f] = fakeSpec{version: "ffmpeg version 6.1", encoders: " V....D libx264  x\n"}
	_, st, reason = e.loc.Check(ctx, SourceBundled, Binaries{f, p})
	if st != StateMissing || !strings.Contains(reason, "aac") {
		t.Fatalf("缺 aac: %s %s", st, reason)
	}
	// ffprobe 不存在（v1 只带 ffmpeg.exe 的情形）
	e.specs[f] = fakeSpec{version: "ffmpeg version 6.1", encoders: encodersOK}
	_, st, reason = e.loc.Check(ctx, SourceBundled, Binaries{f, filepath.Join(e.binDir, "nope")})
	if st != StateMissing || !strings.Contains(reason, "ffprobe") {
		t.Fatalf("缺 ffprobe: %s %s", st, reason)
	}
	// ffprobe 版本过低
	e.specs[p] = fakeSpec{version: "ffprobe version 5.0"}
	_, st, _ = e.loc.Check(ctx, SourceBundled, Binaries{f, p})
	if st != StateOutdated {
		t.Fatalf("ffprobe 过低应为 outdated: %s", st)
	}
	// 输出无法识别
	e.specs[p] = fakeSpec{version: "ffprobe version 6.1"}
	e.specs[f] = fakeSpec{version: "garbage", encoders: encodersOK}
	_, st, _ = e.loc.Check(ctx, SourceBundled, Binaries{f, p})
	if st != StateMissing {
		t.Fatalf("垃圾输出应为 missing: %s", st)
	}
}

func TestCheckGitBuild(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	f, p := e.add(t, e.binDir, "N-12345-gabc1234")
	e.specs[p] = fakeSpec{version: "ffprobe version N-12345-gabc1234"}
	info, st, _ := e.loc.Check(ctx, SourceBundled, Binaries{f, p})
	if st != StateReady || info.Known {
		t.Fatalf("git 构建编码器齐全应接受: %s %+v", st, info)
	}
	// 编码器缺失则仍然拒绝
	e.specs[f] = fakeSpec{version: "ffmpeg version N-12345-gabc1234", encoders: " A....D aac  x\n"}
	if _, st, _ = e.loc.Check(ctx, SourceBundled, Binaries{f, p}); st != StateMissing {
		t.Fatalf("git 构建缺编码器应拒绝: %s", st)
	}
}

func TestLocateWindowsNames(t *testing.T) {
	e := newEnv(t)
	e.loc.GOOS = "windows"
	f := touch(t, filepath.Join(e.binDir, "ffmpeg.exe"))
	p := touch(t, filepath.Join(e.binDir, "ffprobe.exe"))
	e.specs[f] = fakeSpec{version: "ffmpeg version 6.1", encoders: encodersOK}
	e.specs[p] = fakeSpec{version: "ffprobe version 6.1"}
	res, _ := e.loc.Locate(context.Background(), "")
	if res.State != StateReady || res.Info.FFmpeg != f {
		t.Fatalf("windows 应使用 .exe: %+v", res)
	}
}

func TestLocateLegacyFallsBackToPathFFprobe(t *testing.T) {
	e := newEnv(t)
	lf := touch(t, filepath.Join(e.exeDir, "ffmpeg", "ffmpeg"))
	e.specs[lf] = fakeSpec{version: "ffmpeg version 6.1", encoders: encodersOK}
	// v1 目录里没有 ffprobe，用 PATH 里的
	sysProbe := touch(t, filepath.Join(e.root, "sys", "ffprobe"))
	e.specs[sysProbe] = fakeSpec{version: "ffprobe version 6.1"}
	e.pathHit["ffprobe"] = sysProbe
	res, _ := e.loc.Locate(context.Background(), "")
	if res.State != StateReady || res.Info.Source != SourceLegacy || res.Info.FFprobe != sysProbe {
		t.Fatalf("legacy 应退回 PATH 的 ffprobe: %+v", res)
	}
}

func TestLocateContextCanceled(t *testing.T) {
	e := newEnv(t)
	e.add(t, e.binDir, "6.1")
	ctx, cancel := context.WithCancel(context.Background())
	e.loc.Run = func(ctx context.Context, exe string, args ...string) (string, error) {
		cancel()
		return "", ctx.Err()
	}
	if _, err := e.loc.Locate(ctx, ""); err == nil {
		t.Fatal("ctx 取消应返回 error")
	}
}

// 下面用真实的 shell 脚本 + ExecRunner 验证端到端（含 proc.Configure 与超时），仅 unix。

func writeScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func fakeToolchain(t *testing.T, dir, version, encoders string) {
	writeScript(t, filepath.Join(dir, "ffmpeg"), fmt.Sprintf(`
case "$*" in
  *-encoders*) printf '%%s' '%s' ;;
  *-version*) echo 'ffmpeg version %s Copyright (c) the FFmpeg developers' ;;
esac
`, encoders, version))
	writeScript(t, filepath.Join(dir, "ffprobe"), fmt.Sprintf(`echo 'ffprobe version %s Copyright'`+"\n", version))
}

func TestLocateWithRealScripts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell 脚本假二进制仅在 unix 上运行")
	}
	dir := t.TempDir()
	fakeToolchain(t, dir, "6.1.1-test", encodersOK)
	l := NewLocator(filepath.Join(dir, "nobin"))
	l.LookPath = func(string) (string, error) { return "", errors.New("no") }
	l.ExeDir = func() (string, error) { return dir, nil }

	res, err := l.Locate(context.Background(), dir)
	if err != nil || res.State != StateReady || res.Info.Version != "6.1.1-test" || res.Info.Major != 6 {
		t.Fatalf("%+v %v", res, err)
	}

	// 低版本
	old := t.TempDir()
	fakeToolchain(t, old, "5.1.2", encodersOK)
	res, _ = l.Locate(context.Background(), old)
	if res.State != StateOutdated {
		t.Fatalf("期望 outdated: %+v", res)
	}

	// 不可执行 → missing
	bad := t.TempDir()
	writeScript(t, filepath.Join(bad, "ffmpeg"), "exit 3\n")
	writeScript(t, filepath.Join(bad, "ffprobe"), "exit 3\n")
	res, _ = l.Locate(context.Background(), bad)
	if res.State != StateMissing {
		t.Fatalf("期望 missing: %+v", res)
	}
}

func TestExecRunnerTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell 脚本仅在 unix 上运行")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "slow")
	writeScript(t, p, "sleep 30\n")
	run := ExecRunner(200 * 1e6) // 200ms
	_, err := run(context.Background(), p, "-version")
	if err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("期望超时错误, got %v", err)
	}
}

func TestLegacyWithoutFFprobeStillUsable(t *testing.T) {
	e := newEnv(t)
	lf := touch(t, filepath.Join(e.exeDir, "ffmpeg", "ffmpeg"))
	e.specs[lf] = fakeSpec{version: "ffmpeg version 6.1", encoders: encodersOK}
	// v1 目录没有 ffprobe，PATH 里也没有
	res, err := e.loc.Locate(context.Background(), "")
	if err != nil || res.State != StateReady || res.Info.Source != SourceLegacy {
		t.Fatalf("缺 ffprobe 的 v1 目录应仍为 ready: %+v %v", res, err)
	}
	if !res.Info.FFprobeMissing || res.Info.FFprobe != "" || res.Info.FFmpeg != lf {
		t.Fatalf("应标记 ffprobe 缺失: %+v", res.Info)
	}
	// 不应尝试运行空路径
	for _, c := range e.calls {
		if strings.HasPrefix(c, " ") {
			t.Fatalf("不应运行空 ffprobe 路径: %q", c)
		}
	}
}

func TestLegacyBrokenPathFFprobeTreatedAsMissing(t *testing.T) {
	e := newEnv(t)
	lf := touch(t, filepath.Join(e.exeDir, "ffmpeg", "ffmpeg"))
	e.specs[lf] = fakeSpec{version: "ffmpeg version 6.1", encoders: encodersOK}
	bad := touch(t, filepath.Join(e.root, "sys", "ffprobe")) // 没有登记 spec → 无法运行
	e.pathHit["ffprobe"] = bad
	res, _ := e.loc.Locate(context.Background(), "")
	if res.State != StateReady || !res.Info.FFprobeMissing || res.Info.FFprobe != "" {
		t.Fatalf("PATH 里的 ffprobe 不可用不应否掉 v1 的 ffmpeg: %+v", res)
	}
}

func TestLegacyOutdatedWithoutFFprobe(t *testing.T) {
	e := newEnv(t)
	lf := touch(t, filepath.Join(e.exeDir, "ffmpeg", "ffmpeg"))
	e.specs[lf] = fakeSpec{version: "ffmpeg version 4.4", encoders: encodersOK}
	res, _ := e.loc.Locate(context.Background(), "")
	if res.State != StateOutdated || !res.Info.FFprobeMissing {
		t.Fatalf("%+v", res)
	}
}

func TestFFprobeRequiredForNonLegacy(t *testing.T) {
	e := newEnv(t)
	// 自带目录和自定义目录都不允许缺 ffprobe
	bf := touch(t, filepath.Join(e.binDir, "ffmpeg"))
	e.specs[bf] = fakeSpec{version: "ffmpeg version 6.1", encoders: encodersOK}
	res, _ := e.loc.Locate(context.Background(), "")
	if res.State != StateMissing {
		t.Fatalf("bundled 缺 ffprobe 应判失败: %+v", res)
	}
	info, st, reason := e.loc.CheckCustom(context.Background(), e.binDir)
	if st != StateMissing || !strings.Contains(reason, "ffprobe") || info.FFprobeMissing {
		t.Fatalf("custom 缺 ffprobe 应判失败: %s %s %+v", st, reason, info)
	}
}

func TestRequireProbe(t *testing.T) {
	t.Cleanup(func() { SetCurrent(nil) })
	SetCurrent(&Binaries{FFmpeg: "/v1/ffmpeg"})
	if _, err := Require(); err != nil {
		t.Fatalf("只需 ffmpeg 的入口应放行: %v", err)
	}
	if _, err := RequireProbe(); !apperr.Is(err, apperr.FFmpegNotFound) {
		t.Fatalf("缺 ffprobe 时 RequireProbe 应返回 FFMPEG_NOT_FOUND: %v", err)
	}
	SetCurrent(&Binaries{FFmpeg: "/a/ffmpeg", FFprobe: "/a/ffprobe"})
	if _, err := RequireProbe(); err != nil {
		t.Fatal(err)
	}
}
