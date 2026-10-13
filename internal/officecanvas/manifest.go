// Package officecanvas 管理办公画布组件的检测、下载占位与导出适配器调用（契约 v0.42，6.19.16）。
//
// 本仓不实现、不打包 Node / PptxGenJS；老板发布组件包并写入 URL / SHA-256 后，再接真下载。
package officecanvas

import "runtime"

// 引导体积（契约 6.19.16.2「数 MB～十几 MB」；精确字节 TBD）。
const (
	GuideDownloadBytes int64 = 16 * 1024 * 1024 // 约 16 MB 占位
	GuideInstallBytes  int64 = GuideDownloadBytes
)

// DiskReserve 是下载前在安装包之外要求的剩余空间（对齐语音识别组件）。
const DiskReserve int64 = 512 << 20 // 512 MiB

// PlaceholderVersion 是未发布前组件版本目录名占位。
const PlaceholderVersion = "0.0.0-tbd"

// PackageSpec 是某一平台的安装包描述。URL / SHA256 为空表示尚未发布。
type PackageSpec struct {
	URL      string
	SHA256   string
	Size     int64 // 精确字节；0 时用引导体积
	Filename string
}

// Manifest 是内置占位清单。老板发版后只改这里的 URL / SHA256 / Size。
// TODO(office-canvas): 填入各平台真实下载地址与校验值。
type Manifest struct {
	Version   string
	Platforms map[string]PackageSpec // key = "<goos>-<goarch>"
}

// builtin 是当前内置占位（URL/SHA 均为空 → canDownload=false）。
var builtin = Manifest{
	Version: PlaceholderVersion,
	Platforms: map[string]PackageSpec{
		"windows-amd64": {Size: GuideDownloadBytes, Filename: "office-canvas-windows-amd64.zip"},
		"darwin-arm64":  {Size: GuideDownloadBytes, Filename: "office-canvas-darwin-arm64.zip"},
		"darwin-amd64":  {Size: GuideDownloadBytes, Filename: "office-canvas-darwin-amd64.zip"},
		"linux-amd64":   {Size: GuideDownloadBytes, Filename: "office-canvas-linux-amd64.zip"},
	},
}

// LoadManifest 返回内置清单（可被测试覆盖）。
func LoadManifest() Manifest { return builtin }

// PlatformKey 返回 "<goos>-<goarch>"。
func PlatformKey() string { return runtime.GOOS + "-" + runtime.GOARCH }

// SpecFor 返回当前平台的包；没有条目时返回零值。
func (m Manifest) SpecFor() PackageSpec {
	return m.Platforms[PlatformKey()]
}

// Configured 表示该包已具备可下载的 URL 与 SHA-256。
func (p PackageSpec) Configured() bool {
	return p.URL != "" && p.SHA256 != ""
}

// EffectiveSize 返回精确 Size，否则引导体积。
func (p PackageSpec) EffectiveSize() int64 {
	if p.Size > 0 {
		return p.Size
	}
	return GuideDownloadBytes
}
