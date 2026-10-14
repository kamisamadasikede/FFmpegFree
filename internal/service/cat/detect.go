package cat

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
)

// v0.47.1（契约 6.19.21）：cat:status 只由服务层发，每条都带全部已注册适配器的 agents[]；
// 检测结果落库到 settings 键 cat.detect，启动先显示上次结果（stale）再后台实时重检。

// DetectKey 是 settings 表里 CLI 检测结果的键（不进 cat.settings）。
const DetectKey = "cat.detect"

// DefaultStatusDebounce 是合并多次状态变化的等待（契约 ≤100ms）。
const DefaultStatusDebounce = 50 * time.Millisecond

// DetectRecord 是 cat.detect 里一家的记录。只用于显示，绝不用来启动进程。
type DetectRecord struct {
	Ready     bool   `json:"ready"`
	Version   string `json:"version"`
	ExePath   string `json:"exePath"`
	CheckedAt int64  `json:"checkedAt"`
}

// checkedAtReporter 由真适配器实现：本进程最近一次实时检测完成的时间（毫秒），0 = 还没检测完。
type checkedAtReporter interface{ CheckedAt() int64 }

type starter interface{ Start() }

// detectState 是服务层的检测状态（并发安全）。
type detectState struct {
	loadOnce sync.Once
	mu       sync.Mutex
	saved    map[string]DetectRecord // 上次落库（含本次运行已写入的）
	written  map[string]int64        // 本进程已写入的 checkedAt（避免重复写）

	timerMu sync.Mutex
	timer   *time.Timer
	emitMu  sync.Mutex // compose + emit 串行：后发的一条一定反映更新的状态
}

func (s *Service) loadDetect() {
	s.det.loadOnce.Do(func() {
		s.det.mu.Lock()
		defer s.det.mu.Unlock()
		s.det.saved = map[string]DetectRecord{}
		s.det.written = map[string]int64{}
		if s.cfg.Store == nil {
			return
		}
		var raw map[string]json.RawMessage
		found, err := s.cfg.Store.GetSetting(context.Background(), DetectKey, &raw)
		if err != nil {
			s.cfg.Logf("cat.detect read: %v", err) // 损坏整体忽略
			return
		}
		if !found {
			return
		}
		for kind, v := range raw {
			if _, ok := s.cfg.Registry.Get(kind); !ok {
				continue // 未知 agentKind 忽略
			}
			var rec DetectRecord
			if json.Unmarshal(v, &rec) != nil || rec.CheckedAt <= 0 {
				continue // 损坏项忽略
			}
			s.det.saved[kind] = rec
		}
	})
}

func (s *Service) savedDetect(kind string) (DetectRecord, bool) {
	s.loadDetect()
	s.det.mu.Lock()
	defer s.det.mu.Unlock()
	r, ok := s.det.saved[kind]
	return r, ok
}

// persistDetect 在某家实时检测完成后覆盖写该家一项；写失败只记日志。
func (s *Service) persistDetect(kind string) {
	a, ok := s.cfg.Registry.Get(kind)
	if !ok || a == nil {
		return
	}
	cr, ok := a.(checkedAtReporter)
	if !ok {
		return
	}
	at := cr.CheckedAt()
	if at <= 0 {
		return
	}
	st := a.Status()
	if st.State == catagent.StateChecking {
		return
	}
	s.loadDetect()
	s.det.mu.Lock()
	defer s.det.mu.Unlock()
	if s.det.written[kind] == at {
		return
	}
	s.det.written[kind] = at
	rec := DetectRecord{Ready: st.State == catagent.StateReady, Version: st.Version, CheckedAt: at}
	if rec.Ready {
		rec.ExePath = a.ExecutablePath()
	}
	s.det.saved[kind] = rec
	if s.cfg.Store == nil {
		return
	}
	snap := make(map[string]DetectRecord, len(s.det.saved))
	for k, v := range s.det.saved {
		snap[k] = v
	}
	if err := s.cfg.Store.SetSetting(context.Background(), DetectKey, snap); err != nil {
		s.cfg.Logf("cat.detect write: %v", err)
	}
}

