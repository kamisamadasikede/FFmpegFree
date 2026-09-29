package system

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"FFmpegFree/internal/apperr"
)

func TestFileFilterNormalize(t *testing.T) {
	ok := map[string][]string{
		"分号":      {"*.mp4;*.mkv"},
		"列表":      {"*.mp4", "*.mkv"},
		"混合去重大小写": {"*.mp4;*.MKV", "*.mkv", " *.mp4 "},
	}
	for name, pats := range ok {
		got, err := FileFilter{Patterns: pats}.NormalizePatterns()
		if err != nil || len(got) != 2 || got[0] != "*.mp4" {
			t.Errorf("%s: %v %v", name, got, err)
		}
	}
	if got, err := (FileFilter{Patterns: []string{"*.mp4", "*.*"}}).NormalizePatterns(); err != nil || got != nil {
		t.Errorf("包含 *.* 应等于不过滤: %v %v", got, err)
	}
	if got, err := (FileFilter{}).NormalizePatterns(); err != nil || got != nil {
		t.Errorf("空过滤器: %v %v", got, err)
	}
	if got, _ := (FileFilter{Patterns: []string{"*.mp?", "*.h264+"}}).NormalizePatterns(); len(got) != 2 {
		t.Errorf("? 和 + 应允许: %v", got)
	}
	for _, bad := range []string{"mp4", "*.", "*.mp4/../x", "*.m p4", `*.mp4"`, "/etc/*", "*.mp4|*.mkv", "*.中文", "**"} {
		if _, err := (FileFilter{Patterns: []string{bad}}).NormalizePatterns(); !apperr.Is(err, apperr.InvalidArgument) {
			t.Errorf("%q 应 INVALID_ARGUMENT: %v", bad, err)
		}
	}
	many := make([]string, maxFilterPatterns+1)
	for i := range many {
		many[i] = "*.a"
	}
	if _, err := (FileFilter{Patterns: many}).NormalizePatterns(); !apperr.Is(err, apperr.InvalidArgument) {
		t.Error("过多的模式应拒绝")
	}
}

func TestFileFilterDialogPattern(t *testing.T) {
	n, p, ok, err := FileFilter{Name: "视频", Patterns: []string{"*.mp4;*.mkv"}}.DialogPattern()
	if err != nil || !ok || n != "视频" || p != "*.mp4;*.mkv" {
		t.Fatalf("%q %q %v %v", n, p, ok, err)
	}
	n, p, ok, _ = FileFilter{Patterns: []string{"*.mp3"}}.DialogPattern()
	if !ok || n != "*.mp3" || p != "*.mp3" {
		t.Fatalf("名字为空用模式: %q %q", n, p)
	}
	if _, _, ok, _ := (FileFilter{Name: "全部"}).DialogPattern(); ok {
		t.Fatal("无模式不应生成过滤器")
	}
}

func TestCleanPickedPaths(t *testing.T) {
	if got := CleanPickedPaths(nil); got == nil || len(got) != 0 {
		t.Fatalf("取消应返回空切片而不是 nil: %#v", got)
	}
	if got := CleanPickedPaths([]string{"", "  "}); got == nil || len(got) != 0 {
		t.Fatalf("%#v", got)
	}
	base := t.TempDir()
	a := filepath.Join(base, "a", "..", "b.mp4")
	got := CleanPickedPaths([]string{a, filepath.Join(base, "b.mp4"), filepath.Join(base, "c.mp4")})
	want := []string{filepath.Join(base, "b.mp4"), filepath.Join(base, "c.mp4")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if runtime.GOOS != "windows" {
		if got := CleanPickedPaths([]string{"rel/x.mp4"}); len(got) != 1 || !filepath.IsAbs(got[0]) {
			t.Fatalf("相对路径应转绝对: %v", got)
		}
	}
}
