package catagent

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// v0.47.1（契约 6.19.21.4）：Windows 上 exec.LookPath 找不到 CLI 时的兜底查找。
// 纯逻辑放这里（任何平台都能单测）；真正读注册表在 winlookup_windows.go，其它平台是空实现。

// winLookupEnv 是兜底查找的全部外部依赖，测试注入假目录 / 假注册表。
type winLookupEnv struct {
	// RegistryPath 返回注册表里机器 PATH（HKLM …\Session Manager\Environment\Path）与用户 PATH（HKCU\Environment\Path）的原值（未展开）。
	RegistryPath func() (machine, user string)
	Getenv       func(string) string
	// IsFile 报告路径是否为已存在的普通文件。
	IsFile func(string) bool
}

// shimExts 是每个目录里依次尝试的扩展名：优先 .exe，再 .cmd，再 .ps1。
var shimExts = []string{".exe", ".cmd", ".ps1"}

var winEnvRef = regexp.MustCompile(`%([^%]+)%`)

// expandWinEnv 展开 REG_EXPAND_SZ 里的 %VAR%；未定义的变量原样保留（与 ExpandEnvironmentStrings 一致）。
func expandWinEnv(s string, getenv func(string) string) string {
	return winEnvRef.ReplaceAllStringFunc(s, func(m string) string {
		name := m[1 : len(m)-1]
		if v := getenv(name); v != "" {
			return v
		}
		return m
	})
}

// registryPathDirs 合并注册表 PATH：机器在前、用户在后，按不区分大小写去重，空项丢弃。
func registryPathDirs(env winLookupEnv) []string {
	if env.RegistryPath == nil {
		return nil
	}
	machine, user := env.RegistryPath()
	seen := map[string]bool{}
	var out []string
	for _, raw := range []string{machine, user} {
		for _, d := range strings.Split(raw, ";") {
			d = strings.TrimSpace(strings.Trim(strings.TrimSpace(d), `"`))
			if d == "" {
				continue
			}
			d = expandWinEnv(d, env.Getenv)
			k := strings.ToLower(strings.TrimRight(d, `\/`))
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, d)
		}
	}
	return out
}

