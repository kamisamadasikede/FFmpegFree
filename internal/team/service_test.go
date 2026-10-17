package team

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/agentexec"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

type fakeAdapter struct {
	run func(catagent.TurnOptions) (catagent.TurnResponse, error)
}

func (*fakeAdapter) Kind() string                                                    { return catagent.KindCodexCLI }
func (*fakeAdapter) ExecutablePath() string                                          { return "fake" }
func (*fakeAdapter) ProtocolVersion() int                                            { return 1 }
func (*fakeAdapter) Status() catagent.Status                                         { return catagent.Status{State: catagent.StateReady} }
func (a *fakeAdapter) Recheck() catagent.Status                                      { return a.Status() }
func (*fakeAdapter) ListModels() ([]catagent.Model, error)                           { return nil, nil }
func (*fakeAdapter) ListThinkLevels() ([]catagent.ThinkLevel, error)                 { return nil, nil }
func (a *fakeAdapter) RunTurn(o catagent.TurnOptions) (catagent.TurnResponse, error) { return a.run(o) }

func testService(t *testing.T, fn func(catagent.TurnOptions) (catagent.TurnResponse, error)) (*Service, *store.Store) {
	t.Helper()
	root := t.TempDir()
	st, e := store.Open(context.Background(), filepath.Join(root, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	rt := agentexec.New()
	reg := catagent.NewRegistry()
	reg.Register(&fakeAdapter{run: fn})
	s, e := New(context.Background(), Config{Repository: &Repository{st.DB()}, Registry: reg, Execution: rt, DataRoot: root})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close(); rt.Close(); st.Close() })
	return s, st
}
func testRoom(t *testing.T, s *Service, count int) (Room, []Member) {
	t.Helper()
	ids := []string{}
	for i := 0; i < count; i++ {
		v, e := s.SaveEmployee(context.Background(), Employee{Name: "员工" + string(rune('甲'+i)), Role: "工程师", AgentKind: catagent.KindCodexCLI, Enabled: true})
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, v.ID)
	}
	r, e := s.CreateRoom(context.Background(), CreateRoom{Name: "测试群", EmployeeIDs: ids})
	if e != nil {
		t.Fatal(e)
	}
	v, e := s.Snapshot(context.Background(), r.ID, 0)
	if e != nil {
		t.Fatal(e)
	}
	return r, v.Members
}
func toolCall(o catagent.TurnOptions, name string, value any) (json.RawMessage, error) {
	b, _ := json.Marshal(value)
	req, _ := http.NewRequest("POST", o.ToolEnv["FFC_TEAM_URL"]+"/v1/tools/"+name, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+o.ToolEnv["FFC_TEAM_TOKEN"])
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	var out json.RawMessage
	e = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 200 {
		return out, conflict(string(out))
	}
	return out, e
}
func eventually(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("condition did not settle")
}

