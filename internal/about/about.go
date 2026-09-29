// Package about 提供“关于”页需要的两项静态信息：内嵌的第三方许可全文和应用版本号。
//
// 许可文本用 go:embed 打进程序，只有白名单里的名字可以读取（不接触文件系统，不存在路径穿越）。
// 版本号在构建时用 -ldflags 注入：
//
//	go build -ldflags "-X FFmpegFree/internal/about.Version=1.2.3"
//	wails build -ldflags "-X FFmpegFree/internal/about.Version=1.2.3"
package about

import (
	_ "embed"
	"fmt"

	"FFmpegFree/internal/apperr"
)

// Version 由构建时的 -ldflags "-X FFmpegFree/internal/about.Version=..." 注入；未注入（空串）时显示 DevVersion。
var Version string

// DevVersion 是没有注入版本号时的显示文本。
const DevVersion = "开发版"

// AppVersion 返回应用版本号，未注入时返回 DevVersion。
func AppVersion() string {
	if Version == "" {
		return DevVersion
	}
	return Version
}

// oflText 是 SIL Open Font License 1.1 全文，Noto Sans SC 的许可。
// 内容与 google/fonts 仓库 ofl/notosanssc/OFL.txt 逐字节相同（与内嵌字体目录 internal/service/doc/fonts/OFL.txt 同一份）。
//
//go:embed OFL.txt
var oflText string

// oflNunitoText 是 Nunito 字体（前端 UI 字体）的 SIL Open Font License 1.1 全文，与 oflText（Noto Sans SC）内容不同。
// 原样拷贝自 frontend/src/assets/fonts/OFL.txt（go:embed 不能跨出包目录，所以在本包里放一份；
// TestNunitoLicenseMatchesFrontend 会在前端那份变化时提示同步）。
//
//go:embed OFL-Nunito.txt
var oflNunitoText string

// licenses 是可读取的许可白名单，key 必须与传入的名字完全一致（区分大小写、不带扩展名）。
var licenses = map[string]string{
	"OFL":        oflText,
	"OFL-Nunito": oflNunitoText,
}

// maxEchoRunes 是错误详情里回显用户输入的最大字符数。
const maxEchoRunes = 32

// LicenseText 返回白名单内许可的全文；未知名字（含空串、带路径、大小写不同）返回 INVALID_ARGUMENT。
func LicenseText(name string) (string, error) {
	if text, ok := licenses[name]; ok {
		return text, nil
	}
	return "", apperr.New(apperr.InvalidArgument, "未知的许可名称").
		WithDetail(fmt.Sprintf("name=%s，可用: OFL, OFL-Nunito", echo(name)))
}

// echo 把用户输入截断并加引号（转义换行等控制字符），避免错误详情里出现超长或多行的原样输入。
func echo(s string) string {
	r := []rune(s)
	if len(r) > maxEchoRunes {
		return fmt.Sprintf("%q…（共 %d 个字符）", string(r[:maxEchoRunes]), len(r))
	}
	return fmt.Sprintf("%q", s)
}
