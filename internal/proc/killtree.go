package proc

import (
	"errors"
	"sync"
)

// errNoJob 表示进程没有对应的 Job Object（创建或加入失败，或已经退出）。
var errNoJob = errors.New("进程不在 Job Object 里")

// killTree 是"结束整个进程树"的回退顺序，与平台无关，所以放在没有构建标签的文件里，Linux 上也能测：
//  1. terminateJob：终结进程所在的 Job Object（Windows；没有 Job 或失败时返回 error）；
//  2. taskkill：`taskkill /T /F`（Windows 自带）；
//  3. killMain：只结束主进程。
//
// 前一步成功就不再往下走；全部失败返回最后一个错误。
func killTree(terminateJob, taskkill, killMain func() error) error {
	if terminateJob != nil {
		if err := terminateJob(); err == nil {
			return nil
		}
	}
	if taskkill != nil {
		if err := taskkill(); err == nil {
			return nil
		}
	}
	return killMain()
}

// jobRegistry 记录"进程 pid → Job 句柄"。句柄用 uintptr 表示，方便在非 Windows 平台上测试登记逻辑。
type jobRegistry struct {
	mu sync.Mutex
	m  map[int]uintptr
}

func (r *jobRegistry) add(pid int, h uintptr) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil {
		r.m = map[int]uintptr{}
	}
	r.m[pid] = h
}

// get 返回登记的句柄（不移除）。
func (r *jobRegistry) get(pid int) (uintptr, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.m[pid]
	return h, ok
}

// take 返回并移除登记的句柄；进程退出时调用，拿到句柄的一方负责关闭它。
func (r *jobRegistry) take(pid int) (uintptr, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.m[pid]
	delete(r.m, pid)
	return h, ok
}

func (r *jobRegistry) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.m)
}
