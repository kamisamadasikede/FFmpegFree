package doc

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/task"
)

const (
	maxBinarySaveSessions = 4
	maxChunkBytes         = 4 << 20 // 4 MiB
	binarySaveTTL         = 10 * time.Minute
)

// DocBinarySaveBegin / Session / Chunk / Commit / Abort / Result — 契约 6.12.49。
type DocBinarySaveBegin struct {
	SourceID   string `json:"sourceId,omitempty"`
	TaskID     string `json:"taskId,omitempty"`
	Mode       string `json:"mode"`
	TargetPath string `json:"targetPath,omitempty"`
	Revision   string `json:"revision,omitempty"`
	TotalBytes int64  `json:"totalBytes"`
}

type DocBinarySaveSession struct {
	SaveID        string `json:"saveId"`
	MaxChunkBytes int    `json:"maxChunkBytes"`
	ExpiresAt     int64  `json:"expiresAt"`
}

type DocBinaryChunk struct {
	SaveID string `json:"saveId"`
	Seq    int    `json:"seq"`
	Data   string `json:"data"`
}

type DocBinaryChunkResult struct {
	ReceivedBytes int64 `json:"receivedBytes"`
}

type DocBinarySaveCommit struct {
	SaveID string `json:"saveId"`
	SHA256 string `json:"sha256"`
}

type DocBinarySaveResult struct {
	Path       string `json:"path"`
	Revision   string `json:"revision"`
	SizeBytes  int64  `json:"sizeBytes"`
	SavedAt    int64  `json:"savedAt"`
	BackupPath string `json:"backupPath,omitempty"`
}

type DocBinarySaveAbort struct {
	SaveID string `json:"saveId"`
}

type binarySaveSession struct {
	id         string
	target     *editTarget
	mode       string // overwrite | save_as
	targetPath string
	revision   string
	total      int64
	received   int64
	nextSeq    int
	expires    time.Time
	dir        string
	part       string
	file       *os.File
	writeKey   string // 真实路径锁 key
}

type binarySaveHub struct {
	mu       sync.Mutex
	sessions map[string]*binarySaveSession
	byPath   map[string]string // pathKey -> saveId
}

func (s *Service) saveHub() *binarySaveHub {
	if s.saves == nil {
		s.saves = &binarySaveHub{sessions: map[string]*binarySaveSession{}, byPath: map[string]string{}}
	}
	return s.saves
}

func (s *Service) docsaveRoot() string {
	if s.cfg.TempRoot != "" {
		return filepath.Join(filepath.Dir(s.cfg.TempRoot), "docsave")
	}
	if s.cfg.DataDir != "" {
		return filepath.Join(s.cfg.DataDir, "tmp", "docsave")
	}
	return ""
}

func (s *Service) sweepSaves() {
	t := time.NewTicker(time.Minute)
	for range t.C {
		s.expireSaves()
	}
}

func (s *Service) expireSaves() {
	h := s.saveHub()
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, sess := range h.sessions {
		if now.After(sess.expires) {
			s.dropSessionLocked(h, id)
		}
	}
}

func (s *Service) dropSessionLocked(h *binarySaveHub, id string) {
	sess := h.sessions[id]
	if sess == nil {
		return
	}
	if sess.file != nil {
		sess.file.Close()
	}
	_ = os.RemoveAll(sess.dir)
	delete(h.sessions, id)
	delete(h.byPath, sess.writeKey)
}

