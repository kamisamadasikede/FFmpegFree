package router

import (
	"FFmpegFree/backend/contollers"
	"FFmpegFree/backend/sse"
	"FFmpegFree/backend/ws"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/cors"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	r.MaxMultipartMemory = 1024 << 20 // 1 GB
	// 添加 CORS 中间件
	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"}, // 允许的前端地址
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials: true, // 是否允许发送 Cookie
	})

	// 使用中间件
	r.Use(func(c *gin.Context) {
		corsMiddleware.ServeHTTP(c.Writer, c.Request, func(w http.ResponseWriter, r *http.Request) {
			c.Next()
		})
	})
	r.Static("/public", "./public")
	// 注册路由
	api := r.Group("/api")

	// WebSocket 推流入口
	r.GET("/ws", ws.HandleWebSocket)

	api.POST("/upload", contollers.Upload)
	api.POST("/uploadSteamup", contollers.UploadSteamup)
	api.GET("/GetConvertingFiles", contollers.GetConvertingFiles)
	api.GET("/selectvideofile", contollers.Selectvideofile)
	api.GET("/getSteamFiles", contollers.GetSteamFiles)
	api.POST("/convert", contollers.Convert)
	api.POST("/steamload", contollers.Steamload)
	api.POST("/StopStream", contollers.StopStream)
	api.GET("/convertup", contollers.Convertup)
	api.GET("/download", contollers.Download)
	api.POST("/deleteUpsc", contollers.DeleteUpsc)
	api.POST("/deleteUp", contollers.DeleteUp)
	api.POST("/RemoveConvertingTask", contollers.RemoveConvertingTask)
	api.POST("/deletesteamVideo", contollers.DeletesteamVideo)
	r.GET("/api/sse", sse.SseHandler)
	api.GET("/GetStreamingFiles", contollers.GetStreamingFiles)
	return r
}

func InitRouter() {
	r := SetupRouter()
	r.Run(":19200")
}
