package genoffice

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/proc"
)

var installMu sync.Mutex

func compatibleNode(node string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "--version")
	proc.Configure(cmd)
	data, err := cmd.Output()
	if err != nil {
		return false
	}
	version := strings.Split(strings.TrimPrefix(strings.TrimSpace(string(data)), "v"), ".")
	if len(version) < 2 {
		return false
	}
	major, _ := strconv.Atoi(version[0])
	minor, _ := strconv.Atoi(version[1])
	return major > 22 || major == 22 && minor >= 12
}
func nodePath(dir string, install bool) (string, error) {
	if node := os.Getenv("FFC_GENOFFICE_NODE"); node != "" && compatibleNode(node) {
		return node, nil
	}
	if node, err := exec.LookPath("node"); err == nil && compatibleNode(node) {
		return node, nil
	}
	file := filepath.Join(dir, "node-v22.20.0", "node.exe")
	if compatibleNode(file) {
		return file, nil
	}
	if !install || runtime.GOOS != "windows" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return "", fmt.Errorf("办公引擎需要 Node.js 22.12 以上版本")
	}
	installMu.Lock()
	defer installMu.Unlock()
	if compatibleNode(file) {
		return file, nil
	}
	arch := "x64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	name := "node-v22.20.0-win-" + arch + ".zip"
	base := "https://nodejs.org/dist/v22.20.0/"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	fetch := func(name string, limit int64) ([]byte, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", base+name, nil)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return nil, fmt.Errorf("办公运行环境下载失败 (%d)", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
		if len(data) > int(limit) {
			return nil, fmt.Errorf("运行环境下载超出大小上限")
		}
		return data, err
	}
	checksums, err := fetch("SHASUMS256.txt", 1<<20)
	if err != nil {
		return "", err
	}
	expected := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			expected = fields[0]
		}
	}
	if len(expected) != 64 {
		return "", fmt.Errorf("运行环境校验信息缺失")
	}
	data, err := fetch(name, 80<<20)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != expected {
		return "", fmt.Errorf("运行环境校验失败")
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	for _, entry := range archive.File {
		if entry.Name != "node-v22.20.0-win-"+arch+"/node.exe" {
			continue
		}
		if entry.UncompressedSize64 > 120<<20 {
			return "", fmt.Errorf("运行环境解压大小超限")
		}
		reader, err := entry.Open()
		if err != nil {
			return "", err
		}
		raw, err := io.ReadAll(io.LimitReader(reader, 120<<20))
		_ = reader.Close()
		if err != nil {
			return "", err
		}
		if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			return "", err
		}
		if err = os.WriteFile(file+".part", raw, 0600); err != nil {
			return "", err
		}
		if err = os.Rename(file+".part", file); err != nil {
			return "", err
		}
		if !compatibleNode(file) {
			return "", fmt.Errorf("办公运行环境启动失败")
		}
		return file, nil
	}
	return "", fmt.Errorf("运行环境安装包缺少 node.exe")
}
