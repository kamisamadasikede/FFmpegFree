package system

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// 本文件全是纯函数：解析 ffmpeg -encoders、PowerShell / system_profiler / lspci 的输出，
// 以及试跑失败原因的归类。不碰系统，方便表驱动测试。

// gpuInfo 是枚举出来的一张显卡。
type gpuInfo struct {
	Name     string
	Vendor   string // nvidia | intel | amd | apple | unknown
	Discrete bool
}

var encoderLineRe = regexp.MustCompile(`^\s*V[A-Z.]{5}\s+(\S+)`)

// parseEncodersList 解析 `ffmpeg -hide_banner -encoders` 的输出，返回视频编码器名的集合。
func parseEncodersList(out string) map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if m := encoderLineRe.FindStringSubmatch(strings.TrimRight(line, "\r")); m != nil && m[1] != "=" { // 图例行 "V..... = Video"
			set[m[1]] = true
		}
	}
	return set
}

// hwEncoders 是各厂商的硬件编码器（h264 在前，hevc 在后）。
var hwEncoders = map[string][2]string{
	VendorNvidia: {"h264_nvenc", "hevc_nvenc"},
	VendorIntel:  {"h264_qsv", "hevc_qsv"},
	VendorAMD:    {"h264_amf", "hevc_amf"},
	VendorApple:  {"h264_videotoolbox", "hevc_videotoolbox"},
}

// vendorBrand 是读不到具体型号时给设备起的名字（会直接显示在界面上）：中文短名，
// 不含编码器名（NVENC / QSV / AMF / VideoToolbox）、驱动名（i915 / amdgpu / nvidia）和括号后缀。Apple 读不到型号统一叫“系统显卡”。
var vendorBrand = map[string]string{
	VendorNvidia: "NVIDIA 显卡", VendorIntel: "Intel 显卡", VendorAMD: "AMD 显卡", VendorApple: "系统显卡",
}

var (
	nvidiaRe = regexp.MustCompile(`(?i)nvidia|geforce|quadro|\brtx\b|\bgtx\b|tesla|titan`)
	amdRe    = regexp.MustCompile(`(?i)\bamd\b|radeon|advanced micro devices|\bati\b`)
	intelRe  = regexp.MustCompile(`(?i)\bintel\b|iris|\buhd graphics|\bhd graphics|\barc\b`)
	appleRe  = regexp.MustCompile(`(?i)\bapple\b`)
	// 虚拟 / 远程 / 软件渲染的显示适配器：不是真显卡，直接忽略。
	virtualGPURe = regexp.MustCompile(`(?i)microsoft basic|microsoft remote|hyper-v|vmware|virtualbox|vbox|parallels|qemu|virtio|red hat.*qxl|qxl|llvmpipe|swiftshader|svga|basic render|remote display|citrix|displaylink|indirect display|idd `)
	// AMD 集显（APU）：Radeon Graphics / Vega N / 780M 这类；RX / Pro 系列是独显。
	amdIntegratedRe = regexp.MustCompile(`(?i)radeon(\(tm\))?\s*(graphics|vega\s*\d*|\d{3,4}m\b)`)
	intelArcRe      = regexp.MustCompile(`(?i)\barc\b`)
)

// vendorFromText 从名字 / 厂商串里判断厂商，判断不了返回 unknown。
func vendorFromText(s string) string {
	switch {
	case nvidiaRe.MatchString(s):
		return VendorNvidia
	case appleRe.MatchString(s):
		return VendorApple
	case amdRe.MatchString(s):
		return VendorAMD
	case intelRe.MatchString(s):
		return VendorIntel
	}
	return VendorUnknown
}

// vendorFromPCIID 把 PCI 厂商号（0x10de / 10DE / VEN_10DE）转成厂商。
func vendorFromPCIID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.TrimPrefix(strings.TrimPrefix(id, "ven_"), "0x")
	switch id {
	case "10de":
		return VendorNvidia
	case "8086":
		return VendorIntel
	case "1002", "1022":
		return VendorAMD
	}
	return VendorUnknown
}

