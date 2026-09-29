package system

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// gpuEnumTimeout 是枚举显卡的每条命令的超时。
const gpuEnumTimeout = 8 * time.Second

// enumerateGPUs 按平台枚举显卡名称。任何失败都降级为空列表，不报错。
// 所有命令都经 env.run（ffmpeg.NewCommand → proc.Configure），Windows 上不会弹控制台窗口。
func (e encoderEnv) enumerateGPUs(ctx context.Context, goos string) []gpuInfo {
	switch goos {
	case "windows":
		// wmic 已废弃：用 PowerShell 的 Get-CimInstance。先把输出编码设成 UTF-8，避免非 ASCII 名字乱码。
		script := "[Console]::OutputEncoding=[System.Text.Encoding]::UTF8;" +
			"Get-CimInstance Win32_VideoController | Select-Object Name,PNPDeviceID | ConvertTo-Json -Compress"
		out, _, err := e.run(ctx, gpuEnumTimeout, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
		if err != nil && strings.TrimSpace(out) == "" {
			return nil
		}
		return parseWindowsVideoControllers(out)
	case "darwin":
		out, _, err := e.run(ctx, gpuEnumTimeout, "system_profiler", "SPDisplaysDataType", "-json")
		if err != nil && strings.TrimSpace(out) == "" {
			return nil
		}
		return parseSystemProfiler(out)
	default:
		if _, err := exec.LookPath("lspci"); err == nil || e.Run != nil {
			out, _, err := e.run(ctx, gpuEnumTimeout, "lspci", "-nn")
			if err == nil || strings.TrimSpace(out) != "" {
				if gpus := parseLspci(out); len(gpus) > 0 {
					return gpus
				}
			}
		}
		return drmGPUs("/sys/class/drm")
	}
}

// drmGPUs 读 /sys/class/drm/card*/device/{vendor,driver}，只有厂商名（lspci 不存在时的兜底）。目录不存在返回空。
func drmGPUs(root string) []gpuInfo {
	dirs, _ := filepath.Glob(filepath.Join(root, "card[0-9]*"))
	var gpus []gpuInfo
	for _, d := range dirs {
		if strings.Contains(filepath.Base(d), "-") { // card0-HDMI-A-1 是连接器，不是显卡
			continue
		}
		vb, err := os.ReadFile(filepath.Join(d, "device", "vendor"))
		if err != nil {
			continue
		}
		c := drmCard{Vendor: strings.TrimSpace(string(vb))}
		if link, err := os.Readlink(filepath.Join(d, "device", "driver")); err == nil {
			c.Driver = filepath.Base(link)
		}
		if g, ok := gpuFromDRM(c); ok {
			gpus = append(gpus, g)
		}
	}
	return dedupeGPUs(gpus)
}
