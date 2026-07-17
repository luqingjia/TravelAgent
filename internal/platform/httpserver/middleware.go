package httpserver

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 本文件实现所有业务路由共用的 Gin 中间件，不能在这里加入知识文档专属规则。
// Middleware 保存所有通用 HTTP 中间件共享的日志器和请求 ID 生成器。
type Middleware struct {
	// logger 输出统一结构化访问日志和 panic 日志。
	logger *slog.Logger
	// newRequestID 生产使用随机实现，测试可以注入固定值。
	newRequestID func() string
}

// NewMiddleware 创建生产中间件集合。
func NewMiddleware(logger *slog.Logger) (Middleware, error) {
	// 生产入口固定使用安全随机请求 ID 生成器。
	return newMiddleware(logger, generateRequestID)
}

// newMiddleware 允许测试注入固定 ID，从而对响应头和日志做稳定断言。
func newMiddleware(logger *slog.Logger, newRequestID func() string) (Middleware, error) {
	// 日志器缺失会让访问日志和 panic 记录无法工作，构造阶段立即失败。
	if logger == nil {
		return Middleware{}, fmt.Errorf("HTTP middleware logger is required")
	}
	// 生成器缺失时无法为没有请求头的调用建立关联 ID。
	if newRequestID == nil {
		return Middleware{}, fmt.Errorf("request ID generator is required")
	}
	// 返回值是小型值对象，可安全复制给 Gin 路由注册代码。
	return Middleware{logger: logger, newRequestID: newRequestID}, nil
}

// RequestID 返回一个为每次请求建立、保存并回传请求 ID 的 Gin 中间件。
func (middleware Middleware) RequestID() gin.HandlerFunc {
	// 返回闭包后，Gin 会为每个请求调用一次闭包内部逻辑。
	return func(context *gin.Context) {
		// 先读取上游网关或客户端提供的请求 ID，并去掉首尾空格。
		requestID := strings.TrimSpace(context.GetHeader(RequestIDHeader))
		// 缺失、超长或包含危险字符时，忽略外部值并生成新 ID。
		if !validRequestID(requestID) {
			requestID = middleware.newRequestID()
			// 测试注入或自定义生成器仍可能返回非法值，所以必须再次校验。
			if !validRequestID(requestID) {
				// 自定义生成器也必须满足相同约束；生产随机实现正常情况下不会进入这里。
				requestID = generateRequestID()
			}
		}

		// 把可信 ID 保存到 Gin Context，后续日志和 Handler 通过私有键读取。
		context.Set(requestIDContextKey, requestID)
		// 同一个 ID 回传响应头，方便调用方与服务端日志进行关联。
		context.Header(RequestIDHeader, requestID)
		// 继续执行访问日志、Recovery 和最终业务 Handler。
		context.Next()
	}
}

// AccessLog 返回一个在请求结束后记录方法、路径、状态、耗时和请求 ID 的结构化日志中间件。
func (middleware Middleware) AccessLog() gin.HandlerFunc {
	return func(context *gin.Context) {
		// 在进入后续中间件前记录开始时间，最终耗时包含完整 Handler 执行。
		startedAt := time.Now()
		// 先执行后续链，返回后才能读取最终状态码并计算完整耗时。
		context.Next()

		// 使用固定字段名输出结构化日志，便于日志平台按字段检索和统计。
		middleware.logger.InfoContext(
			context.Request.Context(),
			"HTTP request completed",
			"request_id", RequestID(context),
			"method", context.Request.Method,
			"path", context.Request.URL.Path,
			"status", context.Writer.Status(),
			"latency_ms", float64(time.Since(startedAt).Microseconds())/1000,
		)
	}
}

// Recovery 返回 Gin panic recovery 中间件。
// Gin 负责捕获 panic，本项目回调只记录结构化信息并返回空 500，绝不把 panic 文本或堆栈写给客户端。
func (middleware Middleware) Recovery() gin.HandlerFunc {
	// io.Discard 禁止 Gin 默认把 panic 堆栈直接打印到非结构化输出。
	return gin.CustomRecoveryWithWriter(io.Discard, func(context *gin.Context, recovered any) {
		// 服务端日志保留 panic 值和请求上下文，但不记录请求正文、密钥或 Authorization 头。
		middleware.logger.ErrorContext(
			context.Request.Context(),
			"HTTP handler panic recovered",
			"request_id", RequestID(context),
			"method", context.Request.Method,
			"path", context.Request.URL.Path,
			"panic", fmt.Sprint(recovered),
		)
		// 客户端只收到空 500，内部 panic 文本和堆栈不会泄漏。
		context.AbortWithStatus(http.StatusInternalServerError)
	})
}
