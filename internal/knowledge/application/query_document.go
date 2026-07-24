package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// 本文件集中放置只需要 Repository 的读取和删除用例，避免 HTTP Handler 直接处理持久化细节。
const (
	// defaultDocumentPage 是调用方没有明确传页码时使用的第一页。
	// 把默认值放在应用层，可以保证 HTTP、命令行或以后新增的入口都遵守同一套分页规则。
	defaultDocumentPage = 1
	// defaultDocumentPageSize 沿用原有接口每页 20 条的行为，避免重构后返回数量悄悄变化。
	defaultDocumentPageSize = 20
)

// GetDocument 查询一份尚未逻辑删除的知识文档。
//
// 应用层先清理并校验文档 ID，再把真正的持久化查询交给仓储适配器。这样空白 ID
// 不会流到 SQL 层，也不会让不同 HTTP Handler 各自复制一套入口校验。
func (s *Service) GetDocument(ctx context.Context, documentID string) (domain.Document, error) {
	// TrimSpace 会去掉用户复制 ID 时可能带上的首尾空格；仓储收到的是可直接用于查询的稳定值。
	documentID = strings.TrimSpace(documentID)
	// 纯空白 ID 与空字符串一样无效，不能继续传给仓储拼接查询条件。
	if documentID == "" {
		return domain.Document{}, fmt.Errorf("%w: document id is required", domain.ErrInvalidArgument)
	}

	// 仓储负责区分“不存在”和真实数据库故障；这里用 %w 包装后，HTTP 层仍能通过 errors.Is
	// 识别 domain.ErrNotFound，同时日志也能看出错误发生在查询文档这一步。
	document, err := s.repo.GetDocument(ctx, documentID)
	// 仓储错误继续保留 %w 错误链，方便上层分类和日志定位。
	if err != nil {
		return domain.Document{}, fmt.Errorf("get document %q: %w", documentID, err)
	}

	// 查询成功后直接返回领域对象，HTTP DTO 转换由适配器负责。
	return document, nil
}

// ListDocuments 分页查询一个知识库中的有效文档，并同时返回符合条件的总数。
//
// page 和 size 小于等于零表示调用方没有提供有效分页值，此时沿用旧接口的第一页、每页
// 20 条默认值。具体 LIMIT/OFFSET 和逻辑删除条件仍由 PostgreSQL 适配器负责。
func (s *Service) ListDocuments(
	ctx context.Context,
	knowledgeBaseID string,
	page int,
	size int,
) ([]domain.Document, int64, error) {
	// 知识库 ID 是查询边界的必要条件。提前拒绝空值，避免仓储误执行缺少业务范围的查询。
	knowledgeBaseID = strings.TrimSpace(knowledgeBaseID)
	// 缺少知识库范围会让列表查询失去业务边界，因此直接返回参数错误。
	if knowledgeBaseID == "" {
		return nil, 0, fmt.Errorf("%w: knowledge base id is required", domain.ErrInvalidArgument)
	}

	// 只修正没有意义的零值和负数，正常的正数由调用方原样控制，保持现有 API 兼容。
	if page <= 0 {
		// 无效页码回退到第一页，维持旧接口的容错行为。
		page = defaultDocumentPage
	}
	if size <= 0 {
		// 无效每页数量回退到 20，不在这里强制修改合法正数。
		size = defaultDocumentPageSize
	}

	// 文档列表和总数由同一个仓储方法返回，避免应用层分别查询后产生不同筛选条件。
	documents, total, err := s.repo.ListDocuments(ctx, knowledgeBaseID, page, size)
	// 列表或计数任一步失败都视为整个查询失败，不能返回不一致的半结果。
	if err != nil {
		return nil, 0, fmt.Errorf("list documents for knowledge base %q: %w", knowledgeBaseID, err)
	}

	// 文档切片和总数来自同一仓储合同，调用方可以据此构造分页响应。
	return documents, total, nil
}

// DeleteDocument 逻辑删除指定文档及其关联数据。
//
// 应用层只负责业务入口校验和错误语义；删除文档、分块与向量时需要怎样使用事务，属于
// PostgreSQL 适配器的职责，不能把 SQL 细节泄漏到这里。
func (s *Service) DeleteDocument(ctx context.Context, documentID string) error {
	// 与查询入口保持一致，先清理用户可能附带的首尾空格。
	documentID = strings.TrimSpace(documentID)
	// 空 ID 无法定位目标文档，不应进入删除事务。
	if documentID == "" {
		return fmt.Errorf("%w: document id is required", domain.ErrInvalidArgument)
	}

	// 删除失败时保留底层错误链。上层既能识别 ErrNotFound，也能记录数据库返回的真实原因。
	if err := s.repo.DeleteDocument(ctx, documentID); err != nil {
		return fmt.Errorf("delete document %q: %w", documentID, err)
	}

	// nil 表示仓储已经完成当前约定的逻辑删除。
	return nil
}
