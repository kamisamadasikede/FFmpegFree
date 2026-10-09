package task

import (
	"time"
)

// Pool 是任务进哪个调度池（契约 6.5 / v0.26 6.12.18）。
type Pool int

const (
	// PoolBatch 是 batch 池（转换组件的转换、安装等），按 Settings.maxConcurrent 排队。默认值。
	PoolBatch Pool = iota
	// PoolDoc 是文档组件池：只有真正启动文档组件进程的 doc_convert 任务进，并发固定 2，FIFO，与 batch 池互不占用。
	PoolDoc
	// PoolFree 不排队、不占任何名额（直播；文档页的 md ↔ html 与简易转换）。
	PoolFree
)

// PoolSelector 是 Runner 可选实现的接口：返回任务进哪个池。没实现的按类型决定（直播 PoolFree，其余 PoolBatch）。
type PoolSelector interface {
	Pool() Pool
}

// DefaultDocConcurrency 是文档组件池的并发数（契约 6.12.18：固定 2）。
const DefaultDocConcurrency = 2

// EventDocQueue 是文档组件池队列变化事件（契约 6.12.15）。
const EventDocQueue = "doc:queue"

// DocQueueItem / DocQueueEvent 是 doc:queue 的 payload。
type DocQueueItem struct {
	ID            string `json:"id"`
	QueuePosition int    `json:"queuePosition"`
}

// DocQueueEvent 是 doc:queue 的 payload：文档组件池里所有排队任务的当前位置。
type DocQueueEvent struct {
	Items []DocQueueItem `json:"items"`
}

// docQueueInterval 是 doc:queue 的节流间隔（4 次/秒）。
const docQueueInterval = 250 * time.Millisecond

func poolOf(e *entry) Pool {
	if IsLive(e.task.Type) {
		return PoolFree
	}
	if ps, ok := e.runner.(PoolSelector); ok {
		return ps.Pool()
	}
	return PoolBatch
}

// docLimit 返回文档组件池的并发数。
func (m *Manager) docLimit() int {
	if m.cfg.DocConcurrency > 0 {
		return m.cfg.DocConcurrency
	}
	return DefaultDocConcurrency
}

// pumpDoc 在文档组件池有空位时从队列取任务启动。
func (m *Manager) pumpDoc() {
	changed := false
	for {
		m.mu.Lock()
		if m.closing || m.docRunning >= m.docLimit() || len(m.docQueue) == 0 {
			m.mu.Unlock()
			break
		}
		e := m.docQueue[0]
		m.docQueue = m.docQueue[1:]
		m.docRunning++
		m.mu.Unlock()
		changed = true
		e.setQueuePosition(nil)
		m.launch(e, PoolDoc)
	}
	if changed {
		m.docQueueChanged()
	}
}

// predictQueuePosition 在任务入队之前（发 task:created / queued 事件时）估计它会排到第几位：
// 不是文档组件池的任务、或池里还有空位（会立即开始）时返回 nil。同时写进 entry 的快照。
func (m *Manager) predictQueuePosition(e *entry) *int {
	if poolOf(e) != PoolDoc {
		return nil
	}
	m.mu.Lock()
	free := m.docLimit() - m.docRunning
	n := len(m.docQueue)
	m.mu.Unlock()
	if n < free {
		e.setQueuePosition(nil)
		return nil
	}
	pos := n
	e.setQueuePosition(&pos)
	p := pos
	return &p
}

func (e *entry) setQueuePosition(p *int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p == nil {
		e.task.QueuePosition = nil
		return
	}
	v := *p
	e.task.QueuePosition = &v
}

// docQueueChanged 重新编号排队任务并（节流后）发 doc:queue。
func (m *Manager) docQueueChanged() {
	m.mu.Lock()
	q := append([]*entry(nil), m.docQueue...)
	m.mu.Unlock()
	for i, e := range q {
		pos := i
		e.setQueuePosition(&pos)
	}
	m.qmu.Lock()
	defer m.qmu.Unlock()
	if m.qtimer != nil {
		return // 已有一次待发，触发时取最新队列
	}
	wait := docQueueInterval - time.Since(m.qlast)
	if wait <= 0 {
		m.qlast = time.Now()
		go m.emitDocQueue()
		return
	}
	m.qtimer = time.AfterFunc(wait, func() {
		m.qmu.Lock()
		m.qtimer = nil
		m.qlast = time.Now()
		m.qmu.Unlock()
		m.emitDocQueue()
	})
}

func (m *Manager) emitDocQueue() {
	m.mu.Lock()
	items := make([]DocQueueItem, 0, len(m.docQueue))
	for i, e := range m.docQueue {
		items = append(items, DocQueueItem{ID: e.task.ID, QueuePosition: i})
	}
	m.mu.Unlock()
	m.emit(EventDocQueue, DocQueueEvent{Items: items})
}

// DocQueue 返回文档组件池当前排队任务的 id（按位置），测试和调试用。
func (m *Manager) DocQueue() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.docQueue))
	for i, e := range m.docQueue {
		out[i] = e.task.ID
	}
	return out
}
