package doc

import (
	"archive/zip"
	"os"
	"testing"
)

// writeBomb 写一个 docx，其 word/document.xml 解压后 size 字节（全 0 压缩得很小）。
func writeBomb(t *testing.T, path string, size int64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1<<20)
	for n := int64(0); n < size; n += int64(len(buf)) {
		w.Write(buf)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
