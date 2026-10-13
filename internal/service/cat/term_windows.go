//go:build windows

package cat

import (
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"FFmpegFree/internal/apperr"
	"golang.org/x/sys/windows"
)

func consoleAvailable() bool { return true }

// consoleSession 是一块伪控制台。按键写进去，输出从管道读出来，窗口上不会再弹一个黑框。
type consoleSession struct {
	mu   sync.Mutex
	done bool
	hpc  windows.Handle
	in   *os.File
	out  *os.File
	proc windows.Handle
	job  windows.Handle
}

func openConsole(cwd string) (*termProc, []io.Reader, error) {
	// 伪控制台要同步管道。官方示例用不可继承的管道，由系统自己复制给控制台宿主。
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, nil, apperr.Wrap(apperr.ProcessFailed, "终端没有启动。", err)
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		return nil, nil, apperr.Wrap(apperr.ProcessFailed, "终端没有启动。", err)
	}
	var hpc windows.Handle
	err := windows.CreatePseudoConsole(windows.Coord{X: 120, Y: 32}, inR, outW, 0, &hpc)
	windows.CloseHandle(inR)
	windows.CloseHandle(outW)
	if err != nil {
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, nil, apperr.Wrap(apperr.ProcessFailed, "终端没有启动。", err)
	}
	_ = windows.SetHandleInformation(inW, windows.HANDLE_FLAG_INHERIT, 0)
	_ = windows.SetHandleInformation(outR, windows.HANDLE_FLAG_INHERIT, 0)
	inFile := os.NewFile(uintptr(inW), "conpty-in")
	outFile := os.NewFile(uintptr(outR), "conpty-out")
	fail := func(cause error) (*termProc, []io.Reader, error) {
		if inFile != nil {
			inFile.Close()
		}
		if outFile != nil {
			outFile.Close()
		}
		if hpc != 0 {
			windows.ClosePseudoConsole(hpc)
		}
		return nil, nil, apperr.Wrap(apperr.ProcessFailed, "终端没有启动。", cause)
	}
	if inFile == nil || outFile == nil {
		return fail(io.ErrClosedPipe)
	}

	com := os.Getenv("ComSpec")
	if com == "" {
		com = "cmd.exe"
	}
	cmdLine, err := windows.UTF16FromString(quoteCmd(com) + " /K chcp 65001")
	if err != nil {
		return fail(err)
	}
	var dir *uint16
	if cwd != "" {
		dir, err = windows.UTF16PtrFromString(cwd)
		if err != nil {
			return fail(err)
		}
	}
	attr, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return fail(err)
	}
	if err := attr.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(&hpc), unsafe.Sizeof(hpc)); err != nil {
		attr.Delete()
		return fail(err)
	}
	si := &windows.StartupInfoEx{}
	si.Cb = uint32(unsafe.Sizeof(*si))
	si.ProcThreadAttributeList = attr.List()
	var pi windows.ProcessInformation
	// 伪控制台接不上时不要弹出系统错误框，这样能马上改用普通输入。
	setErrorMode := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetErrorMode")
	prevMode, _, _ := setErrorMode.Call(0x0001 | 0x0002)
	defer setErrorMode.Call(prevMode)
	err = windows.CreateProcess(
		nil,
		&cmdLine[0],
		nil,
		nil,
		false,
		windows.EXTENDED_STARTUPINFO_PRESENT,
		nil,
		dir,
		&si.StartupInfo,
		&pi,
	)
	runtime.KeepAlive(cmdLine)
	runtime.KeepAlive(hpc)
	runtime.KeepAlive(si)
	attr.Delete()
	if err != nil {
		return fail(err)
	}
	_ = windows.CloseHandle(pi.Thread)
	session := &consoleSession{hpc: hpc, in: inFile, out: outFile, proc: pi.Process}
	session.attachJob()
	writer := consoleWriter{c: session}
	p := &termProc{
		stdin:  writer,
		resize: session.resize,
		end:    session.shutdown,
		wait:   session.wait,
		alive:  session.alive,
	}
	return p, []io.Reader{outFile}, nil
}

func quoteCmd(path string) string {
	if strings.ContainsAny(path, " \t\"") {
		return `"` + strings.ReplaceAll(path, `"`, ``) + `"`
	}
	return path
}

func (c *consoleSession) attachJob() {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return
	}
	if err := windows.AssignProcessToJobObject(job, c.proc); err != nil {
		windows.CloseHandle(job)
		return
	}
	c.job = job
}

func (c *consoleSession) alive() bool {
	if c.proc == 0 {
		return false
	}
	// 258 是 WaitForSingleObject 的 WAIT_TIMEOUT：进程还在。
	ev, _ := windows.WaitForSingleObject(c.proc, 250)
	return ev == 258
}

func (c *consoleSession) resize(cols, rows int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hpc == 0 {
		return io.ErrClosedPipe
	}
	return windows.ResizePseudoConsole(c.hpc, windows.Coord{X: int16(cols), Y: int16(rows)})
}

func (c *consoleSession) write(p []byte) (int, error) {
	c.mu.Lock()
	f := c.in
	c.mu.Unlock()
	if f == nil {
		return 0, io.ErrClosedPipe
	}
	return f.Write(p)
}

func (c *consoleSession) shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return
	}
	if c.in != nil {
		_ = c.in.Close()
		c.in = nil
	}
	if c.job != 0 {
		_ = windows.TerminateJobObject(c.job, 1)
	} else if c.proc != 0 {
		_ = windows.TerminateProcess(c.proc, 1)
	}
	if c.hpc != 0 {
		windows.ClosePseudoConsole(c.hpc)
		c.hpc = 0
	}
}

func (c *consoleSession) wait() (int, error) {
	windows.WaitForSingleObject(c.proc, windows.INFINITE)
	var code uint32
	_ = windows.GetExitCodeProcess(c.proc, &code)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.proc != 0 {
		_ = windows.CloseHandle(c.proc)
		c.proc = 0
	}
	if c.job != 0 {
		_ = windows.CloseHandle(c.job)
		c.job = 0
	}
	if c.hpc != 0 {
		windows.ClosePseudoConsole(c.hpc)
		c.hpc = 0
	}
	if c.out != nil {
		_ = c.out.Close()
		c.out = nil
	}
	if c.in != nil {
		_ = c.in.Close()
		c.in = nil
	}
	c.done = true
	return int(code), nil
}

type consoleWriter struct{ c *consoleSession }

func (w consoleWriter) Write(p []byte) (int, error) { return w.c.write(p) }

func (w consoleWriter) Close() error {
	w.c.shutdown()
	return nil
}
