//go:build windows

package catagent

import (
	"encoding/binary"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unsafe"

	"FFmpegFree/internal/proc"

	"golang.org/x/sys/windows"
)

func readOSProcessCommands() []processCommand {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return nil
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return nil
	}
	var out []processCommand
	for {
		name := windows.UTF16ToString(pe.ExeFile[:])
		if interestingExe(name) {
			if line, err := commandLine(pe.ProcessID); err == nil && line != "" {
				out = append(out, processCommand{pid: pe.ProcessID, line: line})
			}
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			break
		}
	}
	return out
}

func interestingExe(name string) bool {
	base := strings.ToLower(name)
	switch base {
	case "grok.exe", "node.exe", "cmd.exe":
		return true
	default:
		return strings.Contains(base, "grok")
	}
}

func commandLine(pid uint32) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	var wow bool
	if err := windows.IsWow64Process(h, &wow); err == nil && wow {
		return "", nil
	}
	var pbi windows.PROCESS_BASIC_INFORMATION
	if err := windows.NtQueryInformationProcess(h, windows.ProcessBasicInformation, unsafe.Pointer(&pbi), uint32(unsafe.Sizeof(pbi)), nil); err != nil {
		return "", err
	}
	if pbi.PebBaseAddress == nil {
		return "", nil
	}
	peb, err := readRemote(h, uintptr(unsafe.Pointer(pbi.PebBaseAddress)), int(unsafe.Sizeof(windows.PEB{})))
	if err != nil {
		return "", err
	}
	paramsAt := remotePtr(peb, int(unsafe.Offsetof(windows.PEB{}.ProcessParameters)))
	if paramsAt == 0 {
		return "", nil
	}
	raw, err := readRemote(h, paramsAt, int(unsafe.Sizeof(windows.RTL_USER_PROCESS_PARAMETERS{})))
	if err != nil {
		return "", err
	}
	var params windows.RTL_USER_PROCESS_PARAMETERS
	var us windows.NTUnicodeString
	cmdOff := int(unsafe.Offsetof(params.CommandLine))
	n := int(remoteU16(raw, cmdOff+int(unsafe.Offsetof(us.Length))))
	buf := remotePtr(raw, cmdOff+int(unsafe.Offsetof(us.Buffer)))
	if n <= 0 || buf == 0 {
		return "", nil
	}
	if n > 32*1024 {
		n = 32 * 1024
	}
	chars, err := readRemote(h, buf, n)
	if err != nil {
		return "", err
	}
	u := make([]uint16, len(chars)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(chars[i*2:])
	}
	return windows.UTF16ToString(u), nil
}

func readRemote(h windows.Handle, addr uintptr, n int) ([]byte, error) {
	buf := make([]byte, n)
	var got uintptr
	err := windows.ReadProcessMemory(h, addr, &buf[0], uintptr(n), &got)
	if err != nil {
		return nil, err
	}
	if int(got) < n {
		buf = buf[:got]
	}
	return buf, nil
}

func remotePtr(buf []byte, off int) uintptr {
	if off < 0 || off+8 > len(buf) {
		return 0
	}
	return uintptr(binary.LittleEndian.Uint64(buf[off:]))
}

func remoteU16(buf []byte, off int) uint16 {
	if off < 0 || off+2 > len(buf) {
		return 0
	}
	return binary.LittleEndian.Uint16(buf[off:])
}

func killOSProcessTree(pid uint32) error {
	if pid == 0 || pid == uint32(os.Getpid()) {
		return nil
	}
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.FormatUint(uint64(pid), 10))
	proc.Configure(cmd)
	return cmd.Run()
}
