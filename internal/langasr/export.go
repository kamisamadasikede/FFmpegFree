package langasr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
)

const MaxCueRunes = 80

// ValidateCues 硬校验（契约 6.18.6）：endMs>startMs、不重叠、单条 ≤80 字。
func ValidateCues(cues []SubtitleCue) error {
	if len(cues) == 0 {
		return apperr.New(apperr.InvalidArgument, "没有可导出的字幕").WithDetail("reason=cue_invalid")
	}
	for i, c := range cues {
		if c.EndMs <= c.StartMs {
			return apperr.New(apperr.InvalidArgument, "字幕时间不正确").WithDetail(fmt.Sprintf("reason=cue_invalid\nindex=%d", i))
		}
		if utf8.RuneCountInString(c.Text) > MaxCueRunes {
			return apperr.New(apperr.InvalidArgument, "单条字幕不能超过 80 个字").WithDetail(fmt.Sprintf("reason=cue_invalid\nindex=%d", i))
		}
	}
	for i := 0; i < len(cues); i++ {
		for j := i + 1; j < len(cues); j++ {
			if overlaps(cues[i], cues[j]) {
				return apperr.New(apperr.InvalidArgument, "字幕时间不能重叠").WithDetail(fmt.Sprintf("reason=cue_invalid\nindex=%d,%d", i, j))
			}
		}
	}
	return nil
}

func overlaps(a, b SubtitleCue) bool {
	return a.StartMs < b.EndMs && b.StartMs < a.EndMs
}

// FormatSRT / FormatVTT 生成 UTF-8（无 BOM）文本。
func FormatSRT(cues []SubtitleCue) string {
	var b strings.Builder
	for i, c := range cues {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, srtTime(c.StartMs), srtTime(c.EndMs), c.Text)
	}
	return b.String()
}

func FormatVTT(cues []SubtitleCue) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, c := range cues {
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n", vttTime(c.StartMs), vttTime(c.EndMs), c.Text)
	}
	return b.String()
}

func srtTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	d := time.Duration(ms) * time.Millisecond
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	milli := int(d.Milliseconds()) % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, milli)
}

func vttTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	d := time.Duration(ms) * time.Millisecond
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	milli := int(d.Milliseconds()) % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, milli)
}

// WriteSubtitleFile 校验并写入 SRT/VTT（UTF-8 无 BOM）。
func WriteSubtitleFile(path, format string, cues []SubtitleCue) error {
	if err := ValidateCues(cues); err != nil {
		return err
	}
	format = strings.ToLower(strings.TrimSpace(format))
	var body string
	switch format {
	case "srt":
		body = FormatSRT(cues)
	case "vtt":
		body = FormatVTT(cues)
	default:
		return apperr.New(apperr.InvalidArgument, "字幕格式不正确").WithDetail("format=" + format)
	}
	if path == "" || !filepath.IsAbs(path) {
		return apperr.New(apperr.InvalidArgument, "保存路径必须是绝对路径").WithDetail(path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return apperr.Wrap(apperr.IOError, "无法创建文件夹", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return apperr.Wrap(apperr.IOError, "保存字幕失败", err)
	}
	return nil
}
