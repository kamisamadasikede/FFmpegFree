package system

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
)

// 硬件编码器检测（契约 9.6，v0.14）：本文件是类型、偏好和 ResolveEncoder；
// 检测流程在 encoders_detect.go，显卡名称枚举在 encoders_gpu.go，纯解析函数在 encoders_parse.go。

// SettingEncoderPreference 是 settings 表里保存编码器偏好的键。
const SettingEncoderPreference = "encoderPreference"

// SettingEncoderPreferenceName 记住偏好指向的设备名：显卡拔掉 / 驱动坏了之后，设置页仍能显示"选的是哪张卡"。
const SettingEncoderPreferenceName = "encoderPreferenceName"

// 偏好的两个固定值；其余取值必须是 ListEncoderDevices 返回的设备 id。
const (
	EncoderAuto = "auto"
	EncoderCPU  = "cpu"
)

// 设备的厂商与类型。
const (
	VendorNvidia  = "nvidia"
	VendorIntel   = "intel"
	VendorAMD     = "amd"
	VendorApple   = "apple"
	VendorUnknown = "unknown"

	KindGPU = "gpu"
	KindCPU = "cpu"
)

// 软件编码器。
const (
	softH264 = "libx264"
	softHEVC = "libx265"
)

// EncoderNames 是某个设备上 h264 / hevc 对应的 ffmpeg 编码器名，空串表示该设备不能编这种格式。
type EncoderNames struct {
	H264 string `json:"h264"`
	HEVC string `json:"hevc"`
}

// EncoderDevice 是一个可选的编码设备（CPU 或一张显卡）。
type EncoderDevice struct {
	ID     string `json:"id"`     // "cpu"，或 "<vendor>-<序号>"，如 nvidia-0；同一次检测内稳定
	Name   string `json:"name"`   // 给人看的名字
	Vendor string `json:"vendor"` // nvidia | intel | amd | apple | unknown
	Kind   string `json:"kind"`   // gpu | cpu
	// Discrete 表示独立显卡；auto 选择时独显优先于集显。CPU 恒为 false。
	Discrete bool         `json:"discrete"`
	Encoders EncoderNames `json:"encoders"`
	// Available 为 true 表示试跑成功，可以用 Encoders 里非空的编码器；
	// 为 false 时 Reason 说明原因（ffmpeg 不含该编码器、驱动缺失、试跑超时、所选设备已不存在……）。
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// EncoderDeviceList 是 ListEncoderDevices 的返回值。Devices 第一项永远是 CPU。
type EncoderDeviceList struct {
	// FFmpegReady 为 false 表示 ffmpeg 还没就绪，没有做任何检测，Devices 只有 CPU；ffmpeg 就绪后再调用即可。
	FFmpegReady bool            `json:"ffmpegReady"`
	Devices     []EncoderDevice `json:"devices"`
}

// cpuDevice 返回 CPU（软件编码）设备。
func cpuDevice() EncoderDevice {
	return EncoderDevice{
		ID: EncoderCPU, Name: "CPU（软件编码）", Vendor: VendorUnknown, Kind: KindCPU,
		Encoders: EncoderNames{H264: softH264, HEVC: softHEVC}, Available: true,
	}
}

// encoderFor 返回设备上 codec（h264 | hevc，也接受 h265）对应的编码器名；不认识的 codec 返回 ""。
func (d EncoderDevice) encoderFor(codec string) string {
	switch normalizeCodec(codec) {
	case "h264":
		return d.Encoders.H264
	case "hevc":
		return d.Encoders.HEVC
	}
	return ""
}

func normalizeCodec(c string) string {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "h264", "avc", "libx264":
		return "h264"
	case "hevc", "h265", "libx265":
		return "hevc"
	}
	return ""
}

// vendorOrder 决定同类显卡之间的先后：nvidia、amd 在前。
var vendorOrder = map[string]int{VendorNvidia: 0, VendorAMD: 1, VendorIntel: 2, VendorApple: 3, VendorUnknown: 4}

