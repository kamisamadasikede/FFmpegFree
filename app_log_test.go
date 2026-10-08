package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAppLogAppendsAndRotates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs") // 目录不存在时自动创建
	f, p, err := openAppLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(dir, "app.log") {
		t.Fatalf("path = %q", p)
	}
	f.WriteString("first\n")
	f.Close()
	f, _, err = openAppLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("second\n")
	f.Close()
	if b, _ := os.ReadFile(p); string(b) != "first\nsecond\n" {
		t.Fatalf("没有追加: %q", b)
	}

	// 超过上限：启动时轮转成 app.log.1，新文件从空开始
	big := bytes.Repeat([]byte("x"), appLogMaxBytes+1)
	if err := os.WriteFile(p, big, 0o644); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(p+".1", []byte("older"), 0o644)
	f, _, err = openAppLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("after\n")
	f.Close()
	if b, _ := os.ReadFile(p); string(b) != "after\n" {
		t.Fatalf("轮转后的新文件 = %q", b)
	}
	if fi, err := os.Stat(p + ".1"); err != nil || fi.Size() != int64(len(big)) {
		t.Fatalf("app.log.1 应是轮转前的文件: %v", err)
	}
}

func TestOpenAppLogEmptyDir(t *testing.T) {
	if _, _, err := openAppLog(""); err == nil {
		t.Fatal("空目录应报错")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("invalid handle") }

// Windows GUI 程序的 stderr 是无效句柄：控制台写失败不能影响写文件（io.MultiWriter 会在第一个失败处停下）。
func TestAppLogWriterIgnoresConsoleError(t *testing.T) {
	var file bytes.Buffer
	w := &appLogWriter{file: &file, console: failWriter{}}
	if n, err := w.Write([]byte("line\n")); err != nil || n != 5 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if !strings.Contains(file.String(), "line") {
		t.Fatalf("文件没写进去: %q", file.String())
	}
}
