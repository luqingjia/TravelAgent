package httpadapter

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/luqingjia/TravelAgent/internal/platform/httpserver"
)

// 本文件是 Gin 路由表的唯一登记位置。新增或修改 API 时必须在这里集中审查，
// Handler 只处理请求，application 只处理业务，二者都不负责拼接 URL。
// NewRouter 创建只包含已批准路由和通用中间件的 Gin Engine。
// Handler 已经通过构造器持有应用服务，路由注册不进行任何服务定位或全局变量读取。
func NewRouter(handler *Handler, middleware httpserver.Middleware) *gin.Engine {
	// 服务不使用 Gin 自带的控制台调试日志，访问记录统一由 slog 中间件输出。
	gin.SetMode(gin.ReleaseMode)
	// gin.New 只创建干净 Engine，不自动安装 Gin 默认日志和 Recovery，避免重复日志格式。
	router := gin.New()
	// 请求 ID 必须最先建立；访问日志包住后续处理；Recovery 在最内层捕获 Handler panic。
	router.Use(middleware.RequestID(), middleware.AccessLog(), middleware.Recovery())

	// 健康检查只证明 HTTP 进程可响应，不执行数据库迁移或外部服务探测。
	router.GET("/health", func(context *gin.Context) {
		// 使用统一响应外壳，保持 code、message、data 三个字段与其他接口一致。
		context.JSON(http.StatusOK, Success(map[string]string{"status": "ok"}))
	})

	// 所有知识文档接口共享 /api/knowledge 前缀，避免每条路由重复写公共部分。
	knowledge := router.Group("/api/knowledge")
	// 上传接口只登记 pending 文档，不在同一个请求里执行耗时向量化。
	knowledge.POST("/bases/:kbID/documents/upload", handler.upload)
	// 分块接口由调用方显式触发解析、切块和 Embedding。
	knowledge.POST("/documents/:docID/chunk", handler.processDocument)
	// 详情接口返回完整文档信息。
	knowledge.GET("/documents/:docID", handler.getDocument)
	// 状态接口沿用详情处理器，保持当前兼容响应结构。
	knowledge.GET("/documents/:docID/status", handler.getDocument)
	// 列表接口按知识库和分页参数查询未删除文档。
	knowledge.GET("/bases/:kbID/documents", handler.listDocuments)
	// 删除接口执行当前仓储定义的逻辑删除。
	knowledge.DELETE("/documents/:docID", handler.deleteDocument)

	// Gin Engine 实现 net/http.Handler，可以直接交给标准库 http.Server。
	return router
}
