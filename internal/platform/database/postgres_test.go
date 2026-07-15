package database

import (
	"context"
	"strings"
	"testing"

	"github.com/luqingjia/TravelAgent/internal/platform/config"
)

// TestOpenRejectsBlankDSN 验证数据库构造器会在调用驱动前拒绝空 DSN。
// 这个测试不建立真实网络连接，只检查启动边界能否给出明确且不泄漏密码的错误。
func TestOpenRejectsBlankDSN(t *testing.T) {
	_, err := Open(context.Background(), config.Database{
		MaxOpenConnections: 10,
		MaxIdleConnections: 5,
	})
	if err == nil {
		t.Fatal("Open() 应拒绝空 POSTGRESQL_DSN")
	}
	if !strings.Contains(err.Error(), "POSTGRESQL_DSN") {
		t.Fatalf("Open() error = %q, want mention POSTGRESQL_DSN", err)
	}
}
