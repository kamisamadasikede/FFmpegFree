package cat

import "testing"

func TestProjectPathKeyFor(t *testing.T) {
	if got := projectPathKeyFor("windows", `C:\Users\Foo\Proj\`); got != `c:\users\foo\proj` {
		t.Fatalf("windows 应转小写并去掉末尾分隔符: %q", got)
	}
	if got := projectPathKeyFor("linux", "/home/Foo/Proj/"); got != "/home/Foo/Proj" {
		t.Fatalf("linux 原样: %q", got)
	}
	if got := projectPathKeyFor("darwin", "/Users/Foo/Proj"); got != "/Users/Foo/Proj" {
		t.Fatalf("macOS 原样（契约 6.19.10.2）: %q", got)
	}
}

func TestValidProjectName(t *testing.T) {
	sixty := ""
	for i := 0; i < 60; i++ {
		sixty += "字"
	}
	cases := map[string]bool{"": false, "  ": false, " 项目 ": true, sixty: true, sixty + "x": false, "a\nb": false, "a\tb": false}
	for in, want := range cases {
		if _, ok := validProjectName(in); ok != want {
			t.Errorf("%q: %v", in, ok)
		}
	}
	long := "/tmp/" + sixty + "多出来"
	if n := defaultProjectName(long); len([]rune(n)) != 60 {
		t.Fatalf("文件夹名应截到 60 个字: %d", len([]rune(n)))
	}
}
