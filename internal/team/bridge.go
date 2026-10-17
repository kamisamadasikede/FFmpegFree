package team

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type capability struct {
	Task       Task
	Generation int64
}
type bridge struct {
	svc      *Service
	mu       sync.Mutex
	tokens   map[string]capability
	server   *http.Server
	listener net.Listener
	url      string
}

func newBridge(s *Service) (*bridge, error) {
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	b := &bridge{svc: s, tokens: map[string]capability{}, listener: l, url: "http://" + l.Addr().String()}
	b.server = &http.Server{Handler: http.HandlerFunc(b.serve), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 16 << 10}
	go b.server.Serve(l)
	return b, nil
}
func (b *bridge) grant(t Task, ms []Member) string {
	var secret [32]byte
	if _, e := rand.Read(secret[:]); e != nil {
		panic(e)
	}
	token := hex.EncodeToString(secret[:])
	var gen int64
	for _, m := range ms {
		if m.ID == t.MemberID {
			gen = m.Generation
		}
	}
	b.mu.Lock()
	b.tokens[token] = capability{t, gen}
	b.mu.Unlock()
	return token
}
func (b *bridge) revoke(token string) { b.mu.Lock(); delete(b.tokens, token); b.mu.Unlock() }
func (b *bridge) close() {
	_ = b.server.Close()
	b.mu.Lock()
	b.tokens = map[string]capability{}
	b.mu.Unlock()
}
func (b *bridge) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" || r.Header.Get("Origin") != "" {
		http.Error(w, "forbidden", 403)
		return
	}
	host, _, e := net.SplitHostPort(r.Host)
	if e != nil || host != "127.0.0.1" {
		http.Error(w, "forbidden", 403)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	b.mu.Lock()
	cap, ok := b.tokens[token]
	b.mu.Unlock()
	if !ok {
		http.Error(w, "invalid capability", 401)
		return
	}
	tool := strings.TrimPrefix(r.URL.Path, "/v1/tools/")
	if r.URL.Path == "/v1/office" {
		tool = "office_compat"
	}
	if tool == r.URL.Path || strings.Contains(tool, "/") {
		http.NotFound(w, r)
		return
	}
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	if e != nil {
		http.Error(w, "request too large", 413)
		return
	}
	if strings.HasPrefix(tool, "office_") {
		var args map[string]any
		if e = json.Unmarshal(body, &args); e != nil || args == nil {
			http.Error(w, "invalid request JSON", 400)
			return
		}
		if tool == "office_compat" {
			action, _ := args["action"].(string)
			tool = "office_" + action
		}
		result, e := b.svc.officeTool(r.Context(), cap, tool, args)
		if e != nil {
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": errorText(e)})
			return
		}
		_, _ = w.Write(result)
		return
	}
	var req toolRequest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&req); e != nil {
		http.Error(w, "invalid request JSON", 400)
		return
	}
	if e = decoder.Decode(&struct{}{}); e != io.EOF {
		http.Error(w, "multiple JSON values", 400)
		return
	}
	result, e := b.svc.tool(r.Context(), cap, tool, req)
	if e != nil {
		w.WriteHeader(409)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": errorText(e)})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

type toolRequest struct {
	RequestID       string   `json:"requestId"`
	ToMemberID      string   `json:"toMemberId"`
	TargetMemberIDs []string `json:"targetMemberIds"`
	Content         string   `json:"content"`
	Title           string   `json:"title"`
	Acceptance      []string `json:"acceptance"`
	DependsOn       []string `json:"dependsOn"`
	Required        bool     `json:"requiredForCompletion"`
	TaskID          string   `json:"taskId"`
	ExpectedRunID   string   `json:"expectedRunId"`
	WaitFor         []string `json:"waitFor"`
	Result          string   `json:"result"`
	Validation      string   `json:"validation"`
	Category        string   `json:"category"`
	Before          int64    `json:"before"`
	Limit           int      `json:"limit"`
	Offset          int      `json:"offset"`
	MessageID       string   `json:"messageId"`
	MemberID        string   `json:"memberId"`
	Query           string   `json:"query"`
	Path            string   `json:"path"`
}

func (s *Service) authorize(t *Tx, cap capability) (Task, error) {
	v, e := t.Task(cap.Task.ID)
	if e != nil {
		return v, e
	}
	m, e := t.Member(v.MemberID)
	if e != nil {
		return v, e
	}
	r, e := t.Room(v.RoomID)
	if e != nil {
		return v, e
	}
	if !m.Active || m.Generation != cap.Generation || r.Archived || v.RunID != cap.Task.RunID || !(v.Status == "running" || v.Status == "waiting_user" || v.Status == "waiting_agent") {
		return v, conflict("当前执行或成员凭证已失效")
	}
	return v, nil
}
func (s *Service) tool(ctx context.Context, cap capability, tool string, req toolRequest) (any, error) {
	var task Task
	_, e := s.cfg.Repository.Write(ctx, func(t *Tx) error { var e error; task, e = s.authorize(t, cap); return e })
	if e != nil {
		return nil, e
	}
	if tool == "team_publish" || tool == "team_delegate" || tool == "team_ask" || tool == "team_report" || tool == "team_steer" {
		intent := map[string]string{"team_publish": "publish", "team_delegate": "new_task", "team_ask": "ask", "team_report": "report", "team_steer": "steer"}[tool]
		targets := req.TargetMemberIDs
		if req.ToMemberID != "" {
			if len(targets) > 0 {
				return nil, bad("目标字段冲突")
			}
			targets = []string{req.ToMemberID}
		}
		if tool == "team_report" && req.Category == "completed" {
			return nil, bad("完成汇报使用 team_finish；应用在任务收尾校验后自动通知负责人")
		}
		if tool == "team_ask" && len(targets) != 1 {
			return nil, bad("每次提问请选择一位接收者，以便准确关联答复")
		}
		if tool == "team_report" {
			if len(targets) != 1 {
				return nil, bad("请选择一位人类成员汇报")
			}
			var recipient Member
			_, e = s.cfg.Repository.Write(ctx, func(t *Tx) error { var e error; recipient, e = t.Member(targets[0]); return e })
			if e != nil {
				return nil, e
			}
			if recipient.Kind != "human" || recipient.RoomID != task.RoomID {
				return nil, bad("汇报接收者必须是本群人类成员")
			}
		}
		result, e := s.command(ctx, task.MemberID, task.ID, Command{APIVersion: 1, RequestID: req.RequestID, RoomID: task.RoomID, Intent: intent, Content: req.Content, Title: req.Title, TargetMemberIDs: targets, TaskID: req.TaskID, ExpectedRunID: req.ExpectedRunID, Acceptance: req.Acceptance, DependsOn: req.DependsOn, Required: req.Required})
		if e != nil {
			return nil, e
		}
		return result, nil
	}
	var out any
	e = s.write(ctx, func(t *Tx) error {
		v, e := s.authorize(t, cap)
		if e != nil {
			return e
		}
		switch tool {
		case "team_open_file":
			// File system and Office work is performed after the transaction below.
			out = v
			return nil
		case "team_members":
			ms, e := t.Members(task.RoomID)
			if e != nil {
				return e
			}
			room, e := t.Room(task.RoomID)
			out = map[string]any{"members": ms, "rosterVersion": room.RosterVersion}
			return e
		case "team_member":
			m, e := t.Member(req.MemberID)
			if e != nil {
				return e
			}
			if m.RoomID != task.RoomID {
				return bad("成员不属于本群")
			}
			if m.Kind == "human" {
				out = map[string]any{"member": m}
				return nil
			}
			emp, e := t.Employee(m.ProfileID)
			out = map[string]any{"member": m, "profile": emp}
			return e
		case "team_task_status":
			if req.TaskID == "" {
				req.TaskID = task.ID
			}
			v, e := t.Task(req.TaskID)
			if e != nil {
				return e
			}
			if v.RoomID != task.RoomID {
				return bad("任务不属于本群")
			}
			out = v
			return nil
		case "team_read_messages":
			if req.MessageID != "" {
				m, e := t.Message(req.MessageID)
				if e != nil {
					return e
				}
				if m.RoomID != task.RoomID {
					return bad("消息不属于本群")
				}
				out = messageSlice(m, req.Offset, req.Limit)
				return nil
			}
			ms, e := t.Messages(task.RoomID, req.Before, req.Limit)
			excerpts := []MessageSlice{}
			for _, m := range ms {
				excerpts = append(excerpts, messageSlice(m, 0, 4000))
			}
			out = excerpts
			return e
		case "team_search_messages":
			if len([]rune(req.Query)) < 2 || len(req.Query) > 200 {
				return bad("搜索词长度为 2–200 字符")
			}
			ms, e := list[Message](ctx, t.tx, "SELECT data FROM team_messages WHERE room_id=? AND instr(json_extract(data,'$.content'),?)>0 ORDER BY room_seq DESC LIMIT 30", task.RoomID, req.Query)
			excerpts := []MessageSlice{}
			for _, m := range ms {
				excerpts = append(excerpts, messageSlice(m, 0, 4000))
			}
			out = excerpts
			return e
		case "team_yield", "team_finish":
		default:
			return bad("不支持此工具")
		}
		if req.RequestID == "" {
			return bad("缺少 requestId")
		}
		raw, _ := encode(req)
		sum := sha256.Sum256([]byte(tool + raw))
		hash := hex.EncodeToString(sum[:])
		source := v.MemberID + ":" + v.RunID + ":" + tool
		old, found, e := t.Command(source, req.RequestID, hash)
		if e != nil {
			return e
		}
		if found {
			out = map[string]any{"accepted": true, "taskId": old.Message.TaskID}
			return nil
		}
		if tool == "team_finish" {
			if req.Validation != "not_run" && req.Validation != "unknown" && req.Validation != "passed" && req.Validation != "failed" {
				return bad("请明确 validation")
			}
			if v.Status == "waiting_user" {
				return conflict("请等待用户回复后再确认交付")
			}
			if strings.TrimSpace(req.Result) == "" || len(req.Result) > 100000 {
				return bad("请提供有效的任务结果")
			}
			v.FinishRequested = true
			v.Result = req.Result
			v.Validation = req.Validation
		} else {
			for _, wid := range unique(req.WaitFor) {
				if wid == v.ID {
					return conflict("不能等待自己")
				}
				child, e := t.Task(wid)
				if e != nil {
					return e
				}
				if child.RoomID != v.RoomID || child.ParentID != v.ID {
					return bad("只能等待当前任务已创建的子任务")
				}
				v.WaitFor = append(v.WaitFor, wid)
			}
			v.WaitFor = unique(v.WaitFor)
			if v.Status != "waiting_user" {
				if len(v.WaitFor) == 0 {
					return bad("请提供需要等待的子任务")
				}
				v.Status = "waiting_agent"
			}
		}
		if e = t.SaveTask(&v); e != nil {
			return e
		}
		if e = t.SaveCommand(source, req.RequestID, hash, CommandResult{Message: Message{TaskID: v.ID}}); e != nil {
			return e
		}
		out = map[string]any{"accepted": true, "taskId": v.ID, "status": v.Status, "instruction": "End the native turn normally now. Do not poll or keep background writers running."}
		return nil
	})
	if e == nil && tool == "team_open_file" {
		return s.artifact(ctx, task.RoomID, task.ID, req.Path, true)
	}
	return out, e
}

// RunTool is handled before starting Wails. The same installed executable acts
// as a capability-scoped tool client, so users do not need Node/Python on PATH.
func RunTool(args []string, input io.Reader, output io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing tool name")
	}
	tool := args[0]
	known := false
	for _, spec := range toolSchemas() {
		if spec.Name == tool {
			known = true
			break
		}
	}
	if !known {
		return errors.New("invalid tool name")
	}
	var body []byte
	var e error
	if len(args) == 3 && args[1] == "--file" {
		f, e := os.Open(args[2])
		if e != nil {
			return e
		}
		defer f.Close()
		body, e = io.ReadAll(io.LimitReader(f, (256<<10)+1))
		if e != nil {
			return e
		}
	} else if len(args) == 3 && args[1] == "--json" {
		body = []byte(args[2])
	} else if len(args) == 1 {
		body, e = io.ReadAll(io.LimitReader(input, (256<<10)+1))
		if e != nil {
			return e
		}
	} else {
		return errors.New("use TOOL [--file path | --json value]")
	}
	if len(body) == 0 {
		body = []byte("{}")
	}
	body = []byte(strings.TrimPrefix(string(body), "\ufeff"))
	if len(body) > 256<<10 || !json.Valid(body) {
		return errors.New("invalid or oversized JSON")
	}
	endpoint, token := os.Getenv("FFC_TEAM_URL"), os.Getenv("FFC_TEAM_TOKEN")
	if !strings.HasPrefix(endpoint, "http://127.0.0.1:") || token == "" {
		return errors.New("team capability is only available inside an active FFC employee run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "POST", endpoint+"/v1/tools/"+tool, strings.NewReader(string(body)))
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	defer client.CloseIdleConnections()
	resp, e := client.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	result, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if e != nil {
		return e
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("team tool: %s", strings.TrimSpace(string(result)))
	}
	_, e = output.Write(result)
	return e
}
