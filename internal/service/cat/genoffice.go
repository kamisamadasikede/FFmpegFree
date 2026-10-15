package cat

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// officeRequest always re-authorizes a path against the calling conversation.
// No caller can open an arbitrary engine session by guessing a document id.
func (s *Service) officeRequest(ctx context.Context, convID string, args map[string]any) (json.RawMessage, error) {
	name, _ := args["path"].(string)
	req := ReadFileRequest{ConvID: convID, RelPath: name}
	abs, rel, err := s.pptPath(ctx, req)
	if err != nil {
		return nil, err
	}
	root, err := s.pptRoot(ctx, req)
	if err != nil {
		return nil, err
	}
	action, _ := args["action"].(string)
	create, _ := args["create"].(bool)
	state, err := s.pptManager().Open(ctx, abs, root, action == "open" && create, s.pptGuard(req, abs))
	if err != nil {
		return nil, err
	}
	if action == "open" {
		s.emit("cat:open-file", OpenFileEvent{convID, rel, filepath.Base(rel)})
		return json.Marshal(state)
	}
	id, _ := state["documentId"].(string)
	if id == "" {
		return nil, fmt.Errorf("办公会话没有返回标识")
	}
	return s.pptManager().Call(ctx, id, args)
}
