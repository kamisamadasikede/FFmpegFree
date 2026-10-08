package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/paths"
)

// ---------- 存储位置（契约 v0.24，6.15.1 / 6.15.2） ----------

// SettingUploadsDir 是自定义上传目录的设置键（"" = <base>/uploads）。
const SettingUploadsDir = "uploadsDir"

// StorageDirs 是 GetStorageDirs / SetStorageDirs 的返回值。
type StorageDirs struct {
	OutputDir         string `json:"outputDir"`         // 实际输出目录（绝对路径）：自定义优先，否则 <base>/output
	UploadsDir        string `json:"uploadsDir"`        // 实际上传目录（绝对路径）：自定义优先，否则 <base>/uploads
	OutputCustom      bool   `json:"outputCustom"`      // true = outputDir 来自设置里的自定义目录
	UploadsCustom     bool   `json:"uploadsCustom"`     // true = uploadsDir 来自设置里的自定义目录
	DefaultOutputDir  string `json:"defaultOutputDir"`  // <base>/output
	DefaultUploadsDir string `json:"defaultUploadsDir"` // <base>/uploads
	BaseKind          string `json:"baseKind"`          // "exe_dir" | "user_data"
	FellBack          bool   `json:"fellBack"`          // true = 程序所在文件夹不可写，<base> 回退到了用户数据目录；macOS .app 包为 false
	OutputAvailable   bool   `json:"outputAvailable"`   // 调用时 outputDir 存在且是文件夹（只 stat，不试写）
	UploadsAvailable  bool   `json:"uploadsAvailable"`  // 调用时 uploadsDir 存在且是文件夹
}

// StorageDirsUpdate 是 SetStorageDirs 的参数：两个字段都要传，"" = 用默认目录。
type StorageDirsUpdate struct {
	OutputDir  string `json:"outputDir"`
	UploadsDir string `json:"uploadsDir"`
}

// StorageKind 是 OpenStorageFolder 的 kind。
const (
	StorageOutput  = "output"
	StorageUploads = "uploads"
	// StorageComponent（PM X5，契约 v0.25.1）：打开当前使用的转换组件可执行文件所在的文件夹，并选中这个文件（Windows / macOS）。
	StorageComponent = "component"
)

// componentNotReadyMessage 是 kind=component 时转换组件没就绪或文件不在的提示（契约 1.1：不出现 ffmpeg）。
const componentNotReadyMessage = "转换组件还没有就绪。"

// SetStorage 注入启动时确定的存储根目录和应用数据目录（app 启动时、Start 之前调用一次）。
func (m *Manager) SetStorage(st paths.Storage, dataDir string) {
	m.stMu.Lock()
	defer m.stMu.Unlock()
	m.stBase, m.dataDir = st, dataDir
}

func (m *Manager) storage() (paths.Storage, string) {
	m.stMu.Lock()
	defer m.stMu.Unlock()
	return m.stBase, m.dataDir
}

// ActualOutputDir 返回实际输出目录：自定义优先，否则 <base>/output（outputDir 传空的地方都用它，6.15.2 第 5 条）。
// 存储根目录没初始化（只在测试里）时退回自定义值（可能为 ""）。
func (m *Manager) ActualOutputDir(ctx context.Context) string {
	if d := m.DefaultOutputDir(ctx); d != "" {
		return d
	}
	st, _ := m.storage()
	return st.Output
}

// ActualUploadsDir 返回实际上传目录：自定义优先，否则 <base>/uploads。
func (m *Manager) ActualUploadsDir(ctx context.Context) string {
	if d := m.UploadsDir(ctx); d != "" {
		return d
	}
	st, _ := m.storage()
	return st.Uploads
}

// UploadsDir 返回设置里的自定义上传目录（"" = 默认）。
func (m *Manager) UploadsDir(ctx context.Context) string {
	m.mu.Lock()
	st := m.cfg.Settings
	m.mu.Unlock()
	if st == nil {
		m.memMu.Lock()
		defer m.memMu.Unlock()
		return m.memUploads
	}
	var d string
	if _, err := st.GetSetting(ctx, SettingUploadsDir, &d); err != nil {
		return ""
	}
	return d
}

func (m *Manager) setUploadsDir(ctx context.Context, d string) error {
	m.mu.Lock()
	st := m.cfg.Settings
	m.mu.Unlock()
	if st == nil {
		m.memMu.Lock()
		defer m.memMu.Unlock()
		m.memUploads = d
		return nil
	}
	return st.SetSetting(ctx, SettingUploadsDir, d)
}

// GetStorageDirs 返回实际路径、默认路径、是否回退和当前是否可用。
func (m *Manager) GetStorageDirs(ctx context.Context) (StorageDirs, error) {
	st, _ := m.storage()
	out, up := m.DefaultOutputDir(ctx), m.UploadsDir(ctx)
	d := StorageDirs{OutputDir: out, UploadsDir: up, OutputCustom: out != "", UploadsCustom: up != "",
		DefaultOutputDir: st.Output, DefaultUploadsDir: st.Uploads, BaseKind: st.BaseKind, FellBack: st.FellBack}
	if d.OutputDir == "" {
		d.OutputDir = st.Output
	}
	if d.UploadsDir == "" {
		d.UploadsDir = st.Uploads
	}
	if d.BaseKind == "" {
		d.BaseKind = paths.BaseUserData
	}
	d.OutputAvailable, d.UploadsAvailable = isDir(d.OutputDir), isDir(d.UploadsDir)
	return d, nil
}

