package doc

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/fsutil"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/task"
)

// params 是 office_pdf 任务的 Params JSON（Retry 用它重建 Runner）。
type params struct {
	Input     string `json:"input"`
	OutputDir string `json:"outputDir"` // 已解析的最终输出目录；空 = 源文件所在文件夹
}

func openErr(err error) error {
	switch {
	case os.IsNotExist(err):
		return apperr.Wrap(apperr.NotFound, "文件不存在", err)
	case errors.Is(err, zip.ErrFormat), errors.Is(err, zip.ErrInsecurePath), errors.Is(err, zip.ErrAlgorithm), errors.Is(err, zip.ErrChecksum), errors.Is(err, io.ErrUnexpectedEOF):
		return invalidOOXML(err)
	}
	return apperr.Wrap(apperr.IOError, "读取文件失败", err)
}

func entryErr(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	var ae *apperr.AppError
	if errors.As(err, &ae) {
		return err
	}
	if errors.Is(err, zip.ErrFormat) || errors.Is(err, zip.ErrChecksum) || errors.Is(err, zip.ErrAlgorithm) || errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "flate") {
		return invalidOOXML(err)
	}
	return apperr.Wrap(apperr.IOError, "读取文件失败", err)
}

// ConvertToPDF 为每个输入文件提交一个 office_pdf 任务（进入 batch 池排队），返回的任务与 inputs 一一对应。
//
// 先整体校验再提交：任何一个不通过整体失败、不提交任何任务，错误 detail：有 reason 的错误首行是 reason=<枚举>、第二行是出错文件的路径，其余首行是出错文件的路径。
// outputDir 为空用 Settings.defaultOutputDir，仍为空则输出到各自源文件所在文件夹；输出名 <源文件名去扩展名>.pdf（经净化），重名追加 (1)、(2)。
func (s *Service) ConvertToPDF(ctx context.Context, inputs []string, outputDir string) ([]task.Task, error) {
	out, err := s.convert(ctx, inputs, outputDir)
	if err != nil && ctx.Err() != nil {
		return out, apperr.Wrap(apperr.Canceled, "操作已取消", ctx.Err())
	}
	return out, err
}

type job struct {
	in, ext, out, dir string
}

func (s *Service) convert(ctx context.Context, inputs []string, outputDir string) ([]task.Task, error) {
	if s.cfg.Tasks == nil {
		return nil, s.internalNotReady("文档服务")
	}
	if len(inputs) == 0 {
		return nil, apperr.New(apperr.InvalidArgument, "没有要转换的文件")
	}
	if len(inputs) > MaxInputsPerSubmit {
		return nil, apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次最多提交 %d 个文件，请分批提交", MaxInputsPerSubmit))
	}
	dir, err := s.resolveOutputDir(ctx, outputDir)
	if err != nil {
		return nil, err
	}
	jobs := make([]job, len(inputs))
	for i, raw := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, apperr.Wrap(apperr.Canceled, "操作已取消", err)
		}
		j, err := s.prepare(ctx, raw, dir)
		if err != nil {
			return nil, withPath(err, raw)
		}
		jobs[i] = j
	}
	var out []task.Task
	for _, j := range jobs {
		t, err := s.submitOne(j)
		if err != nil {
			return out, err // 已提交的保留（不回滚）
		}
		out = append(out, t)
	}
	return out, nil
}

// prepare 校验一个输入并决定预期输出路径。
func (s *Service) prepare(ctx context.Context, raw, dir string) (job, error) {
	if strings.TrimSpace(raw) == "" || !filepath.IsAbs(raw) {
		return job{}, apperr.New(apperr.InvalidArgument, "文件路径必须是绝对路径")
	}
	in, _, err := paths.Normalize(raw)
	if err != nil {
		return job{}, apperr.Wrap(apperr.InvalidArgument, "路径不合法", err)
	}
	ext := extLower(in)
	if err := checkExt(ext); err != nil {
		return job{}, err
	}
	fi, err := os.Stat(in)
	if err != nil {
		return job{}, readErr("无法读取文件", err)
	}
	if fi.IsDir() {
		return job{}, apperr.New(apperr.InvalidArgument, "这是文件夹，不是文件")
	}
	if !fi.Mode().IsRegular() {
		return job{}, apperr.New(apperr.InvalidArgument, "不是普通文件")
	}
	if fi.Size() > MaxInputBytes {
		return job{}, reasonErr(apperr.InvalidArgument, "文件超过 100 MiB", reasonTooLarge, fmt.Sprintf("%d 字节", fi.Size()))
	}
	if err := validateOOXML(in, ext); err != nil {
		return job{}, err
	}
	// 字体规则：只有“没有内嵌字体也没有可用系统字体”时才需要提前扫描文本（其他情况必然能选出字体）。
	if s.fonts.embedded == nil && s.fonts.firstSystem() == nil {
		m, err := s.extract(ctx, in, ext)
		if err != nil {
			return job{}, err
		}
		if _, ok := s.fonts.choose(m.runes); !ok {
			return job{}, errNoFont
		}
	}
	outDir := dir
	if outDir == "" {
		outDir = filepath.Dir(in)
	}
	desired, err := s.desiredOutput(in, outDir)
	if err != nil {
		return job{}, err
	}
	return job{in: in, ext: ext, out: desired, dir: dir}, nil
}

