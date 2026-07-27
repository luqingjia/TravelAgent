package postgres_test

import (
	. "github.com/luqingjia/TravelAgent/internal/knowledge/adapter/postgres"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// TestCreateDocumentRejectsInvalidDomainBeforeSQL 验证仓储写入前会先校验领域对象，坏数据不会进入 SQL 执行阶段。
func TestCreateDocumentRejectsInvalidDomainBeforeSQL(t *testing.T) {
	repository, err := NewRepository(&sqlx.DB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	err = repository.CreateDocument(t.Context(), domain.Document{ID: "", Status: domain.DocumentStatus("unknown")})
	if err == nil {
		t.Fatal("CreateDocument() 应拒绝非法领域对象")
	}
}
