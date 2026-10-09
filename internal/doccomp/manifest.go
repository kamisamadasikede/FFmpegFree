// Package doccomp 管理文档组件（LibreOffice，headless 运行）的检测、下载、准备和单次转换（契约 v0.26，6.12.11~6.12.12）。
//
// 界面上它只叫“文档组件”：本包返回的 message 不出现组件的真实名字，
// 唯一例外是 Linux 上未就绪时的 LinuxHint（契约 6.12.11）。
package doccomp

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"
)

//go:embed manifest.json
var manifestJSON []byte

// 镜像名：InstallDocComponent 的 mirror 只接受这两个。
const (
	MirrorDefault = ""
	MirrorCN      = "cn"
)

// InstallBytesApprox 是解包后大约占用的磁盘空间（1.5 GiB，按 MSI File 表统计；契约 v0.26.1 installBytes）。
const InstallBytesApprox int64 = 1610612736

// DiskReserve 是下载前在安装包之外要求的剩余空间（解包用，契约 6.12.12 第 1 步）。
const DiskReserve int64 = 2 << 30

// LinuxHint 是 Linux 上文档组件未就绪时唯一允许出现组件真实名字的提示（契约 6.12.11）。
const LinuxHint = "请先在系统里安装 LibreOffice，然后重启应用。"

// LinuxOutdatedHint 是 Linux 上系统安装版本太旧（state=outdated）时的提示（契约 v0.27，6.12.24）。
const LinuxOutdatedHint = "系统里的 LibreOffice 版本太旧，请升级到 7.2 或更高版本，然后重启应用。"

// MinMajor / MinMinor 是系统安装要求的最低版本（7.2，CSV 导出选工作表从 7.2 起有）。
const (
	MinMajor = 7
	MinMinor = 2
)

// DownloadMin* 是应用下载的组件要求的最低版本前三段（契约 6.12.55：26.2.6）。
const (
	DownloadMinMajor = 26
	DownloadMinMinor = 2
	DownloadMinPatch = 6
)

// OutdatedDownloadHint 是 Windows / macOS 上应用下载的组件版本太旧时的提示（契约 6.12.55）。
const OutdatedDownloadHint = "文档组件版本太旧，请重新下载。"

// Manifest 是 manifest.json。
type Manifest struct {
	SchemaVersion int                `json:"schemaVersion"`
	Note          string             `json:"note,omitempty"`
	Version       string             `json:"version"`
	Build         string             `json:"build"`
	Platforms     map[string]Package `json:"platforms"`
}

// Package 是一个平台的安装包。
type Package struct {
	Type    string              `json:"type"` // msi | dmg
	Size    int64               `json:"size"`
	SHA256  string              `json:"sha256"`
	URLs    []string            `json:"urls"`              // 按顺序：主地址（重定向器）、官方归档
	Mirrors map[string][]string `json:"mirrors,omitempty"` // 选了镜像时先试镜像，再试 URLs
}

// LoadManifest 解析内置清单。
func LoadManifest() (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		return Manifest{}, fmt.Errorf("解析文档组件清单失败: %w", err)
	}
	return m, nil
}

// PlatformKey 返回 "<goos>-<goarch>"。
func PlatformKey() string { return runtime.GOOS + "-" + runtime.GOARCH }

// urlsFor 按镜像返回尝试顺序：镜像地址在前，然后是默认地址（镜像不可用时退回）。
func (p Package) urlsFor(mirror string) []string {
	var out []string
	if mirror != MirrorDefault {
		out = append(out, p.Mirrors[mirror]...)
	}
	return append(out, p.URLs...)
}
