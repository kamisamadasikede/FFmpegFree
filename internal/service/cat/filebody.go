package cat

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
)

// 文本一次读完的上限。再大就整份不返回，避免把字符从中间切开。
const maxCatTextBytes = 256 * 1024

// 图片、文档、表格和媒体一次读完的上限。
const maxCatBinaryBytes = 8 * 1024 * 1024

const (
	kindText     = "text"
	kindImage    = "image"
	kindMedia    = "media"
	kindPDF      = "pdf"
	kindDocx     = "docx"
	kindXlsx     = "xlsx"
	kindPptx     = "pptx"
	kindDoc      = "doc"
	kindBinary   = "binary"
	kindTooLarge = "tooLarge"
)

const (
	msgFileReadFailed  = "文件读取失败，请重试。"
	msgFileWriteFailed = "文件保存失败，请重试。"
	msgFileTooBig      = "这个文件太大，不能在这里修改。"
	msgFileNotEditable = "这个文件不能在这里修改。"
	msgFileTextOnly    = "只能保存文本。"
	msgFileNotFound    = "找不到这个文件。"
	msgFileIsDir       = "这是一个文件夹。"
)

func errFileNotFound() error { return apperr.New(apperr.NotFound, msgFileNotFound) }
func errFileIsDir() error    { return apperr.New(apperr.InvalidArgument, msgFileIsDir) }
func errFileTooBig() error   { return apperr.New(apperr.InvalidArgument, msgFileTooBig) }
func errFileNotEditable() error {
	return apperr.New(apperr.InvalidArgument, msgFileNotEditable)
}
func errFileTextOnly() error { return apperr.New(apperr.InvalidArgument, msgFileTextOnly) }

// ReadCatFile 读取对话根下的一个文件，供侧边预览。不跟随符号链接。
func (s *Service) ReadCatFile(ctx context.Context, req ReadFileRequest) (ReadFileResult, error) {
	abs, rel, info, err := s.resolveCatFile(ctx, req.ConvID, req.RelPath, msgFileReadFailed)
	if err != nil {
		return ReadFileResult{}, err
	}
	kind, mime := kindFromName(info.Name())
	out := ReadFileResult{
		RelPath: rel,
		Name:    info.Name(),
		Size:    info.Size(),
		ModTime: modMilli(info),
	}
	if kind == kindDoc {
		out.Kind = kindDoc
		return out, nil
	}
	capn := maxCatTextBytes
	if knownBinaryKind(kind) {
		capn = maxCatBinaryBytes
	}
	if info.Size() > int64(capn) {
		out.Kind = kindTooLarge
		return out, nil
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return ReadFileResult{}, errFileNotFound()
		}
		return ReadFileResult{}, apperr.Wrap(apperr.IOError, msgFileReadFailed, err)
	}
	if int64(len(b)) > int64(capn) {
		out.Kind = kindTooLarge
		out.Size = int64(len(b))
		return out, nil
	}
	out.Size = int64(len(b))
	if knownBinaryKind(kind) {
		out.Kind = kind
		out.Mime = mime
		out.DataBase64 = base64.StdEncoding.EncodeToString(b)
		return out, nil
	}
	if isProbablyBinary(b) || !utf8.Valid(b) {
		out.Kind = kindBinary
		return out, nil
	}
	out.Kind = kindText
	out.Mime = mime
	if out.Mime == "" {
		out.Mime = "text/plain"
	}
	out.Language = languageOf(info.Name())
	out.Content = string(b)
	out.Editable = true
	return out, nil
}

