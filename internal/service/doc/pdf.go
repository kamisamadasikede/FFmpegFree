package doc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/store"
)

const maxHandles = 1024

type handle struct {
	id      string
	path    string
	pathKey string
	token   string
	real    string      // 登记时 EvalSymlinks 的真实路径
	info    os.FileInfo // 登记时的文件信息，ReadPDFChunk 每次用 os.SameFile 对照
}

// handles 是 PDF 句柄登记表：id → 路径。ReadPDFChunk 只按 id 查表，不拼路径。
type handles struct {
	mu    sync.Mutex
	byID  map[string]*handle
	byKey map[string]*handle
	order []string // 登记顺序，超过 maxHandles 淘汰最旧的
}

func newHandles() *handles {
	return &handles{byID: map[string]*handle{}, byKey: map[string]*handle{}}
}

func newHandleID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// put 登记（同一路径复用同一 id）；返回句柄和被淘汰的句柄（调用方撤销其 token）。
func (h *handles) put(path, key, real string, info os.FileInfo) (*handle, []*handle, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if x, ok := h.byKey[key]; ok {
		x.path, x.real, x.info = path, real, info // 同一路径：复用 id，刷新真实路径与文件信息
		return x, nil, nil
	}
	hid, err := newHandleID()
	if err != nil {
		return nil, nil, err
	}
	x := &handle{id: hid, path: path, pathKey: key, real: real, info: info}
	h.byID[hid], h.byKey[key] = x, x
	h.order = append(h.order, hid)
	var evicted []*handle
	for len(h.order) > maxHandles {
		old := h.byID[h.order[0]]
		h.order = h.order[1:]
		if old != nil {
			delete(h.byID, old.id)
			delete(h.byKey, old.pathKey)
			evicted = append(evicted, old)
		}
	}
	return x, evicted, nil
}

func (h *handles) get(hid string) (handle, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	x, ok := h.byID[hid]
	if !ok {
		return handle{}, false
	}
	return *x, true
}

func (h *handles) setToken(hid, tok string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if x, ok := h.byID[hid]; ok {
		x.token = tok
	}
}

// removeByKey 按 path_key 撤销句柄，返回被撤销的句柄。
func (h *handles) removeByKey(key string) *handle {
	h.mu.Lock()
	defer h.mu.Unlock()
	x, ok := h.byKey[key]
	if !ok {
		return nil
	}
	delete(h.byKey, key)
	delete(h.byID, x.id)
	for i, o := range h.order {
		if o == x.id {
			h.order = append(h.order[:i], h.order[i+1:]...)
			break
		}
	}
	return x
}

// OpenPDF 校验并登记一个 PDF，返回句柄，同时写入 / 更新最近列表。
//
// 路径必须绝对、存在、是普通文件、可读、扩展名 .pdf、前 1024 字节内含 %PDF-、大小 ≤ 512 MiB。
// 加密 PDF 也能打开（密码由前端 pdf.js 处理）。size > 64 MiB 时 URL 是 /local/<token>（需要 localassets 接入），否则为空。
func (s *Service) OpenPDF(ctx context.Context, path string) (PDFSource, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return PDFSource{}, apperr.New(apperr.InvalidArgument, "PDF 路径必须是绝对路径").WithDetail(path)
	}
	p, key, err := paths.Normalize(path)
	if err != nil {
		return PDFSource{}, apperr.Wrap(apperr.InvalidArgument, "路径不合法", err)
	}
	if !strings.EqualFold(filepath.Ext(p), ".pdf") {
		return PDFSource{}, apperr.New(apperr.InvalidArgument, "只支持 .pdf 文件").WithDetail(p)
	}
	f, fi, err := openRegular(p)
	if err != nil {
		return PDFSource{}, err
	}
	defer f.Close()
	if fi.Size() > MaxPDFBytes {
		return PDFSource{}, apperr.New(apperr.InvalidArgument, "文件超过 512 MiB").WithDetail(fmt.Sprintf("%d 字节", fi.Size()))
	}
	head := make([]byte, 1024)
	n, rerr := io.ReadFull(f, head)
	if rerr != nil && !errors.Is(rerr, io.EOF) && !errors.Is(rerr, io.ErrUnexpectedEOF) {
		return PDFSource{}, apperr.Wrap(apperr.IOError, "读取文件失败", rerr)
	}
	if !bytes.Contains(head[:n], []byte("%PDF-")) {
		return PDFSource{}, apperr.New(apperr.InvalidArgument, "不是 PDF 文件").WithDetail(p)
	}

	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return PDFSource{}, readErr("无法读取文件", err)
	}
	h, evicted, err := s.h.put(p, key, real, fi)
	if err != nil {
		return PDFSource{}, apperr.Wrap(apperr.Internal, "生成句柄失败", err)
	}
	for _, e := range evicted {
		s.revoke(e.token)
	}
	src := PDFSource{ID: h.id, Path: p, Name: filepath.Base(p), Size: fi.Size()}
	if fi.Size() > WholeLoadBytes && s.cfg.Local != nil {
		e, err := s.cfg.Local.Register(p)
		if err != nil {
			return PDFSource{}, apperr.Wrap(apperr.IOError, "登记预览地址失败", err)
		}
		s.h.setToken(h.id, e.Token)
		src.URL = e.URL
	}
	if s.cfg.Recent != nil {
		if _, err := s.cfg.Recent.UpsertDocRecent(ctx, key, store.DocRecent{ID: id.New(), Path: p, Name: src.Name, Size: fi.Size()}); err != nil {
			return PDFSource{}, apperr.Wrap(apperr.IOError, "记录最近打开失败", err)
		}
	}
	return src, nil
}

