//go:build darwin

package doccomp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"FFmpegFree/internal/proc"
)

// preparePackage：hdiutil 挂载 dmg、ditto 拷出 LibreOffice.app、卸载、清 quarantine（契约 6.12.12）。
func preparePackage(ctx context.Context, pkg, staging, tmp string) error {
	mnt, err := os.MkdirTemp(tmp, "doc-mnt-")
	if err != nil {
		return installFailed("hdiutil=mkdir")
	}
	defer os.Remove(mnt)
	run := func(name string, args ...string) (int, error) {
		cmd := exec.Command(name, args...)
		proc.Configure(cmd)
		return runTree(ctx, cmd)
	}
	if code, err := run("/usr/bin/hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mnt, pkg); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return installFailed(codeDetail("hdiutil", code, "attach"))
	}
	detach := func() {
		// 取消时也要卸载：用不受 ctx 影响的命令
		cmd := exec.Command("/usr/bin/hdiutil", "detach", mnt)
		proc.Configure(cmd)
		if err := cmd.Run(); err != nil {
			cmd = exec.Command("/usr/bin/hdiutil", "detach", "-force", mnt)
			proc.Configure(cmd)
			_ = cmd.Run()
		}
	}
	defer detach()
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return installFailed("hdiutil=mkdir")
	}
	if code, err := run("/usr/bin/ditto", filepath.Join(mnt, "LibreOffice.app"), filepath.Join(staging, "LibreOffice.app")); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return installFailed(codeDetail("hdiutil", code, "ditto"))
	}
	_, _ = run("/usr/bin/xattr", "-dr", "com.apple.quarantine", filepath.Join(staging, "LibreOffice.app"))
	return nil
}
