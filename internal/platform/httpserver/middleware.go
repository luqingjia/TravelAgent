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

// Middleware 保存所有通用 HTTP 中间件共享的日志器和请求 ID 生成器。
type Middleware struct {
	logger       *slog.Logger
	newRequestID func() string
}

// NewMiddleware 创建生产中间件集合。
func NewMiddleware(logger *slog.Logger) (Middleware, error) {
	return newMiddleware(logger, generateRequestID)
}

// newMiddleware 允许测试注入固定 ID，从而对响应头和日志做稳定断言。
func newMiddleware(logger *slog.Logger, newRequestID func() string) (Middleware, error) {
	if logger == nil {
		return Middleware{}, fmt.Errorf("HTTP middleware logger is required")
	}
	if newRequestID == nil {
		return Middleware{}, fmt.Errorf("request ID generator is required")
	}
	return Middleware{logger: logger, newRequestID: newRequestID}, nil
}

// RequestID 返回一个为每次请求建立、保存并回传请求 ID 的 Gin 中间件。
func (middleware Middleware) RequestID() gin.HandlerFunc {
	return func(context *gin.Context) {
		requestID := strings.TrimSpace(context.GetHeader(RequestIDHeader))
		if !validRequestID(requestID) {
			requestID = middleware.newRequestID()
			if !validRequestID(requestID) {
				// 自定义生成器也必须满足相同约束；生产随机实现正常情况下不会进入这里。
				requestID = generateRequestID()
			}
		}

		context.Set(requestIDContextKey, requestID)
		context.Header(RequestIDHeader, requestID)
		context.Next()
	}
}

// AccessLog 返回一个在请求结束后记录方法、路径、状态、耗时和请求 ID 的结构化日志中间件。
func (middleware Middleware) AccessLog() gin.HandlerFunc {
	return func(context *gin.Context) {
		startedAt := time.Now()
		context.Next()

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
	return gin.CustomRecoveryWithWriter(io.Discard, func(context *gin.Context, recovered any) {
		middleware.logger.ErrorContext(
			context.Request.Context(),
			"HTTP handler panic recovered",
			"request_id", RequestID(context),
			"method", context.Request.Method,
			"path", context.Request.URL.Path,
			"panic", fmt.Sprint(recovered),
		)
		context.AbortWithStatus(http.StatusInternalServerError)
	})
}
