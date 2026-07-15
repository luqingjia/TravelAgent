package application

import (
	"context"
	"errors"
	"testing"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// TestQueryUseCasesRejectBlankIdentifiers 验证空 ID 不会被直接传到 SQL，避免出现无意义的全表条件或难懂错误。
func TestQueryUseCasesRejectBlankIdentifiers(t *testing.T) {
	service, repo, _, _ := newProcessingService(t)

	if _, err := service.GetDocument(context.Background(), " "); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("GetDocument() error = %v, want ErrInvalidArgument", err)
	}
	if _, _, err := service.ListDocuments(context.Background(), " ", 1, 20); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("ListDocuments() error = %v, want ErrInvalidArgument", err)
	}
	if err := service.DeleteDocument(context.Background(), " "); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("DeleteDocument() error = %v, want ErrInvalidArgument", err)
	}
	if len(repo.getDocIDs)+len(repo.listRequests)+len(repo.deleteDocIDs) != 0 {
		t.Fatalf("repository should not be called for blank identifiers")
	}
}

// TestListDocumentsAppliesStablePaginationDefaults 验证未提供分页参数时沿用旧接口的 page=1、size=20。
func TestListDocumentsAppliesStablePaginationDefaults(t *testing.T) {
	service, repo, _, _ := newProcessingService(t)
	repo.listDocuments = []domain.Document{processingDocumentFixture()}
	repo.listTotal = 1

	documents, total, err := service.ListDocuments(context.Background(), "kb-1", 0, 0)
	if err != nil {
		t.Fatalf("ListDocuments() error = %v", err)
	}
	if len(documents) != 1 || total != 1 {
		t.Fatalf("documents = %d, total = %d", len(documents), total)
	}
	if len(repo.listRequests) != 1 {
		t.Fatalf("ListDocuments repository calls = %d, want 1", len(repo.listRequests))
	}
	request := repo.listRequests[0]
	if request.kbID != "kb-1" || request.page != 1 || request.size != 20 {
		t.Fatalf("list request = %#v", request)
	}
}

// TestGetAndDeleteDocumentDelegateAfterValidation 验证查询和删除在完成入口校验后只调用一次仓储。
func TestGetAndDeleteDocumentDelegateAfterValidation(t *testing.T) {
	service, repo, _, _ := newProcessingService(t)
	repo.getDocument = processingDocumentFixture()

	document, err := service.GetDocument(context.Background(), "doc-1")
	if err != nil || document.ID != "doc-1" {
		t.Fatalf("GetDocument() = (%#v, %v)", document, err)
	}
	if err := service.DeleteDocument(context.Background(), "doc-1"); err != nil {
		t.Fatalf("DeleteDocument() error = %v", err)
	}
	if len(repo.getDocIDs) != 1 || len(repo.deleteDocIDs) != 1 {
		t.Fatalf("get calls = %d, delete calls = %d", len(repo.getDocIDs), len(repo.deleteDocIDs))
	}
}
