package app_test

import (
	"context"
	. "github.com/luqingjia/TravelAgent/internal/app"
	"strings"
	"testing"

	"github.com/luqingjia/TravelAgent/internal/platform/config"
)

// TestNewRejectsInvalidConfigurationBeforeRuntimeAssembly 验证组合根会先做集中配置校验，配置缺失时不会继续创建外部依赖。
func TestNewRejectsInvalidConfigurationBeforeRuntimeAssembly(t *testing.T) {
	_, err := New(context.Background(), config.Config{})
	if err == nil || !strings.Contains(err.Error(), "validate application configuration") {
		t.Fatalf("New() error = %v, want validation failure", err)
	}
}

// TestAppCloseAllowsNilReceiver 验证清理路径可以安全处理 nil App，便于启动失败兜底调用。
func TestAppCloseAllowsNilReceiver(t *testing.T) {
	var runtime *App
	if err := runtime.Close(); err != nil {
		t.Fatalf("nil App Close() error = %v", err)
	}
}
