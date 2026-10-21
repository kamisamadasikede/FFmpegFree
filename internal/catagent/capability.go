package catagent

import "fmt"

const (
	CapabilityChat     = "chat"
	CapabilityReadOnly = "read_only"
	CapabilityWrite    = "write"
)

// Capabilities are applied before transport selection and session loading.
// Restricted calls always use a new ephemeral session, preventing writable
// native sessions, live steering or configured approval bypasses from leaking in.
func normalizeTurnCapability(opts TurnOptions, kind string) (TurnOptions, error) {
	switch opts.Capability {
	case "", CapabilityWrite:
		if !opts.NoTools {
			return opts, nil
		}
	case CapabilityChat, CapabilityReadOnly:
	default:
		return opts, fmt.Errorf("unsupported execution capability: %s", opts.Capability)
	}
	if opts.Capability == CapabilityChat && kind == KindCodexCLI {
		return opts, fmt.Errorf("Codex CLI does not provide a verified tool-free chat mode; select read_only")
	}
	opts.Resume = false
	opts.Ephemeral = true
	opts.LiveSteer = false
	opts.PermissionMode = "read-only"
	// Grok's verified tool switch is all-or-none. Its read-only ceiling therefore
	// uses context-only analysis until a verified read-tool allowlist is available.
	if kind == KindCatBuild {
		opts.NoTools = true
	}
	return opts, nil
}