func (s *Service) revoke(token string) {
	if token != "" && s.cfg.Local != nil {
		s.cfg.Local.Revoke(token)
	}
}

// openRegular 打开路径并要求它是普通文件（跟随符号链接后）。
func openRegular(p string) (*os.File, os.FileInfo, error) {
	// 先 Stat 再 Open：对 FIFO 直接 Open 会一直阻塞；打开后再 Stat 并用 SameFile 确认打开的就是刚才检查的那个文件。
	pre, err := os.Stat(p)
	if err != nil {
		return nil, nil, readErr("无法读取文件", err)
	}
	if err := notRegular(pre, p); err != nil {
		return nil, nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, nil, readErr("无法读取文件", err)
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, apperr.Wrap(apperr.IOError, "无法读取文件", err)
	}
	if err := notRegular(fi, p); err != nil {
		f.Close()
		return nil, nil, err
	}
	if !os.SameFile(pre, fi) {
		f.Close()
		return nil, nil, apperr.New(apperr.IOError, "文件在读取时被替换，请重试").WithDetail(p)
	}
	return f, fi, nil
}

func notRegular(fi os.FileInfo, p string) error {
	if fi.IsDir() {
		return apperr.New(apperr.InvalidArgument, "这是文件夹，不是文件").WithDetail(p)
	}
	if !fi.Mode().IsRegular() {
		return apperr.New(apperr.InvalidArgument, "不是普通文件").WithDetail(p)
	}
	return nil
}

