//go:build windows

package catagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadOwnCommandLine(t *testing.T) {
	pid := uint32(os.Getpid())
	line, err := commandLine(pid)
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if base == "" || !strings.Contains(strings.ToLower(line), strings.ToLower(base)) {
		t.Fatalf("command line %q does not contain %q", line, base)
	}
}
