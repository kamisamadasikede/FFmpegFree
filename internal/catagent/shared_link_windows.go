//go:build windows

package catagent

import (
	"encoding/binary"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// Junctions need no elevation. Paths are never interpreted by a shell.
func createSharedDirectoryLink(link, target string) error {
	if e := os.Symlink(target, link); e == nil {
		return nil
	}
	target, e := filepath.Abs(target)
	if e != nil {
		return e
	}
	substitute := "\\??\\" + target
	if strings.HasPrefix(target, "\\\\") {
		substitute = "\\??\\UNC\\" + strings.TrimPrefix(target, "\\\\")
	}
	sub, print := utf16.Encode([]rune(substitute)), utf16.Encode([]rune(target))
	size := 16 + (len(sub)+len(print)+2)*2
	if size > 16<<10 {
		return os.ErrInvalid
	}
	data := make([]byte, size)
	binary.LittleEndian.PutUint32(data[0:4], windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(data[4:6], uint16(size-8))
	binary.LittleEndian.PutUint16(data[10:12], uint16(len(sub)*2))
	binary.LittleEndian.PutUint16(data[12:14], uint16((len(sub)+1)*2))
	binary.LittleEndian.PutUint16(data[14:16], uint16(len(print)*2))
	for i, v := range sub {
		binary.LittleEndian.PutUint16(data[16+i*2:], v)
	}
	offset := 16 + (len(sub)+1)*2
	for i, v := range print {
		binary.LittleEndian.PutUint16(data[offset+i*2:], v)
	}
	if e = os.Mkdir(link, 0700); e != nil {
		return e
	}
	success := false
	defer func() {
		if !success {
			_ = os.Remove(link)
		}
	}()
	ptr, e := windows.UTF16PtrFromString(link)
	if e != nil {
		return e
	}
	handle, e := windows.CreateFile(ptr, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if e != nil {
		return e
	}
	defer windows.CloseHandle(handle)
	var returned uint32
	e = windows.DeviceIoControl(handle, windows.FSCTL_SET_REPARSE_POINT, &data[0], uint32(len(data)), nil, 0, &returned, nil)
	if e != nil {
		return e
	}
	success = true
	return nil
}
