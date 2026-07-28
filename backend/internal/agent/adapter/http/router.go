package httpadapter

import "github.com/gin-gonic/gin"

// RegisterRoutes 在已有 Gin Engine 上注册 Agent 路由。
// 不创建 Engine、不安装中间件，根 Router 由 platform/httpserver 统一管理。
func RegisterRoutes(router gin.IRoutes, handler *Handler) {
	if router == nil || handler == nil {
		return
	}
	router.GET("/api/agent/models", handler.listModels)
	router.POST("/api/agent/chat", handler.chat)
	router.POST("/api/agent/chat/stream", handler.chatStream)
}
