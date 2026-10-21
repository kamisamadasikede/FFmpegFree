package cat

import (
	"context"
	"fmt"
	"math"
	"time"

	"FFmpegFree/internal/store"
)

const maxAutoGroupRetries = 3

type groupRetryKey struct{}
type groupRetryState struct {
	chainID, memberID, taskID string
	count                     int
	reason                    string
	freshSession              bool
}

func groupRetryFor(ctx context.Context, chainID, memberID, taskID string) groupRetryState {
	state, _ := ctx.Value(groupRetryKey{}).(groupRetryState)
	if state.chainID != chainID || state.memberID != memberID || state.taskID != taskID {
		return groupRetryState{chainID: chainID, memberID: memberID, taskID: taskID}
	}
	return state
}

func groupQueueOwner(ctx context.Context) *memberQueueState {
	owner, _ := ctx.Value(groupQueueOwnerKey{}).(*memberQueueState)
	return owner
}

func ownsGroupQueue(ctx context.Context, memberID string) bool {
	groupQueueMu.Lock()
	defer groupQueueMu.Unlock()
	owner := groupQueueOwner(ctx)
	return owner != nil && groupQueues[memberID] == owner
}

// Retry only after the old native process and its workspace snapshot have exited.
// The turn stays registered during backoff so both stop buttons can cancel it.
func (s *Service) retryGroupTurn(ctx context.Context, g store.CatGroup, m store.CatGroupMember, chainID string, hop int, afterSeq int64, reassign bool, taskID string, turn *turnState, reason string) bool {
	retry := groupRetryFor(ctx, chainID, m.ID, taskID)
	if retry.count >= maxAutoGroupRetries || groupContextStopped(ctx) || !ownsGroupQueue(ctx, m.ID) {
		return false
	}
	turn.mu.Lock()
	missingSession := turn.sessionMissing
	turn.mu.Unlock()
	s.lastMu.Lock()
	last := s.lastTurns[turn.convID]
	noRetry := last != nil && last.turnID == turn.turnID && last.noRetry
	s.lastMu.Unlock()
	// A missing resume session is recoverable once by starting without resume.
	// Unsupported providers and repeated session loss require user intervention.
	if noRetry && (!missingSession || retry.freshSession) {
		return false
	}
	retry.count++
	retry.reason = reason
	retry.freshSession = retry.freshSession || missingSession
	delay := s.cfg.GroupRetryDelay
	if delay <= 0 {
		delay = 2 * time.Second
	}
	delay *= time.Duration(1 << (retry.count - 1))
	text := fmt.Sprintf("本轮未完成：%s。将在 %.0f 秒后自动重试（%d/%d）。", truncateRunes(reason, 300), math.Ceil(delay.Seconds()), retry.count, maxAutoGroupRetries)
	if missingSession {
		text = "原会话已中断，将以新会话恢复这条请求。" + text
	}

	s.groupStartMu.Lock()
	if groupContextStopped(ctx) || !ownsGroupQueue(ctx, m.ID) {
		s.groupStartMu.Unlock()
		return false
	}
	if taskID != "" {
		s.groupWorkMu.Lock()
		work, err := s.cfg.Store.GetCatGroupWorkflow(ctx, chainID)
		found := false
		if err == nil && work.Status != "stopped" {
			for i := range work.Tasks {
				task := &work.Tasks[i]
				if task.ID == taskID && task.MemberID == m.ID && task.Status == "running" && task.TurnID == turn.turnID {
					task.Status, task.TurnID, task.Result = "queued", "", text
					task.Evidence = ""
					task.RetryAt = time.Now().Add(delay).UnixMilli()
					found = true
					break
				}
			}
		}
		if found {
			err = s.cfg.Store.SaveCatGroupWorkflow(ctx, &work)
		}
		s.groupWorkMu.Unlock()
		if err != nil || !found {
			s.groupStartMu.Unlock()
			return false
		}
		s.emitGroupWorkflow(work)
	}
	msg, err := s.cfg.Store.InsertCatGroupMessage(ctx, store.CatGroupMessage{
		GroupID: g.ID, Kind: "system", SystemKind: "retry", MemberID: m.ID,
		ChainID: chainID, Hop: hop, TurnID: turn.turnID, Text: text,
	})
	if err == nil {
		s.emitGroupMessage(msg)
	}
	s.groupStartMu.Unlock()

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return true
	case <-timer.C:
	}
	s.groupStartMu.Lock()
	defer s.groupStartMu.Unlock()
	if groupContextStopped(ctx) || !ownsGroupQueue(ctx, m.ID) {
		return true
	}
	if taskID != "" {
		s.groupWorkMu.Lock()
		work, err := s.cfg.Store.GetCatGroupWorkflow(ctx, chainID)
		ready := false
		if err == nil && work.Status != "stopped" && !work.Dismissed {
			for _, task := range work.Tasks {
				if task.ID == taskID && task.MemberID == m.ID && task.Status == "queued" && task.TurnID == "" {
					ready = true
					break
				}
			}
		}
		s.groupWorkMu.Unlock()
		if !ready {
			s.emitGroupMember(m, nil, groupQueueOwner(ctx))
			return true
		}
	}
	if a, managed, err := s.assignmentFor(ctx, chainID, m.ID, taskID); err != nil {
		return false
	} else if managed {
		claimed, err := s.cfg.Store.ClaimCatGroupAssignment(context.Background(), a.ID, "running", "retrying", turn.turnID)
		if err != nil || !claimed {
			return false
		}
	}
	nextCtx := context.WithValue(context.WithoutCancel(ctx), groupRetryKey{}, retry)
	item := groupQueueItem{ctx: nextCtx, chainID: chainID, hop: hop, afterSeq: afterSeq, reassign: reassign, fromSeq: afterSeq, taskID: taskID}
	groupQueueMu.Lock()
	owner := groupQueues[m.ID]
	if owner == groupQueueOwner(ctx) {
		// Keep the current request ahead of later messages, without another user message.
		owner.items = append([]groupQueueItem{item}, owner.items...)
	}
	groupQueueMu.Unlock()
	s.emitGroupMemberSnapshot(m)
	return true
}
