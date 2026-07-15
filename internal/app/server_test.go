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
	server := newTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}), 200*time.Millisecond)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	// t.Context 由测试框架控制，另建可主动取消的子 context 来模拟 SIGTERM。
	ctx, cancel := context.WithCancel(t.Context())
	runError := make(chan error, 1)
	go func() { runError <- server.runWithListener(ctx, listener) }()

	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("GET temporary server: %v", err)
	}
	_ = response.Body.Close()
	cancel()

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
	server := newTestServer(t, http.NotFoundHandler(), 100*time.Millisecond)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	_ = listener.Close()

	err = server.runWithListener(t.Context(), listener)
	if err == nil || !strings.Contains(err.Error(), "serve HTTP") {
		t.Fatalf("runWithListener() error = %v, want serve error", err)
	}
}

// TestServerReturnsShutdownTimeout 验证正在执行且不退出的 Handler 会受到 shutdown deadline 限制。
func TestServerReturnsShutdownTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := newTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		writer.WriteHeader(http.StatusOK)
	}), 20*time.Millisecond)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	runError := make(chan error, 1)
	go func() { runError <- server.runWithListener(ctx, listener) }()
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		client := &http.Client{Timeout: time.Second}
		response, requestErr := client.Get("http://" + listener.Addr().String())
		if requestErr == nil {
			_ = response.Body.Close()
		}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking handler did not start")
	}
	cancel()

	select {
	case err := <-runError:
		if err == nil || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("shutdown error = %v, want context deadline exceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not return after shutdown timeout")
	}
	close(release)
	<-requestDone
}

// newTestServer 使用安全默认值构造 Server，测试只覆盖生命周期，不依赖项目环境变量。
func newTestServer(t *testing.T, handler http.Handler, shutdownTimeout time.Duration) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
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
