package postgres_test

import (
	. "github.com/luqingjia/TravelAgent/internal/knowledge/adapter/postgres"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
)

// TestNewRepositoryValidatesDatabase 验证组合根不能把 nil 数据库连接注入仓储。
func TestNewRepositoryValidatesDatabase(t *testing.T) {
	if _, err := NewRepository(nil); err == nil {
		t.Fatal("NewRepository(nil) 应返回错误")
	}

	repository, err := NewRepository(&sqlx.DB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	var _ application.DocumentRepository = repository
}