// knownInstallDirs 是 6.19.21.4 查找目录表（顺序即表中顺序）。
// 标 GUESS 的目录需真机 Windows 核过才算定稿；本版只作兜底。
func knownInstallDirs(agentKind string, getenv func(string) string) []string {
	join := func(envName string, rest ...string) string {
		base := getenv(envName)
		if base == "" {
			return ""
		}
		return filepath.Join(append([]string{base}, rest...)...)
	}
	var dirs []string
	switch agentKind {
	case KindClaudeCLI:
		dirs = []string{
			join("USERPROFILE", ".local", "bin"),                 // 官方文档有据；真机待核
			join("APPDATA", "npm"),                               // GUESS：npm 默认全局 prefix
			join("LOCALAPPDATA", "Microsoft", "WinGet", "Links"), // GUESS：WinGet 惯例
		}
	case KindCodexCLI:
		if d := strings.TrimSpace(getenv("CODEX_INSTALL_DIR")); d != "" {
			dirs = append(dirs, d) // 官方安装脚本：设了 CODEX_INSTALL_DIR 就查它
		} else {
			dirs = append(dirs, join("LOCALAPPDATA", "Programs", "OpenAI", "Codex", "bin")) // 安装脚本有据；真机待核
		}
		dirs = append(dirs, join("APPDATA", "npm")) // GUESS
	case KindCatBuild:
		dirs = []string{ // 全部 GUESS：未找到官方 Windows 安装位置文档
			join("USERPROFILE", ".local", "bin"),
			join("LOCALAPPDATA", "Programs", "grok"),
			join("APPDATA", "npm"),
		}
	}
	out := dirs[:0]
	for _, d := range dirs {
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

// findInDirs 按目录顺序、每个目录内 .exe → .cmd → .ps1 找第一个存在的 <base><ext>。
func findInDirs(base string, dirs []string, isFile func(string) bool) string {
	for _, d := range dirs {
		for _, ext := range shimExts {
			p := filepath.Join(d, base+ext)
			if isFile(p) {
				return p
			}
		}
	}
	return ""
}

// winFallbackFind：① 重读注册表 PATH 查找；② 查已知安装目录；③ 都没有返回 ""。
// 不修改进程或系统 PATH。base 不带扩展名（claude / codex / grok）。
func winFallbackFind(agentKind, base string, env winLookupEnv) string {
	if env.Getenv == nil {
		env.Getenv = os.Getenv
	}
	if env.IsFile == nil {
		env.IsFile = isRegularFile
	}
	if p := findInDirs(base, registryPathDirs(env), env.IsFile); p != "" {
		return p
	}
	return findInDirs(base, knownInstallDirs(agentKind, env.Getenv), env.IsFile)
}

func isRegularFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// defaultWinLookupEnv 用真注册表（windows）/ 空实现（其它平台）。
func defaultWinLookupEnv() winLookupEnv {
	return winLookupEnv{RegistryPath: readRegistryPath, Getenv: os.Getenv, IsFile: isRegularFile}
}

// DetectTimeout 是单家 CLI 一次检测的上限（v0.47.1 架构师补充：15 秒）；超时记为检测失败。测试可改小。
var DetectTimeout = 15 * time.Second

// winFallback 是适配器在 LookPath 全部失败后调用的兜底（测试可替换）。
var winFallback = func(agentKind, base string) string {
	return winFallbackFind(agentKind, base, defaultWinLookupEnv())
}

// winFallbackEnabled：只在 Windows 上走兜底（测试可替换）。
var winFallbackEnabled = func() bool { return runtime.GOOS == "windows" }

// isShimExt 报告路径是否是 .cmd / .bat / .ps1 包装（需经 cmd.exe / powershell 启动，提示词不得走命令行）。
func isShimExt(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".cmd", ".bat", ".ps1":
		return true
	}
	return false
}

// cmdMetaChars 是 cmd.exe 的元字符：在命令行里一律加 ^ 转义。
var cmdMetaChars = regexp.MustCompile(`([()\][%!^"` + "`" + `<>&|;, *?])`)

// quoteWinArg 按 MSVCRT 规则给一个参数加引号（反斜杠 + 引号）。
func quoteWinArg(arg string) string {
	var b strings.Builder
	b.WriteByte('"')
	bs := 0
	for _, r := range arg {
		switch r {
		case '\\':
			bs++
			continue
		case '"':
			b.WriteString(strings.Repeat(`\`, bs*2+1))
			b.WriteRune('"')
		default:
			b.WriteString(strings.Repeat(`\`, bs))
			b.WriteRune(r)
		}
		bs = 0
	}
	b.WriteString(strings.Repeat(`\`, bs*2))
	b.WriteByte('"')
	return b.String()
}

// escapeCmdArg 先按 MSVCRT 加引号，再对 cmd 元字符（含 " % ^ & | < >）加 ^。
// .cmd 包装会把 %* 再展开一次，所以转义两遍（与 cross-spawn 的做法一致）。
func escapeCmdArg(arg string) string {
	s := quoteWinArg(arg)
	s = cmdMetaChars.ReplaceAllString(s, "^$1")
	return cmdMetaChars.ReplaceAllString(s, "^$1")
}

// cmdShimCommandLine 拼 `cmd.exe /d /s /c ""<完整路径>" <参数…>"` 整行（给 SysProcAttr.CmdLine）。
// 路径用真引号包住（路径里可有空格、&）；参数逐个转义。提示词不得出现在 args 里（调用方保证走 stdin / 临时文件）。
func cmdShimCommandLine(comspec, exe string, args []string) string {
	var b strings.Builder
	b.WriteString(`"` + exe + `"`)
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(escapeCmdArg(a))
	}
	return `"` + comspec + `" /d /s /c "` + b.String() + `"`
}

// ps1ShimArgs 是 .ps1 包装的 argv：powershell.exe -NoProfile -ExecutionPolicy Bypass -File <路径> <参数…>。
func ps1ShimArgs(exe string, args []string) []string {
	return append([]string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", exe}, args...)
}

// shimStdinArgs（6.19.21.4）：入口是 .cmd / .ps1 包装时，用户输入（提示词、规则）不得经命令行传。
// Claude：去掉 -p 后的提示词（claude -p 无提示词参数时读 stdin），规则改写进临时文件走 --append-system-prompt-file。
// Codex：最后一个参数（提示词，规则已前缀进去）换成 "-"（codex exec / exec resume：`-` = 从 stdin 读）。
// 返回新 argv、要写进 stdin 的内容、清理函数（删临时文件；可为 nil）。
func shimStdinArgs(kind cliKind, args []string, tempDir string) (out []string, stdin string, cleanup func(), err error) {
	out = append([]string(nil), args...)
	switch kind {
	case cliKindCodex:
		if len(out) == 0 {
			return out, "", nil, nil
		}
		stdin = out[len(out)-1]
		out[len(out)-1] = "-"
		return out, stdin, nil, nil
	default:
		if len(out) >= 2 && out[0] == "-p" {
			stdin = out[1]
			out = append(out[:1:1], out[2:]...)
		}
		for i := 0; i+1 < len(out); i++ {
			if out[i] != "--append-system-prompt" {
				continue
			}
			f, ferr := os.CreateTemp(tempDir, "cat-rules-*.txt")
			if ferr != nil {
				return nil, "", nil, ferr
			}
			name := f.Name()
			_, werr := f.WriteString(out[i+1])
			cerr := f.Close()
			if werr != nil || cerr != nil {
				_ = os.Remove(name)
				if werr == nil {
					werr = cerr
				}
				return nil, "", nil, werr
			}
			out[i], out[i+1] = "--append-system-prompt-file", name
			cleanup = func() { _ = os.Remove(name) }
			break
		}
		return out, stdin, cleanup, nil
	}
}
