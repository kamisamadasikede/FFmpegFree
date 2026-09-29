package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// 真实下载验证：默认跳过（约 60~100 MB 网络流量）。
// 运行：FFMPEGFREE_REAL_INSTALL=1 GOTOOLCHAIN=local go test ./internal/ffmpeg -run RealInstall -v -timeout 20m
// 可选 FFMPEGFREE_REAL_MIRROR=cn。会按当前平台的清单条目下载、校验 SHA256、解压并实际运行 -version / -encoders。
func TestRealInstall(t *testing.T) {
	if os.Getenv("FFMPEGFREE_REAL_INSTALL") != "1" {
		t.Skip("设置 FFMPEGFREE_REAL_INSTALL=1 启用真实下载测试")
	}
	m, err := DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	in := NewInstaller(m, NewLocator(""), filepath.Join(root, "bin"), filepath.Join(root, "tmp"))
	var last Progress
	info, err := in.Install(context.Background(), os.Getenv("FFMPEGFREE_REAL_MIRROR"), func(p Progress) {
		if p.Phase != last.Phase {
			t.Logf("阶段 %s", p.Phase)
		}
		last = p
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("安装成功: %+v", info)
	if info.Major < MinMajor && info.Known {
		t.Fatalf("版本过低: %+v", info)
	}
}
