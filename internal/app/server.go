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
	// httpServer 是标准库真正负责协议解析和 Handler 调用的服务器。
	httpServer *http.Server
	// shutdownTimeout 是 Context 取消后允许在途请求完成的最长时间。
	shutdownTimeout time.Duration
	// logger 记录启动和关闭生命周期事件。
	logger *slog.Logger
}

// NewServer 校验 HTTP 配置和依赖，并把所有生产超时显式写入 http.Server。
func NewServer(configuration config.HTTP, handler http.Handler, logger *slog.Logger) (*Server, error) {
	// 端口配置转换成整数只用于范围校验，Addr 仍保留字符串拼接。
	port, err := strconv.Atoi(configuration.Port)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("HTTP port must be between 1 and 65535")
	}
	// Handler 可能是接口内有类型的 nil 指针，需要反射辅助判断。
	if isNilHTTPHandler(handler) {
		return nil, fmt.Errorf("HTTP handler is required")
	}
	// 日志器缺失时无法记录服务生命周期。
	if logger == nil {
		return nil, fmt.Errorf("HTTP server logger is required")
	}
	// 所有 http.Server 超时和关闭超时都必须为正数。
	if configuration.ReadHeaderTimeout <= 0 ||
		configuration.ReadTimeout <= 0 ||
		configuration.WriteTimeout <= 0 ||
		configuration.IdleTimeout <= 0 ||
		configuration.ShutdownTimeout <= 0 {
		return nil, fmt.Errorf("HTTP server timeouts must be positive")
	}

	// 校验完成后把配置显式映射到标准库 Server 字段。
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
	// net.Listen 先绑定真实端口，端口占用或权限错误会在这里立即暴露。
	listener, err := net.Listen("tcp", server.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen HTTP on %s: %w", server.httpServer.Addr, err)
	}
	// 监听成功后复用统一生命周期函数，测试也可以传临时端口 Listener。
	return server.runWithListener(ctx, listener)
}

// runWithListener 使用调用方提供的 listener 运行服务。
// 生产由 Run 调用，测试传 127.0.0.1:0，因此不会占用固定开发端口。
func (server *Server) runWithListener(ctx context.Context, listener net.Listener) error {
	// 缓冲为 1 的通道保存 Serve 最终结果，发送方不会因主协程切换关闭分支而阻塞。
	serveErrors := make(chan error, 1)
	// Serve 会长期阻塞，因此在独立 goroutine 中运行。
	go func() {
		// 缓冲通道保证即使主协程先进入关停分支，这个 goroutine 也不会因为发送结果而泄漏。
		serveErrors <- server.httpServer.Serve(listener)
	}()

	// 监听成功后记录实际地址，测试使用 127.0.0.1:0 时也能看到系统分配端口。
	server.logger.InfoContext(ctx, "HTTP server started", "address", listener.Addr().String())
	// 同时等待服务器主动结束或进程 Context 取消。
	select {
	case serveErr := <-serveErrors:
		// Shutdown 导致的标准 ErrServerClosed 属于正常结束。
		if errors.Is(serveErr, http.ErrServerClosed) {
			return nil
		}
		// 其他 Serve 错误表示监听或连接处理异常。
		return fmt.Errorf("serve HTTP: %w", serveErr)

	case <-ctx.Done():
		// 不能直接把已经取消的父 context 传给 Shutdown，否则它会立即失败，完全没有收尾窗口。
		shutdownContext, cancel := context.WithTimeout(context.Background(), server.shutdownTimeout)
		// 函数结束时释放独立 Shutdown Context 的计时器。
		defer cancel()

		// 记录配置的最大关停等待时间，不输出请求内容。
		server.logger.Info("HTTP server shutting down", "timeout", server.shutdownTimeout)
		// Shutdown 停止接收新连接，并等待正在处理的请求完成。
		if shutdownErr := server.httpServer.Shutdown(shutdownContext); shutdownErr != nil {
			// 到达 deadline 后强制关闭连接，防止进程永久卡在不退出的 Handler。
			_ = server.httpServer.Close()
			// 等待 Serve goroutine 退出，避免函数返回后留下后台 goroutine。
			<-serveErrors
			return fmt.Errorf("shutdown HTTP server: %w", shutdownErr)
		}

		// 优雅关闭成功后仍读取 Serve 最终结果，确认没有额外异常。
		serveErr := <-serveErrors
		// ErrServerClosed 以外错误在关闭期间仍应报告。
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP during shutdown: %w", serveErr)
		}
		// Shutdown 和 Serve 都正常结束时返回 nil。
		return nil
	}
}

// isNilHTTPHandler 识别接口内部装着 nil 指针的情况，避免 Server 构造成功后请求到来才 panic。
func isNilHTTPHandler(handler http.Handler) bool {
	// 普通 nil 接口直接判定缺失。
	if handler == nil {
		return true
	}
	// 取得接口内部动态值，识别有类型的 nil 指针。
	value := reflect.ValueOf(handler)
	// 只有可为 nil 的反射种类才能调用 IsNil。
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		// 返回真实 nil 状态。
		return value.IsNil()
	default:
		// 结构体等值类型一定不是 nil。
		return false
	}
}
