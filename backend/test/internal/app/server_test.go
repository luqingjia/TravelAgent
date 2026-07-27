package app_test

import (
	. "github.com/luqingjia/TravelAgent/internal/app"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/luqingjia/TravelAgent/internal/platform/config"
)

// TestNewServerValidatesConstructionInputs 验证 HTTP Server 在启动监听前拒绝非法端口、空 Handler 和非法超时。
func TestNewServerValidatesConstructionInputs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	valid := config.HTTP{
		Port:              "8081",
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
		ShutdownTimeout:   time.Second,
	}

	tests := []struct {
		name    string
		cfg     config.HTTP
		handler http.Handler
		logger  *slog.Logger
	}{
		{name: "非法端口", cfg: func() config.HTTP { cfg := valid; cfg.Port = "0"; return cfg }(), handler: http.NotFoundHandler(), logger: logger},
		{name: "缺少 Handler", cfg: valid, handler: nil, logger: logger},
		{name: "缺少日志器", cfg: valid, handler: http.NotFoundHandler(), logger: nil},
		{name: "非法超时", cfg: func() config.HTTP { cfg := valid; cfg.ShutdownTimeout = 0; return cfg }(), handler: http.NotFoundHandler(), logger: logger},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(tt.cfg, tt.handler, tt.logger)
			if err == nil || server != nil {
				t.Fatalf("NewServer() = (%#v, %v), want construction error", server, err)
			}
		})
	}
}

// TestNewServerAcceptsValidConfiguration 验证合法配置可以构造标准 HTTP 生命周期对象。
func TestNewServerAcceptsValidConfiguration(t *testing.T) {
	server, err := NewServer(config.HTTP{
		Port:              "8081",
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
		ShutdownTimeout:   time.Second,
	}, http.NotFoundHandler(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil || server == nil {
		t.Fatalf("NewServer() = (%#v, %v), want success", server, err)
	}
}
