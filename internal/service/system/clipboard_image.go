package system

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/apperr"
)

func sniffImage(raw []byte) (data []byte, ext string, ok bool) {
	if len(raw) == 0 || len(raw) > 32<<20 {
		return nil, "", false
	}
	if i := bytes.Index(raw, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}); i >= 0 && i < 512 {
		return raw[i:], ".png", true
	}
	if len(raw) >= 3 && raw[0] == 0xFF && raw[1] == 0xD8 && raw[2] == 0xFF {
		return raw, ".jpg", true
	}
	if len(raw) >= 6 && (string(raw[:6]) == "GIF87a" || string(raw[:6]) == "GIF89a") {
		return raw, ".gif", true
	}
	if len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP" {
		return raw, ".webp", true
	}
	return nil, "", false
}

func knownImageExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

// SavePastedFile 把粘贴进来的图片字节写成临时文件，返回绝对路径。
func SavePastedFile(name string, data []byte) (string, error) {
	data, ext, ok := sniffImage(data)
	if !ok {
		return "", apperr.New(apperr.InvalidArgument, "不是图片")
	}
	if name != "" && knownImageExt(name) && strings.EqualFold(filepath.Ext(name), ext) {
		// 保留调用方已经对上的扩展名。
	} else {
		name = "paste" + ext
	}
	ext = filepath.Ext(name)
	dir := filepath.Join(os.TempDir(), "ffc-paste")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "paste-*"+ext)
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func SavePastedFileBase64(name, dataBase64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return "", apperr.New(apperr.InvalidArgument, "图片数据无效")
	}
	return SavePastedFile(name, raw)
}

func dibToPNG(dib []byte) ([]byte, error) {
	if len(dib) < 40 {
		return nil, apperr.New(apperr.InvalidArgument, "图片数据无效")
	}
	headerSize := int(le32(dib[0:4]))
	if headerSize < 40 || headerSize > len(dib) {
		return nil, apperr.New(apperr.InvalidArgument, "图片数据无效")
	}
	width := int(int32(le32(dib[4:8])))
	heightRaw := int32(le32(dib[8:12]))
	topDown := heightRaw < 0
	height := int(heightRaw)
	if topDown {
		height = -height
	}
	if width <= 0 || height <= 0 || width > 20000 || height > 20000 {
		return nil, apperr.New(apperr.InvalidArgument, "图片数据无效")
	}
	bitCount := int(le16(dib[14:16]))
	compression := le32(dib[16:20])
	if (compression != 0 && compression != 3) || (bitCount != 24 && bitCount != 32) {
		return nil, apperr.New(apperr.InvalidArgument, "图片数据无效")
	}
	pixelOff := headerSize
	if compression == 3 && headerSize == 40 {
		pixelOff += 12
	}
	if pixelOff > len(dib) {
		return nil, apperr.New(apperr.InvalidArgument, "图片数据无效")
	}
	pixels := dib[pixelOff:]
	stride := ((width*bitCount/8 + 3) / 4) * 4
	if stride <= 0 || len(pixels) < stride*height {
		return nil, apperr.New(apperr.InvalidArgument, "图片数据无效")
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	bpp := bitCount / 8
	redAt, greenAt, blueAt := 2, 1, 0
	if compression == 3 && pixelOff >= 12 {
		masks := dib[pixelOff-12 : pixelOff]
		redAt, greenAt, blueAt = maskByte(le32(masks[0:4])), maskByte(le32(masks[4:8])), maskByte(le32(masks[8:12]))
	}
	for y := 0; y < height; y++ {
		srcY := y
		if !topDown {
			srcY = height - 1 - y
		}
		row := pixels[srcY*stride:]
		for x := 0; x < width; x++ {
			i := x * bpp
			r, g, b := dibByte(row, i, redAt, bpp), dibByte(row, i, greenAt, bpp), dibByte(row, i, blueAt, bpp)
			a := byte(255)
			if bitCount == 32 && row[i+3] != 0 {
				a = row[i+3]
			}
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
		}
	}
	if !imageHasColor(img) {
		return nil, apperr.New(apperr.InvalidArgument, "图片数据无效")
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func dibByte(row []byte, i, which, bpp int) byte {
	if which < 0 || which >= bpp || i+which >= len(row) {
		return 0
	}
	return row[i+which]
}

func maskByte(mask uint32) int {
	switch mask {
	case 0x000000FF:
		return 0
	case 0x0000FF00:
		return 1
	case 0x00FF0000:
		return 2
	default:
		return 2
	}
}

func imageHasColor(img *image.NRGBA) bool {
	b := img.Bounds()
	stepX, stepY := 1, 1
	if b.Dx() > 48 {
		stepX = b.Dx() / 48
	}
	if b.Dy() > 48 {
		stepY = b.Dy() / 48
	}
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			c := img.NRGBAAt(x, y)
			if c.R > 8 || c.G > 8 || c.B > 8 {
				return true
			}
		}
	}
	return false
}

func le16(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }
func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