// classifyGPU 由名字（和已知厂商）生成 gpuInfo，并判断独显 / 集显。虚拟适配器返回 ok=false。
func classifyGPU(name, vendorHint string) (gpuInfo, bool) {
	name = strings.TrimSpace(name)
	if name == "" || virtualGPURe.MatchString(name) {
		return gpuInfo{}, false
	}
	v := vendorHint
	if v == "" || v == VendorUnknown {
		v = vendorFromText(name)
	}
	g := gpuInfo{Name: name, Vendor: v}
	switch v {
	case VendorNvidia:
		g.Discrete = true
	case VendorAMD:
		g.Discrete = !amdIntegratedRe.MatchString(name)
	case VendorIntel:
		g.Discrete = intelArcRe.MatchString(name) // Arc 是独显，UHD / Iris / HD 是集显
	}
	return g, true
}

// dedupeGPUs 按 厂商+名字 去重，保持出现顺序。
func dedupeGPUs(in []gpuInfo) []gpuInfo {
	seen := map[string]int{}
	var out []gpuInfo
	for _, g := range in {
		k := g.Vendor + "\x00" + g.Name
		seen[k]++
		if seen[k] == 1 {
			out = append(out, g)
		}
	}
	return out
}

// parseWindowsVideoControllers 解析
//
//	Get-CimInstance Win32_VideoController | Select-Object Name,PNPDeviceID | ConvertTo-Json -Compress
//
// 的输出：只有一张卡时是对象，多张是数组。无法解析返回 nil。
func parseWindowsVideoControllers(out string) []gpuInfo {
	out = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "\ufeff"))
	if out == "" {
		return nil
	}
	type item struct {
		Name        string `json:"Name"`
		PNPDeviceID string `json:"PNPDeviceID"`
	}
	var items []item
	if strings.HasPrefix(out, "[") {
		if json.Unmarshal([]byte(out), &items) != nil {
			return nil
		}
	} else {
		var one item
		if json.Unmarshal([]byte(out), &one) != nil {
			return nil
		}
		items = []item{one}
	}
	var gpus []gpuInfo
	for _, it := range items {
		hint := VendorUnknown
		if m := pnpVenRe.FindStringSubmatch(it.PNPDeviceID); m != nil {
			hint = vendorFromPCIID(m[1])
		}
		if g, ok := classifyGPU(it.Name, hint); ok {
			gpus = append(gpus, g)
		}
	}
	return dedupeGPUs(gpus)
}

var pnpVenRe = regexp.MustCompile(`(?i)VEN_([0-9a-f]{4})`)

