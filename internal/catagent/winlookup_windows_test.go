//go:build windows

package catagent

import (
	"context"
	"strings"
	"testing"
)

func TestRunCmdWrapsCmdShimKeepsHiddenWindow(t *testing.T) {
	a := NewClaudeAdapter(CLIAgentConfig{})
	cmd := a.runCmd(context.Background(), `C:\Users\A B\npm\claude.cmd`, "-p", "--output-format", "stream-json")
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow 丢了")
	}
	if !strings.Contains(cmd.SysProcAttr.CmdLine, `/d /s /c ""C:\Users\A B\npm\claude.cmd" `) {
		t.Fatalf("CmdLine: %s", cmd.SysProcAttr.CmdLine)
	}
	ps := a.runCmd(context.Background(), `C:\x\claude.ps1`, "-v")
	if !strings.HasSuffix(strings.ToLower(ps.Path), "powershell.exe") || ps.Args[len(ps.Args)-1] != "-v" {
		t.Fatalf("ps1: %s %v", ps.Path, ps.Args)
	}
}
