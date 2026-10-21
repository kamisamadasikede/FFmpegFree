package cat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

func TestGroupAutomaticRetryRecoversFailures(t *testing.T) {
	for _, mode := range []string{"error", "idle", "timeout", "empty", "session"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			started := make(chan catagent.TurnOptions, 8)
			s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
				started <- o
				if calls.Add(1) == 1 {
					switch mode {
					case "idle":
						return catagent.TurnResponse{}, apperr.New(apperr.CatReplyFailed, catagent.MsgIdleTimeout).WithDetail(catagent.ReasonIdleTimeout)
					case "timeout":
						return catagent.TurnResponse{}, apperr.New(apperr.CatReplyFailed, "等待过久").WithDetail("reason=timeout")
					case "empty":
						return catagent.TurnResponse{}, nil
					case "session":
						return catagent.TurnResponse{}, apperr.New(apperr.CatReplyFailed, "原会话不存在").WithDetail(catagent.ReasonSessionNotFound)
					default:
						return catagent.TurnResponse{}, errors.New("temporary failure")
					}
				}
				return catagent.TurnResponse{Message: catagent.WireMessage{Content: "恢复完成"}}, nil
			})
			ctx := context.Background()
			sent, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "处理这条请求", Mentions: []string{ms[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			s.Wait()
			if calls.Load() != 2 {
				t.Fatalf("calls=%d", calls.Load())
			}
			first, second := nextGroupCall(t, started), nextGroupCall(t, started)
			if first.TurnID == second.TurnID || !strings.Contains(second.Messages[0].Content, "从中断处继续") {
				t.Fatal("retry must get a new turn and retain recovery instructions")
			}
			if mode == "session" && second.Resume {
				t.Fatal("missing native session must start fresh")
			}
			msgs, err := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			counts := map[string]int{}
			for _, msg := range msgs {
				if msg.ChainID != sent.ChainID {
					t.Fatalf("retry changed request: %+v", msg)
				}
				counts[msg.Kind]++
				counts[msg.SystemKind]++
				if msg.Kind == "member" && msg.Text != "恢复完成" {
					t.Fatalf("partial failure delivered: %+v", msg)
				}
			}
			if counts["user"] != 1 || counts["member"] != 1 || counts["retry"] != 1 || counts["ack"] != 2 || counts["failed"] != 0 {
				t.Fatalf("messages=%v", counts)
			}
			if groupRunningSnapshot(ms[0].ID) != nil {
				t.Fatal("finished retry left member busy")
			}
		})
	}
}

func TestGroupAutomaticRetryBudget(t *testing.T) {
	var calls atomic.Int32
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		calls.Add(1)
		return catagent.TurnResponse{}, errors.New("persistent failure")
	})
	ctx := context.Background()
	if _, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "执行", Mentions: []string{ms[0].ID}}); err != nil {
		t.Fatal(err)
	}
	s.Wait()
	if calls.Load() != 4 {
		t.Fatalf("expected initial run + three retries, got %d", calls.Load())
	}
	msgs, _ := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
	retries, failed := 0, 0
	for _, msg := range msgs {
		if msg.SystemKind == "retry" {
			retries++
		}
		if msg.SystemKind == "failed" {
			failed++
			if !strings.Contains(msg.Text, "自动重试 3 次") {
				t.Fatalf("missing recovery guidance: %+v", msg)
			}
		}
	}
	if retries != 3 || failed != 1 {
		t.Fatalf("retries=%d failures=%d", retries, failed)
	}
}