// WriteCatFile 覆盖对话根下已有的文本文件。不新建、不跟随符号链接。
func (s *Service) WriteCatFile(ctx context.Context, req WriteFileRequest) (WriteFileResult, error) {
	abs, rel, info, err := s.resolveCatFile(ctx, req.ConvID, req.RelPath, msgFileWriteFailed)
	if err != nil {
		return WriteFileResult{}, err
	}
	kind, _ := kindFromName(info.Name())
	if knownBinaryKind(kind) {
		return WriteFileResult{}, errFileNotEditable()
	}
	if info.Size() > int64(maxCatTextBytes) {
		return WriteFileResult{}, errFileTooBig()
	}
	existing, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return WriteFileResult{}, errFileNotFound()
		}
		return WriteFileResult{}, apperr.Wrap(apperr.IOError, msgFileWriteFailed, err)
	}
	if int64(len(existing)) > int64(maxCatTextBytes) || isProbablyBinary(existing) || !utf8.Valid(existing) {
		if int64(len(existing)) > int64(maxCatTextBytes) {
			return WriteFileResult{}, errFileTooBig()
		}
		return WriteFileResult{}, errFileNotEditable()
	}
	if len(req.Content) > maxCatTextBytes {
		return WriteFileResult{}, errFileTooBig()
	}
	if strings.Contains(req.Content, "\x00") || !utf8.ValidString(req.Content) {
		return WriteFileResult{}, errFileTextOnly()
	}
	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return WriteFileResult{}, errFileNotFound()
		}
		return WriteFileResult{}, apperr.Wrap(apperr.IOError, msgFileWriteFailed, err)
	}
	if _, werr := f.Write([]byte(req.Content)); werr != nil {
		f.Close()
		return WriteFileResult{}, apperr.Wrap(apperr.IOError, msgFileWriteFailed, werr)
	}
	if err := f.Close(); err != nil {
		return WriteFileResult{}, apperr.Wrap(apperr.IOError, msgFileWriteFailed, err)
	}
	out := WriteFileResult{RelPath: rel, Size: int64(len(req.Content)), ModTime: modMilli(info)}
	if st, lerr := os.Lstat(abs); lerr == nil && st.Mode()&os.ModeSymlink == 0 {
		out.Size = st.Size()
		out.ModTime = modMilli(st)
	}
	return out, nil
}

func (s *Service) resolveCatFile(ctx context.Context, convID, rel, ioMsg string) (string, string, os.FileInfo, error) {
	c, err := s.requireConv(ctx, convID)
	if err != nil {
		return "", "", nil, err
	}
	root, err := s.conversationRoot(ctx, c)
	if err != nil {
		return "", "", nil, err
	}
	return resolveCatFile(root, rel, ioMsg)
}