var errNoFont = reasonErr(apperr.Unsupported, "没有可用的 Unicode 字体", reasonNoFont, "文档含有 Latin-1 以外的字符，但没有可用的字体")

func checkExt(ext string) error {
	switch ext {
	case "docx", "xlsx", "pptx":
		return nil
	}
	for _, u := range unsupportedFormats {
		if u.ext == ext {
			return reasonErr(apperr.Unsupported, "暂不支持这种格式", reasonFormat, "."+ext+"："+u.reason)
		}
	}
	if ext == "" {
		return reasonErr(apperr.Unsupported, "暂不支持这种格式", reasonFormat, "文件没有扩展名")
	}
	return reasonErr(apperr.Unsupported, "暂不支持这种格式", reasonFormat, "."+ext)
}

// validateOOXML 检查文件确实是含必需部件的 OOXML：OLE 头（加密或旧格式改了扩展名）UNSUPPORTED，
// 不是 zip / 缺部件 / 条目解压后 > 256 MiB INVALID_ARGUMENT。
func validateOOXML(path, ext string) error {
	f, _, err := openRegular(path)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 4)
	n, _ := io.ReadFull(f, head)
	if n == 4 && bytes.Equal(head, []byte{0xD0, 0xCF, 0x11, 0xE0}) {
		return reasonErr(apperr.Unsupported, "暂不支持这种格式", reasonEncrypted, "加密文档不支持（或旧版格式改了扩展名），请先另存为未加密的 docx、xlsx 或 pptx")
	}
	if err := checkZipEntries(path); err != nil {
		return err
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return openErr(err)
	}
	defer zr.Close()
	if len(zr.File) > MaxZipEntries {
		return errTooManyEntries(uint64(len(zr.File)))
	}
	var need bool
	for _, e := range zr.File {
		if e.UncompressedSize64 > uint64(maxEntryBytes) {
			return reasonErr(apperr.InvalidArgument, "不是有效的 OOXML 文件", reasonTooLarge, e.Name+" 解压后超过 256 MiB")
		}
		switch ext {
		case "docx":
			need = need || e.Name == "word/document.xml"
		case "xlsx":
			need = need || e.Name == "xl/workbook.xml"
		case "pptx":
			need = need || slideRe.MatchString(e.Name)
		}
	}
	if !need {
		return reasonErr(apperr.InvalidArgument, "不是有效的 OOXML 文件", reasonInvalidOOXML, "缺少 "+requiredPart(ext))
	}
	return nil
}

func requiredPart(ext string) string {
	switch ext {
	case "docx":
		return "word/document.xml"
	case "xlsx":
		return "xl/workbook.xml"
	}
	return "ppt/slides/slide<n>.xml"
}

// desiredOutput 计算预期输出路径 <outDir>/<净化后的名字>.pdf。净化规则用 fsutil（与 Edit 共用）；
// 需要时（Windows）把名字缩短到整条路径（含 .part 和最坏 "(99)" 重名后缀）不超过 259 个 UTF-16 单元。
func (s *Service) desiredOutput(in, outDir string) (string, error) {
	stemIn := strings.TrimSuffix(filepath.Base(in), filepath.Ext(in))
	stem := fsutil.SanitizeFileNameOr(stemIn, "document")
	if mp := s.maxPath(); mp > 0 {
		for fsutil.OutputPathLength(outDir, stem, ".pdf") > mp {
			rs := []rune(stem)
			if len(rs) <= 1 {
				return "", apperr.New(apperr.InvalidArgument, "输出路径太长").WithDetail(outDir)
			}
			stem = fsutil.SanitizeFileNameOr(string(rs[:len(rs)-1]), "document")
		}
	}
	return filepath.Join(outDir, stem+".pdf"), nil
}

func (s *Service) maxPath() int {
	if s.maxP < 0 {
		return 0
	}
	return s.maxP
}

