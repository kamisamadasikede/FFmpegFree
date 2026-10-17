package cat

import (
	"slices"
	"testing"
)

func TestExtractWritePathsFromTool(t *testing.T) {
	root := "/p/proj"
	cases := []struct {
		label string
		want  []string
	}{
		{"write backend/api.go\nbackend/api.go", []string{"backend/api.go", "backend/api.go"}},
		{"apply_patch\nadd /p/proj/backend/x.go\nupdate src/y.ts", []string{"backend/x.go", "src/y.ts"}},
		{`Write {"content":"x","file_path":"/p/proj/backend/c.go"}`, []string{"backend/c.go"}},
		{`Edit {"file_path":"src/a.ts","old_string":"a","new_string":"b"}`, []string{"src/a.ts"}},
		{`Read {"file_path":"/p/proj/backend/c.go"}`, nil},
		{"read_file\nsrc/main.rs\nsrc/lib.rs", nil},
		{"shell go test ./...", nil},
		{"edit_file\nsrc/a.go\n\n- old.go\n+ new.go", []string{"src/a.go"}},
	}
	for _, c := range cases {
		if got := extractWritePathsFromTool(c.label, root); !slices.Equal(got, c.want) {
			t.Errorf("%q → %v, want %v", c.label, got, c.want)
		}
	}
}
