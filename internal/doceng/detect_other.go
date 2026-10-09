//go:build !windows

package doceng

// DetectOfficeWPS 非 Windows 没有本机 Office / WPS。
func DetectOfficeWPS() (office, wps *Detected) { return nil, nil }

// ProcessRunning 非 Windows 桩。
func ProcessRunning(exeNames ...string) bool { return false }

// PresentationBusy 非 Windows 桩。
func PresentationBusy(engineID string) bool { return false }
