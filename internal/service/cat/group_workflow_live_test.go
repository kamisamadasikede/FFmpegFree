package cat

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

// Opt-in: uses the installed, authenticated Codex against a temporary project.
// No application database, user group or repository source is modified.
func TestGroupWorkflow_LiveCodex(t *testing.T) {
	if os.Getenv("FFC_GROUP_LIVE_QA") != "1" {
		t.Skip("set FFC_GROUP_LIVE_QA=1 to run real CLI acceptance")
	}
	live := catagent.NewCodexAdapter(catagent.CLIAgentConfig{DataRoot: t.TempDir(), DataTemp: t.TempDir()})
	if st := live.Recheck(); st.State != catagent.StateReady {
		t.Fatalf("Codex not ready: %s", st.State)
	}
	s, g, ms, root := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if o.JSONSchema == "" {
			o.Timeout = 3 * time.Minute
		}
		return live.RunTurn(o)
	})
	ctx := context.Background()
	names := []string{"后端", "前端", "测试"}
	for i := range ms {
		ms[i].Name, ms[i].Title, ms[i].AgentKind, ms[i].ModelID = names[i], names[i], catagent.KindCodexCLI, "gpt-6.1-sol"
		ms[i].Charter.Owns = []string{"backend/ 的接口实现", "frontend/ 的渲染实现", "运行 acceptance.cjs 联调验收"}[i]
		ms[i].Charter.NotTouch = "不要修改 acceptance.cjs，不访问临时项目之外的文件，不安装依赖，不创建服务"
		ms[i].Charter.Dirs = []string{[]string{"backend/", "frontend/", "."}[i]}
		if err := s.cfg.Store.UpdateCatGroupMember(ctx, ms[i]); err != nil {
			t.Fatal(err)
		}
	}
	acceptance := `const test = require('node:test'); const assert = require('node:assert/strict');
test('backend contract',()=>{const {listTasks}=require('./backend/tasks.cjs');assert.deepEqual(listTasks(),[{id:'1',title:'demo'}])});
test('frontend accepts backend data',()=>{const {renderTasks}=require('./frontend/render.cjs'); const {listTasks}=require('./backend/tasks.cjs');assert.equal(renderTasks(listTasks()),'demo');assert.equal(renderTasks([]),'')});`
	if err := os.WriteFile(filepath.Join(root, "acceptance.cjs"), []byte(acceptance), 0600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var bad []string
	s.cfg.Emit = func(name string, payload any) {
		if name != "cat:group-workflow" {
			return
		}
		w := payload.(store.CatGroupWorkflow)
		states := map[string]string{}
		for _, task := range w.Tasks {
			states[task.ID] = task.Status
		}
		for _, task := range w.Tasks {
			if task.Status != "running" {
				continue
			}
			for _, dep := range task.DependsOn {
				if states[dep] != "done" {
					mu.Lock()
					bad = append(bad, task.ID+" started before "+dep)
					mu.Unlock()
				}
			}
		}
	}
	request := "实现这个临时项目的任务列表功能并完成联调验收。请自动分给后端、前端、测试：后端在 backend/tasks.cjs 导出同步 listTasks() 返回 [{id:'1',title:'demo'}]；前端在 frontend/render.cjs 导出同步 renderTasks(items)，返回 title 用逗号连接的字符串，空数组返回空字符串。前后端可以并行，完成后由测试执行 node --test acceptance.cjs，所有测试通过再统一汇总。只实现这两个小模块，不装依赖、不起服务、不修改验收文件、不访问项目外文件，不需要向用户确认。"
	start := time.Now()
	sent, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: request})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Minute)
	var w store.CatGroupWorkflow
	for time.Now().Before(deadline) {
		w, err = s.cfg.Store.GetCatGroupWorkflow(ctx, sent.ChainID)
		if err == nil && (w.Status == "completed" || w.Status == "blocked" || w.Status == "stopped") {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if err != nil || w.Status != "completed" {
		t.Fatalf("real CLI workflow incomplete: %+v, err=%v", w, err)
	}
	s.Wait()
	mu.Lock()
	failures := append([]string{}, bad...)
	mu.Unlock()
	if len(failures) > 0 {
		t.Fatal(failures)
	}
	check := exec.Command("node", "--test", "acceptance.cjs")
	check.Dir = root
	out, err := check.CombinedOutput()
	if err != nil {
		t.Fatalf("independent acceptance failed: %v\n%s", err, out)
	}
	msgs, err := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	summaries := 0
	for _, msg := range msgs {
		if msg.SystemKind == "workflow_summary" {
			summaries++
		}
		if msg.Kind == "member" && strings.Contains(msg.Text, groupTaskResultTag) {
			t.Fatal("internal result leaked into group")
		}
	}
	if summaries != 1 {
		t.Fatalf("expected one summary, got %d", summaries)
	}
	for _, task := range w.Tasks {
		t.Logf("%s: %s attempts=%d evidence=%s", task.Kind, task.Status, task.Attempts, task.Evidence)
	}
	t.Logf("real CLI acceptance completed in %s, %d tasks, independent node tests passed", time.Since(start).Round(time.Second), len(w.Tasks))
	// The same no-mention path must still support ordinary introductions.
	greet, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "你们介绍下自己，一人一句话即可，不用查看文件"})
	if err != nil {
		t.Fatal(err)
	}
	s.Wait()
	msgs, _ = s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
	replies := map[string]bool{}
	for _, msg := range msgs {
		if msg.ChainID == greet.ChainID && msg.Kind == "member" {
			replies[msg.MemberID] = true
		}
	}
	if len(replies) != len(ms) {
		t.Fatal(fmt.Sprintf("no-mention greeting replies=%d, want %d", len(replies), len(ms)))
	}
	if _, err := s.cfg.Store.GetCatGroupWorkflow(ctx, greet.ChainID); err == nil {
		t.Fatal("greeting created a development plan")
	}
	t.Log("no-mention introduction received replies from all three employees")
}

