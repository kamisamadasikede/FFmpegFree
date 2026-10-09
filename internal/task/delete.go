package task

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"FFmpegFree/internal/apperr"
)

// DeleteResult 是 ConvertService.DeleteRecords / DeleteSource 的结果（契约 6.14.2）。
type DeleteResult struct {
	DeletedTaskIDs   []string        `json:"deletedTaskIds"`
	DeletedSourceIDs []string        `json:"deletedSourceIds"`
	DeletedFiles     int             `json:"deletedFiles"`
	Failures         []DeleteFailure `json:"failures"`
}

// DeleteFailure 是没删成的文件 / 记录（契约 6.14.4 的固定枚举和文案）。
type DeleteFailure struct {
	TaskID   string `json:"taskId"`             // 副本删不掉时为 ""
	SourceID string `json:"sourceId,omitempty"` // v0.24：只有副本删不掉的那一条有（6.15.6）
	Path     string `json:"path,omitempty"`
	Reason   string `json:"reason"`
	Message  string `json:"message"`
}

// NewCopyDeleteFailure 是副本删不掉的那一条（契约 6.15.7 第 3、4 步）：taskId 为 ""，带 sourceId。
func NewCopyDeleteFailure(sourceID, path, reason string) DeleteFailure {
	return DeleteFailure{SourceID: sourceID, Path: path, Reason: reason, Message: deleteMessages[reason]}
}

// ClassifyRemoveErr 把删除文件的错误归到 in_use / permission / io（副本删除用）。
func ClassifyRemoveErr(err error) string { return classifyRemoveErr(err) }

// AllowReveal 让删除失败留下的文件在 10 分钟内可以 RevealInFolder（副本删不掉时用，契约 6.15.7 第 4 步）。
func (m *Manager) AllowReveal(path string) { m.allowRevealOfFailure(path, path) }

// DeleteFailure.Reason 的固定枚举（只追加）。
const (
	DeleteInUse         = "in_use"
	DeletePermission    = "permission"
	DeleteNotTaskOutput = "not_task_output"
	DeleteIO            = "io"
	DeleteStillRunning  = "still_running"
)

var deleteMessages = map[string]string{
	DeleteInUse:         "文件正在被使用，没有删除",
	DeletePermission:    "没有权限删除这个文件",
	DeleteNotTaskOutput: "文件已被替换或移动，没有删除",
	DeleteIO:            "删除文件失败",
	DeleteStillRunning:  "任务还没停下来，没有删除这条记录",
}

func newDeleteFailure(id, path, reason string) DeleteFailure {
	return DeleteFailure{TaskID: id, Path: path, Reason: reason, Message: deleteMessages[reason]}
}

// NewDeleteResult 返回各列表都是 [] 的空结果。
func NewDeleteResult() DeleteResult {
	return DeleteResult{DeletedTaskIDs: []string{}, DeletedSourceIDs: []string{}, Failures: []DeleteFailure{}}
}

// removeFile 是删除输出文件的入口，测试里替换以模拟“文件正在被使用”等错误。
var removeFile = os.Remove

// isInUse 判断删除失败是不是“文件正在被使用”。
func isInUse(err error) bool { return isPlatformInUse(err) }

// classifyRemoveErr 把删除输出文件的错误归到契约 6.14.4 的 reason。
func classifyRemoveErr(err error) string {
	switch {
	case isInUse(err):
		return DeleteInUse
	case errors.Is(err, fs.ErrPermission):
		return DeletePermission
	}
	return DeleteIO
}

func (m *Manager) deleteWait() time.Duration {
	if m.cfg.DeleteWait > 0 {
		return m.cfg.DeleteWait
	}
	return 10 * time.Second
}

