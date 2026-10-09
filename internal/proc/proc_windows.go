//go:build windows

package proc

import (
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// configure 隐藏子进程窗口：HideWindow（SW_HIDE）+ CREATE_NO_WINDOW。
// 后者让控制台程序（ffmpeg、grok，以及 grok.cmd 之类经 cmd.exe 的包装）根本不分配可见控制台，
// 它再派生的控制台子进程继承这个无窗口控制台，也不会闪黑窗口；对 GUI 程序（soffice）无影响。
func configure(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

// jobs 记录每个受管进程对应的 Job Object 句柄。
var jobs jobRegistry

// start 启动进程并把它放进一个新的 Job Object（JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE）。
//
// 作用：Job 句柄由本进程持有，应用崩溃 / 被任务管理器结束时系统关闭句柄，Job 里的所有进程
// （ffmpeg 和它的子孙）被系统一并结束，不会留下孤儿 ffmpeg 继续占用摄像头 / 推流 / 文件。
// 进程正常退出时我们主动关闭句柄，同样会结束它遗留的子孙进程（与 unix 上"整组结束"一致）。
//
// 创建 Job / 加入 Job 失败（例如 Windows 7 上外层已有不允许嵌套的 Job）不影响启动：
// 这种进程不在 jobs 里，kill 时走 taskkill 回退。
// 已知窗口：Start 返回到加入 Job 之间极短的时间里，进程派生的子进程不会进 Job（ffmpeg 启动时不会立刻派生子进程）。
func start(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	attachJob(cmd.Process.Pid)
	return nil
}

func attachJob(pid int) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return
	}
	// SYNCHRONIZE 用来等进程退出；SET_QUOTA | TERMINATE 是 AssignProcessToJobObject 要求的权限。
	ph, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		windows.CloseHandle(job)
		return
	}
	if err := windows.AssignProcessToJobObject(job, ph); err != nil {
		windows.CloseHandle(ph)
		windows.CloseHandle(job)
		return
	}
	jobs.add(pid, uintptr(job))
	// 进程退出后关闭 Job 句柄（KILL_ON_JOB_CLOSE：顺带结束遗留的子孙进程）并释放登记。
	go func() {
		windows.WaitForSingleObject(ph, windows.INFINITE)
		windows.CloseHandle(ph)
		if h, ok := jobs.take(pid); ok {
			windows.CloseHandle(windows.Handle(h))
		}
	}()
}

// kill 结束整个进程树：先终结 Job Object；没有 Job（创建 / 加入失败）或失败时退回 `taskkill /T /F`
// （ffmpeg 通过 shell 包装或滤镜派生子进程时，只 Kill 主进程会留下孤儿）；再失败只结束主进程。
func kill(cmd *exec.Cmd) error {
	pid := cmd.Process.Pid
	return killTree(
		func() error {
			h, ok := jobs.get(pid)
			if !ok {
				return errNoJob
			}
			return windows.TerminateJobObject(windows.Handle(h), 1)
		},
		func() error {
			tk := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
			configure(tk)
			return tk.Run()
		},
		cmd.Process.Kill,
	)
}

func interrupt(cmd *exec.Cmd) error { return ErrInterruptUnsupported }