// gpuRank 越小越优先：独显（nvidia / amd / intel Arc）在前，集显在后。
func gpuRank(d EncoderDevice) int {
	r := 100
	if d.Discrete {
		r = 0
	}
	v, ok := vendorOrder[d.Vendor]
	if !ok {
		v = len(vendorOrder)
	}
	return r + v
}

// ResolveEncoder 把偏好解析成具体的编码器（纯函数，不碰系统）：
//
//   - pref 为 "auto"（或空）：在 devices 里挑 available 且有 codec 对应编码器的显卡，独显优先于集显，
//     nvidia / amd 独显在前，同级保持 devices 里的顺序；没有则用 CPU。auto 落到 CPU 不算回退（fallback=false）。
//   - pref 为 "cpu"：用 CPU，fallback=false。
//   - pref 为设备 id：该设备存在、available 且有 codec 对应编码器就用它；否则回退 CPU，fallback=true。
//
// codec 是 "h264" 或 "hevc"（也接受 "h265"）；其他值返回 ("", "", false)。
// 返回 (编码器名, 设备 id, 是否发生回退)。devices 里没有 CPU 项时用 libx264 / libx265。
func ResolveEncoder(pref string, devices []EncoderDevice, codec string) (encoderName, deviceID string, fallback bool) {
	c := normalizeCodec(codec)
	if c == "" {
		return "", "", false
	}
	cpu := cpuDevice()
	for _, d := range devices {
		if d.Kind == KindCPU && d.ID == EncoderCPU {
			cpu = d
			break
		}
	}
	useCPU := func(fb bool) (string, string, bool) { return cpu.encoderFor(c), EncoderCPU, fb }
	usable := func(d EncoderDevice) bool { return d.Kind == KindGPU && d.Available && d.encoderFor(c) != "" }

	switch pref = strings.TrimSpace(pref); pref {
	case "", EncoderAuto:
		var gpus []EncoderDevice
		for _, d := range devices {
			if usable(d) {
				gpus = append(gpus, d)
			}
		}
		sort.SliceStable(gpus, func(i, j int) bool { return gpuRank(gpus[i]) < gpuRank(gpus[j]) })
		if len(gpus) > 0 {
			return gpus[0].encoderFor(c), gpus[0].ID, false
		}
		return useCPU(false)
	case EncoderCPU:
		return useCPU(false)
	}
	for _, d := range devices {
		if d.ID == pref && usable(d) {
			return d.encoderFor(c), d.ID, false
		}
	}
	return useCPU(true)
}

// ---------- 偏好（settings 表，键 encoderPreference） ----------

// validEncoderID 是设备 id 的语法：小写字母数字开头，只含小写字母、数字、_ 和 -。
var validEncoderID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// GetEncoderPreference 返回保存的编码器偏好："auto" | "cpu" | 设备 id，默认 "auto"。
// 保存的设备现在不存在或不可用时仍返回原值（不改写、不报错），ListEncoderDevices 会把它标成 available=false 并说明原因。
func (m *Manager) GetEncoderPreference(ctx context.Context) string {
	m.mu.Lock()
	st := m.cfg.Settings
	m.mu.Unlock()
	var v string
	if st == nil {
		m.memMu.Lock()
		v = m.memEnc
		m.memMu.Unlock()
	} else if _, err := st.GetSetting(ctx, SettingEncoderPreference, &v); err != nil {
		v = ""
	}
	if v = strings.TrimSpace(v); v == "" {
		return EncoderAuto
	}
	return v
}

