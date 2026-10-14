//go:build windows

package catagent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// readRegistryPath 重读注册表里的机器 / 用户 PATH（应用先于 CLI 安装启动时，进程自身 PATH 是旧的）。
func readRegistryPath() (machine, user string) {
	read := func(root registry.Key, path string) string {
		k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
		if err != nil {
			return ""
		}
		defer k.Close()
		v, _, err := k.GetStringValue("Path")
		if err != nil {
			return ""
		}
		return v
	}
	machine = read(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`)
	user = read(registry.CURRENT_USER, `Environment`)
	return machine, user
}

func systemExe(name string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	switch name {
	case "cmd.exe":
		if c := os.Getenv("ComSpec"); c != "" {
			return c
		}
		return filepath.Join(root, "System32", "cmd.exe")
	default:
		return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	}
}

// wrapShim 把 .cmd / .bat / .ps1 改成经 cmd.exe / powershell.exe 启动（6.19.21.4）。
// 在 proc.Configure 之后调用：保留 HideWindow / CREATE_NO_WINDOW，只追加 CmdLine。
func wrapShim(cmd *exec.Cmd, name string, args []string) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".cmd", ".bat":
		comspec := systemExe("cmd.exe")
		cmd.Path = comspec
		cmd.Args = append([]string{comspec}, args...)
		if cmd.SysProcAttr == nil {
			cmd.SysProcAttr = &syscall.SysProcAttr{}
		}
		cmd.SysProcAttr.CmdLine = cmdShimCommandLine(comspec, name, args)
	case ".ps1":
		ps := systemExe("powershell.exe")
		cmd.Path = ps
		cmd.Args = append([]string{ps}, ps1ShimArgs(name, args)...)
	}
}
