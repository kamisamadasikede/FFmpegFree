package catagent

import (
	"strings"
	"testing"
)

func TestRestrictedCapabilityCannotResumeOrBypass(t *testing.T) {
	for _, kind := range []string{KindCodexCLI, KindClaudeCLI, KindCatBuild} {
		t.Run(kind, func(t *testing.T) {
			o, err := normalizeTurnCapability(TurnOptions{Capability: CapabilityReadOnly, Resume: true, LiveSteer: true, PermissionMode: ModeCodexBypass}, kind)
			if err != nil || o.Resume || o.LiveSteer || !o.Ephemeral {
				t.Fatalf("unsafe opts %+v %v", o, err)
			}
			if kind == KindCodexCLI {
				a := &CLIAgentAdapter{kind: cliKindCodex}
				args := strings.Join(a.buildCodexArgs(o, "analyze", "work", "writable-session", true), " ")
				if strings.Contains(args, "resume") || strings.Contains(args, "dangerously-bypass") || !strings.Contains(args, "--sandbox read-only") {
					t.Fatal(args)
				}
			}
			if kind == KindClaudeCLI {
				a := &CLIAgentAdapter{kind: cliKindClaude}
				args := strings.Join(a.buildClaudeArgs(o, "analyze", "writable-session", true), " ")
				if strings.Contains(args, "--resume") || !strings.Contains(args, "--tools Read,Glob,Grep") {
					t.Fatal(args)
				}
			}
			if kind == KindCatBuild && !o.NoTools {
				t.Fatal("unverified read tools enabled")
			}
		})
	}
	if _, err := normalizeTurnCapability(TurnOptions{Capability: "unknown"}, KindCatBuild); err == nil {
		t.Fatal("unknown capability accepted")
	}
	if _, err := normalizeTurnCapability(TurnOptions{Capability: CapabilityChat}, KindCodexCLI); err == nil {
		t.Fatal("unverified tool-free capability accepted")
	}
}
func TestLegacyNoToolsCodexResumeIsRestricted(t *testing.T) {
	a := &CLIAgentAdapter{kind: cliKindCodex}
	args := strings.Join(a.buildCodexArgs(TurnOptions{NoTools: true, PermissionMode: ModeCodexBypass}, "route", "work", "previous", true), " ")
	if strings.Contains(args, "resume") || strings.Contains(args, "dangerously-bypass") || !strings.Contains(args, "--sandbox read-only") {
		t.Fatal(args)
	}
}
