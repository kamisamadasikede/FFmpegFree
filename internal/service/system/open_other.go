//go:build !windows

package system

import (
	"bytes"
	"errors"
	"os/exec"
	"runtime"
	"time"

	"FFmpegFree/internal/proc"
)

// openDefault：macOS open <path>，其他 xdg-open <path>；最多等 5 秒拿退出码，超过按成功（进程留在后台自己退出）。
func openDefault(path string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	cmd := exec.Command(name, path)
	proc.Configure(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return nil
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return classifyOpenExit(runtime.GOOS, ee.ExitCode(), stderr.String())
		}
		return err
	case <-time.After(openWaitSeconds * time.Second):
		return nil
	}
}
