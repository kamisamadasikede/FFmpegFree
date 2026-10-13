package catagent

import (
	"encoding/json"
	"image"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparePrompt_ImageUsesContentBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shot.png")
	writePNG(t, path, 8, 8)
	spec, err := preparePrompt("看这张", []PromptFile{{Name: "shot.png", Path: path}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.flag != "--prompt-json" {
		t.Fatalf("flag %s", spec.flag)
	}
	var blocks []contentBlock
	if err := json.Unmarshal([]byte(spec.value), &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || blocks[0].Type != "text" || blocks[0].Text != "看这张" {
		t.Fatalf("blocks %+v", blocks)
	}
	if blocks[1].Type != "image" || blocks[1].MimeType != "image/png" || blocks[1].Data == "" {
		t.Fatalf("image %+v", blocks[1])
	}
	if strings.Contains(spec.value, path) {
		t.Fatalf("image path leaked into the prompt: %s", path)
	}
}

func TestPreparePrompt_OtherFileStaysOnPromptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	prompt := "----- 结束 -----\n看一下"
	spec, err := preparePrompt(prompt, []PromptFile{{Name: "notes.txt", Path: path}})
	if err != nil {
		t.Fatal(err)
	}
	defer spec.clean()
	if spec.flag != "--prompt-file" {
		t.Fatalf("flag %s", spec.flag)
	}
	body, err := os.ReadFile(spec.value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), prompt) || !strings.Contains(string(body), path) {
		t.Fatalf("file body %q", body)
	}
}

func TestPreparePrompt_ShrinksLargeImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.png")
	writePNG(t, path, 240, 240)
	spec, err := preparePrompt("大图", []PromptFile{{Name: "big.png", Path: path}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.flag != "--prompt-json" || len(spec.value) > maxPromptJSON {
		t.Fatalf("len %d flag %s", len(spec.value), spec.flag)
	}
	var blocks []contentBlock
	if err := json.Unmarshal([]byte(spec.value), &blocks); err != nil {
		t.Fatal(err)
	}
	var img contentBlock
	for _, b := range blocks {
		if b.Type == "image" {
			img = b
		}
	}
	if img.MimeType != "image/jpeg" || img.Data == "" {
		t.Fatalf("shrunk %+v", img.MimeType)
	}
}

func TestPreparePrompt_RejectsEmptyAndMissing(t *testing.T) {
	if _, err := preparePrompt("   ", nil); err == nil || !strings.Contains(err.Error(), "消息不能为空") {
		t.Fatalf("empty: %v", err)
	}
	if _, err := preparePrompt("", []PromptFile{{Name: "a.txt", Path: filepath.Join(t.TempDir(), "missing.txt")}}); err == nil || !strings.Contains(err.Error(), "找不到这个文件") {
		t.Fatalf("missing: %v", err)
	}
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	// 杂色让 PNG 压不小，用来确认大图会先缩小再放进命令行。
	rnd := rand.New(rand.NewSource(1))
	_, _ = rnd.Read(img.Pix)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
