package ffmpeg

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// hwFallbackReason 枚举必须在三处一致：Go 常量、契约 9.7 的“取值”一行、前端 taskTypes.ts 里 hwFallbackReason 的注释。
// 少一个或多一个都会失败（前端文案 errors/encoderMessages.ts 目前不按原因区分，见契约 9.7）。
func TestHWFallbackReasonEnumConsistent(t *testing.T) {
	goSet := []string{ReasonDeviceUnavailable, ReasonNVENCInit, ReasonQSVInit, ReasonAMFInit, ReasonVTInit, ReasonEncoderMissing, ReasonEncoderStart}
	sort.Strings(goSet)
	tok := regexp.MustCompile("[a-z][a-z0-9]*(?:_[a-z0-9]+)+|videotoolbox_failed")

	check := func(file, marker string, extract func(line string) string) {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Skipf("读不到 %s: %v", file, err)
		}
		var line string
		for _, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, marker) {
				line = extract(l)
				break
			}
		}
		if line == "" {
			t.Fatalf("%s 里找不到 %q", file, marker)
		}
		got := map[string]bool{}
		for _, m := range tok.FindAllString(line, -1) {
			got[m] = true
		}
		var list []string
		for k := range got {
			list = append(list, k)
		}
		sort.Strings(list)
		if strings.Join(list, ",") != strings.Join(goSet, ",") {
			t.Errorf("%s 的枚举与 Go 常量不一致:\n got %v\nwant %v", file, list, goSet)
		}
	}
	// 契约：`hwFallbackReason` 取值（…）：`a`、`b`…（只取冒号之后到句末）
	check("../../docs/architecture/contract.md", "`hwFallbackReason` 取值（", func(l string) string {
		i := strings.Index(l, "）：")
		if i < 0 {
			return ""
		}
		return l[i:]
	})
	// TS：/** 回退原因（…）：a / b / c */
	check("../../frontend/src/api/taskTypes.ts", "回退原因（", func(l string) string {
		i := strings.Index(l, "）：")
		if i < 0 {
			return ""
		}
		return l[i:]
	})
}
