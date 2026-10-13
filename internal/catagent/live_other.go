//go:build !windows

package catagent

func readOSProcessCommands() []processCommand { return nil }

func killOSProcessTree(uint32) error { return nil }
