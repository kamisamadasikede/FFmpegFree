package team

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/id"
)

func (s *Service) steer(ctx context.Context, source string, c Command) (CommandResult, error) {
	out := CommandResult{Deliveries: []Delivery{}}
	raw, _ := encode(c)
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])
	var task Task
	var deliver bool
	e := s.write(ctx, func(t *Tx) error {
		old, found, e := t.Command(source, c.RequestID, hash)
		if e != nil {
			return e
		}
		if found {
			out = old
			return nil
		}
		task, e = t.Task(c.TaskID)
		if e != nil {
			return e
		}
		if task.RoomID != c.RoomID || task.RunID != c.ExpectedRunID || task.Status != "running" || !task.CanSteer {
			return conflict("当前任务不支持即时输入或执行轮次已变化，补充内容已保留在输入框")
		}
		if c.ExpectedRevision > 0 && task.Revision != c.ExpectedRevision {
			return conflict("任务版本已变化")
		}
		ms, e := t.Members(c.RoomID)
		if e != nil {
			return e
		}
		sender, ok := activeMember(ms, source)
		if !ok {
			return bad("来源成员已失效")
		}
		target, ok := activeMember(ms, task.MemberID)
		if !ok {
			return conflict("目标成员已离群")
		}
		_ = target
		if len(c.TargetMemberIDs) != 1 || c.TargetMemberIDs[0] != task.MemberID {
			return bad("即时输入必须关联同一目标员工")
		}
		if sender.Kind == "agent" {
			related := false
			for _, v := range []string{task.ParentID, task.ID} {
				p, e := t.Task(v)
				if e == nil && p.MemberID == source {
					related = true
				}
			}
			if !related {
				return bad("只能干预与自己协作关联的任务")
			}
		}
		out.Message = Message{ID: id.New(), RoomID: c.RoomID, FromMemberID: source, TaskID: task.ID, RunID: task.RunID, Kind: "steer", Content: c.Content, Mentions: []string{task.MemberID}, Status: "sending", CreatedAt: time.Now().UnixMilli()}
		if e = t.SaveMessage(&out.Message); e != nil {
			return e
		}
		out.Deliveries = []Delivery{{MemberID: task.MemberID, TaskID: task.ID, Status: "sending"}}
		if e = t.SaveCommand(source, c.RequestID, hash, out); e != nil {
			return e
		}
		deliver = true
		return nil
	})
	if e != nil || !deliver {
		return out, e
	}
	s.mu.Lock()
	r := s.runs[task.ID]
	s.mu.Unlock()
	var f catagent.SteerFunc
	if r != nil {
		r.mu.Lock()
		if !r.closed && r.task.RunID == task.RunID {
			f = r.steer
		}
		r.mu.Unlock()
	}
	status := "rejected"
	errText := "目标执行已结束，补充内容未送达"
	if f != nil {
		sendCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		e = f(sendCtx, c.Content, nil)
		cancel()
		if e == nil {
			status = "native_accepted"
			errText = ""
		} else {
			status = "unknown"
			errText = errorText(e)
		}
	}
	out.Message.Status = status
	out.Deliveries[0].Status = status
	out.Deliveries[0].Error = errText
	e = s.write(context.WithoutCancel(ctx), func(t *Tx) error {
		if e := t.SaveMessage(&out.Message); e != nil {
			return e
		}
		if status == "native_accepted" {
			v, e := t.Task(task.ID)
			if e != nil {
				return e
			}
			v.Instruction += "\n\n用户/同事即时补充（消息 " + out.Message.ID + "）：\n" + c.Content
			if e = t.SaveTask(&v); e != nil {
				return e
			}
		}
		b, e := encode(out)
		if e != nil {
			return e
		}
		return t.exec("UPDATE team_commands SET result=? WHERE source_id=? AND request_id=?", b, source, c.RequestID)
	})
	return out, e
}
