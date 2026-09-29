package ffmpeg

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

//go:embed manifest.json
var defaultManifestJSON []byte

// 镜像名。安装接口的 mirror 参数只接受空串（默认源）和这里列出的名字。
const (
	MirrorDefault = ""
	MirrorCN      = "cn"
)

// ValidMirror 判断 mirror 参数是否合法：只接受 "" 和 "cn"。
func ValidMirror(m string) bool { return m == MirrorDefault || m == MirrorCN }

// Manifest 是下载清单（契约 9.3）：按 "<goos>-<goarch>" 分平台。
type Manifest struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Note          string              `json:"note,omitempty"`
	Platforms     map[string]Platform `json:"platforms"`
}

// Platform 是一个平台的下载信息。Available 为 false 表示该平台没有经过验证的固定下载源，
// 此时 Archives 为空，Note 说明原因；安装接口返回 UNSUPPORTED_PLATFORM。
type Platform struct {
	Available bool      `json:"available"`
	Version   string    `json:"version"`
	Source    string    `json:"source"`
	Note      string    `json:"note,omitempty"`
	Archives  []Archive `json:"archives"`
}

// Archive 是一个需要下载的压缩包，以及要从中取出的文件。
type Archive struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Type   string `json:"type"` // zip | tar.xz
	// Extract 列出要取出的文件：Path 是压缩包内的完整路径，Name 是落地文件名（ffmpeg / ffmpeg.exe 等）。
	Extract []ExtractItem `json:"extract"`
	// Mirrors 是按镜像名分的备用地址，必须返回与 URL 完全相同的文件（同一 SHA256）。
	// 没有核实过的镜像不要写；该镜像名下没有条目时，按默认源下载。
	Mirrors map[string][]string `json:"mirrors,omitempty"`
}

// ExtractItem 见 Archive.Extract。
type ExtractItem struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// DefaultManifest 返回内置清单（internal/ffmpeg/manifest.json，go:embed）。
func DefaultManifest() (*Manifest, error) { return ParseManifest(defaultManifestJSON) }

// ParseManifest 解析并校验清单。
func ParseManifest(b []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("解析下载清单失败: %w", err)
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) validate() error {
	for key, p := range m.Platforms {
		if !p.Available {
			continue
		}
		if len(p.Archives) == 0 {
			return fmt.Errorf("平台 %s 标记可用但没有下载项", key)
		}
		for i, a := range p.Archives {
			switch {
			case !strings.HasPrefix(a.URL, "https://") && !strings.HasPrefix(a.URL, "http://"):
				return fmt.Errorf("平台 %s 第 %d 项 URL 无效", key, i)
			case len(a.SHA256) != 64:
				return fmt.Errorf("平台 %s 第 %d 项 SHA256 无效", key, i)
			case a.Type != "zip" && a.Type != "tar.xz":
				return fmt.Errorf("平台 %s 第 %d 项类型 %q 不支持", key, i, a.Type)
			case len(a.Extract) == 0:
				return fmt.Errorf("平台 %s 第 %d 项没有要提取的文件", key, i)
			}
		}
	}
	return nil
}

// PlatformKey 返回当前平台的清单键，如 windows-amd64。
func PlatformKey() string { return runtime.GOOS + "-" + runtime.GOARCH }

// Source 是一个压缩包的下载地址序列（按顺序尝试，同一份数据共用一个 .part 续传）。
type Source struct {
	Archive Archive
	URLs    []string
}

// Plan 是 Resolve 的结果。
type Plan struct {
	Platform string
	Version  string
	Sources  []Source
	// MirrorFallback 为 true 表示请求了镜像，但清单里没有可用的镜像条目，实际用默认源。
	MirrorFallback bool
}

// Resolve 选出平台条目并展开镜像。mirror 必须是 "" 或 "cn"。
// 镜像地址排在前面，默认地址始终作为最后的兜底。
func (m *Manifest) Resolve(platform, mirror string) (Plan, error) {
	if !ValidMirror(mirror) {
		return Plan{}, fmt.Errorf("不支持的镜像: %q", mirror)
	}
	p, ok := m.Platforms[platform]
	if !ok || !p.Available {
		return Plan{}, &UnavailableError{Platform: platform, Note: p.Note}
	}
	plan := Plan{Platform: platform, Version: p.Version}
	usedMirror := false
	for _, a := range p.Archives {
		var urls []string
		if mirror != MirrorDefault {
			urls = append(urls, a.Mirrors[mirror]...)
		}
		if len(urls) > 0 {
			usedMirror = true
		}
		urls = append(urls, a.URL)
		plan.Sources = append(plan.Sources, Source{Archive: a, URLs: urls})
	}
	plan.MirrorFallback = mirror != MirrorDefault && !usedMirror
	return plan, nil
}

// UnavailableError 表示当前平台没有可用的下载源。
type UnavailableError struct {
	Platform string
	Note     string
}

func (e *UnavailableError) Error() string {
	if e.Note != "" {
		return fmt.Sprintf("平台 %s 暂无可用的 ffmpeg 下载源: %s", e.Platform, e.Note)
	}
	return fmt.Sprintf("平台 %s 暂无可用的 ffmpeg 下载源", e.Platform)
}
