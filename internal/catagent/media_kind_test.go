package catagent

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMediaKind(t *testing.T) {
	kind, mime, ok := MediaKind("A.MP4")
	if !ok || kind != "video" || mime != "video/mp4" {
		t.Fatalf("%s %s %v", kind, mime, ok)
	}
	if _, _, ok := MediaKind("main.go"); ok {
		t.Fatal("go file")
	}
	if _, _, ok := MediaKind("note.docx"); !ok {
		t.Fatal("docx")
	}
}

func TestClassifyPreviewPath_Device(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("device names")
	}
	if _, _, _, err := ClassifyPreviewPath(`C:\nul.png`); err == nil || !strings.Contains(err.Error(), "设备名") {
		t.Fatal(err)
	}
	if _, _, _, err := ClassifyPreviewPath(filepath.Join("relative", "a.png")); err == nil || !strings.Contains(err.Error(), "本机上的文件") {
		t.Fatal(err)
	}
}
