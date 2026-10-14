//go:build !windows

package catagent

import "os/exec"

// readRegistryPath 非 Windows 无注册表。
func readRegistryPath() (machine, user string) { return "", "" }

// wrapShim 非 Windows 不需要改写。
func wrapShim(cmd *exec.Cmd, name string, args []string) {}