// resolveOutputDir 解析输出目录：参数 > 设置里的默认目录 > 空（源文件同目录）。
// 必须是绝对路径，拒绝 \\?\ 和 \\.\ 前缀，不能在应用数据目录内，已存在的必须是文件夹。
func (s *Service) resolveOutputDir(ctx context.Context, dir string) (string, error) {
	if dir == "" && s.cfg.DefaultOutputDir != nil {
		dir = s.cfg.DefaultOutputDir(ctx)
	}
	if dir == "" {
		return "", nil
	}
	if strings.HasPrefix(dir, `\\?\`) || strings.HasPrefix(dir, `\\.\`) || strings.HasPrefix(dir, `//?/`) || strings.HasPrefix(dir, `//./`) {
		return "", apperr.New(apperr.InvalidArgument, "输出目录不能使用设备路径前缀").WithDetail(dir)
	}
	if !filepath.IsAbs(dir) {
		return "", apperr.New(apperr.InvalidArgument, "输出目录必须是绝对路径").WithDetail(dir)
	}
	dir = filepath.Clean(dir)
	if fi, err := os.Stat(dir); err == nil && !fi.IsDir() {
		return "", apperr.New(apperr.InvalidArgument, "输出位置不是文件夹").WithDetail(dir)
	}
	if s.cfg.DataDir != "" && insideDir(s.cfg.DataDir, dir) {
		return "", apperr.New(apperr.InvalidArgument, "输出目录不能在应用数据目录内").WithDetail("outputDir 不能在应用数据目录内\n" + dir)
	}
	if err := writableDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// insideDir 判断 p 是否等于 root 或在 root 之内（先 Clean，再对已存在的最长前缀做 EvalSymlinks；Windows / macOS 不区分大小写）。
func insideDir(root, p string) bool {
	r, q := realish(root), realish(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		r, q = strings.ToLower(r), strings.ToLower(q)
	}
	if q == r {
		return true
	}
	if !strings.HasSuffix(r, string(filepath.Separator)) {
		r += string(filepath.Separator)
	}
	return strings.HasPrefix(q, r)
}

// realish 解析路径里已存在部分的符号链接，不存在的尾部原样接上。
func realish(p string) string {
	p = filepath.Clean(p)
	rest := ""
	for cur := p; ; {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

func (s *Service) submitOne(j job) (task.Task, error) {
	pj, _ := json.Marshal(params{Input: j.in, OutputDir: j.dir})
	spec := task.Spec{
		Type:       task.TypeOfficePDF,
		Title:      filepath.Base(j.in) + " → PDF",
		InputPaths: []string{j.in},
		OutputPath: j.out,
		Params:     string(pj),
	}
	return s.cfg.Tasks.Submit(spec, s.newRunner(j))
}

func (s *Service) newRunner(j job) task.Runner {
	return task.RunnerFunc(func(ctx context.Context, report func(task.Progress)) (string, error) {
		logw := task.LogWriter(ctx)
		fmt.Fprintf(logw, "转换 %s → %s（仅文字，无图片和样式）\n", j.in, j.out)
		m, err := s.extract(ctx, j.in, j.ext)
		if err != nil {
			return "", err
		}
		choice, ok := s.fonts.choose(m.runes)
		if !ok {
			return "", errNoFont
		}
		return task.RunWithPart(ctx, j.out, func(part string) error {
			return renderPDF(ctx, m, choice, part, report, logw)
		})
	})
}

// retryFactory 用 Params 重建 Runner：重新校验输入（文件被删返回 NOT_FOUND，不产生新任务）和输出目录。
func (s *Service) retryFactory(old task.Task) (task.Runner, error) {
	var p params
	if err := json.Unmarshal([]byte(old.Params), &p); err != nil || p.Input == "" {
		return nil, apperr.New(apperr.InvalidArgument, "转换任务参数无效，无法重试")
	}
	ctx := context.Background()
	dir := ""
	if p.OutputDir != "" {
		var err error
		if dir, err = s.resolveOutputDirStrict(p.OutputDir); err != nil {
			return nil, err
		}
	}
	j, err := s.prepare(ctx, p.Input, dir)
	if err != nil {
		return nil, withPath(err, p.Input)
	}
	return s.newRunner(j), nil
}

// resolveOutputDirStrict 校验已解析好的输出目录（不再套用默认目录）。
func (s *Service) resolveOutputDirStrict(dir string) (string, error) {
	cp := *s
	cp.cfg.DefaultOutputDir = nil
	return cp.resolveOutputDir(context.Background(), dir)
}

// writableDir 检查输出目录可写：已存在的必须是可写目录（IO_ERROR），不存在的则最近的已存在上级必须是可写目录。
// 用创建再删除一个探测文件判断（比看权限位可靠：ACL、只读挂载、root 都能正确反映）。
func writableDir(dir string) error {
	cur := dir
	for {
		fi, err := os.Stat(cur)
		if err == nil {
			if !fi.IsDir() {
				return apperr.New(apperr.InvalidArgument, "输出位置不是文件夹").WithDetail(cur)
			}
			f, err := os.CreateTemp(cur, ".ffmpegfree-write-*")
			if err != nil {
				return apperr.Wrap(apperr.IOError, "输出目录不可写", err).WithDetail(cur + "\n" + err.Error())
			}
			name := f.Name()
			f.Close()
			os.Remove(name)
			return nil
		}
		if !os.IsNotExist(err) {
			return apperr.Wrap(apperr.IOError, "无法访问输出目录", err)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return apperr.New(apperr.IOError, "输出目录的上级不存在").WithDetail(dir)
		}
		cur = parent
	}
}
