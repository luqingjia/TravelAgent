package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/luqingjia/TravelAgent/internal/platform/config"
)

// TestServerRunWithListenerStopsAfterContextCancellation 验证服务收到进程取消信号后停止接收请求并正常退出。
func TestServerRunWithListenerStopsAfterContextCancellation(t *testing.T) {
	// Handler 返回 204，先证明服务真的能接收请求，再测试取消后的优雅退出。
	server := newTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}), 200*time.Millisecond)
	// 端口写 0 让操作系统分配临时端口，避免和开发机上的真实服务冲突。
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	// t.Context 由测试框架控制，另建可主动取消的子 context 来模拟 SIGTERM。
	ctx, cancel := context.WithCancel(t.Context())
	// 缓冲通道接收后台 Server 的最终结果，防止测试协程退出时发送方被卡住。
	runError := make(chan error, 1)
	go func() { runError <- server.runWithListener(ctx, listener) }()

	// 真实发起一次 HTTP 请求，确认监听器已进入 Serve，而不是只启动了 goroutine。
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("GET temporary server: %v", err)
	}
	_ = response.Body.Close()
	// 模拟进程收到退出信号，Server 应停止监听并等待在途请求收尾。
	cancel()

	// 两秒只是测试保护上限；正常路径应在配置的 200ms 关停窗口内返回。
	select {
	case err := <-runError:
		if err != nil {
			t.Fatalf("Run() after cancel error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after context cancellation")
	}
}

// TestServerReturnsServeFailure 验证监听器已经关闭时，Serve 错误会被包装后返回给进程入口。
func TestServerReturnsServeFailure(t *testing.T) {
	// 使用已经关闭的监听器，稳定制造 http.Server.Serve 的启动失败。
	server := newTestServer(t, http.NotFoundHandler(), 100*time.Millisecond)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	_ = listener.Close()

	// 执行后错误必须带上“提供 HTTP 服务”这一层操作语境，便于入口日志定位。
	err = server.runWithListener(t.Context(), listener)
	if err == nil || !strings.Contains(err.Error(), "serve HTTP") {
		t.Fatalf("runWithListener() error = %v, want serve error", err)
	}
}

// TestServerReturnsShutdownTimeout 验证正在执行且不退出的 Handler 会受到 shutdown deadline 限制。
func TestServerReturnsShutdownTimeout(t *testing.T) {
	// started 表示请求已经进入 Handler；release 控制这个请求何时真正结束。
	started := make(chan struct{})
	release := make(chan struct{})
	// Handler 故意阻塞，制造优雅关停时间不够的场景。
	server := newTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		writer.WriteHeader(http.StatusOK)
	}), 20*time.Millisecond)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	// Server 和客户端请求都放到后台运行，主测试负责精确控制取消时点。
	ctx, cancel := context.WithCancel(t.Context())
	runError := make(chan error, 1)
	go func() { runError <- server.runWithListener(ctx, listener) }()
	// requestDone 保证测试结束前客户端 goroutine 已退出，不留下并发泄漏。
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		client := &http.Client{Timeout: time.Second}
		response, requestErr := client.Get("http://" + listener.Addr().String())
		if requestErr == nil {
			_ = response.Body.Close()
		}
	}()

	// 必须等请求确实进入阻塞点后再取消，否则无法覆盖“存在在途请求”的关停分支。
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking handler did not start")
	}
	// 触发优雅关停；20ms 后仍未结束的 Handler 应让 Shutdown 返回超时错误。
	cancel()

	// Server 必须把 context deadline exceeded 保留在错误链里，供进程入口识别。
	select {
	case err := <-runError:
		if err == nil || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("shutdown error = %v, want context deadline exceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not return after shutdown timeout")
	}
	// 最后放行 Handler 并等待客户端结束，避免测试把仍在运行的 goroutine 留给后续用例。
	close(release)
	<-requestDone
}

// newTestServer 使用安全默认值构造 Server，测试只覆盖生命周期，不依赖项目环境变量。
func newTestServer(t *testing.T, handler http.Handler, shutdownTimeout time.Duration) *Server {
	// 标记为帮助函数后，构造失败会把行号报告到具体测试，而不是这里。
	t.Helper()
	// 日志写入黑洞，生命周期测试只关注返回值和时序。
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// 除关停时间由场景传入外，其余 HTTP 参数使用稳定合法值。
	server, err := NewServer(config.HTTP{
		Port:              "8081",
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
		ShutdownTimeout:   shutdownTimeout,
	}, handler, logger)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return server
}
