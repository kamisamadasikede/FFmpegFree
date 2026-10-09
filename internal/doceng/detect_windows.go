//go:build windows

package doceng

import (
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	wordProgIDs       = []string{"Word.Application"}
	excelProgIDs      = []string{"Excel.Application"}
	powerPointProgIDs = []string{"PowerPoint.Application"}
	wpsWordProgIDs    = []string{"KWPS.Application", "WPS.Application"}
	wpsSheetProgIDs   = []string{"KET.Application", "ET.Application"}
	wpsSlideProgIDs   = []string{"KWPP.Application", "WPP.Application"}
)

func DetectOfficeWPS() (office, wps *Detected) {
	ow := detectFamily(wordProgIDs, "WINWORD.EXE")
	oe := detectFamily(excelProgIDs, "EXCEL.EXE")
	op := detectFamily(powerPointProgIDs, "POWERPNT.EXE")
	if ow != nil || oe != nil || op != nil {
		d := &Detected{ID: IDOffice, Name: NameOffice, Installed: true, Available: true}
		var fams []string
		if ow != nil && officeMajorOK(ow.version) {
			d.WordProgID, d.WordExe = ow.progID, ow.exe
			fams = append(fams, FamilyText)
			if d.Version == "" {
				d.Version = ow.version
			}
		}
		if oe != nil && officeMajorOK(oe.version) {
			d.ExcelProgID, d.ExcelExe = oe.progID, oe.exe
			d.ExcelMajor = majorOf(oe.version)
			fams = append(fams, FamilySheet)
			if d.Version == "" {
				d.Version = oe.version
			}
		}
		if op != nil && officeMajorOK(op.version) {
			d.PowerPointProgID, d.PowerPointExe = op.progID, op.exe
			fams = append(fams, FamilySlide)
			if d.Version == "" {
				d.Version = op.version
			}
		}
		d.Families = fams
		if len(fams) > 0 {
			office = d
		}
	}

	ww := detectFamily(wpsWordProgIDs, "wps.exe")
	ws := detectFamily(wpsSheetProgIDs, "et.exe", "wps.exe")
	wp := detectFamily(wpsSlideProgIDs, "wpp.exe", "wps.exe")
	if ww != nil || ws != nil || wp != nil {
		d := &Detected{ID: IDWPS, Name: NameWPS, Installed: true, Available: true}
		var fams []string
		if ww != nil {
			d.WordProgID, d.WordExe = ww.progID, ww.exe
			fams = append(fams, FamilyText)
			if d.Version == "" {
				d.Version = ww.version
			}
		}
		if ws != nil {
			d.ExcelProgID, d.ExcelExe = ws.progID, ws.exe
			fams = append(fams, FamilySheet)
			if d.Version == "" {
				d.Version = ws.version
			}
		}
		if wp != nil {
			d.PowerPointProgID, d.PowerPointExe = wp.progID, wp.exe
			fams = append(fams, FamilySlide)
			if d.Version == "" {
				d.Version = wp.version
			}
		}
		d.Families = fams
		if len(fams) > 0 {
			wps = d
		}
	}
	return office, wps
}

type progHit struct {
	progID, exe, version string
}

func detectFamily(progIDs []string, wantExe ...string) *progHit {
	want := map[string]bool{}
	for _, w := range wantExe {
		want[strings.ToLower(w)] = true
	}
	for _, id := range progIDs {
		exe, ver, ok := resolveProgID(id)
		if !ok {
			continue
		}
		base := strings.ToLower(filepath.Base(exe))
		if !want[base] {
			continue
		}
		return &progHit{progID: id, exe: exe, version: ver}
	}
	return nil
}

func resolveProgID(progID string) (exe, version string, ok bool) {
	k, err := registry.OpenKey(registry.CLASSES_ROOT, progID+`\CLSID`, registry.QUERY_VALUE)
	if err != nil {
		return "", "", false
	}
	clsid, _, err := k.GetStringValue("")
	k.Close()
	if err != nil || clsid == "" {
		return "", "", false
	}
	k2, err := registry.OpenKey(registry.CLASSES_ROOT, `CLSID\`+clsid+`\LocalServer32`, registry.QUERY_VALUE)
	if err != nil {
		return "", "", false
	}
	defer k2.Close()
	raw, _, err := k2.GetStringValue("")
	if err != nil || raw == "" {
		return "", "", false
	}
	exe = unquotePath(raw)
	if exe == "" {
		return "", "", false
	}
	return exe, fileVersion(exe), true
}

func unquotePath(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) {
		if i := strings.Index(s[1:], `"`); i >= 0 {
			return s[1 : 1+i]
		}
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func fileVersion(exe string) string {
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(exe, &zero)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(exe, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}
	var block *windows.VS_FIXEDFILEINFO
	var blen uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&block), &blen); err != nil || block == nil {
		return ""
	}
	maj := block.FileVersionMS >> 16
	min := block.FileVersionMS & 0xffff
	patch := block.FileVersionLS >> 16
	build := block.FileVersionLS & 0xffff
	return strconv.FormatUint(uint64(maj), 10) + "." + strconv.FormatUint(uint64(min), 10) + "." +
		strconv.FormatUint(uint64(patch), 10) + "." + strconv.FormatUint(uint64(build), 10)
}

func officeMajorOK(ver string) bool {
	m := majorOf(ver)
	return m == 0 || m >= 14
}

func majorOf(ver string) int {
	if ver == "" {
		return 0
	}
	i := strings.IndexByte(ver, '.')
	if i < 0 {
		i = len(ver)
	}
	n, err := strconv.Atoi(ver[:i])
	if err != nil {
		return 0
	}
	return n
}

func ProcessRunning(exeNames ...string) bool {
	want := map[string]bool{}
	for _, n := range exeNames {
		want[strings.ToLower(n)] = true
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return false
	}
	for {
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if want[name] {
			return true
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			break
		}
	}
	return false
}

func PresentationBusy(engineID string) bool {
	switch engineID {
	case IDOffice:
		return ProcessRunning("POWERPNT.EXE")
	case IDWPS:
		return ProcessRunning("wpp.exe", "wps.exe")
	}
	return false
}

func SnapshotPIDs(exeBase string) map[uint32]struct{} {
	out := map[uint32]struct{}{}
	want := strings.ToLower(exeBase)
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return out
	}
	for {
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if name == want {
			out[pe.ProcessID] = struct{}{}
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			break
		}
	}
	return out
}

func DiffPID(before, after map[uint32]struct{}) uint32 {
	var found uint32
	n := 0
	for pid := range after {
		if _, ok := before[pid]; !ok {
			found = pid
			n++
		}
	}
	if n != 1 {
		return 0
	}
	return found
}

func KillPID(pid uint32) {
	if pid == 0 {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_ = windows.TerminateProcess(h, 1)
}