// SetEncoderPreference 保存编码器偏好。取值必须是 "auto"、"cpu"，或当前 ListEncoderDevices 里存在的设备 id
// （已存在但暂时 available=false 的设备也允许保存）；其他值返回 INVALID_ARGUMENT 且不保存。
// 校验设备 id 会用到检测结果：有缓存直接用，没有缓存时先检测一次（每个编码器最多 5 秒）。
func (m *Manager) SetEncoderPreference(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	name := ""
	if id != EncoderAuto && id != EncoderCPU {
		if !validEncoderID.MatchString(id) {
			return apperr.New(apperr.InvalidArgument, "编码器偏好必须是 auto、cpu 或设备 id").WithDetail(id)
		}
		list, err := m.listEncoderDevices(ctx, false, false)
		if err != nil {
			return err
		}
		found := false
		for _, d := range list.Devices {
			if d.ID == id {
				found, name = true, d.Name
				break
			}
		}
		if !found {
			return apperr.New(apperr.InvalidArgument, "没有这个编码设备").WithDetail(id)
		}
	}
	m.mu.Lock()
	st := m.cfg.Settings
	m.mu.Unlock()
	if st == nil {
		m.memMu.Lock()
		m.memEnc, m.memEncName = id, name
		m.memMu.Unlock()
		return nil
	}
	if err := st.SetSetting(ctx, SettingEncoderPreference, id); err != nil {
		return apperr.Wrap(apperr.IOError, "保存设置失败", err)
	}
	if err := st.SetSetting(ctx, SettingEncoderPreferenceName, name); err != nil {
		return apperr.Wrap(apperr.IOError, "保存设置失败", err)
	}
	return nil
}

