package catagent

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
)

// CLI 版本兼容（契约 v0.43）。每条分支都要有 changelog 或真机 help 依据，见 PR 表。
const (
	// Claude CHANGELOG 1.0.109：「Added partial message streaming support via --include-partial-messages」。
	// 我们每轮都传这个开关，低于此版本必然报参数错。
	claudeMinVersion = "1.0.109"
	// Claude CHANGELOG 2.1.200：「Changed the "default" permission mode to "Manual" …
	// --permission-mode manual … accepted alongside default」。更早的版本只认 default。
	claudeManualSince = "2.1.200"
)

// parseCLIVersion 取 "2.1.296" / "codex-cli 0.162.1" / "grok 1.0.40 (…)" 里的 x.y.z。
var cliVersionRe = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

func parseCLIVersion(s string) ([3]int, bool) {
	var v [3]int
	m := cliVersionRe.FindStringSubmatch(s)
	if m == nil {
		return v, false
	}
	for i := 0; i < 3; i++ {
		if m[i+1] != "" {
			v[i], _ = strconv.Atoi(m[i+1])
		}
	}
	return v, true
}

// cliVersionBelow：ver 能解析且严格小于 min 才为真；解析不了按「不低于」处理（不误报过旧）。
func cliVersionBelow(ver, min string) bool {
	a, ok := parseCLIVersion(ver)
	if !ok {
		return false
	}
	b, _ := parseCLIVersion(min)
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// 版本探测缓存：同一二进制路径 + mtime + 大小只跑一次 --version。
type versionKey struct {
	path  string
	mtime time.Time
	size  int64
}

var (
	versionCacheMu sync.Mutex
	versionCache   = map[versionKey]string{}
)

func cachedCLIVersion(exe string, probe func(string) string) string {
	st, err := os.Stat(exe)
	if err != nil {
		return probe(exe)
	}
	k := versionKey{exe, st.ModTime(), st.Size()}
	versionCacheMu.Lock()
	v, ok := versionCache[k]
	versionCacheMu.Unlock()
	if ok {
		return v
	}
	v = probe(exe)
	versionCacheMu.Lock()
	versionCache[k] = v
	versionCacheMu.Unlock()
	return v
}

// 子命令拒收参数的典型报错（clap / commander 风格）。
var argRejectedRe = regexp.MustCompile(`(?i)(unexpected argument|unknown option|unrecognized option|unknown argument|invalid value|error: option '[^']*' argument)`)

func cliArgRejected(stderr string) bool { return argRejectedRe.MatchString(stderr) }

func cliLabel(kind string) string {
	switch kind {
	case KindClaudeCLI:
		return "Claude"
	case KindCodexCLI:
		return "Codex"
	default:
		return "Grok Build"
	}
}

// outdatedCLIErr：CLI 过旧 / 拒收参数。沿用 CAT_REPLY_FAILED（可重试），不加新错误码。
func outdatedCLIErr(kind, version, raw string) *apperr.AppError {
	if version == "path" {
		version = ""
	}
	raw = strings.ReplaceAll(strings.TrimSpace(raw), "\r", "")
	raw = strings.ReplaceAll(truncate(raw, 2000), "\n", "\\n")
	detail := strings.Join([]string{
		"reason=cli_outdated",
		"agentKind=" + kind,
		"cliVersion=" + version,
		"raw=" + raw,
	}, "\n")
	return apperr.New(apperr.CatReplyFailed, cliLabel(kind)+" CLI 版本过旧，请升级后重试。").WithDetail(detail)
}
