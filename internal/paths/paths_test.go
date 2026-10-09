package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAndEnsure(t *testing.T) {
	root := t.TempDir()
	d, err := Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{d.Bin, d.Thumbs, d.Logs, d.Temp} {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			t.Fatalf("%s 没有创建", dir)
		}
	}
	if d.DB != filepath.Join(root, "app.db") {
		t.Fatalf("DB 路径不对: %s", d.DB)
	}
}

func TestNormalize(t *testing.T) {
	p, key, err := Normalize("a/../b/Video.MP4")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(p) || strings.Contains(p, "..") {
		t.Fatalf("应为干净的绝对路径: %s", p)
	}
	if caseInsensitiveFS && key != strings.ToLower(p) {
		t.Fatalf("大小写不敏感平台 key 应为小写: %s", key)
	}
	if !caseInsensitiveFS && key != p {
		t.Fatalf("大小写敏感平台 key 应等于 path: %s", key)
	}
	if _, _, err := Normalize("  "); err == nil {
		t.Fatal("空路径应报错")
	}
}

func TestVideosDir(t *testing.T) {
	home := "/home/u"
	noenv := func(string) string { return "" }
	if got := videosDir("darwin", home, noenv); got != filepath.Join(home, "Movies") {
		t.Fatalf("macOS 应为 ~/Movies: %s", got)
	}
	if got := videosDir("windows", home, noenv); got != filepath.Join(home, "Videos") {
		t.Fatalf("Windows 应为 ~/Videos: %s", got)
	}
	env := func(k string) string {
		if k == "XDG_VIDEOS_DIR" {
			return "$HOME/视频"
		}
		return ""
	}
	if got := videosDir("linux", home, env); got != "/home/u/视频" {
		t.Fatalf("Linux 应读 XDG_VIDEOS_DIR: %s", got)
	}
}

func TestParseUserDirs(t *testing.T) {
	content := "# comment\nXDG_DESKTOP_DIR=\"$HOME/Desktop\"\nXDG_VIDEOS_DIR=\"$HOME/Vids\"\n"
	if got := parseUserDirs(content, "XDG_VIDEOS_DIR", "/home/u"); got != "/home/u/Vids" {
		t.Fatalf("解析 user-dirs.dirs 失败: %s", got)
	}
	if got := parseUserDirs(`XDG_VIDEOS_DIR="$HOME/"`, "XDG_VIDEOS_DIR", "/home/u"); got != "" {
		t.Fatalf("等于 $HOME 表示禁用，应返回空: %s", got)
	}
}

func TestKeyForCaseRules(t *testing.T) {
	if got := KeyFor("windows", `C:\Users\Foo\Proj`); got != `c:\users\foo\proj` {
		t.Fatalf("windows 应转小写: %q", got)
	}
	if got := KeyFor("darwin", "/Users/Foo/Proj/"); got != "/users/foo/proj" {
		t.Fatalf("darwin 应转小写并 Clean: %q", got)
	}
	if got := KeyFor("linux", "/home/Foo/Proj/"); got != "/home/Foo/Proj" {
		t.Fatalf("linux 区分大小写: %q", got)
	}
	abs, key, err := Normalize("/tmp/Some/Dir")
	if err != nil || key != Key(abs) {
		t.Fatalf("Key 应与 Normalize 的 key 一致: %q %q %v", key, Key(abs), err)
	}
}