// EncoderPreferenceInfo 是当前偏好的可显示形式：设置页据此显示"自动 / CPU / 具体显卡名"。
type EncoderPreferenceInfo struct {
	ID string `json:"id"` // auto | cpu | 设备 id（与 GetEncoderPreference 一致）
	// Name 是给人看的名字："自动"、"CPU（软件编码）"，或设备名。设备已不在列表里时用保存偏好时记下的名字；从没记过则为空。
	Name string `json:"name"`
	// Available 为 false 表示偏好指向的设备现在不可用（不存在 / 试跑失败），实际编码会回退 CPU。auto 和 cpu 恒为 true。
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// GetEncoderPreferenceInfo 返回偏好的 id、显示名和当前是否可用。ffmpeg 未就绪时没法判断可用性，
// 设备偏好按 available=false、reason 说明处理，名字用保存的那个。
func (m *Manager) GetEncoderPreferenceInfo(ctx context.Context) (EncoderPreferenceInfo, error) {
	pref := m.GetEncoderPreference(ctx)
	switch pref {
	case EncoderAuto:
		return EncoderPreferenceInfo{ID: pref, Name: "自动", Available: true}, nil
	case EncoderCPU:
		return EncoderPreferenceInfo{ID: pref, Name: cpuDevice().Name, Available: true}, nil
	}
	info := EncoderPreferenceInfo{ID: pref, Name: m.savedEncoderName(ctx)}
	list, err := m.listEncoderDevices(ctx, false, false)
	if err != nil {
		return EncoderPreferenceInfo{}, err
	}
	if !list.FFmpegReady {
		info.Reason = "转换组件还没有就绪，暂时无法确认这个设备"
		return info, nil
	}
	for _, d := range list.Devices {
		if d.ID == pref {
			info.Name, info.Available, info.Reason = d.Name, d.Available, d.Reason
			return info, nil
		}
	}
	info.Reason = missingDeviceReason
	return info, nil
}

const missingDeviceReason = "没有检测到这个设备（可能已拔掉、驱动变化，或转换组件已更换）"

func (m *Manager) savedEncoderName(ctx context.Context) string {
	m.mu.Lock()
	st := m.cfg.Settings
	m.mu.Unlock()
	if st == nil {
		m.memMu.Lock()
		defer m.memMu.Unlock()
		return m.memEncName
	}
	var n string
	if _, err := st.GetSetting(ctx, SettingEncoderPreferenceName, &n); err != nil {
		return ""
	}
	return n
}

// ---------- ListEncoderDevices（带缓存） ----------

// encoderCache 是一次检测的结果。
type encoderCache struct {
	key     string // ffmpeg 路径 + 版本
	gen     uint64 // 检测开始时的失效代数
	devices []EncoderDevice
}

// encoderState 是 Manager 里和编码器检测有关的状态。
type encoderState struct {
	detectMu sync.Mutex // 串行化检测：并发调用只跑一次，后来的直接吃缓存
	cacheMu  sync.Mutex
	cache    *encoderCache
	gen      uint64 // 失效代数，受 cacheMu 保护
	env      encoderEnv
}

// invalidateEncoders 让缓存失效。只拿 cacheMu，不会被正在进行的检测挡住（setIf 在持有 m.mu 时调用它）。
func (m *Manager) invalidateEncoders() {
	m.enc.cacheMu.Lock()
	m.enc.gen++
	m.enc.cache = nil
	m.enc.cacheMu.Unlock()
}

// ListEncoderDevices 返回可用的编码设备（第一项永远是 CPU）。结果按 ffmpeg 路径 + 版本缓存；
// ffmpeg 重新变为 ready（安装完成、手动指定、重新检测）时缓存失效。
// ffmpeg 未就绪时不检测，返回只有 CPU 的列表且 FFmpegReady=false，不报错。没有显卡的机器同样只返回 CPU。
// 当前偏好指向不存在的设备时，列表末尾会追加一项 available=false 的占位（带原因），偏好本身不改。
func (m *Manager) ListEncoderDevices(ctx context.Context) (EncoderDeviceList, error) {
	return m.listEncoderDevices(ctx, false, true)
}

// RefreshEncoderDevices 丢弃缓存并重新检测，用于用户装了驱动或换了显卡后手动刷新。
func (m *Manager) RefreshEncoderDevices(ctx context.Context) (EncoderDeviceList, error) {
	return m.listEncoderDevices(ctx, true, true)
}

func (m *Manager) listEncoderDevices(ctx context.Context, refresh, withPref bool) (EncoderDeviceList, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	st := m.Status()
	if st.State != ffmpeg.StateReady || st.Path == "" {
		return EncoderDeviceList{FFmpegReady: false, Devices: m.applyEncoderPref(ctx, []EncoderDevice{cpuDevice()}, withPref)}, nil
	}
	key := st.Path + "\x00" + st.Version

	m.enc.detectMu.Lock()
	defer m.enc.detectMu.Unlock()

	m.enc.cacheMu.Lock()
	gen := m.enc.gen
	c := m.enc.cache
	m.enc.cacheMu.Unlock()
	if !refresh && c != nil && c.key == key && c.gen == gen {
		return EncoderDeviceList{FFmpegReady: true, Devices: m.applyEncoderPref(ctx, c.devices, withPref)}, nil
	}

	devices, cacheable := detectEncoderDevices(ctx, m.enc.env, st.Path)
	if err := ctx.Err(); err != nil {
		return EncoderDeviceList{}, apperr.Wrap(apperr.Canceled, "检测编码器被中断", err)
	}
	if cacheable {
		m.enc.cacheMu.Lock()
		if m.enc.gen == gen { // 检测期间没有被失效才写入
			m.enc.cache = &encoderCache{key: key, gen: gen, devices: devices}
		}
		m.enc.cacheMu.Unlock()
	}
	return EncoderDeviceList{FFmpegReady: true, Devices: m.applyEncoderPref(ctx, devices, withPref)}, nil
}

// applyEncoderPref 返回 devices 的副本；偏好指向的设备不在列表里时追加一个不可用的占位项。
func (m *Manager) applyEncoderPref(ctx context.Context, devices []EncoderDevice, withPref bool) []EncoderDevice {
	out := append([]EncoderDevice(nil), devices...)
	if !withPref {
		return out
	}
	pref := m.GetEncoderPreference(ctx)
	if pref == EncoderAuto || pref == EncoderCPU {
		return out
	}
	for _, d := range out {
		if d.ID == pref {
			return out
		}
	}
	name := m.savedEncoderName(ctx)
	if name == "" {
		name = pref
	}
	return append(out, EncoderDevice{
		ID: pref, Name: name, Vendor: vendorFromID(pref), Kind: KindGPU,
		Available: false, Reason: missingDeviceReason,
	})
}

// encoderProbeTimeout 是每个硬件编码器试跑一帧的超时。
const encoderProbeTimeout = 5 * time.Second

// encoderProbeParallel 是同时试跑的编码器数量上限。
const encoderProbeParallel = 2

// encoderEnv 是检测依赖的外部能力，测试里替换；零值用真实实现。
type encoderEnv struct {
	// Run 运行 ffmpeg / 系统工具并返回 stdout；nil 时用 ffmpeg.ExecRunner（隐藏控制台窗口、带超时）。
	// 传入的 ctx 已经带了该次调用的超时。
	Run ffmpeg.Runner
	// GPUs 枚举显卡；nil 时按平台用真实实现，失败一律降级为空。
	GPUs         func(ctx context.Context) []gpuInfo
	GOOS         string        // 空 = runtime.GOOS
	ProbeTimeout time.Duration // 单个编码器试跑超时，0 = 5 秒
	Parallel     int           // 试跑并发上限，0 = 2
}

func (e encoderEnv) probeTimeout() time.Duration {
	if e.ProbeTimeout > 0 {
		return e.ProbeTimeout
	}
	return encoderProbeTimeout
}

func (e encoderEnv) parallel() int {
	if e.Parallel > 0 {
		return e.Parallel
	}
	return encoderProbeParallel
}

// run 在 timeout 内运行 exe，返回 stdout、是否因超时结束、错误。父 ctx 被取消不算超时。
func (e encoderEnv) run(ctx context.Context, timeout time.Duration, exe string, args ...string) (out string, timedOut bool, err error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if e.Run != nil {
		out, err = e.Run(cctx, exe, args...)
	} else {
		// 真实执行器自带的超时比 cctx 多留 2 秒，让 cctx 先到期，超时判断只看 cctx。
		out, err = ffmpeg.ExecRunner(timeout+2*time.Second)(cctx, exe, args...)
	}
	if err != nil && ctx.Err() == nil && cctx.Err() == context.DeadlineExceeded {
		timedOut = true
	}
	return out, timedOut, err
}

// String 只为调试输出。
func (d EncoderDevice) String() string {
	return fmt.Sprintf("%s(%s,%s,available=%v,h264=%q,hevc=%q)", d.ID, d.Name, d.Vendor, d.Available, d.Encoders.H264, d.Encoders.HEVC)
}

// vendorFromID 从 "<vendor>-<n>" 形式的设备 id 推断厂商，认不出来是 unknown。
func vendorFromID(id string) string {
	v, _, _ := strings.Cut(id, "-")
	if _, ok := vendorOrder[v]; ok {
		return v
	}
	return VendorUnknown
}

// EncoderResolver 返回任务用的编码器解析器（契约 9.7）：用 ListEncoderDevices 的缓存结果 + 当前偏好 + ResolveEncoder。
// 缓存还没有时会先检测一次（每个编码器最多 5 秒，之后走缓存）；检测出错一律按 CPU。
// codec 是 "h264" 或 "hevc"。选了具体设备但它不可用时 Fallback=true。
func (m *Manager) EncoderResolver() ffmpeg.EncoderResolver {
	return func(ctx context.Context, codec string) ffmpeg.EncoderChoice {
		if ctx == nil {
			ctx = context.Background()
		}
		list, err := m.ListEncoderDevices(ctx)
		if err != nil {
			return ffmpeg.EncoderChoice{}
		}
		name, dev, fb := ResolveEncoder(m.GetEncoderPreference(ctx), list.Devices, codec)
		return ffmpeg.EncoderChoice{Encoder: name, Device: dev, Fallback: fb}
	}
}