// ReadPDFChunk 按句柄读 PDF 的一段字节（契约 6.12.4 第 2 点，校验顺序即优先级）：
//  1. length 1~1 MiB、offset ≥ 0、offset > MaxInt64-length 都是 INVALID_ARGUMENT（比较写成减法，不写 offset+length）；
//  2. id 只查登记表（重启后失效、被 RemoveRecentPDFs 撤销 → NOT_FOUND），路径只来自登记表；
//  3. 每次重新校验：EvalSymlinks 与登记时一致、仍是普通文件、os.SameFile 与登记时相同（否则 NOT_FOUND），当前大小 ≤ 512 MiB（否则 INVALID_ARGUMENT）；
//  4. offset ≥ 当前大小返回 length=0、eof=true；否则读 min(length, 当前大小-offset) 字节。size 字段是本次读取时的当前大小。
func (s *Service) ReadPDFChunk(hid string, offset int64, length int) (PDFChunk, error) {
	if length < 1 || length > ChunkBytes {
		return PDFChunk{}, apperr.New(apperr.InvalidArgument, "length 必须在 1 到 1 MiB 之间").WithDetail(fmt.Sprint(length))
	}
	if offset < 0 {
		return PDFChunk{}, apperr.New(apperr.InvalidArgument, "offset 不能为负").WithDetail(fmt.Sprint(offset))
	}
	if offset > math.MaxInt64-int64(length) {
		return PDFChunk{}, apperr.New(apperr.InvalidArgument, "offset 超出范围").WithDetail(fmt.Sprint(offset))
	}
	h, ok := s.h.get(hid)
	if !ok {
		return PDFChunk{}, apperr.New(apperr.NotFound, "PDF 句柄已失效，请重新打开")
	}
	gone := apperr.New(apperr.NotFound, "PDF 文件不存在或已被替换，请重新打开").WithDetail(h.path)
	// 先按路径 Stat（对 FIFO 直接 Open 会阻塞），再打开后对同一个 fd 再 Stat。
	real, err := filepath.EvalSymlinks(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			return PDFChunk{}, gone
		}
		return PDFChunk{}, apperr.Wrap(apperr.IOError, "读取文件失败", err)
	}
	if real != h.real {
		return PDFChunk{}, gone
	}
	pre, err := os.Stat(real)
	if err != nil {
		if os.IsNotExist(err) {
			return PDFChunk{}, gone
		}
		return PDFChunk{}, apperr.Wrap(apperr.IOError, "读取文件失败", err)
	}
	if !pre.Mode().IsRegular() || !os.SameFile(h.info, pre) {
		return PDFChunk{}, gone
	}
	f, err := os.Open(real)
	if err != nil {
		if os.IsNotExist(err) {
			return PDFChunk{}, gone
		}
		return PDFChunk{}, apperr.Wrap(apperr.IOError, "读取文件失败", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return PDFChunk{}, apperr.Wrap(apperr.IOError, "读取文件失败", err)
	}
	if !fi.Mode().IsRegular() || !os.SameFile(h.info, fi) {
		return PDFChunk{}, gone
	}
	size := fi.Size()
	if size > MaxPDFBytes {
		return PDFChunk{}, apperr.New(apperr.InvalidArgument, "文件超过 512 MiB").WithDetail(fmt.Sprintf("%d 字节", size))
	}
	if offset >= size {
		return PDFChunk{Offset: offset, Length: 0, EOF: true, Size: size, Data: []byte{}}, nil
	}
	n := int64(length)
	if remain := size - offset; n > remain { // offset < size，不会溢出
		n = remain
	}
	buf := make([]byte, n)
	got, err := f.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return PDFChunk{}, apperr.Wrap(apperr.IOError, "读取文件失败", err)
	}
	if got == 0 { // 文件在 stat 之后被截短
		return PDFChunk{Offset: offset, Length: 0, EOF: true, Size: size, Data: []byte{}}, nil
	}
	return PDFChunk{Offset: offset, Length: got, EOF: offset+int64(got) >= size, Size: size, Data: buf[:got]}, nil
}

// ListRecentPDFs 按打开时间倒序返回最近打开的 PDF（默认 20，最大 200）；文件已删除的 exists=false，记录保留。
func (s *Service) ListRecentPDFs(ctx context.Context, limit int) ([]PDFFile, error) {
	out := []PDFFile{}
	if s.cfg.Recent == nil {
		return out, nil
	}
	rows, err := s.cfg.Recent.ListDocRecent(ctx, limit)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "读取最近打开失败", err)
	}
	for _, r := range rows {
		pf := PDFFile{ID: r.ID, Path: r.Path, Name: r.Name, Size: r.Size, OpenedAt: r.OpenedAt}
		if fi, err := os.Stat(r.Path); err == nil && fi.Mode().IsRegular() {
			pf.Exists, pf.Size = true, fi.Size()
		}
		out = append(out, pf)
	}
	return out, nil
}

// RemoveRecentPDFs 删除最近打开记录（不删文件），同时撤销对应的句柄和 /local/<token>。
// 一次最多 500 个 id，超过 INVALID_ARGUMENT；不存在的 id 忽略。
func (s *Service) RemoveRecentPDFs(ctx context.Context, ids []string) error {
	if len(ids) > MaxRemoveIDs {
		return apperr.New(apperr.InvalidArgument, "一次最多删除 500 条记录")
	}
	if s.cfg.Recent == nil || len(ids) == 0 {
		return nil
	}
	keys, err := s.cfg.Recent.DeleteDocRecent(ctx, ids)
	if err != nil {
		return apperr.Wrap(apperr.IOError, "删除记录失败", err)
	}
	for _, k := range keys {
		if h := s.h.removeByKey(k); h != nil {
			s.revoke(h.token)
		}
	}
	return nil
}
