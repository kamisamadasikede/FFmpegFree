package catagent

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"FFmpegFree/internal/apperr"
)

const (
	// maxAttachFiles 是一条消息能带的文件数。再多输入框也放不下。
	maxAttachFiles = 20
	// maxAttachBytes 是单个文件读进内存的上限。再大就拒绝，避免一次读进上百兆。
	maxAttachBytes int64 = 15 << 20
	// maxPromptJSON 是内容块 JSON 的上限。Windows 一条命令行最长 32767，
	// 还要给程序路径、工作目录和其它开关留地方。装不下的图片会先缩小再放进去。
	maxPromptJSON = 24000
)

// PromptFile 是这一轮要一起发出去的本机文件。
type PromptFile struct {
	Name string
	Path string
}

type promptSpec struct {
	flag  string
	value string
	clean func()
}

type loadedImg struct {
	name string
	mime string
	raw  []byte
	orig []byte
	step int
}

type contentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// 缩小阶梯：边长递减，同时降低 JPEG 质量。原图能放下时不走这里。
var shrinkPlan = []struct {
	edge    int
	quality int
}{
	{1600, 72},
	{1280, 64},
	{960, 56},
	{720, 48},
	{480, 40},
	{360, 32},
	{240, 26},
	{160, 22},
	{96, 18},
}

