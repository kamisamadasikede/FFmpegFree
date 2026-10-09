package cat_test

import (
	"context"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func TestDeleteCatConversationIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	c, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteCatConversation(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteCatConversation(ctx, c.ID); err != nil {
		t.Fatalf("第二次删除应返回 nil: %v", err)
	}
	if err := svc.DeleteCatConversation(ctx, "never-existed"); err != nil {
		t.Fatalf("不存在的 id 应返回 nil: %v", err)
	}
	if err := svc.DeleteCatConversation(ctx, "  "); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("空 id 仍应 INVALID_ARGUMENT: %v", err)
	}

	// 项目内对话：删了项目后再删对话也是 nil
	p, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	in, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	in2, _ := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	if err := svc.DeleteCatConversation(ctx, in.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteCatConversation(ctx, in.ID); err != nil {
		t.Fatalf("项目内对话再删应返回 nil: %v", err)
	}
	if err := svc.DeleteCatProject(ctx, cat.DeleteProjectRequest{ID: p.Project.ID}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteCatConversation(ctx, in2.ID); err != nil {
		t.Fatalf("随项目删掉的对话再删应返回 nil: %v", err)
	}
}