// resolveCatFile 把相对路径解析成根下的普通文件。任何一段是符号链接都拒绝。
func resolveCatFile(root, rel, ioMsg string) (string, string, os.FileInfo, error) {
	root = filepath.Clean(root)
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return "", "", nil, errFileIsDir()
	}
	if strings.Contains(rel, ":") || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", "", nil, errFilePath()
	}
	slash := strings.ReplaceAll(rel, `\`, "/")
	if strings.HasPrefix(slash, "/") {
		return "", "", nil, errFilePath()
	}
	for _, seg := range strings.Split(slash, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", "", nil, errFilePath()
		}
	}
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(slash)))
	relOut, err := filepath.Rel(root, target)
	if err != nil || !lexicalInside(root, target) || relOut == "." {
		return "", "", nil, errFilePath()
	}
	var segs []string
	for _, seg := range strings.Split(relOut, string(filepath.Separator)) {
		if seg != "" && seg != "." {
			segs = append(segs, seg)
		}
	}
	if len(segs) == 0 {
		return "", "", nil, errFilePath()
	}
	cur := root
	parent := ""
	var info os.FileInfo
	for i, seg := range segs {
		cur = filepath.Join(cur, seg)
		fi, lerr := os.Lstat(cur)
		if lerr != nil {
			if os.IsNotExist(lerr) {
				return "", "", nil, errFileNotFound()
			}
			return "", "", nil, apperr.Wrap(apperr.IOError, ioMsg, lerr)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", "", nil, errFilePath()
		}
		last := i == len(segs)-1
		if !last {
			if !fi.IsDir() {
				return "", "", nil, errFileNotFound()
			}
		} else if fi.IsDir() {
			return "", "", nil, errFileIsDir()
		} else if !fi.Mode().IsRegular() {
			return "", "", nil, errFilePath()
		}
		if parent == "" {
			parent = seg
		} else {
			parent += "/" + seg
		}
		info = fi
	}
	return cur, parent, info, nil
}

func modMilli(fi os.FileInfo) int64 {
	if fi == nil || fi.ModTime().IsZero() {
		return 0
	}
	return fi.ModTime().UnixMilli()
}

// isProbablyBinary 空内容当文本；出现 0 字节，或控制字节超过样本的 30%，当二进制。
func isProbablyBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	sample := b
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	controls := 0
	for _, c := range sample {
		if c == 0 {
			return true
		}
		if c < 32 && c != '\t' && c != '\n' && c != '\f' && c != '\r' {
			controls++
		}
	}
	if len(b) > len(sample) && bytes.IndexByte(b, 0) >= 0 {
		return true
	}
	return controls*100 > len(sample)*30
}

func knownBinaryKind(kind string) bool {
	switch kind {
	case kindImage, kindMedia, kindPDF, kindDocx, kindXlsx, kindPptx, kindDoc:
		return true
	default:
		return false
	}
}

func kindFromName(name string) (string, string) {
	ext := strings.ToLower(filepath.Ext(name))
	if mime, ok := imageMimes[ext]; ok {
		return kindImage, mime
	}
	if mime, ok := mediaMimes[ext]; ok {
		return kindMedia, mime
	}
	switch ext {
	case ".pdf":
		return kindPDF, "application/pdf"
	case ".docx":
		return kindDocx, "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return kindXlsx, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".xlsm":
		return kindXlsx, "application/vnd.ms-excel.sheet.macroEnabled.12"
	case ".xls":
		return kindXlsx, "application/vnd.ms-excel"
	case ".pptx":
		return kindPptx, "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".doc":
		return kindDoc, ""
	case ".svg":
		return kindText, "image/svg+xml"
	default:
		return "", ""
	}
}

func languageOf(name string) string {
	base := strings.ToLower(filepath.Base(name))
	if base == "dockerfile" {
		return "dockerfile"
	}
	if lang, ok := textLang[strings.ToLower(filepath.Ext(name))]; ok {
		return lang
	}
	return "plaintext"
}

var imageMimes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".avif": "image/avif",
}

var mediaMimes = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".m4a":  "audio/mp4",
	".ogg":  "audio/ogg",
	".opus": "audio/ogg",
	".flac": "audio/flac",
	".weba": "audio/webm",
}

var textLang = map[string]string{
	".go":        "go",
	".py":        "python",
	".js":        "javascript",
	".jsx":       "javascript",
	".mjs":       "javascript",
	".cjs":       "javascript",
	".ts":        "typescript",
	".tsx":       "typescript",
	".vue":       "html",
	".json":      "json",
	".jsonc":     "json",
	".yml":       "yaml",
	".yaml":      "yaml",
	".md":        "markdown",
	".markdown":  "markdown",
	".html":      "html",
	".htm":       "html",
	".css":       "css",
	".scss":      "scss",
	".xml":       "xml",
	".svg":       "xml",
	".sh":        "shell",
	".bash":      "shell",
	".zsh":       "shell",
	".sql":       "sql",
	".java":      "java",
	".c":         "cpp",
	".h":         "cpp",
	".cc":        "cpp",
	".cpp":       "cpp",
	".hpp":       "cpp",
	".cs":        "csharp",
	".rs":        "rust",
	".toml":      "ini",
	".ini":       "ini",
	".txt":       "plaintext",
	".log":       "plaintext",
	".srt":       "plaintext",
	".rb":        "ruby",
	".php":       "php",
	".lua":       "lua",
	".kt":        "kotlin",
	".swift":     "swift",
	".ps1":       "powershell",
	".bat":       "bat",
	".cmd":       "bat",
	".gitignore": "plaintext",
	".env":       "plaintext",
	".mod":       "go",
	".sum":       "plaintext",
}
