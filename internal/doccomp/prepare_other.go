//go:build !windows && !darwin

package doccomp

import "context"

// preparePackage：Linux 不下载，只检测（契约 6.12.12），走不到这里。
func preparePackage(ctx context.Context, pkg, staging, tmp string) error {
	return installFailed("check=unsupported_platform")
}