func TestHumanMentionDispatchAndIdempotency(t *testing.T) {
	var calls atomic.Int32
	var observed string
	s, _ := testService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		calls.Add(1)
		observed = o.Messages[0].Content
		o.OnTextDelta("已完成")
		_, e := toolCall(o, "team_finish", map[string]any{"requestId": "finish", "result": "已交付", "validation": "not_run"})
		return catagent.TurnResponse{}, e
	})
	r, ms := testRoom(t, s, 2)
	var agent Member
	for _, m := range ms {
		if m.Kind == "agent" {
			agent = m
			break
		}
	}
	c := Command{RequestID: "one", RoomID: r.ID, Intent: "new_task", Content: "请设计界面", TargetMemberIDs: []string{agent.ID}}
	a, e := s.Command(context.Background(), c)
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.Command(context.Background(), c)
	if e != nil || a.Message.ID != b.Message.ID {
		t.Fatalf("idempotency: %v", e)
	}
	eventually(t, func() bool {
		v, _ := s.Snapshot(context.Background(), r.ID, 0)
		return len(v.Tasks) == 1 && v.Tasks[0].Status == "completed"
	})
	v, e := s.Snapshot(context.Background(), r.ID, 0)
	if e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 1 || len(v.Inbox) != 1 || v.Tasks[0].Result != "已交付" {
		t.Fatalf("calls=%d inbox=%v task=%+v", calls.Load(), v.Inbox, v.Tasks)
	}
	for _, m := range ms {
		if !strings.Contains(observed, m.ID) {
			t.Errorf("missing roster member %s", m.ID)
		}
	}
	c.Content = "changed"
	if _, e = s.Command(context.Background(), c); e == nil {
		t.Fatal("request id content conflict accepted")
	}
	if len(v.Messages) < 3 {
		t.Fatalf("missing public results: %+v", v.Messages)
	}
}
func TestEmployeeDelegationYieldAndResume(t *testing.T) {
	var parentCalls atomic.Int32
	var childID string
	s, _ := testService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		var contextData map[string]json.RawMessage
		raw := o.Messages[0].Content
		start := strings.Index(raw, "{")
		if e := json.Unmarshal([]byte(raw[start:]), &contextData); e != nil {
			return catagent.TurnResponse{}, e
		}
		var self Member
		_ = json.Unmarshal(contextData["self"], &self)
		if self.ID == childID {
			_, e := toolCall(o, "team_finish", map[string]any{"requestId": "finish", "result": "同事交付", "validation": "not_run"})
			return catagent.TurnResponse{}, e
		}
		if parentCalls.Add(1) == 1 {
			out, e := toolCall(o, "team_delegate", map[string]any{"requestId": "delegate", "toMemberId": childID, "content": "完成数据方案", "requiredForCompletion": true})
			if e != nil {
				return catagent.TurnResponse{}, e
			}
			var res CommandResult
			_ = json.Unmarshal(out, &res)
			_, e = toolCall(o, "team_yield", map[string]any{"requestId": "yield", "waitFor": []string{res.Deliveries[0].TaskID}})
			return catagent.TurnResponse{}, e
		}
		_, e := toolCall(o, "team_finish", map[string]any{"requestId": "finish", "result": "整合完成", "validation": "not_run"})
		return catagent.TurnResponse{}, e
	})
	r, ms := testRoom(t, s, 2)
	agents := []Member{}
	for _, m := range ms {
		if m.Kind == "agent" {
			agents = append(agents, m)
		}
	}
	childID = agents[1].ID
	_, e := s.Command(context.Background(), Command{RequestID: "root", RoomID: r.ID, Intent: "new_task", Content: "请协作完成方案", TargetMemberIDs: []string{agents[0].ID}})
	if e != nil {
		t.Fatal(e)
	}
	eventually(t, func() bool {
		v, _ := s.Snapshot(context.Background(), r.ID, 0)
		return len(v.Tasks) == 2 && v.Tasks[0].Status == "completed" && v.Tasks[1].Status == "completed"
	})
	if parentCalls.Load() != 2 {
		t.Fatalf("parent runs %d", parentCalls.Load())
	}
}
func TestBridgeRejectsOriginAndRevokedCapabilities(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan catagent.TurnOptions, 1)
	s, _ := testService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		select {
		case <-gate:
		case <-o.Ctx.Done():
		}
		return catagent.TurnResponse{}, nil
	})
	r, ms := testRoom(t, s, 1)
	var m Member
	for _, v := range ms {
		if v.Kind == "agent" {
			m = v
		}
	}
	_, e := s.Command(context.Background(), Command{RequestID: "a", RoomID: r.ID, Intent: "new_task", Content: "test", TargetMemberIDs: []string{m.ID}})
	if e != nil {
		t.Fatal(e)
	}
	var o catagent.TurnOptions
	select {
	case o = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("not started")
	}
	req, _ := http.NewRequest("POST", o.ToolEnv["FFC_TEAM_URL"]+"/v1/tools/team_members", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+o.ToolEnv["FFC_TEAM_TOKEN"])
	req.Header.Set("Origin", "https://untrusted.example")
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal(resp.StatusCode)
	}
	if e = s.SetMember(context.Background(), r.ID, m.ProfileID, false); e != nil {
		t.Fatal(e)
	}
	if _, e = toolCall(o, "team_members", map[string]any{}); e == nil {
		t.Fatal("removed member retained tool access")
	}
	close(gate)
}
