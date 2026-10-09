package doccomp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/proc"
)

// DefaultTimeout 是单个任务从启动组件到退出的上限（契约 6.12.18）。
const DefaultTimeout = 5 * time.Minute

// Job 是一次组件转换（一条命令，契约 6.12.11）。
type Job struct {
	Exe       string        // 组件可执行文件（Windows soffice.com）
	Input     string        // 输入文件
	ConvertTo string        // --convert-to 的值，不带 shell 引号，如 `docx:MS Word 2007 XML`
	InFilter  string        // --infilter 的值（可空），如 `Text (encoded):UTF8`
	WorkDir   string        // 本任务的临时目录：profile / out 建在它下面
	OutExt    string        // 期望的输出扩展名（不带点），如 "docx"
	Timeout   time.Duration // 0 = DefaultTimeout；md 的两步共用时由调用方传剩余时间
}

// FileURL 把本机路径转成 file:/// URL（正斜杠，空格等按 URL 编码），给 -env:UserInstallation 用。
func FileURL(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // Windows：C:/x → /C:/x
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// Args 返回不含可执行文件的参数列表（契约 6.12.11 的顺序）。
func (j Job) Args(profile, outDir string) []string {
	args := []string{"--headless", "--invisible", "--norestore", "--nologo", "--nodefault", "--nolockcheck", "--nofirststartwizard",
		"-env:UserInstallation=" + FileURL(profile)}
	if j.InFilter != "" {
		args = append(args, "--infilter="+j.InFilter)
	}
	return append(args, "--convert-to", j.ConvertTo, "--outdir", outDir, j.Input)
}

var errNoOutput = errors.New("没有产出目标文件")

// Convert 跑一次组件转换，返回临时输出目录里唯一的目标文件。
// 超时 → DOC_TIMEOUT（已结束整个进程树）；ctx 取消 → ctx.Err()（同样结束进程树）；
// stderr 含 “source file could not be loaded” → DOC_CORRUPT；其他非零退出 / 没有产出 → DOC_COMPONENT_CRASHED。
func Convert(ctx context.Context, j Job) (string, error) {
	timeout := j.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	profile := filepath.Join(j.WorkDir, "profile")
	outDir := filepath.Join(j.WorkDir, "out")
	for _, d := range []string{profile, outDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", apperr.Wrap(apperr.IOError, "无法创建临时文件夹", err)
		}
	}
	if err := SeedProfile(profile); err != nil { // 宏不执行、链接不更新（profile.go）
		return "", apperr.Wrap(apperr.IOError, "无法创建临时文件夹", err)
	}
	cmd := exec.Command(j.Exe, j.Args(profile, outDir)...)
	cmd.Dir = j.WorkDir
	var stdout, stderr tailBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	proc.Configure(cmd)
	if err := proc.Start(cmd); err != nil {
		return "", apperr.Wrap(apperr.DocComponentCrashed, "文档组件意外退出，请重试。", err).WithDetail("exit=start")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var werr error
	select {
	case werr = <-done:
	case <-timer.C:
		_ = proc.Kill(cmd)
		<-done
		return "", apperr.New(apperr.DocTimeout, "文件处理太久没完成，可能已损坏，请检查后重试。").
			WithDetail(fmt.Sprintf("timeoutSec=%d", int(timeout/time.Second)))
	case <-ctx.Done():
		_ = proc.Kill(cmd)
		<-done
		return "", ctx.Err()
	}
	errText := sanitize(stderr.String()+"\n"+stdout.String(), j)
	if strings.Contains(stderr.String(), "source file could not be loaded") || strings.Contains(stdout.String(), "source file could not be loaded") {
		return "", apperr.New(apperr.DocCorrupt, "文件打不开，可能已损坏或不是有效的文档。").WithDetail(crashDetail(cmd, werr, errText))
	}
	out, ferr := findOutput(outDir, j.OutExt)
	if werr != nil || ferr != nil {
		return "", apperr.New(apperr.DocComponentCrashed, "文档组件意外退出，请重试。").WithDetail(crashDetail(cmd, werr, errText))
	}
	return out, nil
}

func crashDetail(cmd *exec.Cmd, werr error, tail string) string {
	code := "0"
	var ee *exec.ExitError
	switch {
	case errors.As(werr, &ee):
		code = strconv.Itoa(ee.ExitCode()) // 被信号结束时为 -1
	case werr != nil:
		code = "wait"
	case cmd.ProcessState != nil:
		code = strconv.Itoa(cmd.ProcessState.ExitCode())
	}
	d := "exit=" + code
	if werr == nil {
		d += "\nnoOutput=1"
	}
	if tail = strings.TrimSpace(tail); tail != "" {
		d += "\n" + tail
	}
	return d
}

// findOutput 找临时输出目录里唯一的目标扩展名文件（html 导出的图片等其他文件忽略）。
func findOutput(dir, ext string) (string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var found []string
	for _, e := range ents {
		if e.Type().IsRegular() && strings.EqualFold(strings.TrimPrefix(filepath.Ext(e.Name()), "."), ext) {
			found = append(found, filepath.Join(dir, e.Name()))
		}
	}
	if len(found) != 1 {
		return "", errNoOutput
	}
	if fi, err := os.Stat(found[0]); err != nil || fi.Size() == 0 {
		return "", errNoOutput
	}
	return found[0], nil
}

var pathLike = regexp.MustCompile(`(?:[A-Za-z]:[\\/]|file:/+|/)[^\s"'<>]*`)

// sanitize 去掉输出里的路径（detail 不含路径，契约 6.12.20），只留尾部 800 字节。
func sanitize(s string, j Job) string {
	for _, p := range []string{j.Input, j.WorkDir, filepath.Dir(j.Exe)} {
		if p != "" {
			s = strings.ReplaceAll(s, p, "")
			s = strings.ReplaceAll(s, filepath.ToSlash(p), "")
		}
	}
	s = pathLike.ReplaceAllString(s, "<path>")
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	s = strings.Join(lines, "\n")
	if len(s) > 800 {
		s = s[len(s)-800:]
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
	}
	return s
}

// tailBuffer 只保留最后 16 KiB 输出。
type tailBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf.Write(p)
	if t.buf.Len() > 32<<10 {
		b := t.buf.Bytes()
		keep := append([]byte(nil), b[len(b)-16<<10:]...)
		t.buf.Reset()
		t.buf.Write(keep)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.String()
}

// versionRe 取 --version 输出里的数字版本（如 "LibreOffice 26.2.6.3 xxx" → 26.2.6.3）。
var versionRe = regexp.MustCompile(`\b(\d+)\.(\d+)(?:\.(\d+))?(?:\.(\d+))?\b`)

// ParseVersion 解析版本号，返回完整数字版本和主 / 次版本。
func ParseVersion(out string) (full string, major, minor int, ok bool) {
	m := versionRe.FindStringSubmatch(out)
	if m == nil {
		return "", 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	return m[0], major, minor, true
}

// VersionOK 判断系统安装版本是否 ≥ 7.2（26.2 这类新版本号按数值比较）。
func VersionOK(major, minor int) bool {
	return major > MinMajor || major == MinMajor && minor >= MinMinor
}

// DownloadVersionOK 判断应用下载的组件是否达到契约写死的版本（前三段 ≥ 26.2.6，6.12.55）。
func DownloadVersionOK(full string) bool {
	parts := strings.Split(full, ".")
	nums := make([]int, 3)
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return false
		}
		nums[i] = n
	}
	if nums[0] != DownloadMinMajor {
		return nums[0] > DownloadMinMajor
	}
	if nums[1] != DownloadMinMinor {
		return nums[1] > DownloadMinMinor
	}
	return nums[2] >= DownloadMinPatch
}
