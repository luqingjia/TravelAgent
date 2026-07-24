package application

import (
	"context"
	"errors"
	"testing"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// TestQueryUseCasesRejectBlankIdentifiers 验证空 ID 不会被直接传到 SQL，避免出现无意义的全表条件或难懂错误。
func TestQueryUseCasesRejectBlankIdentifiers(t *testing.T) {
	// 使用带调用记录的 fake 仓储，证明参数校验发生在仓储调用之前。
	service, repo, _, _ := newProcessingService(t)

	// 单文档查询的空编号应统一归类为参数错误。
	if _, err := service.GetDocument(context.Background(), " "); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("GetDocument() error = %v, want ErrInvalidArgument", err)
	}
	// 列表查询缺少知识库编号时也不能下发 SQL。
	if _, _, err := service.ListDocuments(context.Background(), " ", 1, 20); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("ListDocuments() error = %v, want ErrInvalidArgument", err)
	}
	// 删除空编号必须被拦住，避免产生无意义或危险的删除条件。
	if err := service.DeleteDocument(context.Background(), " "); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("DeleteDocument() error = %v, want ErrInvalidArgument", err)
	}
	// 三类仓储记录之和为零，说明所有非法请求都停在应用入口。
	if len(repo.getDocIDs)+len(repo.listRequests)+len(repo.deleteDocIDs) != 0 {
		t.Fatalf("repository should not be called for blank identifiers")
	}
}

// TestListDocumentsAppliesStablePaginationDefaults 验证未提供分页参数时沿用旧接口的 page=1、size=20。
func TestListDocumentsAppliesStablePaginationDefaults(t *testing.T) {
	// fake 预设返回一条文档和总数一，便于同时检查结果透传和请求参数。
	service, repo, _, _ := newProcessingService(t)
	repo.listDocuments = []domain.Document{processingDocumentFixture()}
	repo.listTotal = 1

	// page=0、size=0 表示调用方没有提供有效分页值，应用层应补成 1 和 20。
	documents, total, err := service.ListDocuments(context.Background(), "kb-1", 0, 0)
	if err != nil {
		t.Fatalf("ListDocuments() error = %v", err)
	}
	if len(documents) != 1 || total != 1 {
		t.Fatalf("documents = %d, total = %d", len(documents), total)
	}
	// 列表用例只允许调用仓储一次，防止重复查询或重复计数。
	if len(repo.listRequests) != 1 {
		t.Fatalf("ListDocuments repository calls = %d, want 1", len(repo.listRequests))
	}
	// 读取 fake 保存的真实入参，确认默认值是在应用层补齐后再下传。
	request := repo.listRequests[0]
	if request.kbID != "kb-1" || request.page != 1 || request.size != 20 {
		t.Fatalf("list request = %#v", request)
	}
}

// TestGetAndDeleteDocumentDelegateAfterValidation 验证查询和删除在完成入口校验后只调用一次仓储。
func TestGetAndDeleteDocumentDelegateAfterValidation(t *testing.T) {
	// 仓储预设一份合法文档，查询和删除都使用非空编号通过入口校验。
	service, repo, _, _ := newProcessingService(t)
	repo.getDocument = processingDocumentFixture()

	// 查询结果应原样返回，不能在应用层意外改动领域对象。
	document, err := service.GetDocument(context.Background(), "doc-1")
	if err != nil || document.ID != "doc-1" {
		t.Fatalf("GetDocument() = (%#v, %v)", document, err)
	}
	// 删除成功只返回 nil，由仓储负责具体持久化动作。
	if err := service.DeleteDocument(context.Background(), "doc-1"); err != nil {
		t.Fatalf("DeleteDocument() error = %v", err)
	}
	// 两个用例各委托一次，证明没有重复访问基础设施。
	if len(repo.getDocIDs) != 1 || len(repo.deleteDocIDs) != 1 {
		t.Fatalf("get calls = %d, delete calls = %d", len(repo.getDocIDs), len(repo.deleteDocIDs))
	}
}
