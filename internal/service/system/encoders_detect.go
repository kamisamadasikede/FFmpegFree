package system

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// probeResult 是一个编码器的试跑结果。
type probeResult struct {
	ok       bool
	timedOut bool
	reason   string
}

// vendorsForOS 返回该平台上要检测的硬件编码厂商（按展示顺序）。
func vendorsForOS(goos string) []string {
	switch goos {
	case "windows":
		return []string{VendorNvidia, VendorAMD, VendorIntel}
	case "darwin":
		return []string{VendorApple}
	default: // linux 及其他：nvenc / qsv / amf 都可能存在（vaapi 暂不做）
		return []string{VendorNvidia, VendorAMD, VendorIntel}
	}
}

// probeArgs 是试跑一帧的参数：黑色 256×256 的 0.1 秒，只编 1 帧，输出到 null。
func probeArgs(encoder string) []string {
	return []string{"-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "lavfi", "-i", "color=c=black:s=256x256:d=0.1",
		"-frames:v", "1", "-c:v", encoder, "-f", "null", "-"}
}

// detectEncoderDevices 检测本机的编码设备，第一项永远是 CPU，不返回错误：
// 任何一步失败都降级（缺哪一块就少哪一块）。cacheable=false 表示结果不该缓存
// （-encoders 没跑成、被取消、或有编码器试跑超时——可能只是驱动一时没醒）。
func detectEncoderDevices(ctx context.Context, env encoderEnv, ffmpegPath string) (devices []EncoderDevice, cacheable bool) {
	devices = []EncoderDevice{cpuDevice()}
	goos := env.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}

	out, _, err := env.run(ctx, 15*time.Second, ffmpegPath, "-hide_banner", "-encoders")
	if err != nil && strings.TrimSpace(out) == "" {
		return devices, false
	}
	have := parseEncodersList(out)

	var gpus []gpuInfo
	if env.GPUs != nil {
		gpus = env.GPUs(ctx)
	} else {
		gpus = env.enumerateGPUs(ctx, goos)
	}
	gpus = dedupeGPUs(gpus)

	// 要试跑的编码器：该平台的每个厂商里，ffmpeg 编出来了的那几个；
	// 显卡枚举成功（非空）时只试有对应显卡的厂商，枚举不出来时全试（试跑才是真相）。
	vendors := vendorsForOS(goos)
	hasGPUOf := map[string]bool{}
	for _, g := range gpus {
		hasGPUOf[g.Vendor] = true
	}
	enumerated := len(gpus) > 0
	var todo []string
	for _, v := range vendors {
		if enumerated && !hasGPUOf[v] {
			continue
		}
		for _, enc := range hwEncoders[v] {
			if have[enc] {
				todo = append(todo, enc)
			}
		}
	}
	res := probeAll(ctx, env, ffmpegPath, todo)
	if ctx.Err() != nil {
		return devices, false
	}
	cacheable = true
	for _, r := range res {
		if r.timedOut {
			cacheable = false
		}
	}

	// 组装设备：每个厂商一组。
	timeoutSec := int(env.probeTimeout() / time.Second)
	for _, v := range vendors {
		names := hwEncoders[v]
		enc := EncoderNames{}
		var firstFail string
		ok := false
		for i, name := range names {
			r, tried := res[name]
			switch {
			case !have[name]:
				// 不含这个编码器
			case !tried:
				// 该厂商没有对应显卡，没试跑
			case r.ok:
				ok = true
				if i == 0 {
					enc.H264 = name
				} else {
					enc.HEVC = name
				}
			case firstFail == "":
				firstFail = classifyProbeError(v, r.reason, r.timedOut, timeoutSec)
			}
		}
		vgpus := gpusOfVendor(gpus, v)
		if goos == "darwin" && v == VendorApple {
			// macOS 的硬件编码统一由 VideoToolbox 调度，只列 Apple 设备；没枚举到就给一个通用项（名字“系统显卡”，不出现编码器名）。
			if len(vgpus) == 0 && ok {
				vgpus = []gpuInfo{{Name: vendorBrand[VendorApple], Vendor: VendorApple}}
			}
		} else if len(vgpus) == 0 && ok {
			// 试跑成功但没枚举到名字（lspci 缺失等）：给一个只有厂商名的项。
			vgpus = []gpuInfo{{Name: vendorBrand[v], Vendor: v, Discrete: v == VendorNvidia}}
		}
		for i, g := range vgpus {
			d := EncoderDevice{ID: v + "-" + strconv.Itoa(i), Name: g.Name, Vendor: v, Kind: KindGPU, Discrete: g.Discrete}
			if ok {
				d.Encoders, d.Available = enc, true
			} else {
				d.Reason = unavailableReason(v, have, names, firstFail)
			}
			devices = append(devices, d)
		}
	}
	// 其他厂商 / 未知厂商的显卡：列出来，标明不支持硬件编码。
	if goos != "darwin" {
		i := map[string]int{}
		for _, g := range gpus {
			if _, supported := hwEncoders[g.Vendor]; supported {
				continue
			}
			v := g.Vendor
			devices = append(devices, EncoderDevice{
				ID: v + "-" + strconv.Itoa(i[v]), Name: g.Name, Vendor: v, Kind: KindGPU, Discrete: g.Discrete,
				Reason: "这张显卡没有对应的硬件编码器支持",
			})
			i[v]++
		}
	}
	return devices, cacheable
}

func gpusOfVendor(gpus []gpuInfo, v string) []gpuInfo {
	var out []gpuInfo
	for _, g := range gpus {
		if g.Vendor == v {
			out = append(out, g)
		}
	}
	return out
}

// unavailableReason 给一个试跑没成功的厂商生成原因。
func unavailableReason(vendor string, have map[string]bool, names [2]string, firstFail string) string {
	if firstFail != "" {
		return firstFail
	}
	if !have[names[0]] && !have[names[1]] {
		return "当前 ffmpeg 不包含 " + hwLabel[vendor] + " 编码器"
	}
	return "没有可用的硬件编码器"
}

// probeAll 并发试跑 encoders（并发数受限），返回 编码器名 → 结果。
func probeAll(ctx context.Context, env encoderEnv, exe string, encoders []string) map[string]probeResult {
	res := make(map[string]probeResult, len(encoders))
	if len(encoders) == 0 {
		return res
	}
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, env.parallel())
	)
	for _, enc := range encoders {
		wg.Add(1)
		go func(enc string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			r := probeOne(ctx, env, exe, enc)
			mu.Lock()
			res[enc] = r
			mu.Unlock()
		}(enc)
	}
	wg.Wait()
	return res
}

func probeOne(ctx context.Context, env encoderEnv, exe, encoder string) probeResult {
	_, timedOut, err := env.run(ctx, env.probeTimeout(), exe, probeArgs(encoder)...)
	if err == nil {
		return probeResult{ok: true}
	}
	return probeResult{timedOut: timedOut, reason: err.Error()}
}
