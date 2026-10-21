package cat

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

func TestGroupRouteFallbackAlwaysDispatchesReply(t *testing.T) {
	cases := []struct {
		name, output, reason string
		err                  error
	}{
		{name: "empty output", reason: "empty"},
		{name: "empty recipients", output: `{"memberIds":[],"reason":"无需开发","tasks":[]}`, reason: "empty"},
		{name: "invalid JSON", output: "交给前端看看", reason: "parse_error"},
		{name: "unknown member", output: `{"memberIds":["nobody"],"tasks":[]}`, reason: "unknown_id"},
		{name: "execution error", reason: "run_error", err: errors.New("route failed")},
		{name: "timeout", reason: "timeout", err: context.DeadlineExceeded},
		{name: "invalid plan", output: `{"memberIds":["$FIRST"],"tasks":[{"id":"bad"}]}`, reason: "parse_error"},
		// Even a valid plan in an unsuccessful response must not be executed.
		{name: "error with plan", output: `{"memberIds":["$FIRST"],"tasks":[{"id":"v","title":"验收","memberId":"$FIRST","instruction":"检查","kind":"verify"}]}`, reason: "run_error", err: errors.New("route failed")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			calls := make(chan catagent.TurnOptions, 8)
			output := tc.output
			s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
				if o.NoTools {
					return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: output}}, tc.err
				}
				calls <- o
				return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "我来查看当前应用界面。"}}, nil
			})
			output = strings.ReplaceAll(output, "$FIRST", ms[0].ID)
			sent, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "我想知道现在应用长什么样，截图给我看看"})
			if err != nil {
				t.Fatal(err)
			}
			s.Wait()
			if len(calls) != 1 {
				t.Fatalf("want one member execution, got %d", len(calls))
			}
			call := <-calls
			if call.ConversationID != ms[0].ID {
				t.Fatalf("wrong fallback member: %s", call.ConversationID)
			}
			if !strings.Contains(call.Rules, "被安排后必须回应用户") {
				t.Fatalf("missing reply rule: %s", call.Rules)
			}
			messages, err := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			replies, routes := 0, 0
			for _, m := range messages {
				if m.ChainID != sent.ChainID {
					continue
				}
				if m.Kind == "member" {
					replies++
				}
				if m.Route == nil {
					continue
				}
				if m.Route.Mode == "undecided" {
					t.Fatalf("message left unanswered: %+v", m)
				}
				if m.SystemKind == "coord" {
					routes++
					if m.Route.Mode != "fallback" || m.Route.MemberID != ms[0].ID || m.Route.Reason != tc.reason {
						t.Fatalf("wrong fallback route: %+v", m.Route)
					}
					detail, err := s.cfg.Store.GetCatGroupRoute(ctx, m.Route.RouteSeq)
					if err != nil || detail.Mode != "fallback" || detail.MemberID != ms[0].ID || detail.Reason != tc.reason || detail.OutputText != output {
						t.Fatalf("fallback lost diagnostics: %+v %v", detail, err)
					}
				}
			}
			if replies != 1 || routes != 1 {
				t.Fatalf("replies=%d routes=%d", replies, routes)
			}
			if _, err := s.cfg.Store.GetCatGroupWorkflow(ctx, sent.ChainID); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("fallback must not execute rejected plan: %v", err)
			}
		})
	}
}

type routeUnavailableAdapter struct{ *groupWorkAdapter }

func (a *routeUnavailableAdapter) Status() catagent.Status  { return catagent.Status{} }
func (a *routeUnavailableAdapter) Recheck() catagent.Status { return a.Status() }

func TestGroupRouteFallbackSkipsUnavailableMembers(t *testing.T) {
	calls := make(chan catagent.TurnOptions, 4)
	run := func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if !o.NoTools {
			calls <- o
		}
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "收到"}}, nil
	}
	s, g, ms, _ := groupWorkTestService(t, run)
	s.cfg.Registry.Register(&routeUnavailableAdapter{&groupWorkAdapter{kind: ms[0].AgentKind, run: run}})
	if _, err := s.SendCatGroupMessage(context.Background(), SendGroupMessageRequest{GroupID: g.ID, Text: "帮我看看"}); err != nil {
		t.Fatal(err)
	}
	s.Wait()
	if len(calls) != 1 {
		t.Fatalf("want one available member, got %d", len(calls))
	}
	if call := <-calls; call.ConversationID != ms[1].ID {
		t.Fatalf("wrong available member: %s", call.ConversationID)
	}
}

func TestGroupRouteFallbackDoesNotReviveStoppedRouting(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	calls := make(chan catagent.TurnOptions, 4)
	s, g, _, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if o.NoTools {
			close(started)
			<-release // Simulate a router that ignores cancellation and fails late.
			return catagent.TurnResponse{}, errors.New("route failed")
		}
		calls <- o
		return catagent.TurnResponse{}, nil
	})
	sent := make(chan error, 1)
	go func() {
		_, err := s.SendCatGroupMessage(context.Background(), SendGroupMessageRequest{GroupID: g.ID, Text: "看看界面"})
		sent <- err
	}()
	<-started
	if err := s.StopCatGroup(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	s.Wait()
	if len(calls) != 0 {
		t.Fatal("stopped routing started a fallback member")
	}
}

// Keep the router's contract and member rules aligned for ordinary questions.
func TestGroupRouteScreenshotQuestionGetsReplyWithoutWorkflow(t *testing.T) {
	var members []store.CatGroupMember
	calls := make(chan catagent.TurnOptions, 4)
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		calls <- o
		if o.NoTools {
			for _, rule := range []string{"每条用户消息都必须至少安排一位成员回复", "请求截图无需开发计划", "仅当 tasks 非空时"} {
				if !strings.Contains(o.Messages[0].Content, rule) {
					t.Errorf("missing routing instruction: %s", rule)
				}
			}
			return catagent.TurnResponse{Message: catagent.WireMessage{Content: `{"memberIds":["` + members[1].ID + `"],"reason":"查看界面","tasks":[]}`}}, nil
		}
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "我来检查当前界面并提供截图。"}}, nil
	})
	members = ms
	sent, err := s.SendCatGroupMessage(context.Background(), SendGroupMessageRequest{GroupID: g.ID, Text: "应用长什么样，截图看看"})
	if err != nil {
		t.Fatal(err)
	}
	s.Wait()
	if len(calls) != 2 {
		t.Fatalf("want router and one member, got %d", len(calls))
	}
	<-calls
	if call := <-calls; call.ConversationID != ms[1].ID {
		t.Fatalf("wrong selected member: %s", call.ConversationID)
	}
	if _, err := s.cfg.Store.GetCatGroupWorkflow(context.Background(), sent.ChainID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("screenshot question created workflow: %v", err)
	}
}
