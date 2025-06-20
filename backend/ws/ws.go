package ws

import (
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"net/http"
	"os/exec"
	"sync"
	"syscall"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type StreamSession struct {
	Cmd   *exec.Cmd
	Stdin io.WriteCloser
	Done  chan struct{}
	URL   string
}

var sessions = make(map[*websocket.Conn]*StreamSession)
var mutex sync.Mutex

func HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}

	mutex.Lock()
	sessions[conn] = nil // 初始化为空
	mutex.Unlock()

	defer func() {
		mutex.Lock()
		delete(sessions, conn)
		mutex.Unlock()
		conn.Close()
	}()

	var session *StreamSession

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			log.Println("Read message error:", err)

			// 如果已有推流任务，清理掉
			if session != nil {
				session.Stop()
			}
			break
		}

		if msgType == websocket.TextMessage {
			// 处理推流地址
			var payload map[string]string
			if err := json.Unmarshal(data, &payload); err == nil {
				if url, ok := payload["url"]; ok && session == nil {
					// 创建 FFmpeg 推流命令
					cmd := exec.Command("./ffmpeg/ffmpeg",
						"-f", "matroska", "-i", "pipe:0",
						"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
						"-pix_fmt", "yuv420p",
						"-f", "flv",
						url,
					)
					cmd.SysProcAttr = &syscall.SysProcAttr{
						HideWindow: true,
					}

					stdin, _ := cmd.StdinPipe()
					done := make(chan struct{})

					session = &StreamSession{
						Cmd:   cmd,
						Stdin: stdin,
						Done:  done,
						URL:   url,
					}

					// 保存到 sessions 中
					mutex.Lock()
					sessions[conn] = session
					mutex.Unlock()

					// 启动 FFmpeg
					go func() {
						log.Printf("Starting FFmpeg push to %s", url)
						err := cmd.Run()
						if err != nil {
							log.Printf("FFmpeg exited with error: %v", err)
						}
						close(done)
					}()
				}
			}
		} else if msgType == websocket.BinaryMessage {
			if session != nil && session.Stdin != nil {
				_, err := session.Stdin.Write(data)
				if err != nil {
					log.Println("Write to FFmpeg stdin error:", err)
					session.Stop()
				}
			}
		}
	}
}
func (s *StreamSession) Stop() {
	if s == nil {
		return
	}

	log.Printf("Stopping FFmpeg push to %s", s.URL)

	// 关闭 stdin
	if s.Stdin != nil {
		s.Stdin.Close()
	}

	// 发送 SIGTERM 终止 FFmpeg 进程
	if s.Cmd != nil && s.Cmd.Process != nil {
		s.Cmd.Process.Kill()
	}

	// 等待结束
	<-s.Done
}
