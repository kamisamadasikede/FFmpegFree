package cat_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func TestRelocateCatProject(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	block := make(chan struct{})
	started := make(chan struct{}, 4)
	var roots []string
	a := &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		roots = append(roots, o.ProjectPath)
		started <- struct{}{}
		select {
		case <-o.Ctx.Done():
			return catagent.TurnResponse{}, o.Ctx.Err()
		case <-block:
		}
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "好"}}, nil
	}}
	svc, rec := newProjSvc(t, st, a, cat.Config{})
	base := t.TempDir()
	oldDir := filepath.Join(base, "old")
	newDir := filepath.Join(base, "new 文件夹")
	otherDir := filepath.Join(base, "other")
	for _, d := range []string{oldDir, newDir, otherDir} {
		_ = os.MkdirAll(d, 0o755)
	}
	_ = os.WriteFile(filepath.Join(newDir, "keep.txt"), []byte("k"), 0o644)
	p, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: oldDir, Name: "原名"})
	other, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: otherDir})
	conv, _ := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: conv.ID, Content: "一"}); err != nil {
		t.Fatal(err)
	}
	<-started

	// 校验
	if _, err := svc.RelocateCatProject(ctx, cat.RelocateProjectRequest{Path: newDir}); !apperr.Is(err, apperr.InvalidArgument) || reasonOf(err) != "" {
		t.Fatalf("空 id: %v", err)
	}
	if _, err := svc.RelocateCatProject(ctx, cat.RelocateProjectRequest{ID: "nope", Path: newDir}); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
	if _, err := svc.RelocateCatProject(ctx, cat.RelocateProjectRequest{ID: p.Project.ID, Path: "rel"}); reasonOf(err) != "reason=project_path" {
		t.Fatal(err)
	}
	if _, err := svc.RelocateCatProject(ctx, cat.RelocateProjectRequest{ID: p.Project.ID, Path: string(filepath.Separator)}); reasonOf(err) != "reason=project_root" {
		t.Fatal(err)
	}
	// 属于另一个项目 → project_duplicate + projectId
	_, err := svc.RelocateCatProject(ctx, cat.RelocateProjectRequest{ID: p.Project.ID, Path: otherDir + string(filepath.Separator)})
	if !apperr.Is(err, apperr.InvalidArgument) || apperr.From(err).Detail != "reason=project_duplicate\nprojectId="+other.Project.ID ||
		apperr.From(err).Message != "这个文件夹已经建过项目了。" {
		t.Fatalf("%v", err)
	}
	// 进行中的一轮 → turn_running，不自动取消
	_, err = svc.RelocateCatProject(ctx, cat.RelocateProjectRequest{ID: p.Project.ID, Path: newDir})
	if !apperr.Is(err, apperr.TaskConflict) || apperr.From(err).Detail != "reason=turn_running" || apperr.From(err).Message != "有对话正在回复，请先停止再换文件夹。" {
		t.Fatalf("%v", err)
	}
	close(block)
	svc.Wait()

	// 同一路径：原样返回，不改 updatedAt
	same, err := svc.RelocateCatProject(ctx, cat.RelocateProjectRequest{ID: p.Project.ID, Path: oldDir + string(filepath.Separator)})
	if err != nil || same.Path != oldDir || same.UpdatedAt != p.Project.UpdatedAt {
		t.Fatalf("%+v %v", same, err)
	}

	// 丢失后重新选择：对话保留、名字不变、清 missing 并发 cat:project
	_ = os.RemoveAll(oldDir)
	if l, _ := svc.ListCatProjects(ctx); !findProj(l, p.Project.ID).Missing {
		t.Fatal("应缺")
	}
	before, _ := svc.GetCatConversation(ctx, conv.ID)
	moved, err := svc.RelocateCatProject(ctx, cat.RelocateProjectRequest{ID: p.Project.ID, Path: newDir})
	if err != nil || moved.Path != newDir || moved.Name != "原名" || moved.Missing || moved.UpdatedAt < p.Project.UpdatedAt {
		t.Fatalf("%+v %v", moved, err)
	}
	after, _ := svc.GetCatConversation(ctx, conv.ID)
	if after.ProjectID != p.Project.ID || len(after.Messages) != len(before.Messages) || after.UpdatedAt != before.UpdatedAt {
		t.Fatalf("对话应原样保留: %+v", after)
	}
	_, pe := rec.snapshot()
	if len(pe) < 2 || pe[len(pe)-1] != (catagent.ProjectEvent{ID: p.Project.ID, Missing: false}) {
		t.Fatalf("应发 missing=false: %+v", pe)
	}
	if b, _ := os.ReadFile(filepath.Join(newDir, "keep.txt")); string(b) != "k" {
		t.Fatal("不应动新文件夹里的文件")
	}
	// 之后的一轮以新路径为沙箱根，并且能继续发
	if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: conv.ID, Content: "二"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	if roots[len(roots)-1] != newDir {
		t.Fatalf("%q", roots)
	}
	// 旧路径现在空出来，可以给别的项目
	_ = os.MkdirAll(oldDir, 0o755)
	if r, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: oldDir}); err != nil || r.Existed {
		t.Fatalf("%+v %v", r, err)
	}
}

func findProj(l []cat.Project, id string) cat.Project {
	for _, p := range l {
		if p.ID == id {
			return p
		}
	}
	return cat.Project{}
}

func TestRevealCatProject(t *testing.T) {
	ctx := context.Background()
	var opened []string
	var openErr error
	svc, rec := newProjSvc(t, openStore(t), nil, cat.Config{OpenFolder: func(dir string) error {
		opened = append(opened, dir)
		return openErr
	}})
	dir := filepath.Join(t.TempDir(), "中文 目录")
	_ = os.MkdirAll(dir, 0o755)
	p, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: dir})

	if err := svc.RevealCatProject(ctx, cat.RevealProjectRequest{}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatal(err)
	}
	if err := svc.RevealCatProject(ctx, cat.RevealProjectRequest{ID: "nope"}); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
	if err := svc.RevealCatProject(ctx, cat.RevealProjectRequest{ID: p.Project.ID}); err != nil || len(opened) != 1 || opened[0] != dir {
		t.Fatalf("%v %v", opened, err)
	}
	openErr = errors.New("no file manager")
	if err := svc.RevealCatProject(ctx, cat.RevealProjectRequest{ID: p.Project.ID}); !apperr.Is(err, apperr.ProcessFailed) {
		t.Fatal(err)
	}
	openErr = nil
	_ = os.RemoveAll(dir)
	err := svc.RevealCatProject(ctx, cat.RevealProjectRequest{ID: p.Project.ID})
	if !apperr.Is(err, apperr.CatProjectMissing) || apperr.From(err).Message != "项目文件夹不见了。" {
		t.Fatal(err)
	}
	if len(opened) != 2 {
		t.Fatal("丢失时不应调用打开")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("不应新建文件夹")
	}
	if _, pe := rec.snapshot(); len(pe) != 1 || !pe[0].Missing {
		t.Fatalf("状态变化应发 cat:project: %+v", pe)
	}
}