// SetStorageDirs 校验并保存两个目录（任一失败整体不生效，INVALID_ARGUMENT），返回新的 StorageDirs。
func (m *Manager) SetStorageDirs(ctx context.Context, req StorageDirsUpdate) (StorageDirs, error) {
	out, err := m.checkStorageDir(req.OutputDir, StorageOutput)
	if err != nil {
		return StorageDirs{}, err
	}
	up, err := m.checkStorageDir(req.UploadsDir, StorageUploads)
	if err != nil {
		return StorageDirs{}, err
	}
	if err := m.setOutputDir(ctx, out); err != nil {
		return StorageDirs{}, apperr.Wrap(apperr.IOError, "保存设置失败", err)
	}
	if err := m.setUploadsDir(ctx, up); err != nil {
		return StorageDirs{}, apperr.Wrap(apperr.IOError, "保存设置失败", err)
	}
	return m.GetStorageDirs(ctx)
}

// checkStorageDir 是 6.15.2 第 2 条的校验（UpdateSettings 也用它）：非空时绝对路径、存在、是文件夹、能写；
// 输出目录还要遵守 6.12 的数据目录规则（v0.24.1：<dataDir>/output 放行）。等于默认目录时按 "" 保存。
func (m *Manager) checkStorageDir(dir, kind string) (string, error) {
	if dir == "" {
		return "", nil
	}
	label := "保存位置"
	if kind == StorageUploads {
		label = "上传位置"
	}
	if !filepath.IsAbs(dir) {
		return "", apperr.New(apperr.InvalidArgument, label+"必须是绝对路径").WithDetail(dir)
	}
	dir = filepath.Clean(dir)
	fi, err := os.Stat(dir)
	switch {
	case err != nil && errors.Is(err, os.ErrNotExist):
		return "", apperr.New(apperr.InvalidArgument, label+"不存在").WithDetail(dir)
	case err != nil:
		return "", apperr.New(apperr.InvalidArgument, label+"无法写入").WithDetail(dir + ": " + err.Error())
	case !fi.IsDir():
		return "", apperr.New(apperr.InvalidArgument, label+"不是文件夹").WithDetail(dir)
	}
	st, dataDir := m.storage()
	if kind == StorageOutput && paths.InsideDataDir(dataDir, dir) {
		return "", apperr.New(apperr.InvalidArgument, label+"不能在应用数据目录内").WithDetail("outputDir 不能在应用数据目录内\n" + dir)
	}
	if err := paths.WriteTest(dir); err != nil {
		return "", apperr.New(apperr.InvalidArgument, label+"无法写入").WithDetail(dir + ": " + err.Error())
	}
	def := st.Output
	if kind == StorageUploads {
		def = st.Uploads
	}
	if def != "" && samePath(def, dir) {
		return "", nil
	}
	return dir, nil
}

// OpenStorageFolder 在系统文件管理器里打开实际输出 / 上传目录本身（6.15.2 第 6 条）。
func (m *Manager) OpenStorageFolder(ctx context.Context, kind string) error {
	var dir string
	var custom bool
	switch kind {
	case StorageOutput:
		custom = m.DefaultOutputDir(ctx) != ""
		dir = m.ActualOutputDir(ctx)
	case StorageUploads:
		custom = m.UploadsDir(ctx) != ""
		dir = m.ActualUploadsDir(ctx)
	case StorageComponent:
		return m.revealComponent()
	default:
		return apperr.New(apperr.InvalidArgument, "kind 只能是 output、uploads 或 component").WithDetail(kind)
	}
	if dir == "" {
		return apperr.New(apperr.Internal, "存储位置尚未初始化")
	}
	if !isDir(dir) {
		if custom {
			return apperr.New(apperr.NotFound, "保存位置不存在，请在设置里重新选择").WithDetail("reason=file\n" + dir)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return apperr.Wrap(apperr.IOError, "无法创建文件夹", err).WithDetail(dir + ": " + err.Error())
		}
	}
	start := m.launch
	if start == nil {
		start = startDetached
	}
	return revealIn(runtime.GOOS, start, dir, nil)
}

// revealComponent 在文件管理器里显示当前使用的转换组件（ffmpeg 可执行文件）：Windows / macOS 打开所在文件夹并选中它，
// Linux 打开所在文件夹。路径只取自检测结果（Status().Path），不接受前端传入，所以不走 RevealInFolder 的白名单。
// 没有就绪（检测中、缺失、版本过旧、安装中……）或文件已经不在：NOT_FOUND，message 是 componentNotReadyMessage。
func (m *Manager) revealComponent() error {
	notReady := apperr.New(apperr.NotFound, componentNotReadyMessage).WithDetail("reason=component")
	st := m.Status()
	if st.State != ffmpeg.StateReady || st.Path == "" || !filepath.IsAbs(st.Path) {
		return notReady
	}
	fi, err := os.Stat(st.Path)
	if err != nil || fi.IsDir() {
		return notReady
	}
	start := m.launch
	if start == nil {
		start = startDetached
	}
	// 路径不回给前端（架构师定）：revealIn 的错误 detail 里可能带路径，换成只有 reason 的错误。
	if err := revealIn(runtime.GOOS, start, st.Path, nil); err != nil {
		ae := apperr.From(err)
		if ae.Code == apperr.NotFound {
			return notReady
		}
		return apperr.New(ae.Code, ae.Message).WithDetail("reason=component")
	}
	return nil
}

func isDir(p string) bool {
	if p == "" {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if caseInsensitivePaths(runtime.GOOS) {
		return strings.EqualFold(a, b)
	}
	return a == b
}
