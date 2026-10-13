package officecanvas

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"FFmpegFree/internal/apperr"
)

// EntryBinary 返回组件根下的入口路径；不存在返回 ""。
// 名字固定：office-canvas / office-canvas.exe（契约 6.19.16.4）。
func EntryBinary(componentRoot string) string {
	if componentRoot == "" {
		return ""
	}
	name := "office-canvas"
	if runtime.GOOS == "windows" {
		name = "office-canvas.exe"
	}
	p := filepath.Join(componentRoot, name)
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return ""
	}
	return p
}

// Deck 是声明式幻灯片 IR（契约 6.19.16.4）；一期骨架只透传。
type Deck struct {
	Title  string  `json:"title,omitempty"`
	Slides []Slide `json:"slides"`
}

// Slide 是单页。
type Slide struct {
	ID      string   `json:"id"`
	Title   string   `json:"title,omitempty"`
	Bullets []string `json:"bullets,omitempty"`
}

// AdapterRequest 是 request.json。
type AdapterRequest struct {
	Version int    `json:"version"`
	Op      string `json:"op"` // preview | export
	Deck    Deck   `json:"deck"`
	OutDir  string `json:"outDir"`
}

// AdapterExportResponse 是 export 成功时的 response.json 要点。
type AdapterExportResponse struct {
	Version  int    `json:"version"`
	PptxPath string `json:"pptxPath"`
	Bytes    int64  `json:"bytes"`
}

// ExportOptions 是一次导出适配器调用。
type ExportOptions struct {
	ComponentRoot string
	WorkDir       string
	Deck          Deck
	Timeout       time.Duration
	Exec          func(ctx context.Context, name string, args ...string) *exec.Cmd
	Logf          func(format string, args ...any)
}

const defaultExportTimeout = 5 * time.Minute

// RunExport 按 6.19.16.4 调用入口二进制做 export。
// 组件未发布 / 入口缺失时返回中文「导出失败：…」。
func RunExport(ctx context.Context, opts ExportOptions) (AdapterExportResponse, error) {
	exe := EntryBinary(opts.ComponentRoot)
	if exe == "" {
		return AdapterExportResponse{}, apperr.New(apperr.Internal, MsgExportNotReady).WithDetail("reason=missing_entry")
	}
	if opts.WorkDir == "" {
		return AdapterExportResponse{}, apperr.New(apperr.InvalidArgument, "导出参数不完整").WithDetail("reason=work_dir")
	}
	if len(opts.Deck.Slides) == 0 {
		return AdapterExportResponse{}, apperr.New(apperr.InvalidArgument, MsgExportNoDeck).WithDetail("reason=empty_deck")
	}
	if err := os.MkdirAll(opts.WorkDir, 0o755); err != nil {
		return AdapterExportResponse{}, apperr.Wrap(apperr.IOError, "无法创建临时文件夹", err)
	}
	reqPath := filepath.Join(opts.WorkDir, "request.json")
	respPath := filepath.Join(opts.WorkDir, "response.json")
	req := AdapterRequest{Version: 1, Op: "export", Deck: opts.Deck, OutDir: opts.WorkDir}
	raw, err := json.Marshal(req)
	if err != nil {
		return AdapterExportResponse{}, apperr.Wrap(apperr.Internal, MsgExportStub, err)
	}
	if err := os.WriteFile(reqPath, raw, 0o644); err != nil {
		return AdapterExportResponse{}, apperr.Wrap(apperr.IOError, MsgExportStub, err)
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultExportTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	run := opts.Exec
	if run == nil {
		run = exec.CommandContext
	}
	cmd := run(cctx, exe, "--component-root", opts.ComponentRoot, "--request", reqPath, "--response", respPath)
	cmd.Dir = opts.WorkDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		if opts.Logf != nil {
			opts.Logf("office-canvas export failed: %v\n%s", err, truncate(string(out), 512))
		}
		return AdapterExportResponse{}, apperr.New(apperr.Internal, MsgExportStub).WithDetail("reason=adapter_exit")
	}
	body, err := os.ReadFile(respPath)
	if err != nil {
		return AdapterExportResponse{}, apperr.New(apperr.Internal, MsgExportStub).WithDetail("reason=no_response")
	}
	var resp AdapterExportResponse
	if err := json.Unmarshal(body, &resp); err != nil || resp.PptxPath == "" {
		return AdapterExportResponse{}, apperr.New(apperr.Internal, MsgExportStub).WithDetail("reason=bad_response")
	}
	fi, err := os.Stat(resp.PptxPath)
	if err != nil || fi.IsDir() {
		return AdapterExportResponse{}, apperr.New(apperr.Internal, MsgExportStub).WithDetail("reason=pptx_missing")
	}
	if resp.Bytes == 0 {
		resp.Bytes = fi.Size()
	}
	return resp, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
