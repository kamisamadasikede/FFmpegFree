package genoffice

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/proc"
)

// xlsxSidecar speaks the official spreadsheet engine's line protocol.
// The browser never chooses a path; this process only opens the file the
// preview host already authorized.
type xlsxSidecar struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[string]chan sidecarResponse
	done    chan struct{}
}

type sidecarResponse struct {
	RequestID string          `json:"requestId"`
	OK        bool            `json:"ok"`
	Result    json.RawMessage `json:"result"`
	Error     *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func writeSidecar(root string) (string, error) {
	file := filepath.Join(root, "2a607bd49bc3", "xlsx-sidecar.exe")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return "", err
	}
	data, err := runtimeFiles.ReadFile("runtime/xlsx-sidecar.exe")
	if err != nil {
		return "", fmt.Errorf("表格引擎还没有准备好")
	}
	current, readErr := os.ReadFile(file)
	if readErr != nil || sha256.Sum256(current) != sha256.Sum256(data) {
		if err = os.WriteFile(file, data, 0o700); err != nil {
			return "", err
		}
	}
	return file, nil
}

func startSidecar(file string) (*xlsxSidecar, error) {
	cmd := exec.Command(file)
	proc.Configure(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err = proc.Start(cmd); err != nil {
		_ = in.Close()
		return nil, fmt.Errorf("表格引擎还没有准备好")
	}
	s := &xlsxSidecar{cmd: cmd, in: in, pending: map[string]chan sidecarResponse{}, done: make(chan struct{})}
	go func() {
		reader := bufio.NewScanner(out)
		reader.Buffer(make([]byte, 64<<10), 96<<20)
		for reader.Scan() {
			var response sidecarResponse
			if json.Unmarshal(reader.Bytes(), &response) != nil || response.RequestID == "" {
				continue
			}
			s.mu.Lock()
			ch := s.pending[response.RequestID]
			delete(s.pending, response.RequestID)
			s.mu.Unlock()
			if ch != nil {
				ch <- response
			}
		}
		_ = cmd.Wait()
		s.mu.Lock()
		for id, ch := range s.pending {
			ch <- sidecarResponse{RequestID: id, Error: &struct {
				Message string `json:"message"`
			}{Message: "表格引擎已退出"}}
			delete(s.pending, id)
		}
		s.mu.Unlock()
		close(s.done)
	}()
	return s, nil
}

func (s *xlsxSidecar) request(ctx context.Context, body map[string]any) (json.RawMessage, error) {
	if s == nil {
		return nil, fmt.Errorf("表格引擎还没有准备好")
	}
	id, _ := body["requestId"].(string)
	if id == "" {
		return nil, fmt.Errorf("表格请求不完整")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	ch := make(chan sidecarResponse, 1)
	s.mu.Lock()
	select {
	case <-s.done:
		s.mu.Unlock()
		return nil, fmt.Errorf("表格引擎已退出")
	default:
	}
	s.pending[id] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.pending, id); s.mu.Unlock() }()
	s.writeMu.Lock()
	_, err = s.in.Write(append(raw, '\n'))
	s.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("表格引擎连接已断开")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, fmt.Errorf("表格引擎已退出")
	case response := <-ch:
		if !response.OK {
			message := "表格没有打开"
			if response.Error != nil && response.Error.Message != "" {
				message = response.Error.Message
			}
			if len(message) > 300 {
				message = message[:300]
			}
			return nil, fmt.Errorf("%s", message)
		}
		if len(response.Result) == 0 {
			return []byte("{}"), nil
		}
		return response.Result, nil
	}
}

func (s *xlsxSidecar) close() {
	if s == nil {
		return
	}
	_ = s.in.Close()
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		_ = proc.Kill(s.cmd)
		<-s.done
	}
}

func (m *Manager) sheetsClient() (*xlsxSidecar, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, fmt.Errorf("办公服务已经关闭")
	}
	current := m.sheets
	m.mu.Unlock()
	if current != nil {
		select {
		case <-current.done:
		default:
			return current, nil
		}
	}
	file, err := writeSidecar(m.cfg.Dir)
	if err != nil {
		return nil, err
	}
	next, err := startSidecar(file)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		next.close()
		return nil, fmt.Errorf("办公服务已经关闭")
	}
	if m.sheets != nil {
		select {
		case <-m.sheets.done:
		default:
			m.mu.Unlock()
			next.close()
			return m.sheets, nil
		}
	}
	m.sheets = next
	m.mu.Unlock()
	return next, nil
}

