package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
)

// ---------- 纯函数：-encoders 解析 ----------

const encodersAll = ` Encoders:
 V..... = Video
 A..... = Audio
 ------
 V....D libx264              libx264 H.264 / AVC (codec h264)
 V....D libx265              libx265 H.265 / HEVC (codec hevc)
 V....D h264_nvenc           NVIDIA NVENC H.264 encoder (codec h264)
 V....D hevc_nvenc           NVIDIA NVENC hevc encoder (codec hevc)
 V..... h264_qsv             H.264 / AVC (Intel Quick Sync Video acceleration) (codec h264)
 V..... hevc_qsv             HEVC (Intel Quick Sync Video acceleration) (codec hevc)
 V....D h264_amf             AMD AMF H.264 Encoder (codec h264)
 V....D hevc_amf             AMD AMF HEVC encoder (codec hevc)
 V....D h264_videotoolbox    VideoToolbox H.264 Encoder (codec h264)
 V....D hevc_videotoolbox    VideoToolbox H.265 Encoder (codec hevc)
 A....D aac                  AAC (Advanced Audio Coding)
`

func TestParseEncodersList(t *testing.T) {
	got := parseEncodersList(encodersAll)
	for _, n := range []string{"libx264", "libx265", "h264_nvenc", "hevc_nvenc", "h264_qsv", "hevc_qsv", "h264_amf", "hevc_amf", "h264_videotoolbox", "hevc_videotoolbox"} {
		if !got[n] {
			t.Errorf("缺 %s", n)
		}
	}
	if got["aac"] || got["Video"] || got["="] || len(got) != 10 {
		t.Errorf("不该包含音频 / 图例行: %v", got)
	}
	if got := parseEncodersList(" V....D libx264 x\r\n V....D h264_nvenc y\r\n"); !got["libx264"] || !got["h264_nvenc"] {
		t.Errorf("CRLF: %v", got)
	}
	if got := parseEncodersList(""); len(got) != 0 {
		t.Error("空输出")
	}
}

// ---------- 纯函数：显卡名称 ----------