// NormalizePromptFiles 检查路径：必须是本机上的普通文件，去掉重复，拒绝设备名和过大的文件。
func NormalizePromptFiles(files []PromptFile) ([]PromptFile, error) {
	if len(files) == 0 {
		return nil, nil
	}
	if len(files) > maxAttachFiles {
		return nil, apperr.New(apperr.InvalidArgument, "一次最多 20 个文件")
	}
	out := make([]PromptFile, 0, len(files))
	seen := map[string]struct{}{}
	for _, f := range files {
		p := filepath.Clean(strings.TrimSpace(f.Path))
		if p == "" || !filepath.IsAbs(p) {
			return nil, apperr.New(apperr.InvalidArgument, "请选择本机上的文件")
		}
		if reservedDevicePath(p) {
			return nil, apperr.New(apperr.InvalidArgument, filepath.Base(p)+" 在 Windows 上是设备名，没法发送。")
		}
		st, err := os.Stat(p)
		if err != nil || st.IsDir() || !st.Mode().IsRegular() {
			return nil, apperr.New(apperr.InvalidArgument, "找不到这个文件")
		}
		if st.Size() > maxAttachBytes {
			return nil, apperr.New(apperr.InvalidArgument, "这个文件太大")
		}
		key := p
		if runtime.GOOS == "windows" {
			key = strings.ToLower(p)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		name := strings.TrimSpace(f.Name)
		if name == "" || name == "." || name == string(filepath.Separator) {
			name = filepath.Base(p)
		}
		out = append(out, PromptFile{Name: name, Path: p})
	}
	return out, nil
}

// preparePrompt 把这一轮收成一个开关。
// 没有图片时仍写临时文件，用 --prompt-file（以 - 开头的句子不能直接当参数）。
// 有图片时收成内容块，用 --prompt-json：图片是 data + mimeType，其它文件把绝对路径写进文字。
func preparePrompt(text string, files []PromptFile) (promptSpec, error) {
	files, err := NormalizePromptFiles(files)
	if err != nil {
		return promptSpec{}, err
	}
	var images []loadedImg
	var paths []string
	for _, f := range files {
		mime, ok := imageMIME(f.Name)
		if !ok {
			mime, ok = imageMIME(f.Path)
		}
		if !ok {
			paths = append(paths, f.Path)
			continue
		}
		raw, rerr := os.ReadFile(f.Path)
		if rerr != nil {
			return promptSpec{}, apperr.New(apperr.InvalidArgument, "找不到这个文件")
		}
		images = append(images, loadedImg{name: f.Name, mime: mime, raw: raw, orig: raw, step: -1})
	}
	body := joinPrompt(text, paths)
	if strings.TrimSpace(body) == "" && len(images) == 0 {
		return promptSpec{}, apperr.New(apperr.InvalidArgument, "消息不能为空")
	}
	if len(images) == 0 {
		path, werr := writePromptFile(body)
		if werr != nil {
			return promptSpec{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, werr).WithDetail("reason=prompt_file")
		}
		return promptSpec{flag: "--prompt-file", value: path, clean: func() { _ = os.Remove(path) }}, nil
	}
	payload, err := imagePromptJSON(body, images)
	if err != nil {
		return promptSpec{}, err
	}
	return promptSpec{flag: "--prompt-json", value: payload}, nil
}

func joinPrompt(text string, paths []string) string {
	if len(paths) == 0 {
		return text
	}
	text = strings.TrimRight(text, "\n")
	var b strings.Builder
	b.WriteString(text)
	for _, p := range paths {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(p)
	}
	return b.String()
}

func imagePromptJSON(text string, images []loadedImg) (string, error) {
	if raw, err := marshalPrompt(text, nil); err == nil && len(raw) > maxPromptJSON {
		return "", apperr.New(apperr.InvalidArgument, "这条消息太长，带不了图片")
	}
	parts := append([]loadedImg(nil), images...)
	for n := 0; n < len(shrinkPlan)*len(parts)+2; n++ {
		raw, err := marshalPrompt(text, parts)
		if err != nil {
			return "", err
		}
		if len(raw) <= maxPromptJSON {
			return string(raw), nil
		}
		idx := largestShrinkable(parts)
		if idx < 0 {
			return "", apperr.New(apperr.InvalidArgument, "图片太大，这一次带不下")
		}
		next, mime, serr := shrinkStep(parts[idx].orig, parts[idx].step+1)
		if serr != nil {
			parts[idx].step = len(shrinkPlan)
			continue
		}
		parts[idx].step++
		parts[idx].raw = next
		parts[idx].mime = mime
	}
	return "", apperr.New(apperr.InvalidArgument, "图片太大，这一次带不下")
}

func marshalPrompt(text string, parts []loadedImg) ([]byte, error) {
	blocks := make([]contentBlock, 0, len(parts)+1)
	if strings.TrimSpace(text) != "" {
		blocks = append(blocks, contentBlock{Type: "text", Text: text})
	}
	for _, p := range parts {
		blocks = append(blocks, contentBlock{
			Type:     "image",
			Data:     base64.StdEncoding.EncodeToString(p.raw),
			MimeType: p.mime,
		})
	}
	if len(blocks) == 0 {
		return nil, apperr.New(apperr.InvalidArgument, "消息不能为空")
	}
	return json.Marshal(blocks)
}

func largestShrinkable(parts []loadedImg) int {
	idx := -1
	best := 0
	for i, p := range parts {
		if p.step >= len(shrinkPlan)-1 {
			continue
		}
		if len(p.raw) > best {
			best = len(p.raw)
			idx = i
		}
	}
	return idx
}

func shrinkStep(orig []byte, step int) ([]byte, string, error) {
	if step < 0 || step >= len(shrinkPlan) {
		return nil, "", apperr.New(apperr.InvalidArgument, "图片太大，这一次带不下")
	}
	img, _, err := image.Decode(bytes.NewReader(orig))
	if err != nil {
		return nil, "", err
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 || b.Dx()*b.Dy() > 24_000_000 {
		return nil, "", apperr.New(apperr.InvalidArgument, "图片太大，这一次带不下")
	}
	plan := shrinkPlan[step]
	dst := downscale(img, plan.edge)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: plan.quality}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/jpeg", nil
}

func downscale(src image.Image, maxEdge int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	edge := w
	if h > edge {
		edge = h
	}
	if maxEdge < 1 || edge <= maxEdge {
		return src
	}
	nw := w * maxEdge / edge
	nh := h * maxEdge / edge
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy0 := b.Min.Y + y*h/nh
		sy1 := b.Min.Y + (y+1)*h/nh
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < nw; x++ {
			sx0 := b.Min.X + x*w/nw
			sx1 := b.Min.X + (x+1)*w/nw
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, a, n uint32
			for yy := sy0; yy < sy1; yy++ {
				for xx := sx0; xx < sx1; xx++ {
					cr, cg, cb, ca := src.At(xx, yy).RGBA()
					r += cr
					g += cg
					bl += cb
					a += ca
					n++
				}
			}
			if n == 0 {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8((r / n) >> 8),
				G: uint8((g / n) >> 8),
				B: uint8((bl / n) >> 8),
				A: uint8((a / n) >> 8),
			})
		}
	}
	return dst
}

func imageMIME(name string) (string, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".gif":
		return "image/gif", true
	case ".webp":
		return "image/webp", true
	default:
		return "", false
	}
}

func reservedDevicePath(p string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	name := p
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimRight(name, ". ")
	if dot := strings.IndexByte(name, '.'); dot >= 0 {
		name = name[:dot]
	}
	switch name {
	case "con", "prn", "aux", "nul":
		return true
	}
	if len(name) == 4 {
		head := name[:3]
		if (head == "com" || head == "lpt") && name[3] >= '1' && name[3] <= '9' {
			return true
		}
	}
	return false
}
