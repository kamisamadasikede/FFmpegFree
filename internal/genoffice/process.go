package genoffice

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"FFmpegFree/internal/proc"
)

//go:embed runtime/*
var runtimeFiles embed.FS

type rpcResponse struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *OfficeError    `json:"error"`
}

type OfficeError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Detail  json.RawMessage `json:"detail,omitempty"`
}

func (e *OfficeError) Error() string { return e.Message }

type engineProcess struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[uint64]chan rpcResponse
	seq     atomic.Uint64
	done    chan struct{}
}

func startEngine(node, file, recovery string) (*engineProcess, error) {
	cmd := exec.Command(node, file)
	cmd.Env = append(os.Environ(), "FFC_OFFICE_RECOVERY="+recovery)
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
	// Never copy tool inputs or credentials into application logs.
	cmd.Stderr = io.Discard
	if err = proc.Start(cmd); err != nil {
		_ = in.Close()
		return nil, err
	}
	p := &engineProcess{cmd: cmd, in: in, pending: make(map[uint64]chan rpcResponse), done: make(chan struct{})}
	go func() {
		reader := bufio.NewScanner(out)
		reader.Buffer(make([]byte, 64<<10), 96<<20)
		for reader.Scan() {
			var response rpcResponse
			if json.Unmarshal(reader.Bytes(), &response) != nil {
				continue
			}
			p.mu.Lock()
			ch := p.pending[response.ID]
			delete(p.pending, response.ID)
			p.mu.Unlock()
			if ch != nil {
				ch <- response
			}
		}
		_ = cmd.Wait()
		p.mu.Lock()
		for id, ch := range p.pending {
			ch <- rpcResponse{Error: &OfficeError{Code: "ENGINE_EXITED", Message: "办公引擎已退出，请重新打开文档"}}
			delete(p.pending, id)
		}
		p.mu.Unlock()
		close(p.done)
	}()
	return p, nil
}
func (p *engineProcess) request(ctx context.Context, request any, result any) error {
	id := p.seq.Add(1)
	raw, err := json.Marshal(map[string]any{"id": id, "request": request})
	if err != nil {
		return err
	}
	if len(raw) > 24<<20 {
		return fmt.Errorf("办公请求超过大小上限")
	}
	ch := make(chan rpcResponse, 1)
	p.mu.Lock()
	p.pending[id] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, id); p.mu.Unlock() }()
	p.writeMu.Lock()
	_, err = p.in.Write(append(raw, '\n'))
	p.writeMu.Unlock()
	if err != nil {
		return fmt.Errorf("办公引擎连接已断开")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return fmt.Errorf("办公引擎已退出，请重新打开文档")
	case response := <-ch:
		if response.Error != nil {
			return response.Error
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(response.Result, result)
	}
}
func (p *engineProcess) close() {
	if p == nil {
		return
	}
	_ = p.in.Close()
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
		_ = proc.Kill(p.cmd)
		<-p.done
	}
}
func writeRuntime(root string) (string, error) {
	file := filepath.Join(root, "2a607bd49bc3", "engine.mjs")
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return "", err
	}
	data, err := runtimeFiles.ReadFile("runtime/engine.mjs")
	if err != nil {
		return "", err
	}
	current, _ := os.ReadFile(file)
	if string(current) != string(data) {
		if err = os.WriteFile(file, data, 0600); err != nil {
			return "", err
		}
	}
	return file, nil
}
