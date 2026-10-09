package cat

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/store"
)

// maxCatFileEntries 是每一层最多返回的条目数（契约 6.19.11.3）。
const maxCatFileEntries = 500

const msgFileListFailed = "文件列表加载失败，请重试。"

func errFilePath() error {
	return apperr.New(apperr.InvalidArgument, "这个路径不能查看。").WithDetail("reason=path")
}

func errFileMissing() error {
	return apperr.New(apperr.NotFound, "找不到这个文件夹。")
}

// ensureConvDir 创建普通对话的新文件夹。旧 cwd 已存在时不另外建新目录。失败只记日志。
func (s *Service) ensureConvDir(convID string) {
	dir := paths.ResolveCatConvDir(s.cfg.DataRoot, convID)
	if dir == "" {
		return
	}
	// 旧对话继续用旧目录，不在新位置再造一个空文件夹。
	if paths.SamePath(dir, paths.LegacyCatCwd(s.cfg.DataRoot, convID)) {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.cfg.Logf("cat conv dir create %s: %v", convID, err)
	}
}

// removeConvDir 删除普通对话自己的文件夹（新位置和旧 cwd，哪个在就删哪个）。
// 只删与这个会话 id 精确对应的目录，失败只记日志。
func (s *Service) removeConvDir(convID string) {
	root := s.cfg.DataRoot
	if root == "" {
		return
	}
	for _, dir := range []string{paths.CatConvDir(root, convID), paths.LegacyCatCwd(root, convID)} {
		if dir == "" || !paths.IsCatConvDir(root, convID, dir) {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			s.cfg.Logf("cat conv dir remove %s: %v", dir, err)
		}
	}
}

func (s *Service) requireConv(ctx context.Context, convID string) (store.CatConversation, error) {
	if s.cfg.Store == nil {
		return store.CatConversation{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	convID = strings.TrimSpace(convID)
	if convID == "" {
		return store.CatConversation{}, apperr.New(apperr.InvalidArgument, "会话编号不能为空")
	}
	c, err := s.cfg.Store.GetCatConversation(ctx, convID)
	if errors.Is(err, sql.ErrNoRows) {
		return store.CatConversation{}, apperr.New(apperr.NotFound, "找不到这个会话")
	}
	if err != nil {
		return store.CatConversation{}, apperr.Wrap(apperr.IOError, "读取会话失败", err)
	}
	return c, nil
}

// conversationRoot 返回当前对话的文件根。项目对话用项目文件夹（不见了则 CAT_PROJECT_MISSING，不创建）。
// 普通对话用 paths.ResolveCatConvDir；目录不在时先建再列。
func (s *Service) conversationRoot(ctx context.Context, c store.CatConversation) (string, error) {
	if c.ProjectID != "" {
		p, err := s.projectForConversation(ctx, c.ProjectID)
		if err != nil {
			return "", err
		}
		return p.Path, nil
	}
	dir := paths.ResolveCatConvDir(s.cfg.DataRoot, c.ID)
	if dir == "" {
		return "", apperr.New(apperr.IOError, msgFileListFailed)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", apperr.Wrap(apperr.IOError, msgFileListFailed, err)
	}
	return dir, nil
}

// ListCatFiles 只列对话根下的一层（契约 6.19.11）。不读文件内容。
func (s *Service) ListCatFiles(ctx context.Context, req ListFilesRequest) (ListFilesResult, error) {
	c, err := s.requireConv(ctx, req.ConvID)
	if err != nil {
		return ListFilesResult{}, err
	}
	root, err := s.conversationRoot(ctx, c)
	if err != nil {
		return ListFilesResult{}, err
	}
	dir, parentRel, err := resolveListDir(root, req.RelPath)
	if err != nil {
		return ListFilesResult{}, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return ListFilesResult{}, errFileMissing()
		}
		return ListFilesResult{}, apperr.Wrap(apperr.IOError, msgFileListFailed, err)
	}
	list := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if hiddenFileName(name) {
			continue
		}
		info, infoErr := e.Info()
		link := e.Type()&os.ModeSymlink != 0
		if infoErr == nil && info.Mode()&os.ModeSymlink != 0 {
			link = true
		}
		item := FileEntry{Name: name, RelPath: childRel(parentRel, name)}
		if link {
			// 符号链接 / junction：不可展开，不跟随，不取其目标的大小。
			list = append(list, item)
			continue
		}
		if (infoErr == nil && info.IsDir()) || (infoErr != nil && e.IsDir()) {
			item.IsDir = true
		}
		if infoErr == nil {
			if !item.IsDir {
				item.Size = info.Size()
			}
			if !info.ModTime().IsZero() {
				item.ModTime = info.ModTime().UnixMilli()
			}
		}
		list = append(list, item)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].IsDir != list[j].IsDir {
			return list[i].IsDir
		}
		li, lj := strings.ToLower(list[i].Name), strings.ToLower(list[j].Name)
		if li != lj {
			return li < lj
		}
		return list[i].Name < list[j].Name
	})
	truncated := false
	if len(list) > maxCatFileEntries {
		list = list[:maxCatFileEntries]
		truncated = true
	}
	return ListFilesResult{Root: root, Entries: list, Truncated: truncated}, nil
}