// BeginDocBinarySave 开始分段保存。
func (s *Service) BeginDocBinarySave(ctx context.Context, req DocBinarySaveBegin) (DocBinarySaveSession, error) {
	if req.Mode != "overwrite" && req.Mode != "save_as" {
		return DocBinarySaveSession{}, apperr.New(apperr.InvalidArgument, "出了点问题，请重试。")
	}
	if req.Mode == "overwrite" && (req.Revision == "" || req.TargetPath != "") {
		return DocBinarySaveSession{}, apperr.New(apperr.InvalidArgument, "出了点问题，请重试。")
	}
	if req.Mode == "save_as" && req.TargetPath == "" {
		return DocBinarySaveSession{}, apperr.New(apperr.InvalidArgument, "出了点问题，请重试。")
	}
	if req.TotalBytes <= 0 {
		return DocBinarySaveSession{}, apperr.New(apperr.InvalidArgument, "出了点问题，请重试。")
	}
	if req.TotalBytes > maxDocxEditBytes {
		return DocBinarySaveSession{}, apperr.New(apperr.InvalidArgument, "内容太多，没法在这里保存。请用默认程序打开编辑。").WithDetail("reason=too_large")
	}
	t, err := s.resolveEditID(ctx, req.SourceID, req.TaskID)
	if err != nil {
		return DocBinarySaveSession{}, err
	}
	if !isDocx(t.Ext) {
		return DocBinarySaveSession{}, apperr.New(apperr.InvalidArgument, "只能保存成同一种格式。").WithDetail("reason=format")
	}
	targetPath := t.WritePath
	if req.Mode == "save_as" {
		if err := s.validateSaveAsPath(ctx, req.TargetPath); err != nil {
			return DocBinarySaveSession{}, err
		}
		targetPath = filepath.Clean(req.TargetPath)
		if normExt(filepath.Ext(targetPath)) != "docx" {
			return DocBinarySaveSession{}, apperr.New(apperr.InvalidArgument, "只能保存成同一种格式。").WithDetail("reason=format")
		}
		if err := s.conflictIfTargetBusy(ctx, targetPath); err != nil {
			return DocBinarySaveSession{}, err
		}
	} else {
		if t.MissingOrig {
			return DocBinarySaveSession{}, apperr.New(apperr.NotFound, "原文件已经不在了，改完只能另存为。").WithDetail("reason=file")
		}
		if err := s.sourceBusy(ctx, t); err != nil {
			return DocBinarySaveSession{}, err
		}
		sum, _, err := fileSHA256(t.WritePath)
		if err != nil {
			return DocBinarySaveSession{}, mapReadWriteErr(err)
		}
		if sum != req.Revision {
			return DocBinarySaveSession{}, apperr.New(apperr.TaskConflict, "文件在别处被改过了，请重新打开，或另存为。").WithDetail("reason=file_changed")
		}
	}
	writeKey := pathKey(realPath(targetPath))
	root := s.docsaveRoot()
	if root == "" {
		return DocBinarySaveSession{}, s.internalNotReady("文档服务")
	}
	// 磁盘空间
	need := req.TotalBytes*2 + 64<<20
	if free, err := diskFreeFor(root); err == nil && free < need {
		return DocBinarySaveSession{}, apperr.New(apperr.ConvertDiskFull, "磁盘空间不足，没有保存。")
	}

	h := s.saveHub()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.sessions) >= maxBinarySaveSessions {
		return DocBinarySaveSession{}, apperr.New(apperr.TaskConflict, "出了点问题，请重试。").WithDetail("reason=saving")
	}
	if _, ok := h.byPath[writeKey]; ok {
		return DocBinarySaveSession{}, apperr.New(apperr.TaskConflict, "出了点问题，请重试。").WithDetail("reason=saving")
	}
	id := newPreviewID()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DocBinarySaveSession{}, mapWriteErr(err)
	}
	part := filepath.Join(dir, "data.part")
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		os.RemoveAll(dir)
		return DocBinarySaveSession{}, mapWriteErr(err)
	}
	exp := time.Now().Add(binarySaveTTL)
	sess := &binarySaveSession{
		id: id, target: t, mode: req.Mode, targetPath: targetPath, revision: req.Revision,
		total: req.TotalBytes, expires: exp, dir: dir, part: part, file: f, writeKey: writeKey,
	}
	h.sessions[id] = sess
	h.byPath[writeKey] = id
	return DocBinarySaveSession{SaveID: id, MaxChunkBytes: maxChunkBytes, ExpiresAt: exp.UnixMilli()}, nil
}

