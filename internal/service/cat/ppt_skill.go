package cat

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed pptskill/SKILL.md pptskill/scripts/office.cjs
var pptSkill embed.FS

// Built-in files live in an application-owned directory. A user skill of the
// same name in the shared library is never overwritten.
func (s *Service) ensurePptSkill() (string, error) {
	if s.cfg.DataRoot == "" {
		return "", fmt.Errorf("应用数据目录尚未就绪")
	}
	s.pptMu.Lock()
	defer s.pptMu.Unlock()
	root := filepath.Join(s.cfg.DataRoot, "cat", "builtin-skills", "ffc-ppt-live")
	for _, name := range []string{"SKILL.md", "scripts/office.cjs"} {
		b, err := pptSkill.ReadFile("pptskill/" + name)
		if err != nil {
			return "", err
		}
		path := filepath.Join(root, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		if err = os.WriteFile(path, b, 0600); err != nil {
			return "", err
		}
	}
	return filepath.Join(root, "SKILL.md"), nil
}
