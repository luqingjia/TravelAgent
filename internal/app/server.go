// Package app 是 TravelAgent 的唯一组合根，负责创建具体依赖并管理进程级资源生命周期。
//
// 业务包不会反向导入这里；cmd/main 也只调用 app.Run，不再亲自组装数据库、存储或 Gin。
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"time"

	"github.com/luqingjia/TravelAgent/internal/platform/config"
)

// Server 包装标准库 http.Server，并统一实现监听、等待取消和有期限优雅关闭。
type Server struct {
	httpServer      *http.Server
	shutdownTimeout time.Duration
	logger          *slog.Logger
}

// NewServer 校验 HTTP 配置和依赖，并把所有生产超时显式写入 http.Server。
func NewServer(configuration config.HTTP, handler http.Handler, logger *slog.Logger) (*Server, error) {
	port, err := strconv.Atoi(configuration.Port)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("HTTP port must be between 1 and 65535")
	}
	if isNilHTTPHandler(handler) {
		return nil, fmt.Errorf("HTTP handler is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("HTTP server logger is required")
	}
	if configuration.ReadHeaderTimeout <= 0 ||
		configuration.ReadTimeout <= 0 ||
		configuration.WriteTimeout <= 0 ||
		configuration.IdleTimeout <= 0 ||
		configuration.ShutdownTimeout <= 0 {
		return nil, fmt.Errorf("HTTP server timeouts must be positive")
	}

	return &Server{
		httpServer: &http.Server{
			Addr:              ":" + configuration.Port,
			Handler:           handler,
			ReadHeaderTimeout: configuration.ReadHeaderTimeout,
			ReadTimeout:       configuration.ReadTimeout,
			WriteTimeout:      configuration.WriteTimeout,
			IdleTimeout:       configuration.IdleTimeout,
		},
		shutdownTimeout: configuration.ShutdownTimeout,
		logger:          logger,
	}, nil
}

// Run 在配置端口创建 TCP listener，然后进入统一生命周期循环。
func (server *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", server.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen HTTP on %s: %w", server.httpServer.Addr, err)
	}
	return server.runWithListener(ctx, listener)
}

// runWithListener 使用调用方提供的 listener 运行服务。
// 生产由 Run 调用，测试传 127.0.0.1:0，因此不会占用固定开发端口。
func (server *Server) runWithListener(ctx context.Context, listener net.Listener) error {
	serveErrors := make(chan error, 1)
	go func() {
		// 缓冲通道保证即使主协程先进入关停分支，这个 goroutine 也不会因为发送结果而泄漏。
		serveErrors <- server.httpServer.Serve(listener)
	}()

	server.logger.InfoContext(ctx, "HTTP server started", "address", listener.Addr().String())
	select {
	case serveErr := <-serveErrors:
		if errors.Is(serveErr, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", serveErr)

	case <-ctx.Done():
		// 不能直接把已经取消的父 context 传给 Shutdown，否则它会立即失败，完全没有收尾窗口。
		shutdownContext, cancel := context.WithTimeout(context.Background(), server.shutdownTimeout)
		defer cancel()

		server.logger.Info("HTTP server shutting down", "timeout", server.shutdownTimeout)
		if shutdownErr := server.httpServer.Shutdown(shutdownContext); shutdownErr != nil {
			// 到达 deadline 后强制关闭连接，防止进程永久卡在不退出的 Handler。
			_ = server.httpServer.Close()
			<-serveErrors
			return fmt.Errorf("shutdown HTTP server: %w", shutdownErr)
		}

		serveErr := <-serveErrors
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP during shutdown: %w", serveErr)
		}
		return nil
	}
}

// isNilHTTPHandler 识别接口内部装着 nil 指针的情况，避免 Server 构造成功后请求到来才 panic。
func isNilHTTPHandler(handler http.Handler) bool {
	if handler == nil {
		return true
	}
	value := reflect.ValueOf(handler)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