func TestGroupStopDuringRetryAllowsNewRequest(t *testing.T) {
	for _, memberOnly := range []bool{false, true} {
		t.Run(fmt.Sprint("member=", memberOnly), func(t *testing.T) {
			var calls atomic.Int32
			retrying := make(chan struct{}, 1)
			s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
				if calls.Add(1) == 1 {
					return catagent.TurnResponse{}, errors.New("retry later")
				}
				return catagent.TurnResponse{Message: catagent.WireMessage{Content: "新请求完成"}}, nil
			})
			s.cfg.GroupRetryDelay = time.Hour
			s.cfg.Emit = func(event string, p any) {
				if event == "cat:group-message" {
					if m, ok := p.(store.CatGroupMessage); ok && m.SystemKind == "retry" {
						retrying <- struct{}{}
					}
				}
			}
			ctx := context.Background()
			if _, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "旧请求", Mentions: []string{ms[0].ID}}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-retrying:
			case <-time.After(5 * time.Second):
				t.Fatal("no retry status")
			}
			if groupRunningSnapshot(ms[0].ID) == nil {
				t.Fatal("stop control disappeared during retry backoff")
			}
			var err error
			if memberOnly {
				err = s.StopCatGroupMember(ctx, ms[0].ID)
			} else {
				err = s.StopCatGroup(ctx, g.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "新请求", Mentions: []string{ms[0].ID}}); err != nil {
				t.Fatal(err)
			}
			s.Wait()
			if calls.Load() != 2 {
				t.Fatalf("old retry resurrected or new request failed: %d", calls.Load())
			}
		})
	}
}

func TestGroupStopThenImmediateSendIgnoresLateOldExecution(t *testing.T) {
	for _, memberOnly := range []bool{false, true} {
		t.Run(fmt.Sprint("member=", memberOnly), func(t *testing.T) {
			started := make(chan catagent.TurnOptions, 4)
			releaseOld, releaseNew := make(chan struct{}), make(chan struct{})
			var onceOld, onceNew sync.Once
			var calls atomic.Int32
			s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
				n := calls.Add(1)
				started <- o
				if n == 1 {
					<-o.Ctx.Done()
					<-releaseOld // Simulate delayed native process cleanup.
					o.OnActivity("tool", "late-old", "old callback", true)
					return catagent.TurnResponse{Message: catagent.WireMessage{Content: "过期结果"}}, nil
				}
				select {
				case <-releaseNew:
				case <-o.Ctx.Done():
					return catagent.TurnResponse{}, o.Ctx.Err()
				}
				return catagent.TurnResponse{Message: catagent.WireMessage{Content: "新的回复"}}, nil
			})
			t.Cleanup(func() { onceOld.Do(func() { close(releaseOld) }); onceNew.Do(func() { close(releaseNew) }) })
			ctx := context.Background()
			if _, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "旧请求", Mentions: []string{ms[0].ID}}); err != nil {
				t.Fatal(err)
			}
			old := nextGroupCall(t, started)
			var err error
			if memberOnly {
				err = s.StopCatGroupMember(ctx, ms[0].ID)
			} else {
				err = s.StopCatGroup(ctx, g.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			next, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "介绍你自己", Mentions: []string{ms[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			waitGroupWork(t, func() bool { r := groupRunningSnapshot(ms[0].ID); return r != nil && r.TurnID != old.TurnID })
			current := groupRunningSnapshot(ms[0].ID)
			if current == nil || current.TurnID == old.TurnID {
				t.Fatal("new request remained in detached old queue")
			}
			msgs, _ := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
			ack := false
			for _, msg := range msgs {
				if msg.ChainID == next.ChainID && msg.SystemKind == "ack" {
					ack = true
				}
			}
			if !ack {
				t.Fatal("new request got no acknowledgement while old execution was cleaning up")
			}
			if calls.Load() != 1 {
				t.Fatal("native executions overlapped")
			}
			onceOld.Do(func() { close(releaseOld) })
			fresh := nextGroupCall(t, started)
			if fresh.TurnID != current.TurnID {
				t.Fatal("wrong request started")
			}
			// Old deferred completion must not clear/drain the replacement queue.
			waitGroupWork(t, func() bool {
				select {
				case <-old.Ctx.Done():
					return true
				default:
					return false
				}
			})
			state := groupRunningSnapshot(ms[0].ID)
			if state == nil || state.TurnID != fresh.TurnID {
				t.Fatal("old completion cleared replacement status")
			}
			onceNew.Do(func() { close(releaseNew) })
			s.Wait()
			msgs, _ = s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
			deliveries := 0
			for _, msg := range msgs {
				if msg.Kind == "member" {
					deliveries++
					if msg.Text != "新的回复" || msg.ChainID != next.ChainID {
						t.Fatalf("stale delivery: %+v", msg)
					}
				}
			}
			if deliveries != 1 || calls.Load() != 2 {
				t.Fatalf("deliveries=%d calls=%d", deliveries, calls.Load())
			}
		})
	}
}