func TestParseWindowsVideoControllers(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []gpuInfo
	}{
		{"单张（对象）", `{"Name":"NVIDIA GeForce RTX 4060","PNPDeviceID":"PCI\\VEN_10DE&DEV_2882&SUBSYS_1"}`,
			[]gpuInfo{{"NVIDIA GeForce RTX 4060", VendorNvidia, true}}},
		{"双显卡（数组）核显 + 独显", `[{"Name":"Intel(R) UHD Graphics 770","PNPDeviceID":"PCI\\VEN_8086&DEV_4680"},{"Name":"AMD Radeon RX 6600","PNPDeviceID":"PCI\\VEN_1002&DEV_73FF"}]`,
			[]gpuInfo{{"Intel(R) UHD Graphics 770", VendorIntel, false}, {"AMD Radeon RX 6600", VendorAMD, true}}},
		{"AMD 核显（APU）", `{"Name":"AMD Radeon(TM) Graphics","PNPDeviceID":"PCI\\VEN_1002&DEV_1638"}`, []gpuInfo{{"AMD Radeon(TM) Graphics", VendorAMD, false}}},
		{"Intel Arc 是独显", `{"Name":"Intel(R) Arc(TM) A770 Graphics","PNPDeviceID":"PCI\\VEN_8086&DEV_56A0"}`, []gpuInfo{{"Intel(R) Arc(TM) A770 Graphics", VendorIntel, true}}},
		{"虚拟适配器忽略", `[{"Name":"Microsoft Basic Display Adapter","PNPDeviceID":"x"},{"Name":"Microsoft Remote Display Adapter","PNPDeviceID":"y"}]`, nil},
		{"名字里没厂商，靠 PNP", `{"Name":"Display Adapter X","PNPDeviceID":"PCI\\VEN_10DE&DEV_1"}`, []gpuInfo{{"Display Adapter X", VendorNvidia, true}}},
		{"未知厂商", `{"Name":"Moore Threads S80","PNPDeviceID":"PCI\\VEN_1ED5&DEV_1"}`, []gpuInfo{{"Moore Threads S80", VendorUnknown, false}}},
		{"BOM + 空白", "\ufeff  {\"Name\":\"NVIDIA GeForce GTX 1650\",\"PNPDeviceID\":\"\"}\r\n", []gpuInfo{{"NVIDIA GeForce GTX 1650", VendorNvidia, true}}},
		{"重复项去重", `[{"Name":"NVIDIA GeForce RTX 3060","PNPDeviceID":""},{"Name":"NVIDIA GeForce RTX 3060","PNPDeviceID":""}]`, []gpuInfo{{"NVIDIA GeForce RTX 3060", VendorNvidia, true}}},
		{"空输出", "", nil},
		{"乱码", "not json", nil},
		{"坏数组", "[{", nil},
	}
	for _, c := range cases {
		if got := parseWindowsVideoControllers(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
}

func TestParseSystemProfiler(t *testing.T) {
	m1 := `{"SPDisplaysDataType":[{"_name":"kHW_AppleM2Item","spdisplays_vendor":"sppci_vendor_Apple","sppci_model":"Apple M2 Pro","sppci_cores":"19"}]}`
	if got := parseSystemProfiler(m1); !reflect.DeepEqual(got, []gpuInfo{{"Apple M2 Pro", VendorApple, false}}) {
		t.Errorf("apple: %+v", got)
	}
	intelMac := `{"SPDisplaysDataType":[{"_name":"Intel Iris Plus","spdisplays_vendor":"sppci_vendor_Intel","sppci_model":"Intel Iris Plus Graphics 655"},{"_name":"AMD","spdisplays_vendor":"sppci_vendor_amd","sppci_model":"AMD Radeon Pro 560X"}]}`
	want := []gpuInfo{{"Intel Iris Plus Graphics 655", VendorIntel, false}, {"AMD Radeon Pro 560X", VendorAMD, true}}
	if got := parseSystemProfiler(intelMac); !reflect.DeepEqual(got, want) {
		t.Errorf("intel mac: %+v", got)
	}
	if got := parseSystemProfiler(`{"SPDisplaysDataType":[{"_name":"NVIDIA GeForce GT 750M"}]}`); len(got) != 1 || got[0].Vendor != VendorNvidia {
		t.Errorf("无 model / vendor 字段: %+v", got)
	}
	for _, bad := range []string{"", "x", `{"SPDisplaysDataType":[]}`, `{}`} {
		if got := parseSystemProfiler(bad); len(got) != 0 {
			t.Errorf("%q: %+v", bad, got)
		}
	}
}

func TestParseLspci(t *testing.T) {
	out := `00:02.0 VGA compatible controller [0300]: Intel Corporation AlderLake-S GT1 [UHD Graphics 770] [8086:4680] (rev 0c)
01:00.0 VGA compatible controller [0300]: NVIDIA Corporation GA106 [GeForce RTX 3060] [10de:2503] (rev a1)
01:00.1 Audio device [0403]: NVIDIA Corporation GA106 High Definition Audio Controller [10de:228e] (rev a1)
03:00.0 Display controller [0380]: Advanced Micro Devices, Inc. [AMD/ATI] Navi 23 [Radeon RX 6600/6600 XT/6600M] [1002:73ff] (rev c1)
04:00.0 3D controller [0302]: NVIDIA Corporation TU117M [GeForce GTX 1650 Mobile / Max-Q] [10de:1f91] (rev a1)
05:00.0 VGA compatible controller: Red Hat, Inc. QXL paravirtual graphic card (rev 05)
`
	got := parseLspci(out)
	want := []gpuInfo{
		{"UHD Graphics 770", VendorIntel, false},
		{"NVIDIA GeForce RTX 3060", VendorNvidia, true},
		{"Radeon RX 6600/6600 XT/6600M", VendorAMD, true},
		{"NVIDIA GeForce GTX 1650 Mobile / Max-Q", VendorNvidia, true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if got := parseLspci("00:1f.3 Audio device: Intel\n"); len(got) != 0 {
		t.Errorf("无显卡: %+v", got)
	}
	if got := parseLspci(""); len(got) != 0 {
		t.Error("空")
	}
}

func TestVendorFromPCIIDAndDRM(t *testing.T) {
	for in, want := range map[string]string{"0x10de": VendorNvidia, "10DE": VendorNvidia, "VEN_8086": VendorIntel, "0x1002": VendorAMD, "0x1234": VendorUnknown, "": VendorUnknown} {
		if got := vendorFromPCIID(in); got != want {
			t.Errorf("%q → %s want %s", in, got, want)
		}
	}
	if g, ok := gpuFromDRM(drmCard{Vendor: "0x10de", Driver: "nvidia"}); !ok || g.Vendor != VendorNvidia || !g.Discrete || !strings.Contains(g.Name, "nvidia") {
		t.Errorf("%+v", g)
	}
	if g, ok := gpuFromDRM(drmCard{Vendor: "0x8086", Driver: "i915"}); !ok || g.Discrete || g.Vendor != VendorIntel {
		t.Errorf("%+v", g)
	}
	if _, ok := gpuFromDRM(drmCard{Vendor: "0x1af4"}); ok {
		t.Error("virtio 不算")
	}
}

func TestDRMGPUsFromSysfs(t *testing.T) {
	root := t.TempDir()
	mk := func(card, vendor string) {
		d := filepath.Join(root, card, "device")
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "vendor"), []byte(vendor+"\n"), 0o644)
	}
	mk("card0", "0x8086")
	mk("card1", "0x10de")
	os.MkdirAll(filepath.Join(root, "card0-HDMI-A-1"), 0o755) // 连接器，忽略
	os.MkdirAll(filepath.Join(root, "renderD128"), 0o755)
	got := drmGPUs(root)
	if len(got) != 2 || got[0].Vendor != VendorIntel || got[1].Vendor != VendorNvidia {
		t.Errorf("%+v", got)
	}
	if got := drmGPUs(filepath.Join(root, "nope")); len(got) != 0 {
		t.Errorf("目录不存在应返回空: %+v", got)
	}
}

func TestClassifyProbeError(t *testing.T) {
	cases := []struct {
		vendor, err string
		timedOut    bool
		want        string
	}{
		{VendorNvidia, "Unknown encoder 'h264_nvenc'", false, "不包含 NVENC"},
		{VendorNvidia, "[h264_nvenc @ 0x1] No NVENC capable devices found", false, "NVIDIA"},
		{VendorNvidia, "Cannot load libcuda.so.1", false, "NVIDIA"},
		{VendorIntel, "Error initializing an internal MFX session: unsupported (-3) qsv device", false, "Intel"},
		{VendorAMD, "AMF failed to initialise", false, "AMD"},
		{VendorApple, "videotoolbox error", false, "VideoToolbox"},
		{VendorNvidia, "", true, "超时"},
		{VendorIntel, "boom\nlast line here", false, "试跑失败：last line here"},
		{VendorIntel, "", false, "无输出"},
	}
	for _, c := range cases {
		if got := classifyProbeError(c.vendor, c.err, c.timedOut, 5); !strings.Contains(got, c.want) {
			t.Errorf("%v: %q 不含 %q", c, got, c.want)
		}
	}
}

// ---------- ResolveEncoder 全分支 ----------

func gpu(id, vendor string, discrete, available bool, h264, hevc string) EncoderDevice {
	return EncoderDevice{ID: id, Name: id, Vendor: vendor, Kind: KindGPU, Discrete: discrete, Available: available, Encoders: EncoderNames{H264: h264, HEVC: hevc}}
}

func TestResolveEncoder(t *testing.T) {
	cpu := cpuDevice()
	nv := gpu("nvidia-0", VendorNvidia, true, true, "h264_nvenc", "hevc_nvenc")
	amd := gpu("amd-0", VendorAMD, true, true, "h264_amf", "hevc_amf")
	intelIGPU := gpu("intel-0", VendorIntel, false, true, "h264_qsv", "hevc_qsv")
	arc := gpu("intel-1", VendorIntel, true, true, "h264_qsv", "hevc_qsv")
	apple := gpu("apple-0", VendorApple, false, true, "h264_videotoolbox", "hevc_videotoolbox")
	nvBad := gpu("nvidia-0", VendorNvidia, true, false, "", "")
	nvH264Only := gpu("nvidia-0", VendorNvidia, true, true, "h264_nvenc", "")

	cases := []struct {
		name     string
		pref     string
		devices  []EncoderDevice
		codec    string
		wantEnc  string
		wantID   string
		wantFall bool
	}{
		{"auto：无显卡 → cpu（不算回退）", "auto", []EncoderDevice{cpu}, "h264", "libx264", "cpu", false},
		{"auto：只有集显 → 集显", "auto", []EncoderDevice{cpu, intelIGPU}, "h264", "h264_qsv", "intel-0", false},
		{"auto：独显 + 集显 → 独显（集显在前也一样）", "auto", []EncoderDevice{cpu, intelIGPU, nv}, "hevc", "hevc_nvenc", "nvidia-0", false},
		{"auto：nvidia 与 amd 独显 → nvidia 在前", "auto", []EncoderDevice{cpu, amd, nv}, "h264", "h264_nvenc", "nvidia-0", false},
		{"auto：amd 独显优于 intel 集显", "auto", []EncoderDevice{cpu, intelIGPU, amd}, "h264", "h264_amf", "amd-0", false},
		{"auto：Arc 独显优于 nvidia 以外的集显", "auto", []EncoderDevice{cpu, intelIGPU, arc}, "h264", "h264_qsv", "intel-1", false},
		{"auto：nvidia 独显优于 Arc", "auto", []EncoderDevice{cpu, arc, nv}, "h264", "h264_nvenc", "nvidia-0", false},
		{"auto：同级保持顺序", "auto", []EncoderDevice{cpu, gpu("nvidia-1", VendorNvidia, true, true, "h264_nvenc", "x"), nv}, "h264", "h264_nvenc", "nvidia-1", false},
		{"auto：跳过不可用显卡", "auto", []EncoderDevice{cpu, nvBad, intelIGPU}, "h264", "h264_qsv", "intel-0", false},
		{"auto：显卡不能编该 codec → 跳过", "auto", []EncoderDevice{cpu, nvH264Only, intelIGPU}, "hevc", "hevc_qsv", "intel-0", false},
		{"auto：全都不可用 → cpu", "auto", []EncoderDevice{cpu, nvBad}, "hevc", "libx265", "cpu", false},
		{"auto：macOS", "auto", []EncoderDevice{cpu, apple}, "h264", "h264_videotoolbox", "apple-0", false},
		{"空偏好等同 auto", "", []EncoderDevice{cpu, nv}, "h264", "h264_nvenc", "nvidia-0", false},
		{"cpu 显式", "cpu", []EncoderDevice{cpu, nv}, "h264", "libx264", "cpu", false},
		{"显式设备可用", "intel-0", []EncoderDevice{cpu, nv, intelIGPU}, "h264", "h264_qsv", "intel-0", false},
		{"显式设备不可用 → cpu + fallback", "nvidia-0", []EncoderDevice{cpu, nvBad}, "h264", "libx264", "cpu", true},
		{"显式设备不存在 → cpu + fallback", "amd-0", []EncoderDevice{cpu, nv}, "hevc", "libx265", "cpu", true},
		{"显式设备不能编该 codec → cpu + fallback", "nvidia-0", []EncoderDevice{cpu, nvH264Only}, "hevc", "libx265", "cpu", true},
		{"显式选了 cpu 类型的非法 id", "cpu-1", []EncoderDevice{cpu}, "h264", "libx264", "cpu", true},
		{"h265 别名", "auto", []EncoderDevice{cpu, nv}, "h265", "hevc_nvenc", "nvidia-0", false},
		{"大小写 / 空白", " auto ", []EncoderDevice{cpu, nv}, "H264", "h264_nvenc", "nvidia-0", false},
		{"devices 里没有 cpu 项也能用", "auto", nil, "h264", "libx264", "cpu", false},
		{"devices 为空 + 显式设备 → fallback", "nvidia-0", nil, "hevc", "libx265", "cpu", true},
		{"未知 codec", "auto", []EncoderDevice{cpu, nv}, "av1", "", "", false},
	}
	for _, c := range cases {
		enc, id, fb := ResolveEncoder(c.pref, c.devices, c.codec)
		if enc != c.wantEnc || id != c.wantID || fb != c.wantFall {
			t.Errorf("%s: got (%q,%q,%v) want (%q,%q,%v)", c.name, enc, id, fb, c.wantEnc, c.wantID, c.wantFall)
		}
	}
}

// ---------- 假 ffmpeg：Runner 夹具 ----------

// fakeFF 按编码器名返回预设的试跑结果，记录调用与并发度。
type fakeFF struct {
	mu        sync.Mutex
	encoders  string           // -encoders 的输出
	encErr    error            // -encoders 失败
	probes    map[string]error // 试跑：编码器 → 错误（nil = 成功；缺省成功）
	hang      map[string]bool  // 试跑：编码器 → 一直阻塞到 ctx 结束（模拟超时）
	calls     []string         // 记录 "encoders" / "probe:<enc>" / "other:<exe>"
	inflight  int32
	maxFlight int32
	delay     time.Duration
	pwsh      string // powershell 输出
	sysprof   string
	lspci     string
}

func (f *fakeFF) run(ctx context.Context, exe string, args ...string) (string, error) {
	base := filepath.Base(exe)
	joined := strings.Join(args, " ")
	rec := func(s string) {
		f.mu.Lock()
		f.calls = append(f.calls, s)
		f.mu.Unlock()
	}
	switch {
	case base == "powershell.exe":
		rec("other:powershell")
		return f.pwsh, nil
	case base == "system_profiler":
		rec("other:system_profiler")
		return f.sysprof, nil
	case base == "lspci":
		rec("other:lspci")
		return f.lspci, nil
	case strings.Contains(joined, "-encoders"):
		rec("encoders")
		return f.encoders, f.encErr
	}
	enc := ""
	for i, a := range args {
		if a == "-c:v" && i+1 < len(args) {
			enc = args[i+1]
		}
	}
	rec("probe:" + enc)
	n := atomic.AddInt32(&f.inflight, 1)
	for {
		m := atomic.LoadInt32(&f.maxFlight)
		if n <= m || atomic.CompareAndSwapInt32(&f.maxFlight, m, n) {
			break
		}
	}
	defer atomic.AddInt32(&f.inflight, -1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.hang[enc] {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return "", f.probes[enc]
}

func (f *fakeFF) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// encFixture 是启动好的 Manager（ffmpeg ready）+ 假 ffmpeg。
type encFixture struct {
	*fixture
	ff *fakeFF
}

func newEncFixture(t *testing.T, goos string, ff *fakeFF, gpus []gpuInfo) *encFixture {
	t.Helper()
	f := newFixture(t)
	// 让定位器找到 good 目录里的 ffmpeg
	dir := filepath.Join(f.root, "good")
	f.install(t, dir)
	f.set.SetSetting(context.Background(), SettingFFmpegPath, dir)
	f.start(t)
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateReady })
	f.mgr.enc.env = encoderEnv{Run: ff.run, GOOS: goos, ProbeTimeout: 300 * time.Millisecond}
	if gpus != nil {
		f.mgr.enc.env.GPUs = func(context.Context) []gpuInfo { return gpus }
	}
	return &encFixture{fixture: f, ff: ff}
}

func find(l EncoderDeviceList, id string) (EncoderDevice, bool) {
	for _, d := range l.Devices {
		if d.ID == id {
			return d, true
		}
	}
	return EncoderDevice{}, false
}

var (
	nvGPU    = gpuInfo{"NVIDIA GeForce RTX 4060", VendorNvidia, true}
	intelGPU = gpuInfo{"Intel(R) UHD Graphics 770", VendorIntel, false}
	amdGPU   = gpuInfo{"AMD Radeon RX 6600", VendorAMD, true}
)

func TestListEncoderDevicesNoGPU(t *testing.T) {
	ff := &fakeFF{encoders: " V....D libx264 x\n V....D libx265 y\n"}
	e := newEncFixture(t, "linux", ff, []gpuInfo{})
	l, err := e.mgr.ListEncoderDevices(context.Background())
	if err != nil || !l.FFmpegReady || len(l.Devices) != 1 {
		t.Fatalf("%+v %v", l, err)
	}
	c := l.Devices[0]
	if c.ID != "cpu" || c.Name != "CPU（软件编码）" || c.Kind != KindCPU || !c.Available || c.Encoders.H264 != "libx264" || c.Encoders.HEVC != "libx265" || c.Reason != "" {
		t.Fatalf("cpu 项不对: %+v", c)
	}
	if ff.count("probe:") != 0 {
		t.Fatalf("没有显卡且 ffmpeg 没有硬件编码器，不该试跑: %v", ff.calls)
	}
}

func TestListEncoderDevicesVendors(t *testing.T) {
	cases := []struct {
		name    string
		goos    string
		gpus    []gpuInfo
		probes  map[string]error
		wantIDs []string // 期望的 available 设备 id（不含 cpu）
		wantEnc map[string][2]string
	}{
		{"nvidia 全通过", "linux", []gpuInfo{nvGPU}, nil, []string{"nvidia-0"}, map[string][2]string{"nvidia-0": {"h264_nvenc", "hevc_nvenc"}}},
		{"nvidia 只支持 h264", "windows", []gpuInfo{nvGPU}, map[string]error{"hevc_nvenc": errors.New("Unknown encoder")}, []string{"nvidia-0"}, map[string][2]string{"nvidia-0": {"h264_nvenc", ""}}},
		{"intel qsv", "windows", []gpuInfo{intelGPU}, nil, []string{"intel-0"}, map[string][2]string{"intel-0": {"h264_qsv", "hevc_qsv"}}},
		{"amd amf", "windows", []gpuInfo{amdGPU}, nil, []string{"amd-0"}, map[string][2]string{"amd-0": {"h264_amf", "hevc_amf"}}},
		{"三家都有", "windows", []gpuInfo{intelGPU, nvGPU, amdGPU}, nil, []string{"nvidia-0", "amd-0", "intel-0"}, nil},
		{"macOS videotoolbox", "darwin", []gpuInfo{{"Apple M2 Pro", VendorApple, false}}, nil, []string{"apple-0"}, map[string][2]string{"apple-0": {"h264_videotoolbox", "hevc_videotoolbox"}}},
		{"macOS 枚举失败仍靠试跑", "darwin", nil, nil, []string{"apple-0"}, nil},
		{"linux 枚举失败：全试，只有 nvenc 成功", "linux", nil, map[string]error{
			"h264_qsv": errors.New("qsv device failed"), "hevc_qsv": errors.New("qsv device failed"),
			"h264_amf": errors.New("AMF failed"), "hevc_amf": errors.New("AMF failed"),
		}, []string{"nvidia-0"}, nil},
	}
	for _, c := range cases {
		ff := &fakeFF{encoders: encodersAll, probes: c.probes}
		e := newEncFixture(t, c.goos, ff, c.gpus)
		l, err := e.mgr.ListEncoderDevices(context.Background())
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var got []string
		for _, d := range l.Devices[1:] {
			if d.Available {
				got = append(got, d.ID)
			}
		}
		if !reflect.DeepEqual(got, c.wantIDs) && !(len(got) == len(c.wantIDs) && len(got) == 0) {
			t.Errorf("%s: 可用设备 %v，期望 %v\n%v", c.name, got, c.wantIDs, l.Devices)
		}
		for id, w := range c.wantEnc {
			d, ok := find(l, id)
			if !ok || d.Encoders.H264 != w[0] || d.Encoders.HEVC != w[1] {
				t.Errorf("%s: %s = %+v，期望 %v", c.name, id, d, w)
			}
		}
		if l.Devices[0].ID != "cpu" {
			t.Errorf("%s: 第一项不是 cpu", c.name)
		}
	}
}

func TestListEncoderDevicesProbeFailureAndMissing(t *testing.T) {
	// 有 nvidia 显卡但试跑失败（驱动缺失）；有 intel 显卡但 ffmpeg 不含 qsv；有 amd 显卡试跑成功
	encs := " V....D libx264 x\n V....D h264_nvenc a\n V....D hevc_nvenc b\n V....D h264_amf c\n"
	ff := &fakeFF{encoders: encs, probes: map[string]error{
		"h264_nvenc": errors.New("Cannot load libcuda.so.1"), "hevc_nvenc": errors.New("Cannot load libcuda.so.1"),
	}}
	e := newEncFixture(t, "windows", ff, []gpuInfo{nvGPU, intelGPU, amdGPU, {"Moore Threads S80", VendorUnknown, false}})
	l, _ := e.mgr.ListEncoderDevices(context.Background())
	nv, _ := find(l, "nvidia-0")
	if nv.Available || !strings.Contains(nv.Reason, "NVIDIA") || nv.Encoders.H264 != "" {
		t.Errorf("nvidia: %+v", nv)
	}
	in, _ := find(l, "intel-0")
	if in.Available || !strings.Contains(in.Reason, "不包含") {
		t.Errorf("intel: %+v", in)
	}
	am, _ := find(l, "amd-0")
	if !am.Available || am.Encoders.H264 != "h264_amf" || am.Encoders.HEVC != "" {
		t.Errorf("amd: %+v", am)
	}
	unk, ok := find(l, "unknown-0")
	if !ok || unk.Available || unk.Reason == "" || unk.Kind != KindGPU {
		t.Errorf("unknown: %+v", unk)
	}
	if ff.count("probe:h264_qsv") != 0 {
		t.Error("ffmpeg 里没有的编码器不该试跑")
	}
}

func TestProbeTimeoutNotCachedAndReported(t *testing.T) {
	ff := &fakeFF{encoders: " V....D libx264 x\n V....D h264_nvenc a\n", hang: map[string]bool{"h264_nvenc": true}}
	e := newEncFixture(t, "linux", ff, []gpuInfo{nvGPU})
	start := time.Now()
	l, err := e.mgr.ListEncoderDevices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("超时应在 ProbeTimeout（300ms）附近返回: %v", el)
	}
	nv, _ := find(l, "nvidia-0")
	if nv.Available || !strings.Contains(nv.Reason, "超时") {
		t.Fatalf("%+v", nv)
	}
	// 超时结果不缓存：驱动恢复后再调用重新试跑
	ff.mu.Lock()
	ff.hang = nil
	ff.mu.Unlock()
	l, _ = e.mgr.ListEncoderDevices(context.Background())
	if nv, _ := find(l, "nvidia-0"); !nv.Available {
		t.Fatalf("超时结果不该被缓存: %+v", nv)
	}
}

