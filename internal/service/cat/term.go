package cat

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/proc"
)

const (
	termEvent     = "cat:term"
	maxTermLive   = 4
	maxTermWrite  = 256 * 1024
	maxTermBuffer = 256 * 1024
)

// TermStartRequest 在这个对话的文件夹里打开一个本机命令行。
type TermStartRequest struct {
	ConvID string `json:"convId"`
}

// TermStartResult 是新开的命令行编号。Console 为真时按键由控制台自己回显。
type TermStartResult struct {
	ID      string `json:"id"`
	Console bool   `json:"console"`
}

// TermWriteRequest 把按键写进命令行。
type TermWriteRequest struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

// TermStopRequest 结束命令行。
type TermStopRequest struct {
	ID string `json:"id"`
}

// TermResizeRequest 改命令行的宽高，单位是字符。
type TermResizeRequest struct {
	ID   string `json:"id"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// TermEvent 是命令行吐出的文字，或结束码。
type TermEvent struct {
	ID   string `json:"id"`
	Data string `json:"data,omitempty"`
	Exit *int   `json:"exit,omitempty"`
}

type termProc struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	resize  func(cols, rows int) error
	end     func()
	wait    func() (int, error)
	alive   func() bool
	pending []byte
}

type termHub struct {
	emit   func(event string, payload any)
	newCmd func(cwd string) *exec.Cmd
	pty    bool
	mu     sync.Mutex
	live   map[string]*termProc
	seq    int
}

func newTermHub(emit func(string, any), newCmd func(string) *exec.Cmd) *termHub {
	pty := newCmd == nil && consoleAvailable()
	if newCmd == nil {
		newCmd = defaultTermCmd
	}
	return &termHub{emit: emit, newCmd: newCmd, pty: pty, live: map[string]*termProc{}}
}

func defaultTermCmd(cwd string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		com := os.Getenv("ComSpec")
		if com == "" {
			com = "cmd.exe"
		}
		cmd = exec.Command(com, "/Q", "/K", "chcp 65001>nul")
	} else {
		cmd = exec.Command("sh")
	}
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	return cmd
}

func (s *Service) termHub() *termHub {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terms == nil {
		s.terms = newTermHub(s.emit, nil)
	}
	return s.terms
}

// StartCatTerm 在对话文件夹里启动命令行。文件夹不可用时退回用户主目录。
func (s *Service) StartCatTerm(ctx context.Context, req TermStartRequest) (TermStartResult, error) {
	cwd := ""
	if root, err := s.convRoot(ctx, req.ConvID); err == nil {
		cwd = root
	}
	if cwd == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cwd = home
		}
	}
	if cwd == "" {
		cwd = "."
	}
	id, console, err := s.termHub().start(cwd)
	if err != nil {
		return TermStartResult{}, err
	}
	return TermStartResult{ID: id, Console: console}, nil
}

// WriteCatTerm 把一段输入写进已经打开的命令行。
func (s *Service) WriteCatTerm(_ context.Context, req TermWriteRequest) error {
	return s.termHub().write(req.ID, req.Data)
}

// StopCatTerm 结束命令行。已经结束的再停一次也算成功。
func (s *Service) StopCatTerm(_ context.Context, req TermStopRequest) error {
	s.termHub().stop(req.ID)
	return nil
}

// ResizeCatTerm 按字符数调整命令行宽高。
func (s *Service) ResizeCatTerm(_ context.Context, req TermResizeRequest) error {
	return s.termHub().resize(req.ID, req.Cols, req.Rows)
}

func (h *termHub) start(cwd string) (string, bool, error) {
	h.mu.Lock()
	if len(h.live) >= maxTermLive {
		h.mu.Unlock()
		return "", false, apperr.New(apperr.TaskConflict, "最多同时开 4 个终端。")
	}
	h.seq++
	id := "t" + itoa(h.seq)
	usePTY := h.pty
	h.mu.Unlock()

	var p *termProc
	var readers []io.Reader
	var err error
	console := false
	if usePTY {
		p, readers, err = openConsole(cwd)
		if err == nil && !consoleStaysUp(p) {
			if p.end != nil {
				p.end()
			}
			if p.wait != nil {
				_, _ = p.wait()
			}
			p = nil
			readers = nil
		}
	}
	if p == nil {
		p, readers, err = h.openPipe(cwd)
	} else {
		console = true
	}
	if err != nil {
		return "", false, err
	}
	h.mu.Lock()
	if len(h.live) >= maxTermLive {
		h.mu.Unlock()
		if p.end != nil {
			p.end()
		}
		return "", false, apperr.New(apperr.TaskConflict, "最多同时开 4 个终端。")
	}
	h.live[id] = p
	h.mu.Unlock()
	for _, r := range readers {
		go h.pump(id, p, r)
	}
	go h.waitProc(id, p)
	return id, console, nil
}

func consoleStaysUp(p *termProc) bool {
	if p == nil || p.alive == nil {
		return true
	}
	return p.alive()
}

func (h *termHub) openPipe(cwd string) (*termProc, []io.Reader, error) {
	cmd := h.newCmd(cwd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, apperr.Wrap(apperr.ProcessFailed, "终端没有启动。", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, apperr.Wrap(apperr.ProcessFailed, "终端没有启动。", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, apperr.Wrap(apperr.ProcessFailed, "终端没有启动。", err)
	}
	proc.Configure(cmd)
	if err := proc.Start(cmd); err != nil {
		return nil, nil, apperr.Wrap(apperr.ProcessFailed, "终端没有启动。", err)
	}
	p := &termProc{
		cmd:   cmd,
		stdin: stdin,
		end: func() {
			_ = stdin.Close()
			_ = proc.Kill(cmd)
		},
		wait: func() (int, error) {
			err := cmd.Wait()
			if err == nil {
				return 0, nil
			}
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return ee.ExitCode(), nil
			}
			return -1, err
		},
	}
	return p, []io.Reader{stdout, stderr}, nil
}

func (h *termHub) write(id, data string) error {
	if len(data) > maxTermWrite {
		return apperr.New(apperr.InvalidArgument, "一次输入太长。")
	}
	h.mu.Lock()
	p := h.live[id]
	h.mu.Unlock()
	if p == nil || p.stdin == nil {
		return apperr.New(apperr.NotFound, "终端已经结束。")
	}
	if _, err := io.WriteString(p.stdin, data); err != nil {
		return apperr.New(apperr.ProcessFailed, "命令没有送出去。")
	}
	return nil
}

func (h *termHub) resize(id string, cols, rows int) error {
	if cols < 2 || rows < 2 || cols > 500 || rows > 200 {
		return apperr.New(apperr.InvalidArgument, "窗口尺寸不对。")
	}
	h.mu.Lock()
	p := h.live[id]
	h.mu.Unlock()
	if p == nil {
		return apperr.New(apperr.NotFound, "终端已经结束。")
	}
	if p.resize == nil {
		return nil
	}
	if err := p.resize(cols, rows); err != nil {
		return apperr.New(apperr.ProcessFailed, "窗口尺寸没有改成。")
	}
	return nil
}

func (h *termHub) stop(id string) {
	h.mu.Lock()
	p := h.live[id]
	h.mu.Unlock()
	if p == nil || p.end == nil {
		return
	}
	p.end()
}

func (h *termHub) pump(id string, p *termProc, r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			h.push(id, p, buf[:n])
		}
		if err != nil {
			h.flush(id, p)
			return
		}
	}
}

func (h *termHub) push(id string, p *termProc, chunk []byte) {
	h.mu.Lock()
	if _, ok := h.live[id]; !ok {
		h.mu.Unlock()
		return
	}
	p.pending = append(p.pending, chunk...)
	if len(p.pending) > maxTermBuffer {
		p.pending = p.pending[len(p.pending)-maxTermBuffer:]
	}
	out, rest := splitUTF8(p.pending)
	p.pending = rest
	emit := h.emit
	h.mu.Unlock()
	if len(out) > 0 && emit != nil {
		emit(termEvent, TermEvent{ID: id, Data: string(out)})
	}
}

func (h *termHub) flush(id string, p *termProc) {
	h.mu.Lock()
	if len(p.pending) == 0 || h.emit == nil {
		p.pending = nil
		h.mu.Unlock()
		return
	}
	data := string(p.pending)
	p.pending = nil
	emit := h.emit
	h.mu.Unlock()
	emit(termEvent, TermEvent{ID: id, Data: data})
}

func (h *termHub) waitProc(id string, p *termProc) {
	code := -1
	if p.wait != nil {
		if c, err := p.wait(); err == nil {
			code = c
		}
	}
	h.mu.Lock()
	delete(h.live, id)
	emit := h.emit
	h.mu.Unlock()
	if emit != nil {
		emit(termEvent, TermEvent{ID: id, Exit: &code})
	}
}

func splitUTF8(buf []byte) (out, rest []byte) {
	i := 0
	for i < len(buf) {
		if !utf8.FullRune(buf[i:]) {
			break
		}
		_, size := utf8.DecodeRune(buf[i:])
		if size <= 0 {
			break
		}
		i += size
	}
	return buf[:i], buf[i:]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