// RevealCatConversationFolder 在系统文件管理器里打开对话的根（契约 6.19.11.5）。
func (s *Service) RevealCatConversationFolder(ctx context.Context, req RevealConversationFolderRequest) error {
	c, err := s.requireConv(ctx, req.ConvID)
	if err != nil {
		return err
	}
	if c.ProjectID != "" {
		return s.RevealCatProject(ctx, RevealProjectRequest{ID: c.ProjectID})
	}
	dir := paths.ResolveCatConvDir(s.cfg.DataRoot, c.ID)
	if dir == "" {
		return apperr.New(apperr.IOError, msgFileListFailed)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return apperr.Wrap(apperr.IOError, msgFileListFailed, err)
	}
	if s.cfg.OpenFolder == nil {
		return apperr.New(apperr.ProcessFailed, catagent.MsgProjectRevealFail)
	}
	if err := s.cfg.OpenFolder(dir); err != nil {
		switch {
		case apperr.Is(err, apperr.InvalidArgument), apperr.Is(err, apperr.ProcessFailed):
			return err
		default:
			return apperr.Wrap(apperr.ProcessFailed, catagent.MsgProjectRevealFail, err)
		}
	}
	return nil
}

// resolveListDir 把 relPath 解析成根下的真实目录。任何一段是符号链接 / junction 都拒绝。
// 第二个返回值是相对根的正斜杠路径（根本身为 ""）。
func resolveListDir(root, rel string) (string, string, error) {
	root = filepath.Clean(root)
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return root, "", nil
	}
	if strings.Contains(rel, ":") || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", "", errFilePath()
	}
	slash := strings.ReplaceAll(rel, `\`, "/")
	if strings.HasPrefix(slash, "/") {
		return "", "", errFilePath()
	}
	for _, seg := range strings.Split(slash, "/") {
		if seg == ".." {
			return "", "", errFilePath()
		}
	}
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(slash)))
	relOut, err := filepath.Rel(root, target)
	if err != nil || !lexicalInside(root, target) {
		return "", "", errFilePath()
	}
	cur := root
	parent := ""
	if relOut != "." {
		for _, seg := range strings.Split(relOut, string(filepath.Separator)) {
			if seg == "" || seg == "." {
				continue
			}
			cur = filepath.Join(cur, seg)
			fi, lerr := os.Lstat(cur)
			if lerr != nil {
				if os.IsNotExist(lerr) {
					return "", "", errFileMissing()
				}
				return "", "", apperr.Wrap(apperr.IOError, msgFileListFailed, lerr)
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				return "", "", errFilePath()
			}
			if !fi.IsDir() {
				return "", "", errFileMissing()
			}
			if parent == "" {
				parent = seg
			} else {
				parent += "/" + seg
			}
		}
	}
	return cur, parent, nil
}

func lexicalInside(root, p string) bool {
	r, q := filepath.Clean(root), filepath.Clean(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		r, q = strings.ToLower(r), strings.ToLower(q)
	}
	if q == r {
		return true
	}
	sep := string(filepath.Separator)
	if !strings.HasSuffix(r, sep) {
		r += sep
	}
	return strings.HasPrefix(q, r)
}

func childRel(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

func hiddenFileName(name string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		switch strings.ToLower(name) {
		case ".git", "node_modules", "target", ".ds_store", "thumbs.db":
			return true
		}
		return false
	}
	switch name {
	case ".git", "node_modules", "target", ".DS_Store", "Thumbs.db":
		return true
	}
	return false
}
