package postgres

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

// TestNewRepositoryValidatesDatabase 验证组合根不能把 nil 数据库连接注入仓储。
func TestNewRepositoryValidatesDatabase(t *testing.T) {
	// 场景一：缺少数据库时必须在启动组装阶段明确失败，不能等到第一个请求才 panic。
	if _, err := NewRepository(nil); err == nil {
		t.Fatal("NewRepository(nil) 应返回错误")
	}

	// 场景二：提供非 nil 连接对象后应成功保存依赖。本测试只验证构造，不发起真实数据库访问。
	database := &sqlx.DB{}
	repository, err := NewRepository(database)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	if repository == nil || repository.db != database {
		t.Fatalf("NewRepository() 没有保存传入的数据库依赖")
	}
}

// TestIsUniqueViolationRecognizesPostgres23505 验证并发重复最终由数据库唯一索引兜底时，
// pgx 的 23505 技术错误会被识别为稳定的重复业务类别。
func TestIsUniqueViolationRecognizesPostgres23505(t *testing.T) {
	duplicate := &pgconn.PgError{Code: "23505"}
	if !isUniqueViolation(fmt.Errorf("insert document: %w", duplicate)) {
		t.Fatal("isUniqueViolation() 应识别错误链中的 PostgreSQL 23505")
	}

	if isUniqueViolation(&pgconn.PgError{Code: "23503"}) {
		t.Fatal("isUniqueViolation() 不应把外键错误 23503 当成重复")
	}
	if isUniqueViolation(nil) {
		t.Fatal("isUniqueViolation(nil) 应返回 false")
	}
}
