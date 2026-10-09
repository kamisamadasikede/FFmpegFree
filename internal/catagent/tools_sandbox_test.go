package catagent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveUnderRootSandbox(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "proj")
	outside := filepath.Join(base, "outside")
	_ = os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	_ = os.MkdirAll(outside, 0o755)
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("in"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "..foo.txt"), []byte("dots"), 0o644)
	_ = os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("out"), 0o644)

	ok := []string{"", ".", "a.txt", "sub", "sub/../a.txt", "..foo.txt"}
	for _, rel := range ok {
		if _, good := ResolveUnderRoot(root, rel); !good {
			t.Errorf("应允许 %q", rel)
		}
	}
	bad := []string{"..", "../outside/secret.txt", "sub/../../outside", filepath.Join(outside, "secret.txt"), "/etc/passwd", `\x`}
	for _, rel := range bad {
		if _, good := ResolveUnderRoot(root, rel); good {
			t.Errorf("应拒绝 %q", rel)
		}
	}
	if _, good := ResolveUnderRoot("", "a.txt"); good {
		t.Error("没有根应一律拒绝")
	}

	if runtime.GOOS != "windows" {
		// 符号链接逃逸（目录与文件）一律拒绝；指向根内的符号链接允许。
		_ = os.Symlink(outside, filepath.Join(root, "escape"))
		_ = os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "leak.txt"))
		_ = os.Symlink(filepath.Join(root, "a.txt"), filepath.Join(root, "inner.txt"))
		for _, rel := range []string{"escape", "escape/secret.txt", "leak.txt"} {
			if _, good := ResolveUnderRoot(root, rel); good {
				t.Errorf("符号链接逃逸应拒绝 %q", rel)
			}
		}
		if _, good := ResolveUnderRoot(root, "inner.txt"); !good {
			t.Error("根内符号链接应允许")
		}
		if _, err := ReadText(root, "leak.txt"); err == nil {
			t.Error("ReadText 不应读到根外文件")
		}
		if _, err := ListDir(root, "escape"); err == nil {
			t.Error("ListDir 不应列出根外目录")
		}
		// 根本身是符号链接：在其真实路径内照常可用。
		link := filepath.Join(base, "rootlink")
		_ = os.Symlink(root, link)
		if s, err := ReadText(link, "a.txt"); err != nil || s != "in" {
			t.Errorf("根是符号链接时应可读: %q %v", s, err)
		}
	}
	if s, err := ReadText(root, "a.txt"); err != nil || s != "in" {
		t.Fatalf("%q %v", s, err)
	}
	if s, err := ListDir(root, ""); err != nil || s == "" {
		t.Fatalf("%q %v", s, err)
	}
}

func TestHandleToolRequestsNoProjectRejects(t *testing.T) {
	res := HandleToolRequests("", []ToolRequest{{ID: "1", Kind: "read_file", Path: "a.txt"}, {ID: "2", Kind: "list_dir"}, {ID: "3", Kind: "write_file"}})
	if len(res) != 3 || res[0].OK || res[1].OK || res[2].OK {
		t.Fatalf("%+v", res)
	}
	if res[0].Content != MsgToolNoProject || res[1].Content != MsgToolNoProject || res[2].Content != MsgToolWriteRef {
		t.Fatalf("%+v", res)
	}
}