func TestEncodersListFailureIsCPUOnlyNoError(t *testing.T) {
	ff := &fakeFF{encErr: errors.New("boom")}
	e := newEncFixture(t, "linux", ff, []gpuInfo{nvGPU})
	l, err := e.mgr.ListEncoderDevices(context.Background())
	if err != nil || !l.FFmpegReady || len(l.Devices) != 1 || l.Devices[0].ID != "cpu" {
		t.Fatalf("%+v %v", l, err)
	}
	// 没缓存：再调一次会重新检测
	e.mgr.ListEncoderDevices(context.Background())
	if ff.count("encoders") != 2 {
		t.Fatalf("失败不缓存: %v", ff.calls)
	}
}

func TestProbeArgsAndConcurrencyLimit(t *testing.T) {
	want := []string{"-f", "lavfi", "-i", "color=c=black:s=256x256:d=0.1", "-frames:v", "1", "-c:v", "h264_nvenc", "-f", "null", "-"}
	got := strings.Join(probeArgs("h264_nvenc"), " ")
	if !strings.Contains(got, strings.Join(want, " ")) {
		t.Errorf("试跑参数: %s", got)
	}
	ff := &fakeFF{encoders: encodersAll, delay: 40 * time.Millisecond}
	e := newEncFixture(t, "windows", ff, []gpuInfo{nvGPU, intelGPU, amdGPU})
	if _, err := e.mgr.ListEncoderDevices(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ff.count("probe:") != 6 {
		t.Errorf("应试跑 6 个: %v", ff.calls)
	}
	if m := atomic.LoadInt32(&ff.maxFlight); m > int32(encoderProbeParallel) || m < 1 {
		t.Errorf("并发上限 %d，实际最大 %d", encoderProbeParallel, m)
	}
}

func TestEncoderCacheAndInvalidation(t *testing.T) {
	ff := &fakeFF{encoders: encodersAll}
	e := newEncFixture(t, "linux", ff, []gpuInfo{nvGPU})
	ctx := context.Background()
	e.mgr.ListEncoderDevices(ctx)
	first := ff.count("probe:")
	e.mgr.ListEncoderDevices(ctx)
	if ff.count("probe:") != first || ff.count("encoders") != 1 {
		t.Fatalf("第二次应命中缓存: %v", ff.calls)
	}
	// Refresh 强制重测
	e.mgr.RefreshEncoderDevices(ctx)
	if ff.count("encoders") != 2 {
		t.Fatalf("Refresh 应重测: %v", ff.calls)
	}
	// ffmpeg 重新就绪（Recheck → ffmpeg:status ready）使缓存失效
	if _, err := e.mgr.Recheck(ctx); err != nil {
		t.Fatal(err)
	}
	e.mgr.ListEncoderDevices(ctx)
	if ff.count("encoders") != 3 {
		t.Fatalf("Recheck 后应重测: %v", ff.calls)
	}
	// ffmpeg 路径变化（缓存 key）也失效：换一个 ffmpeg
	dir2 := filepath.Join(e.root, "good2", "good")
	e.install(t, dir2)
	if _, err := e.mgr.SetPath(ctx, dir2); err != nil {
		t.Fatal(err)
	}
	e.mgr.ListEncoderDevices(ctx)
	if ff.count("encoders") != 4 {
		t.Fatalf("换路径后应重测: %v", ff.calls)
	}
}

func TestInvalidationDuringDetectDoesNotCacheStale(t *testing.T) {
	ff := &fakeFF{encoders: encodersAll, delay: 80 * time.Millisecond}
	e := newEncFixture(t, "linux", ff, []gpuInfo{nvGPU})
	done := make(chan struct{})
	go func() { e.mgr.ListEncoderDevices(context.Background()); close(done) }()
	time.Sleep(30 * time.Millisecond)
	e.mgr.invalidateEncoders() // 检测中途失效
	<-done
	e.mgr.enc.cacheMu.Lock()
	c := e.mgr.enc.cache
	e.mgr.enc.cacheMu.Unlock()
	if c != nil {
		t.Fatal("检测期间被失效，结果不该写入缓存")
	}
}

func TestFFmpegNotReadyReturnsCPUOnly(t *testing.T) {
	m := NewManager() // 没 Start：checking
	l, err := m.ListEncoderDevices(context.Background())
	if err != nil || l.FFmpegReady || len(l.Devices) != 1 || l.Devices[0].ID != "cpu" {
		t.Fatalf("%+v %v", l, err)
	}
	ff := &fakeFF{encoders: encodersAll}
	f := newFixture(t) // 启动但 ffmpeg missing
	f.start(t)
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
	f.mgr.enc.env = encoderEnv{Run: ff.run}
	l, err = f.mgr.ListEncoderDevices(context.Background())
	if err != nil || l.FFmpegReady || len(l.Devices) != 1 || ff.count("encoders") != 0 {
		t.Fatalf("ffmpeg 未就绪不该检测: %+v %v %v", l, err, ff.calls)
	}
}

func TestListEncoderDevicesCanceled(t *testing.T) {
	ff := &fakeFF{encoders: encodersAll, hang: map[string]bool{"h264_nvenc": true, "hevc_nvenc": true}}
	e := newEncFixture(t, "linux", ff, []gpuInfo{nvGPU})
	e.mgr.enc.env.ProbeTimeout = 10 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	if _, err := e.mgr.ListEncoderDevices(ctx); !apperr.Is(err, apperr.Canceled) {
		t.Fatalf("应返回 CANCELED: %v", err)
	}
}

func TestEnumerateGPUsByPlatformUsesParsers(t *testing.T) {
	pw := `{"Name":"NVIDIA GeForce RTX 4060","PNPDeviceID":"PCI\\VEN_10DE"}`
	ff := &fakeFF{
		encoders: encodersAll, pwsh: pw,
		sysprof: `{"SPDisplaysDataType":[{"sppci_model":"Apple M1","spdisplays_vendor":"sppci_vendor_Apple"}]}`,
		lspci:   "01:00.0 VGA compatible controller [0300]: NVIDIA Corporation GA106 [GeForce RTX 3060] [10de:2503] (rev a1)\n",
	}
	for goos, wantCall := range map[string]string{"windows": "other:powershell", "darwin": "other:system_profiler", "linux": "other:lspci"} {
		e := newEncFixture(t, goos, ff, nil)
		e.mgr.enc.env.GPUs = nil // 走真实枚举（Run 是假的）
		l, err := e.mgr.RefreshEncoderDevices(context.Background())
		if err != nil || len(l.Devices) < 2 {
			t.Fatalf("%s: %+v %v", goos, l, err)
		}
		if ff.count(wantCall) == 0 {
			t.Errorf("%s: 没调用 %s: %v", goos, wantCall, ff.calls)
		}
	}
}

// ---------- 偏好 ----------

func TestEncoderPreferencePersistence(t *testing.T) {
	ff := &fakeFF{encoders: encodersAll}
	e := newEncFixture(t, "linux", ff, []gpuInfo{nvGPU})
	ctx := context.Background()
	m := e.mgr
	if got := m.GetEncoderPreference(ctx); got != "auto" {
		t.Fatalf("默认应是 auto: %q", got)
	}
	for _, id := range []string{"cpu", "nvidia-0", "auto"} {
		if err := m.SetEncoderPreference(ctx, id); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if got := m.GetEncoderPreference(ctx); got != id {
			t.Fatalf("%s: 读到 %q", id, got)
		}
	}
	// 真的落在 settings 表里，键 encoderPreference
	m.SetEncoderPreference(ctx, "nvidia-0")
	var raw string
	if ok, err := e.set.GetSetting(ctx, SettingEncoderPreference, &raw); err != nil || !ok || raw != "nvidia-0" {
		t.Fatalf("settings 里没有: %q %v %v", raw, ok, err)
	}
	// 新 Manager 共用同一个 settings → 读到同一个值（重启后仍在）
	m2 := NewManager()
	m2.cfg = Config{Settings: e.set}
	if got := m2.GetEncoderPreference(ctx); got != "nvidia-0" {
		t.Fatalf("重启后: %q", got)
	}
}

func TestSetEncoderPreferenceInvalid(t *testing.T) {
	ff := &fakeFF{encoders: encodersAll}
	e := newEncFixture(t, "linux", ff, []gpuInfo{nvGPU})
	ctx := context.Background()
	e.mgr.SetEncoderPreference(ctx, "cpu")
	for _, bad := range []string{"", "  ", "nvidia-9", "amd-0", "Nvidia-0", "../x", "a b", "auto;drop", strings.Repeat("a", 40), "中文"} {
		err := e.mgr.SetEncoderPreference(ctx, bad)
		if bad == "" || strings.TrimSpace(bad) == "" {
			// 空白按 "" 处理：不是合法 id
		}
		if !apperr.Is(err, apperr.InvalidArgument) {
			t.Errorf("%q 应 INVALID_ARGUMENT: %v", bad, err)
		}
		if got := e.mgr.GetEncoderPreference(ctx); got != "cpu" {
			t.Fatalf("%q 非法时不应改动原值: %q", bad, got)
		}
	}
}

func TestSetEncoderPreferenceAllowsUnavailableExistingDevice(t *testing.T) {
	ff := &fakeFF{encoders: encodersAll, probes: map[string]error{"h264_nvenc": errors.New("No NVENC capable devices found"), "hevc_nvenc": errors.New("No NVENC capable devices found")}}
	e := newEncFixture(t, "linux", ff, []gpuInfo{nvGPU})
	ctx := context.Background()
	if err := e.mgr.SetEncoderPreference(ctx, "nvidia-0"); err != nil {
		t.Fatalf("存在但暂时不可用的设备可以保存: %v", err)
	}
	l, _ := e.mgr.ListEncoderDevices(ctx)
	if d, _ := find(l, "nvidia-0"); d.Available || d.Reason == "" {
		t.Fatalf("%+v", d)
	}
	if enc, id, fb := ResolveEncoder(e.mgr.GetEncoderPreference(ctx), l.Devices, "h264"); enc != "libx264" || id != "cpu" || !fb {
		t.Fatalf("不可用应回退: %s %s %v", enc, id, fb)
	}
}

func TestPreferenceToMissingDeviceKeptAndMarkedUnavailable(t *testing.T) {
	ff := &fakeFF{encoders: encodersAll}
	e := newEncFixture(t, "linux", ff, []gpuInfo{nvGPU})
	ctx := context.Background()
	// 直接写库模拟"以前选过、现在显卡没了"
	e.set.SetSetting(ctx, SettingEncoderPreference, "amd-0")
	if got := e.mgr.GetEncoderPreference(ctx); got != "amd-0" {
		t.Fatalf("Get 保持原值: %q", got)
	}
	l, _ := e.mgr.ListEncoderDevices(ctx)
	d, ok := find(l, "amd-0")
	if !ok || d.Available || d.Reason == "" {
		t.Fatalf("应有一项 available=false 的占位: %+v", l.Devices)
	}
	if l.Devices[0].ID != "cpu" {
		t.Fatal("cpu 仍是第一项")
	}
	// 占位项不写进缓存：偏好改回 auto 后消失
	e.mgr.SetEncoderPreference(ctx, "auto")
	l, _ = e.mgr.ListEncoderDevices(ctx)
	if _, ok := find(l, "amd-0"); ok {
		t.Fatal("占位项只在偏好指向它时出现")
	}
	// cpu 偏好不会追加占位
	e.mgr.SetEncoderPreference(ctx, "cpu")
	if l, _ = e.mgr.ListEncoderDevices(ctx); len(l.Devices) != 2 {
		t.Fatalf("%v", l.Devices)
	}
}

func TestEncoderPreferenceWithoutSettingsStore(t *testing.T) {
	m := NewManager()
	ctx := context.Background()
	if m.GetEncoderPreference(ctx) != "auto" {
		t.Fatal("默认 auto")
	}
	if err := m.SetEncoderPreference(ctx, "cpu"); err != nil || m.GetEncoderPreference(ctx) != "cpu" {
		t.Fatalf("内存兜底: %v", err)
	}
	if err := m.SetEncoderPreference(ctx, "nvidia-0"); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("ffmpeg 未就绪时只有 cpu，nvidia-0 不存在: %v", err)
	}
}

