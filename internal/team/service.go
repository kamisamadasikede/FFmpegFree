package team

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/agentexec"
	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/genoffice"
	"FFmpegFree/internal/id"
)

type Config struct {
	Repository *Repository
	Registry   *catagent.Registry
	Execution  *agentexec.Runtime
	Office     *genoffice.Manager
	DataRoot   string
	Resolve    func(context.Context, string, string) (catagent.TurnOptions, agentexec.Scope, error)
	Emit       func(string, any)
	Notify     func(Notification) error
	Logf       func(string, ...any)
}
type Service struct {
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	runs   map[string]*run
	wg     sync.WaitGroup
	wake   chan struct{}
	bridge *bridge
}
type run struct {
	mu        sync.Mutex
	task      Task
	cancel    context.CancelFunc
	steer     catagent.SteerFunc
	text      string
	activity  string
	dirty     bool
	closed    bool
	inputMode string
}

func bad(s string) error      { return apperr.New(apperr.InvalidArgument, s) }
func conflict(s string) error { return apperr.New(apperr.TaskConflict, s) }
func terminal(s string) bool {
	return s == "completed" || s == "failed" || s == "cancelled" || s == "needs_review"
}
func activeMember(ms []Member, id string) (Member, bool) {
	for _, m := range ms {
		if m.ID == id && m.Active {
			return m, true
		}
	}
	return Member{}, false
}
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func New(ctx context.Context, cfg Config) (*Service, error) {
	if cfg.Repository == nil || cfg.Registry == nil || cfg.Execution == nil {
		return nil, errors.New("群服务依赖未就绪")
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{cfg: cfg, ctx: ctx, cancel: cancel, runs: map[string]*run{}, wake: make(chan struct{}, 1)}
	if cfg.Logf == nil {
		s.cfg.Logf = func(string, ...any) {}
	}
	if e := s.write(ctx, func(t *Tx) error {
		if e := t.exec("UPDATE team_notification_outbox SET status='expired' WHERE status IN ('pending','sending')"); e != nil {
			return e
		}
		if _, e := t.Human(); errors.Is(e, sql.ErrNoRows) {
			if e = t.SaveHuman(Human{ID: id.New(), Name: "我", Role: "负责人", NotifyMentions: true}); e != nil {
				return e
			}
		} else if e != nil {
			return e
		}
		pending, e := t.AllPending()
		if e != nil {
			return e
		}
		for _, task := range pending {
			// Never replay uncertain native side effects after restart.
			if task.Status == "running" || task.Status == "starting" || task.Status == "stopping" || task.Status == "queued" {
				task.Status = "needs_review"
				task.Error = "应用上次退出时任务未确认完成；请检查结果后继续，系统未自动重放。"
				task.CanSteer = false
				if e = t.SaveTask(&task); e != nil {
					return e
				}
				record, e := one[RunRecord](ctx, t.tx, "SELECT data FROM team_runs WHERE id=?", task.RunID)
				if e == nil {
					record.Status = "needs_review"
					record.Error = task.Error
					if e = t.SaveRun(record); e != nil {
						return e
					}
				} else if !errors.Is(e, sql.ErrNoRows) {
					return e
				}
			}
		}
		for _, old := range pending {
			current, e := t.Task(old.ID)
			if e != nil {
				return e
			}
			if terminal(current.Status) {
				if e = s.wakeParents(t, current); e != nil {
					return e
				}
			}
		}
		return nil
	}); e != nil {
		cancel()
		return nil, e
	}
	b, e := newBridge(s)
	if e != nil {
		cancel()
		return nil, e
	}
	s.bridge = b
	s.wg.Add(1)
	go s.loop()
	return s, nil
}
func (s *Service) write(ctx context.Context, fn func(*Tx) error) error {
	events, e := s.cfg.Repository.Write(ctx, fn)
	if e != nil {
		return e
	}
	for _, ev := range events {
		if s.cfg.Emit != nil {
			s.cfg.Emit("team:event", ev)
		}
	}
	if len(events) > 0 {
		s.signal()
	}
	return nil
}
func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Service) Bootstrap(ctx context.Context) (Bootstrap, error) {
	v, e := s.cfg.Repository.Bootstrap(ctx)
	v.Modes = map[string][]catagent.AgentMode{}
	for _, kind := range []string{catagent.KindCodexCLI, catagent.KindClaudeCLI, catagent.KindCatBuild} {
		v.Modes[kind] = catagent.ModesFor(kind)
		if kind == catagent.KindCatBuild {
			modes := v.Modes[kind][:0]
			for _, m := range v.Modes[kind] {
				if m.ID != catagent.ModeAcceptEdits {
					modes = append(modes, m)
				}
			}
			v.Modes[kind] = modes
		}
	}
	return v, e
}
func (s *Service) Snapshot(ctx context.Context, roomID string, before int64) (Snapshot, error) {
	return s.cfg.Repository.Snapshot(ctx, roomID, before)
}
func (s *Service) Events(ctx context.Context, roomID string, after int64) ([]Event, error) {
	return s.cfg.Repository.Events(ctx, roomID, after)
}
func (s *Service) SaveHuman(ctx context.Context, v Human) (Human, error) {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || utf8.RuneCountInString(v.Name) > 60 || utf8.RuneCountInString(v.Role) > 200 {
		return v, bad("请填写有效的名字和身份")
	}
	e := s.write(ctx, func(t *Tx) error {
		old, e := t.Human()
		if e != nil {
			return e
		}
		v.ID = old.ID
		if e = t.SaveHuman(v); e != nil {
			return e
		}
		return refreshMembers(t, v.ID, v.Name, v.Role)
	})
	return v, e
}
func refreshMembers(t *Tx, profileID, name, role string) error {
	ms, e := list[Member](t.ctx, t.tx, "SELECT data FROM team_members WHERE profile_id=?", profileID)
	if e != nil {
		return e
	}
	for _, m := range ms {
		m.Name, m.Role = name, role
		if e = t.SaveMember(m); e != nil {
			return e
		}
		r, e := t.Room(m.RoomID)
		if e != nil {
			return e
		}
		r.RosterVersion++
		if e = t.SaveRoom(r); e != nil {
			return e
		}
		if e = t.Event(r.ID, "roster", r); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) SaveEmployee(ctx context.Context, v Employee) (Employee, error) {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || utf8.RuneCountInString(v.Name) > 60 || !catagent.Phase1Creatable(v.AgentKind) {
		return v, bad("请填写员工名字并选择已接入的 CLI")
	}
	if utf8.RuneCountInString(v.Role+v.Specialties+v.Focus+v.Standards+v.Knowledge) > 12000 {
		return v, bad("员工职责资料过长")
	}
	if v.Model != "" && v.Model != "default" && !validModel(v.Model) {
		return v, bad("模型 ID 不正确")
	}
	if v.Transport != "" && v.Transport != "auto" && !((v.AgentKind == catagent.KindCodexCLI && v.Transport == "app-server") || (v.AgentKind == catagent.KindClaudeCLI && v.Transport == "stream-json") || (v.AgentKind == catagent.KindCatBuild && v.Transport == "stdio")) {
		return v, bad("执行通道与 CLI 不匹配")
	}
	if v.AgentKind == catagent.KindCatBuild && v.PermissionMode == catagent.ModeAcceptEdits {
		return v, bad("群聊的 Grok 原生通道暂不支持此权限模式，请选择只读、自动或全部放开")
	}
	e := s.write(ctx, func(t *Tx) error {
		if v.ID == "" {
			v.ID = id.New()
			v.Version = 1
		} else {
			old, e := t.Employee(v.ID)
			if e != nil {
				return e
			}
			if old.Version != v.Version {
				return conflict("员工资料已更新，请刷新后重试")
			}
			v.Version++
		}
		v.PermissionMode = catagent.NormalizeMode(v.AgentKind, v.PermissionMode)
		if e := t.SaveEmployee(v); e != nil {
			return e
		}
		return refreshMembers(t, v.ID, v.Name, v.Role)
	})
	return v, e
}
func validModel(v string) bool {
	if len(v) > 200 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.:/", r)) {
			return false
		}
	}
	return true
}
func (s *Service) CreateRoom(ctx context.Context, req CreateRoom) (Room, error) {
	r := Room{ID: id.New(), Name: strings.TrimSpace(req.Name), ProjectID: req.ProjectID, RosterVersion: 1, UpdatedAt: time.Now().UnixMilli()}
	if r.Name == "" || utf8.RuneCountInString(r.Name) > 100 || len(req.EmployeeIDs) == 0 || len(req.EmployeeIDs) > 32 {
		return r, bad("群名不能为空，请选择 1–32 位员工")
	}
	if req.ProjectID != "" {
		_, scope, e := s.cfg.Resolve(ctx, req.ProjectID, catagent.KindCodexCLI)
		if e != nil {
			return r, e
		}
		r.Workspace = scope.Workspace
	} else {
		r.Workspace = filepath.Join(s.cfg.DataRoot, "team", "rooms", r.ID, "workspace")
		if e := os.MkdirAll(r.Workspace, 0700); e != nil {
			return r, e
		}
	}
	root, e := agentexec.CanonicalRoot(r.Workspace)
	if e != nil {
		return r, e
	}
	r.Workspace = root
	e = s.write(ctx, func(t *Tx) error {
		h, e := t.Human()
		if e != nil {
			return e
		}
		if e = t.SaveRoom(r); e != nil {
			return e
		}
		me := Member{ID: id.New(), RoomID: r.ID, Kind: "human", ProfileID: h.ID, Name: h.Name, Role: h.Role, Active: true, Generation: 1}
		if e = t.SaveMember(me); e != nil {
			return e
		}
		seen := map[string]bool{}
		for _, eid := range req.EmployeeIDs {
			if seen[eid] {
				continue
			}
			seen[eid] = true
			v, e := t.Employee(eid)
			if e != nil {
				return e
			}
			m := Member{ID: id.New(), RoomID: r.ID, Kind: "agent", ProfileID: v.ID, Name: v.Name, Role: v.Role, Active: true, Generation: 1}
			if e = t.SaveMember(m); e != nil {
				return e
			}
			if req.CoordinatorID == eid {
				r.CoordinatorID = m.ID
			}
		}
		if e = t.SaveRoom(r); e != nil {
			return e
		}
		return t.Event(r.ID, "room", r)
	})
	return r, e
}
func (s *Service) UpdateRoom(ctx context.Context, req Room) (Room, error) {
	e := s.write(ctx, func(t *Tx) error {
		old, e := t.Room(req.ID)
		if e != nil {
			return e
		}
		if req.RosterVersion != old.RosterVersion {
			return conflict("群信息已变化，请刷新")
		}
		ms, e := t.Members(old.ID)
		if e != nil {
			return e
		}
		if req.CoordinatorID != "" {
			m, ok := activeMember(ms, req.CoordinatorID)
			if !ok || m.Kind != "agent" {
				return bad("协调人必须是本群员工")
			}
		}
		old.Name = strings.TrimSpace(req.Name)
		if old.Name == "" || utf8.RuneCountInString(old.Name) > 100 {
			return bad("群名不正确")
		}
		old.Paused, old.Archived, old.CoordinatorID = req.Paused, req.Archived, req.CoordinatorID
		old.RosterVersion++
		old.UpdatedAt = time.Now().UnixMilli()
		req = old
		if e = t.SaveRoom(old); e != nil {
			return e
		}
		return t.Event(old.ID, "room", old)
	})
	return req, e
}
func (s *Service) SetMember(ctx context.Context, roomID, employeeID string, enabled bool) error {
	return s.write(ctx, func(t *Tx) error {
		r, e := t.Room(roomID)
		if e != nil {
			return e
		}
		if r.Archived {
			return conflict("群已归档")
		}
		v, e := t.Employee(employeeID)
		if e != nil {
			return e
		}
		ms, e := t.Members(roomID)
		if e != nil {
			return e
		}
		m := Member{ID: id.New(), RoomID: roomID, Kind: "agent", ProfileID: v.ID, Name: v.Name, Role: v.Role, Generation: 1}
		for _, old := range ms {
			if old.ProfileID == employeeID {
				m = old
				if enabled && !m.Active {
					m.Generation++
				}
			}
		}
		m.Active = enabled
		if e = t.SaveMember(m); e != nil {
			return e
		}
		if !enabled {
			tasks, e := t.Tasks(roomID)
			if e != nil {
				return e
			}
			for _, task := range tasks {
				if task.MemberID == m.ID && task.Status == "queued" {
					task.Status = "cancelled"
					task.Error = "员工已离群"
					if e = t.SaveTask(&task); e != nil {
						return e
					}
				}
			}
			if r.CoordinatorID == m.ID {
				r.CoordinatorID = ""
			}
		}
		r.RosterVersion++
		if e = t.SaveRoom(r); e != nil {
			return e
		}
		return t.Event(roomID, "roster", r)
	})
}
func (s *Service) Command(ctx context.Context, c Command) (CommandResult, error) {
	var source string
	_, e := s.cfg.Repository.Write(ctx, func(t *Tx) error {
		ms, e := t.Members(c.RoomID)
		if e != nil {
			return e
		}
		for _, m := range ms {
			if m.Active && m.Kind == "human" {
				source = m.ID
				return nil
			}
		}
		return bad("本人不在此群")
	})
	if e != nil {
		return CommandResult{}, e
	}
	return s.command(ctx, source, "", c)
}
func (s *Service) command(ctx context.Context, source, parentID string, c Command) (CommandResult, error) {
	out := CommandResult{Deliveries: []Delivery{}}
	if c.APIVersion != 0 && c.APIVersion != 1 {
		return out, bad("不支持此工具协议版本")
	}
	c.APIVersion = 1
	if c.RequestID == "" || len(c.RequestID) > 200 || strings.TrimSpace(c.Content) == "" || utf8.RuneCountInString(c.Content) > 50000 {
		return out, bad("请求 ID 或消息内容不正确")
	}
	if c.Intent == "steer" {
		return s.steer(ctx, source, c)
	}
	if c.Intent != "new_task" && c.Intent != "reply" && c.Intent != "publish" && c.Intent != "report" && c.Intent != "ask" {
		return out, bad("消息意图不正确")
	}
	raw, _ := encode(c)
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])
	e := s.write(ctx, func(t *Tx) error {
		previous, found, e := t.Command(source, c.RequestID, hash)
		if e != nil {
			return conflict(e.Error())
		}
		if found {
			out = previous
			return nil
		}
		r, e := t.Room(c.RoomID)
		if e != nil {
			return e
		}
		if r.Archived {
			return conflict("群已归档")
		}
		ms, e := t.Members(r.ID)
		if e != nil {
			return e
		}
		sender, ok := activeMember(ms, source)
		if !ok {
			return bad("来源成员已失效")
		}
		if parentID != "" {
			p, e := t.Task(parentID)
			if e != nil {
				return e
			}
			if p.RoomID != r.ID || p.MemberID != source || p.Status != "running" {
				return conflict("任务工具凭证已失效")
			}
			if r.Paused && (c.Intent == "new_task" || c.Intent == "ask") {
				return conflict("此群已暂停自动委派")
			}
		}
		if c.ReplyToID != "" {
			msg, e := t.Message(c.ReplyToID)
			if e != nil {
				return e
			}
			if msg.RoomID != r.ID {
				return bad("引用消息不属于本群")
			}
			if c.Intent == "reply" {
				c.TargetMemberIDs = []string{msg.FromMemberID}
			}
		}
		targets := append([]string(nil), c.TargetMemberIDs...)
		if len(targets) == 0 && c.Intent == "new_task" && r.CoordinatorID != "" {
			targets = []string{r.CoordinatorID}
		}
		if len(targets) == 0 && (c.Intent == "new_task" || c.Intent == "ask" || c.Intent == "reply") {
			return bad("请 @ 一位员工或设置协调人")
		}
		if len(targets) > 33 {
			return bad("接收人数过多")
		}
		seen := map[string]bool{}
		targets = unique(targets)
		for _, mid := range targets {
			if _, ok = activeMember(ms, mid); !ok {
				return bad("接收成员已离群，请修改 @ 对象")
			}
			seen[mid] = true
		}
		_ = seen
		out.Message = Message{ID: id.New(), RoomID: r.ID, FromMemberID: source, TaskID: parentID, ReplyToID: c.ReplyToID, Kind: c.Intent, Content: c.Content, Mentions: targets, Status: "sent", CreatedAt: time.Now().UnixMilli()}
		if e = t.SaveMessage(&out.Message); e != nil {
			return e
		}
		for _, mid := range targets {
			m, _ := activeMember(ms, mid)
			d := Delivery{MemberID: mid, Status: "persisted"}
			if m.Kind == "human" {
				if e = s.mention(t, r, sender, m, out.Message, c.Intent == "ask"); e != nil {
					return e
				}
				if c.Intent == "ask" && parentID != "" {
					p, e := t.Task(parentID)
					if e != nil {
						return e
					}
					p.Status, p.ResumeRequested, p.CanSteer = "waiting_user", false, false
					if e = t.SaveTask(&p); e != nil {
						return e
					}
				}
			} else if c.Intent == "new_task" || c.Intent == "ask" || c.Intent == "reply" {
				var continued bool
				if c.Intent == "reply" {
					continued, e = s.reply(t, c.ReplyToID, m, out.Message, &d)
					if e != nil {
						return e
					}
				}
				if !continued {
					task, e := s.newTask(t, r, m, source, parentID, out.Message, c)
					if e != nil {
						return e
					}
					d.TaskID, d.Status = task.ID, "queued"
				}
			}
			out.Deliveries = append(out.Deliveries, d)
		}
		return t.SaveCommand(source, c.RequestID, hash, out)
	})
	return out, e
}
func unique(xs []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
func (s *Service) newTask(t *Tx, r Room, m Member, source, parent string, msg Message, c Command) (Task, error) {
	tasks, e := t.Tasks(r.ID)
	if e != nil {
		return Task{}, e
	}
	n := 0
	for _, v := range tasks {
		if !terminal(v.Status) {
			n++
		}
	}
	if n >= 100 {
		return Task{}, conflict("本群待完成任务达到 100 项，请先处理现有任务")
	}
	if parent != "" {
		depth := 0
		pid := parent
		for pid != "" {
			p, e := t.Task(pid)
			if e != nil {
				return Task{}, e
			}
			if p.MemberID == m.ID {
				return Task{}, conflict("委派形成员工等待循环，请直接使用当前任务或发布消息")
			}
			for _, dep := range c.DependsOn {
				if dep == p.ID {
					return Task{}, conflict("子任务不能依赖自己的上级任务，否则会形成等待循环")
				}
			}
			pid = p.ParentID
			depth++
			if depth >= 8 {
				return Task{}, conflict("子任务深度达到上限")
			}
		}
	}
	for _, dep := range c.DependsOn {
		v, e := t.Task(dep)
		if e != nil {
			return Task{}, e
		}
		if v.RoomID != r.ID {
			return Task{}, bad("依赖不属于本群")
		}
	}
	v := Task{ID: id.New(), RoomID: r.ID, MemberID: m.ID, RequesterID: source, ParentID: parent, MessageID: msg.ID, SessionID: id.New(), Title: clip(c.Title, 120), Instruction: c.Content, Acceptance: c.Acceptance, Status: "queued", Required: c.Required, DependsOn: unique(c.DependsOn), CreatedAt: time.Now().UnixMilli(), Validation: "not_run", InputMode: "none"}
	if v.Title == "" {
		v.Title = clip(c.Content, 65)
	}
	e = t.SaveTask(&v)
	return v, e
}
func (s *Service) mention(t *Tx, r Room, from, to Member, msg Message, question bool) error {
	in := InboxItem{ID: id.New(), RoomID: r.ID, MessageID: msg.ID, TaskID: msg.TaskID, RecipientID: to.ID, NeedsReply: question, CreatedAt: time.Now().UnixMilli(), NotificationStatus: "pending"}
	h, e := t.Human()
	if e != nil {
		return e
	}
	if !h.NotifyMentions {
		in.NotificationStatus = "disabled"
	}
	if e = t.SaveInbox(in); e != nil {
		return e
	}
	if h.NotifyMentions {
		return t.Outbox(in.ID, Notification{ID: in.ID, RoomID: r.ID, MessageID: msg.ID, TaskID: msg.TaskID, Title: r.Name + " · " + from.Name + " @ 你", Body: clip(msg.Content, 160)})
	}
	return nil
}
func (s *Service) reply(t *Tx, replyTo string, target Member, msg Message, d *Delivery) (bool, error) {
	items, e := t.Inbox(msg.RoomID)
	if e != nil {
		return false, e
	}
	for _, in := range items {
		if in.MessageID != replyTo || !in.NeedsReply || in.AnsweredAt != 0 {
			continue
		}
		task, e := t.Task(in.TaskID)
		if e != nil {
			return false, e
		}
		if task.MemberID != target.ID {
			return false, bad("回复目标与原任务不一致")
		}
		if task.Status == "running" || task.Status == "starting" {
			return false, conflict("原任务仍在执行；请使用立即引导，或等员工进入等待答复状态")
		}
		if task.Status != "waiting_user" {
			return false, conflict("原问题对应任务已结束或状态发生变化")
		}
		in.AnsweredAt = time.Now().UnixMilli()
		if e = t.SaveInbox(in); e != nil {
			return false, e
		}
		task.Instruction += "\n\n用户引用回复（消息 " + msg.ID + "）：\n" + msg.Content
		task.Status = "waiting_user"
		task.ResumeRequested = true
		if !s.nativeRunning(task.ID) {
			task.Status = "queued"
			task.ResumeRequested = false
		}
		task.FinishRequested = false
		task.Result = ""
		task.Error = ""
		if e = t.SaveTask(&task); e != nil {
			return false, e
		}
		d.TaskID, d.Status = task.ID, "queued"
		return true, nil
	}
	return false, nil
}

func (s *Service) nativeRunning(taskID string) bool {
	s.mu.Lock()
	r := s.runs[taskID]
	s.mu.Unlock()
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.closed
}
func (s *Service) MarkRead(ctx context.Context, roomID string, messageIDs []string) error {
	ids := map[string]bool{}
	for _, v := range messageIDs {
		ids[v] = true
	}
	return s.write(ctx, func(t *Tx) error {
		items, e := t.Inbox(roomID)
		if e != nil {
			return e
		}
		for _, v := range items {
			if ids[v.MessageID] && v.ReadAt == 0 {
				v.ReadAt = time.Now().UnixMilli()
				if e = t.SaveInbox(v); e != nil {
					return e
				}
			}
		}
		return nil
	})
}
func (s *Service) Stop(ctx context.Context, taskID string) error {
	var cancelIDs []string
	e := s.write(ctx, func(t *Tx) error {
		v, e := t.Task(taskID)
		if e != nil {
			return e
		}
		if terminal(v.Status) {
			return nil
		}
		tasks, e := t.Tasks(v.RoomID)
		if e != nil {
			return e
		}
		selected := map[string]bool{v.ID: true}
		for changed := true; changed; {
			changed = false
			for _, child := range tasks {
				if selected[child.ParentID] && !selected[child.ID] {
					selected[child.ID] = true
					changed = true
				}
			}
		}
		for _, task := range tasks {
			if !selected[task.ID] || terminal(task.Status) {
				continue
			}
			cancelIDs = append(cancelIDs, task.ID)
			task.Status = "cancelled"
			if s.nativeRunning(task.ID) {
				task.Status = "stopping"
			}
			task.CanSteer, task.ResumeRequested = false, false
			task.Error = "任务已取消，已有文件修改保留"
			if e = t.SaveTask(&task); e != nil {
				return e
			}
			if terminal(task.Status) {
				if e = s.wakeParents(t, task); e != nil {
					return e
				}
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	s.mu.Lock()
	for _, tid := range cancelIDs {
		if v := s.runs[tid]; v != nil {
			v.cancel()
		}
	}
	s.mu.Unlock()
	return nil
}
func (s *Service) Continue(ctx context.Context, taskID, content string) error {
	return s.write(ctx, func(t *Tx) error {
		v, e := t.Task(taskID)
		if e != nil {
			return e
		}
		if v.Status != "needs_review" && v.Status != "failed" && v.Status != "waiting_user" && v.Status != "cancelled" {
			return conflict("任务当前不能手动继续")
		}
		if strings.TrimSpace(content) == "" || len(content) > 200000 {
			return bad("请填写继续要求，系统不会自动重放原操作")
		}
		v.Instruction += "\n\n用户明确要求继续：\n" + content
		v.Status = "queued"
		if s.nativeRunning(v.ID) {
			return conflict("原生轮次仍在退出，请稍后继续，或使用当前轮次输入")
		}
		v.FinishRequested = false
		v.ResumeRequested = false
		v.WaitFor = []string{}
		v.Result = ""
		v.Validation = "not_run"
		v.Error = ""
		return t.SaveTask(&v)
	})
}
func (s *Service) Close() {
	s.cancel()
	s.mu.Lock()
	for _, r := range s.runs {
		r.cancel()
	}
	s.mu.Unlock()
	if s.bridge != nil {
		s.bridge.close()
	}
	s.wg.Wait()
}
func (s *Service) logError(where string, e error) {
	if e != nil {
		s.cfg.Logf("team %s: %v", where, e)
	}
}
func errorText(e error) string {
	if e == nil {
		return ""
	}
	if ae := apperr.From(e); ae != nil {
		return ae.Message
	}
	return fmt.Sprint(e)
}
