package cat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/localassets"
)

func TestPreviewCatMedia_Image(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(path, []byte{0x89, 0x50}, 0o644); err != nil {
		t.Fatal(err)
	}
	reg := localassets.New(localassets.Config{})
	s := New(Config{Preview: reg})
	got, err := s.PreviewCatMedia(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "image" || got.Mime != "image/png" || got.Name != "shot.png" || !strings.HasPrefix(got.URL, "/local/") {
		t.Fatalf("%+v", got)
	}
	again, err := s.PreviewCatMedia(context.Background(), path)
	if err != nil || again.URL != got.URL {
		t.Fatalf("reuse %+v %v", again, err)
	}
}

func TestPreviewCatMedia_Rejects(t *testing.T) {
	s := New(Config{Preview: localassets.New(localassets.Config{})})
	if _, err := s.PreviewCatMedia(context.Background(), filepath.Join(t.TempDir(), "a.go")); err == nil || !strings.Contains(err.Error(), "不能在这里预览") {
		t.Fatal(err)
	}
	if _, err := s.PreviewCatMedia(context.Background(), filepath.Join(t.TempDir(), "missing.png")); err == nil || !strings.Contains(err.Error(), "找不到这个文件") {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(path, []byte{1}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{}).PreviewCatMedia(context.Background(), path); err == nil || !strings.Contains(err.Error(), "还不能预览") {
		t.Fatal(err)
	}
}