func (m *Manager) serveSheets(w http.ResponseWriter, r *http.Request, base string) {
	d, status, message := m.fileDocument(r, base, true)
	if d == nil {
		rejectFile(w, status, message)
		return
	}
	if d.page != "/sheets.html" {
		rejectFile(w, 404, "当前页面不支持这个文件")
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		rejectFile(w, 403, "来源不被接受")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var body map[string]any
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		rejectFile(w, 400, "表格请求不完整")
		return
	}
	command, _ := body["command"].(string)
	client, err := m.sheetsClient()
	if err != nil {
		rejectFile(w, 409, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	switch command {
	case "open":
		m.openSheet(ctx, w, d, client)
	case "read_range", "read_formula_cells", "read_row_outline", "read_media", "close":
		m.sheetCommand(ctx, w, d, client, command, body)
	default:
		rejectFile(w, 400, "表格请求不完整")
	}
}

func (m *Manager) openSheet(ctx context.Context, w http.ResponseWriter, d *document, client *xlsxSidecar) {
	digest, size, err := fileDigest(d.path)
	if err != nil {
		rejectFile(w, 409, "文件读取失败")
		return
	}
	raw, err := client.request(ctx, map[string]any{
		"version": 1, "requestId": secret(), "command": "open", "path": d.path, "locale": "zh",
	})
	if err != nil {
		rejectFile(w, 409, err.Error())
		return
	}
	var opened map[string]any
	if json.Unmarshal(raw, &opened) != nil {
		rejectFile(w, 409, "表格没有打开")
		return
	}
	session, _ := opened["sessionId"].(string)
	if session == "" {
		rejectFile(w, 409, "表格没有打开")
		return
	}
	opened["path"] = d.path
	opened["sha256"] = digest
	opened["fileBytes"] = size
	opened["readOnly"] = false
	opened["needsSaveAs"] = false
	opened["restoredFromRecovery"] = false
	opened["automaticRecoveryDisabled"] = true
	d.mu.Lock()
	previous := d.session
	d.session = session
	d.mu.Unlock()
	if previous != "" && previous != session {
		_, _ = client.request(ctx, map[string]any{"version": 1, "requestId": secret(), "command": "close", "sessionId": previous})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(opened)
}

func (m *Manager) sheetCommand(ctx context.Context, w http.ResponseWriter, d *document, client *xlsxSidecar, command string, body map[string]any) {
	d.mu.Lock()
	session := d.session
	d.mu.Unlock()
	if session == "" {
		rejectFile(w, 409, "表格没有打开")
		return
	}
	request := map[string]any{"version": 1, "requestId": secret(), "command": command, "sessionId": session}
	switch command {
	case "read_range":
		sheetID, ok := sheetID(body["sheetId"])
		if !ok || !rangeOK(body["range"]) {
			rejectFile(w, 400, "表格请求不完整")
			return
		}
		request["sheetId"] = sheetID
		request["range"] = body["range"]
	case "read_formula_cells", "read_row_outline":
		sheetID, ok := sheetID(body["sheetId"])
		if !ok {
			rejectFile(w, 400, "表格请求不完整")
			return
		}
		request["sheetId"] = sheetID
	case "read_media":
		visualID, _ := body["visualId"].(string)
		if visualID == "" || len(visualID) > 200 {
			rejectFile(w, 400, "表格请求不完整")
			return
		}
		request["visualId"] = visualID
	case "close":
		d.mu.Lock()
		d.session = ""
		d.mu.Unlock()
	}
	raw, err := client.request(ctx, request)
	if err != nil {
		rejectFile(w, 409, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
}

func sheetID(value any) (string, bool) {
	id, _ := value.(string)
	if id == "" || len(id) > 200 {
		return "", false
	}
	return id, true
}

func rangeOK(value any) bool {
	area, _ := value.(map[string]any)
	if area == nil {
		return false
	}
	nums := [4]int{}
	for i, key := range []string{"startRow", "endRow", "startColumn", "endColumn"} {
		number, ok := area[key].(float64)
		if !ok || number < 0 || number > 1048576 || number != float64(int(number)) {
			return false
		}
		nums[i] = int(number)
	}
	if nums[0] > nums[1] || nums[2] > nums[3] {
		return false
	}
	rows := nums[1] - nums[0] + 1
	columns := nums[3] - nums[2] + 1
	return rows <= 100000 && columns <= 16384 && rows*columns <= 100000
}

func fileDigest(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	sum := sha256.New()
	n, err := io.Copy(sum, f)
	if err != nil {
		return "", 0, err
	}
	if n > fileByteLimit {
		return "", 0, fmt.Errorf("文件太大")
	}
	return hex.EncodeToString(sum.Sum(nil)), n, nil
}
