package cat

import (
	"context"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/agentexec"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/genoffice"
)

// TeamOffice shares the existing embedded editor and renderer, never a second
// office engine or an ordinary-chat conversation masquerading as a team.
func (s *Service) TeamOffice() *genoffice.Manager { return s.pptManager() }

func projectConfigKey(root string) string {
	if root == "" {
		return "global"
	}
	p, e := agentexec.CanonicalRoot(root)
	if e != nil {
		p = filepath.Clean(root)
	}
	return strings.ToLower(p)
}

// TeamExecution resolves existing project/provider settings without creating an
// ordinary conversation or mutating the registry's shared provider overlay.
func (s *Service) TeamExecution(ctx context.Context, projectID, kind string) (catagent.TurnOptions, agentexec.Scope, error) {
	root := ""
	if projectID != "" {
		p, e := s.projectForConversation(ctx, projectID)
		if e != nil {
			return catagent.TurnOptions{}, agentexec.Scope{}, e
		}
		root = p.Path
	}
	opts := catagent.TurnOptions{Provider: s.providerOverlayFor(ctx, kind), Rules: s.workspaceRulesForTurn(ctx, projectID), Skills: skillsOverlayForProject(kind, root)}
	scope := agentexec.Scope{Workspace: root, NativeConfigKey: projectConfigKey(root), Prepare: func(ctx context.Context) error { s.syncSharingBeforeTurn(ctx, root); return ctx.Err() }}
	return opts, scope, nil
}
