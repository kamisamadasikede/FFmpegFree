package router

import (
	"FFmpegFree/backend/contollers"
	"github.com/gin-gonic/gin"
	"github.com/rs/cors"
	"net/http"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()
	r.Static("/public", "./public")
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

	// 注册路由
	api := r.Group("/api")

	api.POST("/upload", contollers.Upload)
	api.GET("/GetConvertingFiles", contollers.GetConvertingFiles)
	api.GET("/selectvideofile", contollers.Selectvideofile)
	api.POST("/convert", contollers.Convert)
	api.GET("/convertup", contollers.Convertup)
	api.GET("/download", contollers.Download)
	api.POST("/deleteUpsc", contollers.DeleteUpsc)
	api.POST("/deleteUp", contollers.DeleteUp)
	return r
}

func InitRouter() {
	r := SetupRouter()
	r.Run(":8000")
}