// AgentChanged 是适配器 OnChange 的落点：落库（检测完成时）并合并发一条全量 cat:status。
func (s *Service) AgentChanged(agentKind string) {
	s.persistDetect(agentKind)
	d := s.cfg.StatusDebounce
	if d == 0 {
		d = DefaultStatusDebounce
	}
	if d < 0 {
		s.emitStatus()
		return
	}
	s.det.timerMu.Lock()
	if s.det.timer == nil {
		s.det.timer = time.AfterFunc(d, func() {
			s.det.timerMu.Lock()
			s.det.timer = nil
			s.det.timerMu.Unlock()
			s.emitStatus()
		})
	}
	s.det.timerMu.Unlock()
}

// emitStatus 组装全量状态并发 cat:status（服务层唯一出口）。
func (s *Service) emitStatus() Status {
	s.det.emitMu.Lock()
	defer s.det.emitMu.Unlock()
	st := s.composeStatus(false)
	s.emit(catagent.EventStatus, st)
	return st
}

// StartDetection：先以 cat.detect 的上次结果发一条（stale），再后台对所有已注册适配器实时检测。
func (s *Service) StartDetection() {
	s.loadDetect()
	s.emitStatus()
	for _, kind := range s.cfg.Registry.Kinds() {
		if a, ok := s.cfg.Registry.Get(kind); ok && a != nil {
			if st, ok := a.(starter); ok {
				st.Start()
			}
		}
	}
}

// RecheckCat 重新检测：agentKind 为空 = 全部；传了只检该家（只有这一行变 checking），
// 返回 / 事件仍是全量 agents[]。未注册 / 未知 agentKind → INVALID_ARGUMENT。
func (s *Service) RecheckCat(ctx context.Context, agentKind string) (Status, error) {
	agentKind = strings.TrimSpace(agentKind)
	kinds := s.cfg.Registry.Kinds()
	if agentKind != "" {
		if a, ok := s.cfg.Registry.Get(agentKind); !ok || a == nil {
			return Status{}, apperr.New(apperr.InvalidArgument, "不认识这个助手")
		}
		kinds = []string{agentKind}
	}
	s.applyAllProviderOverlays(ctx)
	// 各家 Recheck 经 OnChange 发全量；结束后再经同一出口发一条最终全量。
	for _, kind := range kinds {
		if a, ok := s.cfg.Registry.Get(kind); ok && a != nil {
			a.Recheck()
		}
	}
	return s.emitStatus(), nil
}

// agentRow 生成一行：实时检测完成过 → 实时结果；否则有落库 → 上次结果（stale + checking）。
func (s *Service) agentRow(kind string, a catagent.Adapter, st Status) catagent.AgentStatus {
	row := agentStatusOf(kind, st)
	row.Checking = st.State == catagent.StateChecking
	if a == nil {
		return row
	}
	if cr, ok := a.(checkedAtReporter); ok {
		at := cr.CheckedAt()
		if at <= 0 {
			if rec, ok := s.savedDetect(kind); ok {
				row.Ready = rec.Ready
				row.Version = rec.Version
				row.Detail = ""
				row.ExePath = rec.ExePath
				ts := rec.CheckedAt
				row.CheckedAt = &ts
				row.Stale = true
				row.Checking = true
			}
			return row
		}
		row.CheckedAt = &at
		if row.Checking {
			// 检测中保留上一次结果（设计 10-14）：只有从未检测过的才是空字段。
			if rec, ok := s.savedDetect(kind); ok {
				row.Ready, row.Version, row.ExePath, row.Detail = rec.Ready, rec.Version, rec.ExePath, ""
				ts := rec.CheckedAt
				row.CheckedAt = &ts
			}
			return row
		}
	}
	if row.Ready && !row.Checking {
		row.ExePath = a.ExecutablePath()
	}
	return row
}