func TestEncoderDeviceJSONShape(t *testing.T) {
	b, _ := jsonMarshal(EncoderDeviceList{FFmpegReady: true, Devices: []EncoderDevice{cpuDevice()}})
	s := string(b)
	for _, k := range []string{`"ffmpegReady":true`, `"id":"cpu"`, `"vendor":"unknown"`, `"kind":"cpu"`, `"available":true`, `"h264":"libx264"`, `"hevc":"libx265"`} {
		if !strings.Contains(s, k) {
			t.Errorf("JSON 缺 %s: %s", k, s)
		}
	}
	if strings.Contains(s, `"reason"`) {
		t.Errorf("reason 为空时不输出: %s", s)
	}
}

var _ = fmt.Sprint

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// ---------- GetEncoderPreferenceInfo：偏好可显示 ----------

func TestGetEncoderPreferenceInfo(t *testing.T) {
	ff := &fakeFF{encoders: encodersAll}
	e := newEncFixture(t, "linux", ff, []gpuInfo{nvGPU})
	ctx := context.Background()
	m := e.mgr
	info, err := m.GetEncoderPreferenceInfo(ctx)
	if err != nil || info != (EncoderPreferenceInfo{ID: "auto", Name: "自动", Available: true}) {
		t.Fatalf("默认: %+v %v", info, err)
	}
	m.SetEncoderPreference(ctx, "cpu")
	if info, _ = m.GetEncoderPreferenceInfo(ctx); info.ID != "cpu" || info.Name != "CPU（软件编码）" || !info.Available {
		t.Fatalf("cpu: %+v", info)
	}
	m.SetEncoderPreference(ctx, "nvidia-0")
	if info, _ = m.GetEncoderPreferenceInfo(ctx); info.ID != "nvidia-0" || info.Name != nvGPU.Name || !info.Available || info.Reason != "" {
		t.Fatalf("具体显卡: %+v", info)
	}
	// 显卡消失（换了机器 / 拔了）：偏好保持，仍带上当时记下的名字，available=false + 原因
	m.enc.env.GPUs = func(context.Context) []gpuInfo { return nil }
	ff.mu.Lock()
	ff.encoders = " V....D libx264 x\n"
	ff.mu.Unlock()
	m.RefreshEncoderDevices(ctx)
	info, _ = m.GetEncoderPreferenceInfo(ctx)
	if info.ID != "nvidia-0" || info.Name != nvGPU.Name || info.Available || info.Reason == "" {
		t.Fatalf("显卡消失: %+v", info)
	}
	l, _ := m.ListEncoderDevices(ctx)
	if d, ok := find(l, "nvidia-0"); !ok || d.Name != nvGPU.Name || d.Available || d.Vendor != VendorNvidia {
		t.Fatalf("占位项应带记住的名字和厂商: %+v", d)
	}
	// 重启后（新 Manager 共用 settings）名字仍在
	m2 := NewManager()
	m2.cfg = Config{Settings: e.set}
	if info, _ := m2.GetEncoderPreferenceInfo(ctx); info.ID != "nvidia-0" || info.Name != nvGPU.Name || info.Available {
		t.Fatalf("重启后: %+v", info)
	}
	// 切回 auto 后名字不再影响
	m.SetEncoderPreference(ctx, "auto")
	if info, _ = m.GetEncoderPreferenceInfo(ctx); info.Name != "自动" {
		t.Fatalf("%+v", info)
	}
}

