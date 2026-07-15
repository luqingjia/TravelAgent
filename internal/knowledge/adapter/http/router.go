package httpadapter

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/luqingjia/TravelAgent/internal/platform/httpserver"
)

// NewRouter 创建只包含已批准路由和通用中间件的 Gin Engine。
// Handler 已经通过构造器持有应用服务，路由注册不进行任何服务定位或全局变量读取。
func NewRouter(handler *Handler, middleware httpserver.Middleware) *gin.Engine {
	// 服务不使用 Gin 自带的控制台调试日志，访问记录统一由 slog 中间件输出。
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	// 请求 ID 必须最先建立；访问日志包住后续处理；Recovery 在最内层捕获 Handler panic。
	router.Use(middleware.RequestID(), middleware.AccessLog(), middleware.Recovery())

	router.GET("/health", func(context *gin.Context) {
		context.JSON(http.StatusOK, Success(map[string]string{"status": "ok"}))
	})

	knowledge := router.Group("/api/knowledge")
	knowledge.POST("/bases/:kbID/documents/upload", handler.upload)
	knowledge.POST("/documents/:docID/chunk", handler.processDocument)
	knowledge.GET("/documents/:docID", handler.getDocument)
	knowledge.GET("/documents/:docID/status", handler.getDocument)
	knowledge.GET("/bases/:kbID/documents", handler.listDocuments)
	knowledge.DELETE("/documents/:docID", handler.deleteDocument)

	return router
}