func TestGroupImages_LiveCodex(t *testing.T) {
	if os.Getenv("FFC_GROUP_LIVE_QA") != "1" {
		t.Skip("set FFC_GROUP_LIVE_QA=1 to run real CLI image acceptance")
	}
	live := catagent.NewCodexAdapter(catagent.CLIAgentConfig{DataRoot: t.TempDir(), DataTemp: t.TempDir()})
	if status := live.Recheck(); status.State != catagent.StateReady {
		t.Fatalf("Codex not ready: %s", status.State)
	}
	var mu sync.Mutex
	routes, workers := 0, 0
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if len(o.Files) != 1 {
			return catagent.TurnResponse{}, fmt.Errorf("expected one native image, got %d", len(o.Files))
		}
		mu.Lock()
		if o.JSONSchema != "" {
			routes++
		} else {
			workers++
		}
		mu.Unlock()
		o.Timeout = 90 * time.Second
		resp, err := live.RunTurn(o)
		if err != nil {
			t.Logf("native image CLI error: %v", err)
		}
		return resp, err
	})
	ctx := context.Background()
	for i := range ms {
		ms[i].AgentKind, ms[i].ModelID = catagent.KindCodexCLI, "gpt-6.1-sol"
		ms[i].Charter.Owns = "解读用户图片并回答问题"
		if err := s.cfg.Store.UpdateCatGroupMember(ctx, ms[i]); err != nil {
			t.Fatal(err)
		}
	}
	path, _ := groupTestImage(t)
	start := time.Now()
	sent, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "请看附图，左右两个区域分别是什么颜色？用一句中文回答即可，不需要开发任务或检查文件。", Files: []SendAttachment{{Path: path}}})
	if err != nil {
		t.Fatal(err)
	}
	// An employee must read the managed copy, not the source file.
	_ = os.Remove(path)
	s.Wait()
	msgs, err := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	correct := 0
	for _, msg := range msgs {
		if msg.ChainID != sent.ChainID || msg.Kind != "member" {
			continue
		}
		if strings.Contains(msg.Text, "红") && strings.Contains(msg.Text, "蓝") && strings.Index(msg.Text, "红") < strings.Index(msg.Text, "蓝") {
			correct++
			t.Logf("vision reply: %s", msg.Text)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if routes != 1 || workers < 1 || correct < 1 {
		t.Fatalf("native image acceptance failed: routes=%d workers=%d correct=%d messages=%+v", routes, workers, correct, msgs)
	}
	t.Logf("real CLI native image routing and reading passed in %s", time.Since(start).Round(time.Second))
}