func TestGetEncoderPreferenceInfoUnavailableProbe(t *testing.T) {
	ff := &fakeFF{encoders: encodersAll, probes: map[string]error{"h264_nvenc": errors.New("No NVENC capable devices found"), "hevc_nvenc": errors.New("No NVENC capable devices found")}}
	e := newEncFixture(t, "linux", ff, []gpuInfo{nvGPU})
	ctx := context.Background()
	e.mgr.SetEncoderPreference(ctx, "nvidia-0")
	info, _ := e.mgr.GetEncoderPreferenceInfo(ctx)
	if info.Name != nvGPU.Name || info.Available || !strings.Contains(info.Reason, "NVIDIA") {
		t.Fatalf("%+v", info)
	}
	// ffmpeg 未就绪：不报错，available=false，名字来自记录
	e.set.SetSetting(ctx, SettingEncoderPreference, "nvidia-0")
	m := NewManager()
	m.cfg = Config{Settings: e.set}
	e.set.SetSetting(ctx, SettingEncoderPreferenceName, "NVIDIA GeForce RTX 4060")
	if info, err := m.GetEncoderPreferenceInfo(ctx); err != nil || info.Available || info.Name != "NVIDIA GeForce RTX 4060" || info.Reason == "" {
		t.Fatalf("%+v %v", info, err)
	}
}
