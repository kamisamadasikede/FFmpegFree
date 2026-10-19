package catagent

import (
	"encoding/base64"
	"os"
)

// Claude accepts native image blocks through stream-json stdin. Images must
// never be embedded in a Windows command line or silently reduced to paths.
func claudeUserInput(text string, files []PromptFile, key string) (map[string]any, error) {
	files, err := NormalizePromptFiles(files)
	if err != nil {
		return nil, err
	}
	var images []map[string]any
	var paths []string
	for _, f := range files {
		mime, image := imageMIME(f.Path)
		if !image {
			paths = append(paths, f.Path)
			continue
		}
		raw, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, err
		}
		images = append(images, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": mime, "data": base64.StdEncoding.EncodeToString(raw)}})
	}
	parts := []map[string]any{{"type": "text", "text": joinPrompt(text, paths)}}
	parts = append(parts, images...)
	return map[string]any{"type": "user", "uuid": key, "message": map[string]any{"role": "user", "content": parts}}, nil
}
