package team

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"FFmpegFree/internal/catagent"
)

func TestToolsReadHumanAndFullUnicodeMessage(t *testing.T) {
	type source struct{ humanID, messageID, content string }
	ready := make(chan source, 1)
	s, _ := testService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		var input source
		select {
		case input = <-ready:
		case <-o.Ctx.Done():
			return catagent.TurnResponse{}, o.Ctx.Err()
		}
		raw, err := toolCall(o, "team_member", map[string]any{"memberId": input.humanID})
		if err != nil {
			return catagent.TurnResponse{}, err
		}
		var profile struct {
			Member Member `json:"member"`
		}
		if err = json.Unmarshal(raw, &profile); err != nil || profile.Member.ID != input.humanID || profile.Member.Kind != "human" {
			return catagent.TurnResponse{}, fmt.Errorf("human profile not returned: %s, %v", raw, err)
		}
		var content strings.Builder
		for offset := 0; ; {
			raw, err = toolCall(o, "team_read_messages", map[string]any{"messageId": input.messageID, "offset": offset, "limit": 4000})
			if err != nil {
				return catagent.TurnResponse{}, err
			}
			var page MessageSlice
			if err = json.Unmarshal(raw, &page); err != nil {
				return catagent.TurnResponse{}, err
			}
			if page.Message.ID != input.messageID || page.Offset != offset || page.NextOffset <= offset {
				return catagent.TurnResponse{}, fmt.Errorf("invalid message cursor: %+v", page)
			}
			content.WriteString(page.Message.Content)
			if !page.Truncated {
				break
			}
			offset = page.NextOffset
		}
		if content.String() != input.content {
			return catagent.TurnResponse{}, fmt.Errorf("original Unicode content changed or lost")
		}
		_, err = toolCall(o, "team_finish", map[string]any{"requestId": "read-complete", "result": "原文和本人身份读取成功", "validation": "not_run"})
		return catagent.TurnResponse{}, err
	})
	room, members := testRoom(t, s, 1)
	var humanID, employeeID string
	for _, m := range members {
		if m.Kind == "human" {
			humanID = m.ID
		} else {
			employeeID = m.ID
		}
	}
	content := strings.Repeat("原始中文🙂", 1700)
	result, err := s.Command(context.Background(), Command{RequestID: "long-message", RoomID: room.ID, Intent: "new_task", Content: content, TargetMemberIDs: []string{employeeID}})
	if err != nil {
		t.Fatal(err)
	}
	ready <- source{humanID, result.Message.ID, content}
	eventually(t, func() bool {
		v, err := s.Snapshot(context.Background(), room.ID, 0)
		return err == nil && len(v.Tasks) == 1 && terminal(v.Tasks[0].Status)
	})
	v, err := s.Snapshot(context.Background(), room.ID, 0)
	if err != nil || v.Tasks[0].Status != "completed" {
		t.Fatalf("read tools failed: tasks=%+v err=%v", v.Tasks, err)
	}
}
