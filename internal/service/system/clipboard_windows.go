//go:build windows

package system

import (
	"bytes"
	"image/png"
	"net/url"
	"os"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const cfHDROP = 15

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	getClipboardData = user32.NewProc("GetClipboardData")
	dragQueryFile    = windows.NewLazySystemDLL("shell32.dll").NewProc("DragQueryFileW")
)

// ClipboardFilePaths 读取剪贴板里复制的文件路径。没有文件时返回空列表。
func ClipboardFilePaths() []string {
	opened, _, _ := openClipboard.Call(0)
	if opened == 0 {
		return nil
	}
	defer closeClipboard.Call()
	handle, _, _ := getClipboardData.Call(cfHDROP)
	if handle == 0 {
		return nil
	}
	count, _, _ := dragQueryFile.Call(uintptr(handle), 0xFFFFFFFF, 0, 0)
	buf := make([]uint16, 32768)
	var paths []string
	for i := uintptr(0); i < count; i++ {
		n, _, _ := dragQueryFile.Call(uintptr(handle), i, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n == 0 {
			continue
		}
		path := windows.UTF16ToString(buf[:n])
		if path == "" {
			continue
		}
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	globalLock              = kernel32.NewProc("GlobalLock")
	globalUnlock            = kernel32.NewProc("GlobalUnlock")
	globalSize              = kernel32.NewProc("GlobalSize")
	registerClipboardFormat = user32.NewProc("RegisterClipboardFormatW")
)

// ClipboardImage 把剪贴板里的截图存成可打开的图片文件。QQ、微信复制的图通常在这里，不在文件列表里。
func ClipboardImage() string {
	opened, _, _ := openClipboard.Call(0)
	if opened == 0 {
		return ""
	}
	defer closeClipboard.Call()
	if path := imagePathFromHTML(clipboardBytes(clipboardFormat("HTML Format"))); path != "" {
		return path
	}
	if path := clipboardNamedFile(); path != "" {
		return path
	}
	enumFormats := user32.NewProc("EnumClipboardFormats")
	var format uintptr
	for {
		next, _, _ := enumFormats.Call(format)
		if next == 0 {
			break
		}
		format = next
		if path := saveSniffedImage(clipboardBytes(format)); path != "" {
			return path
		}
	}
	for _, format := range []uintptr{8, 17} {
		pngBytes, err := dibToPNG(clipboardBytes(format))
		if err != nil {
			continue
		}
		path, err := SavePastedFile("paste.png", pngBytes)
		if err == nil {
			return path
		}
	}
	return ""
}

func saveSniffedImage(raw []byte) string {
	data, ext, ok := sniffImage(raw)
	if !ok {
		return ""
	}
	if ext == ".png" {
		if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
			return ""
		}
	}
	path, err := SavePastedFile("paste"+ext, data)
	if err != nil {
		return ""
	}
	return path
}

func clipboardNamedFile() string {
	for _, name := range []string{"FileNameW", "FileName"} {
		raw := clipboardBytes(clipboardFormat(name))
		if len(raw) < 2 {
			continue
		}
		var path string
		if strings.HasSuffix(name, "W") {
			chars := unsafe.Slice((*uint16)(unsafe.Pointer(&raw[0])), len(raw)/2)
			path = windows.UTF16ToString(chars)
		} else if i := bytes.IndexByte(raw, 0); i > 0 {
			path = string(raw[:i])
		}
		if path == "" || !knownImageExt(path) {
			continue
		}
		st, err := os.Stat(path)
		if err != nil || st.IsDir() || st.Size() == 0 {
			continue
		}
		return path
	}
	return ""
}

var winImagePath = regexp.MustCompile(`(?i)[A-Za-z]:[\\/](?:[^\\/:*?"<>|\r\n]+[\\/])*[^\\/:*?"<>|\r\n]+\.(?:png|jpe?g|gif|webp|bmp)`)

func imagePathFromHTML(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	text := string(raw)
	for _, match := range winImagePath.FindAllString(text, 8) {
		path := strings.ReplaceAll(match, "/", `\`)
		st, err := os.Stat(path)
		if err == nil && !st.IsDir() && st.Size() > 0 {
			return path
		}
	}
	for _, token := range []string{"file:///", "file://"} {
		rest := text
		for {
			i := strings.Index(rest, token)
			if i < 0 {
				break
			}
			rest = rest[i:]
			end := strings.IndexAny(rest, "\"' \t\r\n<>")
			if end < 0 {
				end = len(rest)
			}
			if path := existingImagePath(rest[:end]); path != "" {
				return path
			}
			rest = rest[len(token):]
		}
	}
	return ""
}

func existingImagePath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Path == "" {
		return ""
	}
	path := parsed.Path
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	path = strings.ReplaceAll(path, "/", `\`)
	if !knownImageExt(path) {
		return ""
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() || st.Size() == 0 {
		return ""
	}
	return path
}

func clipboardFormat(name string) uintptr {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0
	}
	format, _, _ := registerClipboardFormat.Call(uintptr(unsafe.Pointer(ptr)))
	return format
}

func clipboardBytes(format uintptr) []byte {
	if format == 0 {
		return nil
	}
	handle, _, _ := getClipboardData.Call(format)
	if handle == 0 {
		return nil
	}
	ptr, _, _ := globalLock.Call(handle)
	if ptr == 0 {
		return nil
	}
	defer globalUnlock.Call(handle)
	size, _, _ := globalSize.Call(handle)
	if size == 0 || size > 32<<20 {
		return nil
	}
	out := make([]byte, size)
	copy(out, unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size))
	return out
}
