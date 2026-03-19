package ws

import (
	"FFmpegFree/backend/live"
	"FFmpegFree/backend/sse"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type streamStartPayload struct {
	Type           string   `json:"type"`
	URL            string   `json:"url"`
	ArchiveEnabled bool     `json:"archiveEnabled"`
	SegmentSeconds int      `json:"segmentSeconds"`
	RelayTargets   []string `json:"relayTargets"`
}

type StreamSession struct {
	Cmd           *exec.Cmd
	Stdin         io.WriteCloser
	Done          chan struct{}
	URL           string
	StreamID      string
	StoppedByUser atomic.Bool
}

var sessions = make(map[*websocket.Conn]*StreamSession)
var mutex sync.Mutex

func HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("upgrade websocket failed: %v", err)
		return
	}

	mutex.Lock()
	sessions[conn] = nil
	mutex.Unlock()

	defer func() {
		mutex.Lock()
		current := sessions[conn]
		delete(sessions, conn)
		mutex.Unlock()

		if current != nil {
			current.Stop()
		}
		_ = conn.Close()
	}()

	var session *StreamSession
	for {
		msgType, data, readErr := conn.ReadMessage()
		if readErr != nil {
			if session != nil {
				session.Stop()
			}
			log.Printf("read websocket message failed: %v", readErr)
			return
		}

		switch msgType {
		case websocket.TextMessage:
			// 首个文本消息作为“会话启动指令”，包含主推流地址与高级参数。
			if session != nil {
				continue
			}
			var payload streamStartPayload
			if err := json.Unmarshal(data, &payload); err != nil {
				log.Printf("parse websocket payload failed: %v", err)
				continue
			}

			created, err := startSession(payload)
			if err != nil {
				log.Printf("start websocket stream failed: %v", err)
				continue
			}

			session = created
			mutex.Lock()
			sessions[conn] = session
			mutex.Unlock()
		case websocket.BinaryMessage:
			// 后续二进制消息是浏览器录屏分片，直接写入 ffmpeg stdin。
			if session == nil || session.Stdin == nil {
				continue
			}
			if _, err := session.Stdin.Write(data); err != nil {
				log.Printf("write websocket media payload failed: %v", err)
				session.Stop()
				continue
			}
			live.Global.AddIngressBytes(session.StreamID, len(data))
		}
	}
}

func startSession(payload streamStartPayload) (*StreamSession, error) {
	streamURL := strings.TrimSpace(payload.URL)
	if streamURL == "" {
		return nil, fmt.Errorf("stream url is required")
	}

	snapshot, err := live.Global.Start(live.StartOptions{
		DisplayName:    "screen-capture",
		Input:          "websocket-capture",
		PrimaryTarget:  streamURL,
		RelayTargets:   payload.RelayTargets,
		ArchiveEnabled: payload.ArchiveEnabled,
		SegmentSeconds: payload.SegmentSeconds,
		Source:         live.StreamSourceScreen,
	})
	if err != nil {
		return nil, fmt.Errorf("prepare live session: %w", err)
	}

	if err := live.EnsureArchiveDir(snapshot.ArchiveDir); err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		return nil, fmt.Errorf("prepare archive directory: %w", err)
	}

	teeOutput, err := live.BuildTeeOutput(snapshot.Targets, snapshot.ArchiveDir, snapshot.SegmentSeconds)
	if err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		return nil, fmt.Errorf("build stream outputs: %w", err)
	}

	args := []string{
		"-f", "matroska",
		"-i", "pipe:0",
		"-map", "0:v:0",
		"-map", "0:a?",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-g", "30",
		"-keyint_min", "30",
		"-sc_threshold", "0",
		"-threads", "0",
		"-c:a", "aac",
		"-b:a", "128k",
		"-progress", "pipe:2",
		"-nostats",
		"-f", "tee",
		teeOutput,
	}
	cmd := exec.Command(live.FFmpegBinaryPath(), args...)
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		return nil, fmt.Errorf("create ffmpeg stdin pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		return nil, fmt.Errorf("create ffmpeg progress pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	live.Global.MarkRunning(snapshot.StreamID)

	session := &StreamSession{
		Cmd:      cmd,
		Stdin:    stdin,
		Done:     make(chan struct{}),
		URL:      streamURL,
		StreamID: snapshot.StreamID,
	}

	go func(streamID string, reader io.Reader) {
		// 解析 ffmpeg 进度流并持续刷新健康指标。
		if progressErr := live.ConsumeProgress(reader, func(key, value string) {
			live.Global.UpdateProgress(streamID, key, value)
		}); progressErr != nil && progressErr != io.EOF {
			live.Global.TouchFailure(streamID, progressErr)
		}
	}(snapshot.StreamID, stderr)

	go func(current *StreamSession) {
		// 进程退出后统一落库终态并广播 SSE，前端可即时感知失败/结束。
		waitErr := current.Cmd.Wait()
		stoppedByUser := current.StoppedByUser.Load()
		live.Global.MarkFinished(current.StreamID, stoppedByUser, waitErr)

		status := "completed"
		errorMsg := fmt.Sprintf("stream completed: %s", current.URL)
		if stoppedByUser {
			status = "stopped"
			errorMsg = fmt.Sprintf("stream stopped by user: %s", current.URL)
		} else if waitErr != nil {
			status = "failed"
			errorMsg = fmt.Sprintf("stream exited with error: %s, err: %v", current.URL, waitErr)
		}

		eventData := map[string]interface{}{
			"streamId":  current.StreamID,
			"filename":  "",
			"streamUrl": current.URL,
			"status":    status,
			"error":     errorMsg,
		}
		jsonData, _ := json.Marshal(eventData)
		sse.BroadcastMessage(string(jsonData))

		close(current.Done)
	}(session)

	return session, nil
}

func (s *StreamSession) Stop() {
	if s == nil {
		return
	}

	s.StoppedByUser.Store(true)
	if s.Stdin != nil {
		_ = s.Stdin.Close()
	}
	if s.Cmd != nil && s.Cmd.Process != nil {
		_ = s.Cmd.Process.Kill()
	}

	select {
	case <-s.Done:
	case <-time.After(3 * time.Second):
	}
}