// parseSystemProfiler 解析 `system_profiler SPDisplaysDataType -json` 的输出。
func parseSystemProfiler(out string) []gpuInfo {
	var doc struct {
		Items []struct {
			Model  string `json:"sppci_model"`
			Name   string `json:"_name"`
			Vendor string `json:"spdisplays_vendor"`
		} `json:"SPDisplaysDataType"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &doc) != nil {
		return nil
	}
	var gpus []gpuInfo
	for _, it := range doc.Items {
		name := firstNonEmpty(it.Model, it.Name)
		hint := vendorFromText(strings.ReplaceAll(it.Vendor, "sppci_vendor_", ""))
		if hint == VendorUnknown {
			hint = vendorFromText(name)
		}
		if g, ok := classifyGPU(name, hint); ok {
			gpus = append(gpus, g)
		}
	}
	return dedupeGPUs(gpus)
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

var (
	lspciLineRe   = regexp.MustCompile(`(?i)^\S+\s+(?:VGA compatible controller|3D controller|Display controller)\s*(?:\[[0-9a-f]{4}\])?:\s*(.+)$`)
	lspciRevRe    = regexp.MustCompile(`\s*\(rev [0-9a-fA-F]+\)\s*$`)
	lspciIDRe     = regexp.MustCompile(`\s*\[[0-9a-fA-F]{4}:[0-9a-fA-F]{4}\]\s*`)
	lspciBracketR = regexp.MustCompile(`\[([^\[\]]+)\]\s*$`)
)

// parseLspci 解析 `lspci`（可带 -nn）的输出：取 VGA / 3D / Display 控制器行。
// 名字优先用方括号里的营销名（"GA106 [GeForce RTX 3060]" → "GeForce RTX 3060"），厂商按整行文字判断。
func parseLspci(out string) []gpuInfo {
	var gpus []gpuInfo
	for _, line := range strings.Split(out, "\n") {
		m := lspciLineRe.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		desc := lspciRevRe.ReplaceAllString(m[1], "")
		desc = strings.TrimSpace(lspciIDRe.ReplaceAllString(desc, " "))
		hint := vendorFromText(desc)
		name := desc
		if b := lspciBracketR.FindStringSubmatch(desc); b != nil {
			name = strings.TrimSpace(b[1])
			// 方括号里只有型号（"GeForce RTX 3060"）时补上厂商前缀，其余原样。
			if hint == VendorNvidia && !strings.Contains(strings.ToLower(name), "nvidia") {
				name = "NVIDIA " + name
			}
			if hint == VendorAMD && !strings.Contains(strings.ToLower(name), "amd") && !strings.Contains(strings.ToLower(name), "radeon") {
				name = "AMD " + name
			}
		}
		if g, ok := classifyGPU(name, hint); ok {
			gpus = append(gpus, g)
		}
	}
	return dedupeGPUs(gpus)
}

// drmCard 是 /sys/class/drm/card*/device 里读到的两项。
type drmCard struct {
	Vendor string // 如 0x10de
	Driver string // 如 nvidia / i915 / amdgpu，可能为空；只供内部判断，不进设备名
}

// gpuFromDRM 在没有 lspci 时用 sysfs 的厂商号造一个只有厂商名的显卡（名字是 vendorBrand 的中文短名，不拼驱动名）。
func gpuFromDRM(c drmCard) (gpuInfo, bool) {
	v := vendorFromPCIID(c.Vendor)
	if v == VendorUnknown {
		return gpuInfo{}, false
	}
	g := gpuInfo{Name: vendorBrand[v], Vendor: v, Discrete: v == VendorNvidia}
	return g, true
}

// 设备 reason 文案统一叫“显卡编码”，不带编码器名（NVENC / QSV / AMF / VideoToolbox）；契约 v0.22。
const (
	reasonNoEncoderInFFmpeg = "当前转换组件不包含这张显卡对应的显卡编码支持"
	reasonNoGPUEncoder      = "没有可用的显卡编码器"
	reasonUnsupportedGPU    = "这张显卡没有对应的显卡编码支持"
)

// classifyProbeError 把一次试跑失败归类成给人看的原因。timedOut 表示是超时。
func classifyProbeError(vendor, errText string, timedOut bool, timeoutSec int) string {
	lower := strings.ToLower(errText)
	switch {
	case timedOut:
		return "试跑超时（超过 " + strconv.Itoa(timeoutSec) + " 秒），驱动可能没有响应"
	case strings.Contains(lower, "unknown encoder") || strings.Contains(lower, "encoder not found"):
		return reasonNoEncoderInFFmpeg
	case strings.Contains(lower, "no nvenc capable devices") || strings.Contains(lower, "cannot load libcuda") ||
		strings.Contains(lower, "cannot load nvcuda") || strings.Contains(lower, "driver does not support the required nvenc api") ||
		strings.Contains(lower, "cuda_error_no_device") || strings.Contains(lower, "no cuda-capable device"):
		return "没有可用的 NVIDIA 显卡，或显卡驱动缺失 / 版本过低"
	case strings.Contains(lower, "qsv") && (strings.Contains(lower, "device") || strings.Contains(lower, "session") || strings.Contains(lower, "unsupported")):
		return "没有可用的 Intel 核显 / 显卡，或驱动缺失（显卡编码初始化失败）"
	case strings.Contains(lower, "amf") && (strings.Contains(lower, "fail") || strings.Contains(lower, "not found") || strings.Contains(lower, "dll")):
		return "没有可用的 AMD 显卡，或显卡驱动缺失 / 版本过低（显卡编码初始化失败）"
	case strings.Contains(lower, "videotoolbox"):
		return "系统没有响应显卡编码"
	}
	return "试跑失败：" + lastLine(errText, 160)
}

// lastLine 取文本最后一个非空行，超过 max 个字符时截断。
func lastLine(s string, max int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	l := strings.TrimSpace(lines[len(lines)-1])
	if r := []rune(l); len(r) > max {
		l = string(r[:max]) + "…"
	}
	if l == "" {
		return "（无输出）"
	}
	return l
}
