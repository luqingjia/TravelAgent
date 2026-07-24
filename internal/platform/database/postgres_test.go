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
	// 连接池数量合法但 DSN 缺失，确保失败只由连接地址为空触发。
	_, err := Open(context.Background(), config.Database{
		MaxOpenConnections: 10,
		MaxIdleConnections: 5,
	})
	// 构造器必须在调用 PostgreSQL 驱动前返回错误，因此测试不会发出网络连接。
	if err == nil {
		t.Fatal("Open() 应拒绝空 POSTGRESQL_DSN")
	}
	// 错误只提示变量名，不回显可能含账号密码的完整 DSN。
	if !strings.Contains(err.Error(), "POSTGRESQL_DSN") {
		t.Fatalf("Open() error = %q, want mention POSTGRESQL_DSN", err)
	}
}
