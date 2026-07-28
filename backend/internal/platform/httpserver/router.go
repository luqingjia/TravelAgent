package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// NewRouter 创建根 Gin Engine，安装通用中间件并注册 /health。
// 业务路由由 knowledge/agent 各自 RegisterRoutes 挂载，避免单一业务包拥有根 Router。
func NewRouter(middleware Middleware) *gin.Engine {
	// 服务不使用 Gin 自带控制台调试日志，访问记录统一由 slog 中间件输出。
	gin.SetMode(gin.ReleaseMode)
	// gin.New 不安装默认日志和 Recovery，避免重复格式。
	router := gin.New()
	// 中间件顺序固定：RequestID → AccessLog → Recovery。
	router.Use(middleware.RequestID(), middleware.AccessLog(), middleware.Recovery())

	// 健康检查只证明 HTTP 进程可响应，不执行数据库迁移或外部探测。
	router.GET("/health", func(context *gin.Context) {
		// 保持与历史知识接口相同的 code/message/data 外壳。
		context.JSON(http.StatusOK, healthSuccess(map[string]string{"status": "ok"}))
	})
	return router
}

// healthResult 是根路由健康检查使用的最小兼容外壳。
// 放在 platform 内避免 knowledge 与 agent 互相导入。
type healthResult struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// healthSuccess 构造成功健康检查响应。
func healthSuccess(data any) healthResult {
	return healthResult{Code: "0", Message: "", Data: data}
}
