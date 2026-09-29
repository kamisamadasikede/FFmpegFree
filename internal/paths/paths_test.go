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