// AppendDocBinaryChunk 追加一段。
func (s *Service) AppendDocBinaryChunk(ctx context.Context, req DocBinaryChunk) (DocBinaryChunkResult, error) {
	h := s.saveHub()
	h.mu.Lock()
	sess := h.sessions[req.SaveID]
	if sess == nil || time.Now().After(sess.expires) {
		if sess != nil {
			s.dropSessionLocked(h, req.SaveID)
		}
		h.mu.Unlock()
		return DocBinaryChunkResult{}, apperr.New(apperr.NotFound, "出了点问题，请重试。").WithDetail("reason=save_session")
	}
	raw, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil || len(raw) == 0 || len(raw) > maxChunkBytes {
		h.mu.Unlock()
		return DocBinaryChunkResult{}, apperr.New(apperr.InvalidArgument, "出了点问题，请重试。")
	}
	if req.Seq != sess.nextSeq {
		h.mu.Unlock()
		return DocBinaryChunkResult{}, apperr.New(apperr.InvalidArgument, "出了点问题，请重试。").WithDetail("reason=chunk_order")
	}
	if sess.received+int64(len(raw)) > sess.total || sess.received+int64(len(raw)) > maxDocxEditBytes {
		s.dropSessionLocked(h, req.SaveID)
		h.mu.Unlock()
		return DocBinaryChunkResult{}, apperr.New(apperr.InvalidArgument, "内容太多，没法在这里保存。请用默认程序打开编辑。").WithDetail("reason=too_large")
	}
	f := sess.file
	h.mu.Unlock()

	if _, err := f.Write(raw); err != nil {
		h.mu.Lock()
		s.dropSessionLocked(h, req.SaveID)
		h.mu.Unlock()
		if task.IsDiskFull(err) {
			return DocBinaryChunkResult{}, apperr.New(apperr.ConvertDiskFull, "磁盘空间不足，没有保存。")
		}
		return DocBinaryChunkResult{}, mapWriteErr(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	sess = h.sessions[req.SaveID]
	if sess == nil {
		return DocBinaryChunkResult{}, apperr.New(apperr.NotFound, "出了点问题，请重试。").WithDetail("reason=save_session")
	}
	sess.received += int64(len(raw))
	sess.nextSeq++
	return DocBinaryChunkResult{ReceivedBytes: sess.received}, nil
}

// CommitDocBinarySave 提交；无论成败结束会话。
func (s *Service) CommitDocBinarySave(ctx context.Context, req DocBinarySaveCommit) (res DocBinarySaveResult, err error) {
	h := s.saveHub()
	h.mu.Lock()
	sess := h.sessions[req.SaveID]
	if sess == nil || time.Now().After(sess.expires) {
		if sess != nil {
			s.dropSessionLocked(h, req.SaveID)
		}
		h.mu.Unlock()
		return DocBinarySaveResult{}, apperr.New(apperr.NotFound, "出了点问题，请重试。").WithDetail("reason=save_session")
	}
	// 取出后从 map 去掉（会话结束）
	delete(h.sessions, req.SaveID)
	delete(h.byPath, sess.writeKey)
	h.mu.Unlock()
	defer func() {
		if sess.file != nil {
			sess.file.Close()
		}
		_ = os.RemoveAll(sess.dir)
	}()

	if err := sess.file.Sync(); err != nil {
		return DocBinarySaveResult{}, mapWriteErr(err)
	}
	sess.file.Close()
	sess.file = nil

	sum, size, err := fileSHA256(sess.part)
	if err != nil {
		return DocBinarySaveResult{}, mapWriteErr(err)
	}
	want := strings.ToLower(strings.TrimSpace(req.SHA256))
	if size != sess.total || sum != want || sess.received != sess.total {
		return DocBinarySaveResult{}, apperr.New(apperr.InvalidArgument, "出了点问题，请重试。").WithDetail("reason=checksum")
	}
	if sess.mode == "overwrite" {
		if err := s.sourceBusy(ctx, sess.target); err != nil {
			return DocBinarySaveResult{}, err
		}
		cur, _, err := fileSHA256(sess.targetPath)
		if err != nil {
			return DocBinarySaveResult{}, mapReadWriteErr(err)
		}
		if cur != sess.revision {
			return DocBinarySaveResult{}, apperr.New(apperr.TaskConflict, "文件在别处被改过了，请重新打开，或另存为。").WithDetail("reason=file_changed")
		}
	}
	if sess.mode == "save_as" {
		// Begin 之后目标可能被别的记录用上：提交前再查一次
		if err := s.conflictIfTargetBusy(ctx, sess.targetPath); err != nil {
			return DocBinarySaveResult{}, err
		}
	}
	if err := validateDocxForSave(sess.part); err != nil {
		return DocBinarySaveResult{}, err
	}
	var backupPath string
	if sess.mode == "overwrite" {
		bp, err := backupDocxOverwrite(sess.targetPath, sess.revision)
		if err != nil {
			return DocBinarySaveResult{}, err
		}
		backupPath = bp
	}
	unlock := lockPath(sess.targetPath)
	defer unlock()
	if err := copyAtomic(sess.part, sess.targetPath); err != nil {
		return DocBinarySaveResult{}, err
	}
	if sess.mode == "overwrite" {
		pruneDocxBackups(sess.targetPath)
	}
	s.allowReveal(sess.targetPath)
	if backupPath != "" {
		s.allowReveal(backupPath)
	}
	if sess.mode == "overwrite" {
		// 只有写回原位置时才刷新副本 / 结果大小；另存为写的是别的位置，不能动原行的副本
		data, _ := os.ReadFile(sess.targetPath)
		s.afterTextSave(ctx, sess.target, data)
	}
	return DocBinarySaveResult{
		Path: sess.targetPath, Revision: sum, SizeBytes: size,
		SavedAt: time.Now().UnixMilli(), BackupPath: backupPath,
	}, nil
}

// AbortDocBinarySave 幂等取消。
func (s *Service) AbortDocBinarySave(req DocBinarySaveAbort) error {
	if req.SaveID == "" {
		return nil
	}
	h := s.saveHub()
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.sessions[req.SaveID]; ok {
		s.dropSessionLocked(h, req.SaveID)
	}
	return nil
}

func diskFreeFor(dir string) (int64, error) {
	_ = os.MkdirAll(dir, 0o755)
	return diskFreeBytes(dir)
}
