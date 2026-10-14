//go:build !windows

package i18n

import (
	"os"
	"strings"
)

// DetectOSLocaleRaw 读取 Linux / macOS 的 locale 环境变量（契约 6.20.3）。
// 顺序：LC_ALL → LC_MESSAGES → LANG。macOS 的 AppleLanguages 由同一环境变量兜底
// （GUI 进程通常已设 LANG）；完整 AppleLanguages 读取留给真机验证。
func DetectOSLocaleRaw() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
