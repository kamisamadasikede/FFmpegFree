package sse

import (
	"github.com/gin-gonic/gin"
	"sync"
)

// upload_controller.go 添加以下代码

var clients = make(map[chan string]bool)
var clientsMutex sync.Mutex

// Register a new client
func registerClient() chan string {
	chanClient := make(chan string)
	clientsMutex.Lock()
	clients[chanClient] = true
	clientsMutex.Unlock()
	return chanClient
}

// Broadcast message to all clients
func BroadcastMessage(message string) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	for clientChan := range clients {
		select {
		case clientChan <- message:
		default:
			// 如果通道满了或客户端断开，移除该客户端
			close(clientChan)
			delete(clients, clientChan)
		}
	}
}

func SseHandler(c *gin.Context) {
	// 设置响应头为 text/event-stream
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// 创建客户端 channel
	messageChan := registerClient()

	// 确保客户端关闭后从 map 中删除
	defer func() {
		clientsMutex.Lock()
		delete(clients, messageChan)
		clientsMutex.Unlock()
	}()

	// 持续监听 channel 消息并发送给前端
	for {
		msg, ok := <-messageChan
		if !ok {
			break
		}
		_, _ = c.Writer.WriteString("data: " + msg + "\n\n")
		c.Writer.Flush()
	}
}
