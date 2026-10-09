package cat

import "testing"

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
