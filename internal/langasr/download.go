package langasr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"FFmpegFree/internal/apperr"
)

type downloadArgs struct {
	Dir, Tmp, Tier, Version string
	Pkg                     PackageSpec
	Emit                    func(event string, payload any)
	Logf                    func(format string, args ...any)
}

// downloadAndPrepare 是下载 + 解包骨架。URL 已配置时由后续 PR 接真实现；
// 当前若误入（Configured 为 true 但管道未完成）返回明确错误。
func downloadAndPrepare(ctx context.Context, a downloadArgs) error {
	_ = ctx
	if !a.Pkg.Configured() {
		return apperr.New(apperr.LangDownloadFailed, MsgNotPublished).WithDetail("reason=not_configured")
	}
	// 骨架：真实 Range 续传 / SHA-256 / 解包对齐文档组件；此处先占位失败，避免半成品装上。
	if a.Logf != nil {
		a.Logf("语音识别组件下载管道尚未接真：url 已配置但骨架未实现完整解包")
	}
	_ = os.MkdirAll(a.Tmp, 0o755)
	_ = filepath.Join(a.Dir, "lang", "asr", a.Tier, a.Version)
	return apperr.New(apperr.LangDownloadFailed, MsgDownloadFailed).
		WithDetail(fmt.Sprintf("phase=downloading\nreason=pipeline_stub\nurl=%s", a.Pkg.URL))
}
