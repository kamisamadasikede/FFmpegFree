package langasr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
)

func TestValidateAndWrite(t *testing.T) {
	cues := []SubtitleCue{
		{ID: "1", Text: "你好", StartMs: 0, EndMs: 1000},
		{ID: "2", Text: "世界", StartMs: 1000, EndMs: 2000},
	}
	if err := ValidateCues(cues); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	srt := filepath.Join(dir, "a.srt")
	if err := WriteSubtitleFile(srt, "srt", cues); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(srt)
	if raw[0] == 0xEF { // BOM
		t.Fatal("must not have BOM")
	}
	if !strings.Contains(string(raw), "00:00:01,000") {
		t.Fatalf("%s", raw)
	}
	vtt := filepath.Join(dir, "a.vtt")
	if err := WriteSubtitleFile(vtt, "vtt", cues); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(vtt)
	if !strings.HasPrefix(string(raw), "WEBVTT\n") {
		t.Fatalf("%s", raw)
	}
}

func TestValidateRejects(t *testing.T) {
	if err := ValidateCues([]SubtitleCue{{Text: "a", StartMs: 10, EndMs: 10}}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
	long := strings.Repeat("字", 81)
	if err := ValidateCues([]SubtitleCue{{Text: long, StartMs: 0, EndMs: 1}}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
	if err := ValidateCues([]SubtitleCue{
		{Text: "a", StartMs: 0, EndMs: 100},
		{Text: "b", StartMs: 50, EndMs: 150},
	}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
}
