// Package langasr 管理语音识别组件的检测、下载占位与 ASR 适配器调用（契约 v0.29，6.18）。
//
// 本仓不实现、不打包 Python CLI / 模型；老板发布组件包并写入 URL / SHA-256 后，再接真下载。
package langasr

import "runtime"

// 档位（Settings.asrTier / InstallLangAsr）。
const (
	TierStandard = "standard"
	TierHD       = "hd"
)

// 引导体积（契约 6.18.3；发布前可改；未配置真实包时仍按档位展示）。
const (
	DownloadBytesStandard int64 = 400 * 1024 * 1024     // 约 400 MB
	DownloadBytesHD       int64 = 1536 * 1024 * 1024    // 约 1.5 GB（高清引导）
	InstallBytesStandard  int64 = DownloadBytesStandard // 占位：解包后约等于包大小，实测后再换
	InstallBytesHD        int64 = DownloadBytesHD
)

// DiskReserve 是下载前在安装包之外要求的剩余空间（对齐文档组件）。
const DiskReserve int64 = 512 << 20 // 512 MiB

// PlaceholderVersion 是未发布前组件版本目录名占位。
const PlaceholderVersion = "0.0.0-tbd"

// PackageSpec 是某一档某一平台的安装包描述。URL / SHA256 为空表示尚未发布。
type PackageSpec struct {
	URL      string
	SHA256   string
	Size     int64 // 精确字节；0 时用档位引导体积
	Filename string
}

// Manifest 是内置占位清单。老板发版后只改这里的 URL / SHA256 / Size。
// TODO(lang-asr): 填入标准 / 高清各平台真实下载地址与校验值。
type Manifest struct {
	Version string
	// Platforms: key = "<tier>/<goos>-<goarch>"，例如 "standard/windows-amd64"。
	Platforms map[string]PackageSpec
}

// builtin 是当前内置占位（URL/SHA 均为空 → canDownload=false）。
var builtin = Manifest{
	Version: PlaceholderVersion,
	Platforms: map[string]PackageSpec{
		"standard/windows-amd64": {Size: DownloadBytesStandard, Filename: "asr-standard-windows-amd64.zip"},
		"standard/darwin-arm64":  {Size: DownloadBytesStandard, Filename: "asr-standard-darwin-arm64.zip"},
		"standard/darwin-amd64":  {Size: DownloadBytesStandard, Filename: "asr-standard-darwin-amd64.zip"},
		"standard/linux-amd64":   {Size: DownloadBytesStandard, Filename: "asr-standard-linux-amd64.zip"},
		"hd/windows-amd64":       {Size: DownloadBytesHD, Filename: "asr-hd-windows-amd64.zip"},
		"hd/darwin-arm64":        {Size: DownloadBytesHD, Filename: "asr-hd-darwin-arm64.zip"},
		"hd/darwin-amd64":        {Size: DownloadBytesHD, Filename: "asr-hd-darwin-amd64.zip"},
		"hd/linux-amd64":         {Size: DownloadBytesHD, Filename: "asr-hd-linux-amd64.zip"},
	},
}

// LoadManifest 返回内置清单（可被测试覆盖）。
func LoadManifest() Manifest { return builtin }

// PlatformKey 返回 "<goos>-<goarch>"。
func PlatformKey() string { return runtime.GOOS + "-" + runtime.GOARCH }

// SpecFor 返回档位 + 当前平台的包；没有条目时返回零值。
func (m Manifest) SpecFor(tier string) PackageSpec {
	if tier != TierHD {
		tier = TierStandard
	}
	return m.Platforms[tier+"/"+PlatformKey()]
}

// DownloadBytesFor 返回档位引导体积（界面「约 xxx MB」）。
func DownloadBytesFor(tier string) int64 {
	if tier == TierHD {
		return DownloadBytesHD
	}
	return DownloadBytesStandard
}

// InstallBytesFor 返回档位解包后约占用。
func InstallBytesFor(tier string) int64 {
	if tier == TierHD {
		return InstallBytesHD
	}
	return InstallBytesStandard
}

// Configured 表示该包已具备可下载的 URL 与 SHA-256。
func (p PackageSpec) Configured() bool {
	return p.URL != "" && p.SHA256 != ""
}

// EffectiveSize 返回精确 Size，否则档位引导体积。
func (p PackageSpec) EffectiveSize(tier string) int64 {
	if p.Size > 0 {
		return p.Size
	}
	return DownloadBytesFor(tier)
}