// DeleteRecords 删除 typ 类型的任务记录（转换页的删除，契约 6.14.4）：
//  1. 校验：有旧类型 id 整体 NOT_FOUND（reason=record）；有不是 typ 的任务整体 INVALID_ARGUMENT；不存在的 id 忽略；按 id 去重；
//  2. 进行中的先按用户取消处理，一共最多等 DeleteWait（默认 10 秒），没停下来的不删（still_running）；
//  3. deleteOutputs 时只删 succeeded 记录登记的输出（安全条件同 Remove，删前调用 beforeRemove(path) 撤销预览 token），
//     不满足安全条件或删除失败：记录照删、文件留着，放进 failures；
//  4. 不管 deleteOutputs，删掉 outputPath 对应的 .part 残留（不计数，失败只记日志）；
//  5. 输入文件永远不碰；
//  6. 一个事务删记录，再删日志，发一次 task:removed。
//
// 文件没删成不是错误。ids 的数量上限由调用方校验。
func (m *Manager) DeleteRecords(ids []string, typ Type, deleteOutputs bool, beforeRemove func(path string)) (DeleteResult, error) {
	res := NewDeleteResult()
	ctx := context.Background()
	ids = dedupe(ids)
	if lf, ok := m.cfg.Store.(LegacyFinder); ok {
		legacy, err := lf.LegacyTaskIDs(ctx, ids)
		if err != nil {
			return res, apperr.Wrap(apperr.Internal, "读取任务失败", err)
		}
		if len(legacy) > 0 {
			return res, RecordNotFound()
		}
	}
	var present []string
	for _, id := range ids {
		t, err := m.Get(id)
		if err != nil {
			if apperr.Is(err, apperr.NotFound) {
				continue
			}
			return res, apperr.Wrap(apperr.Internal, "读取任务失败", err)
		}
		if typ == TypeConvert && !IsRecordTask(t) || typ != TypeConvert && t.Type != typ {
			return res, apperr.New(apperr.InvalidArgument, "只能删除转换记录")
		}
		present = append(present, id)
	}

	// 2. 进行中的先取消，一共最多等 deleteWait。
	var waiting []*entry
	m.mu.Lock()
	for _, id := range present {
		if e, ok := m.entries[id]; ok {
			waiting = append(waiting, e)
		}
	}
	m.mu.Unlock()
	for _, e := range waiting {
		if err := m.Cancel(e.task.ID); err != nil && !apperr.Is(err, apperr.TaskConflict) {
			m.logf("删除前取消任务 %s 失败: %v", e.task.ID, err)
		}
	}
	if len(waiting) > 0 {
		deadline := time.NewTimer(m.deleteWait())
		defer deadline.Stop()
	wait:
		for _, e := range waiting {
			select {
			case <-e.done:
			case <-deadline.C:
				break wait
			}
		}
	}

	var toDelete []string
	recs := map[string]Task{}
	for _, id := range present {
		m.mu.Lock()
		_, active := m.entries[id]
		m.mu.Unlock()
		if active {
			res.Failures = append(res.Failures, newDeleteFailure(id, "", DeleteStillRunning))
			continue
		}
		t, err := m.cfg.Store.GetTask(ctx, id)
		if err != nil {
			continue // 期间被别人删掉了
		}
		if t.Status.Active() { // 库里还是进行中（不应发生）：按没停下来处理
			res.Failures = append(res.Failures, newDeleteFailure(id, "", DeleteStillRunning))
			continue
		}
		recs[id] = t
		toDelete = append(toDelete, id)

		// 3. 输出文件。
		if deleteOutputs && t.Status == StatusSucceeded && t.OutputPath != "" {
			switch reason := unsafeToDeleteOutput(t); reason {
			case "skip":
			case "":
				if beforeRemove != nil {
					beforeRemove(t.OutputPath)
				}
				if err := removeFile(t.OutputPath); err != nil {
					if !errors.Is(err, os.ErrNotExist) {
						m.logf("删除任务 %s 的输出 %s 失败: %v", t.ID, t.OutputPath, err)
						res.Failures = append(res.Failures, newDeleteFailure(id, t.OutputPath, classifyRemoveErr(err)))
						m.allowRevealOfFailure(res.Failures[len(res.Failures)-1].Path, t.OutputPath)
					}
				} else {
					res.DeletedFiles++
				}
			default:
				m.logf("不删除任务 %s 的输出 %s：%s", t.ID, t.OutputPath, reason)
				res.Failures = append(res.Failures, newDeleteFailure(id, t.OutputPath, DeleteNotTaskOutput))
				// not_task_output 的路径也是这条记录登记的输出（删除只尝试登记的输出），只放行它本身。
				m.allowRevealOfFailure(res.Failures[len(res.Failures)-1].Path, t.OutputPath)
			}
		}
		// 4. .part 残留（别的任务正占着这个名字时不碰）；重转的临时文件兜底再删一次（6.17.6，名字里带 taskId）。
		if t.OutputPath != "" && !m.namer.heldByOther(t.OutputPath, t.ID) {
			if err := removeStalePart(t.OutputPath, t.StartedAt); err != nil {
				m.logf("删除任务 %s 的 .part 残留失败: %v", t.ID, err)
			}
		}
		m.removeReconvertTemp(t)
	}

	// 6. 删记录、删日志、发事件。
	if len(toDelete) > 0 {
		deleted, err := m.cfg.Store.DeleteTasks(ctx, toDelete)
		if err != nil {
			return res, apperr.Wrap(apperr.Internal, "删除记录失败", err)
		}
		for _, id := range deleted {
			for _, p := range m.cleanupFiles(recs[id], false) {
				m.logf("清理任务 %s 的日志失败: %s", id, p)
			}
		}
		if len(deleted) > 0 {
			res.DeletedTaskIDs = deleted
			m.emit(EventRemoved, RemovedEvent{IDs: deleted})
		}
	}
	return res, nil
}

// TaskFile 返回任务登记的文件路径（契约 6.14.3 GetPreviewURL / OpenWithSystem / RevealRecord / GetRecordThumbnail）：
// which=input 取 inputPaths[0]；which=output 取 outputPath，只有 succeeded 才有输出，且不能是符号链接。
// 不存在 / 旧类型 NOT_FOUND（reason=record）；不是 succeeded、路径为空、文件不在或不是普通文件 NOT_FOUND（reason=file）。
// 返回任务本身，供调用方检查类型等。
func (m *Manager) TaskFile(taskID, which string) (string, Task, error) {
	if which != "input" && which != "output" {
		return "", Task{}, apperr.New(apperr.InvalidArgument, "参数不正确").WithDetail(`which 只能是 "input" 或 "output"`)
	}
	t, err := m.Get(taskID)
	if err != nil {
		if apperr.Is(err, apperr.NotFound) {
			return "", Task{}, RecordNotFound()
		}
		return "", Task{}, apperr.Wrap(apperr.Internal, "读取任务失败", err)
	}
	var p string
	if which == "input" {
		if len(t.InputPaths) > 0 {
			p = t.InputPaths[0]
		}
		if p == "" || !absPath(p) {
			return "", t, FileNotFound()
		}
		fi, err := os.Stat(p)
		if err != nil || !fi.Mode().IsRegular() {
			return "", t, FileNotFound()
		}
		return p, t, nil
	}
	p = t.OutputPath
	// 重转中的记录按 succeeded 处理，作用在旧文件上（契约 6.17.4）。
	if (t.Status != StatusSucceeded && !t.Reconverting) || p == "" || !absPath(p) {
		return "", t, FileNotFound()
	}
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() { // 符号链接的 Mode 不是 Regular
		return "", t, FileNotFound()
	}
	return p, t, nil
}

func absPath(p string) bool { return filepath.IsAbs(p) }
