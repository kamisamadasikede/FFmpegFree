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
	for _, name := range []string{"a.md", "a.markdown", "a.mdown", "a.mkd"} {
		if k, _, ok := MediaKind(name); !ok || k != "markdown" {
			t.Fatalf("%s %s", name, k)
		}
	}
	if k, _, ok := MediaKind("a.txt"); !ok || k != "text" {
		t.Fatal(k)
	}
	if k, _, ok := MediaKind("a.csv"); !ok || k != "csv" {
		t.Fatal(k)
	}
	if _, _, ok := MediaKind("a.doc"); ok {
		t.Fatal("doc")
	}
	if _, _, ok := MediaKind("a.rtf"); ok {
		t.Fatal("rtf")
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
