package httpadapter

import (
	"github.com/gin-gonic/gin"

	"github.com/luqingjia/TravelAgent/internal/platform/httpserver"
)

// 本文件是知识文档 Gin 路由表的登记位置。
// 根 Engine、中间件和 /health 由 platform/httpserver 统一创建；本包只注册业务路径。

// RegisterRoutes 在已有 Router 上注册知识文档接口。
// Handler 已通过构造器持有应用服务，路由注册不做服务定位。
func RegisterRoutes(router gin.IRoutes, handler *Handler) {
	if router == nil || handler == nil {
		return
	}
	// 所有知识文档接口共享 /api/knowledge 前缀。
	// 若传入的是 *gin.Engine 或 RouterGroup，Group 仍可用。
	if engine, ok := router.(*gin.Engine); ok {
		registerKnowledgeRoutes(engine.Group("/api/knowledge"), handler)
		return
	}
	if group, ok := router.(*gin.RouterGroup); ok {
		registerKnowledgeRoutes(group.Group("/api/knowledge"), handler)
		return
	}
	// 其他 gin.IRoutes 实现：直接挂完整路径。
	router.POST("/api/knowledge/bases/:kbID/documents/upload", handler.upload)
	router.POST("/api/knowledge/documents/:docID/chunk", handler.processDocument)
	router.GET("/api/knowledge/documents/:docID", handler.getDocument)
	router.GET("/api/knowledge/documents/:docID/status", handler.getDocument)
	router.GET("/api/knowledge/bases/:kbID/documents", handler.listDocuments)
	router.DELETE("/api/knowledge/documents/:docID", handler.deleteDocument)
}

// registerKnowledgeRoutes 在 /api/knowledge 分组下登记六条兼容路径。
func registerKnowledgeRoutes(knowledge gin.IRoutes, handler *Handler) {
	knowledge.POST("/bases/:kbID/documents/upload", handler.upload)
	knowledge.POST("/documents/:docID/chunk", handler.processDocument)
	knowledge.GET("/documents/:docID", handler.getDocument)
	knowledge.GET("/documents/:docID/status", handler.getDocument)
	knowledge.GET("/bases/:kbID/documents", handler.listDocuments)
	knowledge.DELETE("/documents/:docID", handler.deleteDocument)
}

// NewRouter 保留兼容包装：创建根 Router 后只注册知识路由。
// 新代码应优先使用 platform/httpserver.NewRouter + RegisterRoutes。
func NewRouter(handler *Handler, middleware httpserver.Middleware) *gin.Engine {
	router := httpserver.NewRouter(middleware)
	RegisterRoutes(router, handler)
	return router
}
