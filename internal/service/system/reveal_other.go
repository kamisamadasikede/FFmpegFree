//go:build !windows

package system

import (
	"os/exec"

	"FFmpegFree/internal/proc"
)

func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	proc.Configure(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
