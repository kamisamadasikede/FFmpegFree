//go:build !windows

package system

func ClipboardFilePaths() []string { return nil }

func ClipboardImage() string { return "" }
