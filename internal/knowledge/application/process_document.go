package application

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// EmbeddingDimensions 是当前数据库 vector(1536) 与模型响应共同遵守的固定维度。
// 修改它不能只改这一处，还必须同步数据库列、迁移脚本、Embedding 配置和相关测试。
const EmbeddingDimensions = 1536

// htmlTagPattern 是 Go MVP 的轻量 HTML 文本提取规则。
// 它只用于现有简单场景，不试图替代完整 HTML 解析器；复杂格式应在未来通过明确适配器扩展。
var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

// ProcessDocument 显式执行一个文档的读取、解析、分块、向量化和原子替换。
//
// 整个流程先在事务外完成所有慢操作和完整性校验，只有结果齐全后才调用仓储替换事务。
// 任何中途失败都会尽力把文档改为 failed，但失败回写错误不会覆盖最初的处理错误。
func (s *Service) ProcessDocument(ctx context.Context, docID string, options domain.ChunkOptions) (domain.Document, error) {
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return domain.Document{}, fmt.Errorf("%w: document id is empty", domain.ErrInvalidArgument)
	}

	// 在修改数据库状态前先验证选项。非法请求不应该把原本可处理的文档留在 processing 状态。
	normalizedOptions, err := options.Normalize()
	if err != nil {
		return domain.Document{}, err
	}

	// TryMarkProcessing 必须由 PostgreSQL 使用条件 UPDATE 原子实现。
	// acquired=false 表示另一个请求已经占有处理权，本请求不能继续读取对象或调用模型。
	document, acquired, err := s.repo.TryMarkProcessing(ctx, docID)
	if err != nil {
		return domain.Document{}, fmt.Errorf("acquire document processing: %w", err)
	}
	if !acquired {
		return domain.Document{}, domain.ErrAlreadyRunning
	}

	completed, err := s.processAfterAcquire(ctx, document, normalizedOptions)
	if err != nil {
		s.recordProcessingFailure(ctx, document, err)
		return domain.Document{}, err
	}
	return completed, nil
}

// processAfterAcquire 执行已经取得处理权之后的主流程。
// 单独拆出这个函数可以清楚地区分“并发抢占”与“准备并提交结果”两个职责。
func (s *Service) processAfterAcquire(ctx context.Context, document domain.Document, options domain.ChunkOptions) (domain.Document, error) {
	// 第一步从对象存储取回原文件。数据库事务此时尚未开始，网络等待不会长期占用数据库锁。
	content, err := s.storage.Get(ctx, document.SourceURI)
	if err != nil {
		return domain.Document{}, fmt.Errorf("read stored document: %w", err)
	}

	// 第二步根据文件类型提取纯文本。当前 MVP 对 pdf/doc/docx 明确返回“不支持”，不会假装解析成功。
	text, err := extractText(content, document.FileType, document.FileName)
	if err != nil {
		return domain.Document{}, fmt.Errorf("extract document text: %w", err)
	}

	// 第三步执行确定性的内存分块。纯空白内容会得到空结果，必须在调用模型前失败。
	chunks, err := ChunkText(text, options)
	if err != nil {
		return domain.Document{}, fmt.Errorf("chunk document text: %w", err)
	}
	if len(chunks) == 0 {
		return domain.Document{}, fmt.Errorf("parsed chunks are empty")
	}

	// 算法只生成内容和位置；应用用例在这里补齐持久化所需的 ID 与归属信息。
	texts := make([]string, len(chunks))
	for index := range chunks {
		chunks[index].ID = s.newID()
		chunks[index].KbID = document.KbID
		chunks[index].DocumentID = document.ID
		// 当前 MVP 没有 tokenizer，沿用旧行为，用 Unicode 字符数作为近似 token 数。
		chunks[index].TokenCount = chunks[index].CharCount
		if err := chunks[index].Validate(); err != nil {
			return domain.Document{}, fmt.Errorf("validate chunk %d: %w", index, err)
		}
		texts[index] = chunks[index].Content
	}

	// 第四步批量调用 Embedding。批量顺序必须与 chunks 顺序一致，后面才能按相同下标配对写库。
	vectors, err := s.embedder.EmbedTexts(ctx, texts)
	if err != nil {
		return domain.Document{}, fmt.Errorf("embed document chunks: %w", err)
	}
	if len(vectors) != len(chunks) {
		return domain.Document{}, fmt.Errorf("embedding count %d does not match chunk count %d", len(vectors), len(chunks))
	}
	for index, vector := range vectors {
		if len(vector) != EmbeddingDimensions {
			return domain.Document{}, fmt.Errorf("embedding %d dimensions = %d, want %d", index, len(vector), EmbeddingDimensions)
		}
	}

	// 所有外部结果都完整后，先由聚合生成合法 completed 状态。
	completed, err := document.MarkCompleted(len(chunks), options.AsMap(), s.now())
	if err != nil {
		return domain.Document{}, fmt.Errorf("complete document aggregate: %w", err)
	}

	// 最后才进入仓储事务：旧 chunk/vector 删除、新数据写入和文档完成状态必须一起提交或一起回滚。
	if err := s.repo.ReplaceDocumentChunks(ctx, completed, chunks, vectors); err != nil {
		return domain.Document{}, fmt.Errorf("replace document chunks: %w", err)
	}
	return completed, nil
}

// recordProcessingFailure 尽力保存最近一次处理错误，同时坚持“原始错误优先”。
func (s *Service) recordProcessingFailure(ctx context.Context, document domain.Document, cause error) {
	// 状态变化仍由领域聚合完成，仓储只负责持久化最终结果，不能在 SQL 中重新拼业务规则。
	failed, transitionErr := document.MarkFailed(cause.Error(), s.now())
	if transitionErr != nil {
		s.logger.ErrorContext(ctx, "build failed document status",
			"document_id", document.ID,
			"processing_error", cause,
			"transition_error", transitionErr,
		)
		return
	}

	if persistErr := s.repo.MarkFailed(ctx, failed); persistErr != nil {
		s.logger.ErrorContext(ctx, "persist failed document status",
			"document_id", document.ID,
			"processing_error", cause,
			"persist_error", persistErr,
		)
	}
}

// extractText 把当前 MVP 支持的文件内容转换为纯文本。
// 不支持的 Office/PDF 类型返回明确错误，调用链会把该错误写入 metadata.lastError。
func extractText(content []byte, fileType string, fileName string) (string, error) {
	extension := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(fileType), "."))
	if extension == "" {
		extension = strings.ToLower(strings.TrimPrefix(filepath.Ext(fileName), "."))
	}

	switch extension {
	case "txt", "md", "markdown":
		return string(content), nil
	case "html", "htm":
		return stripHTML(string(content)), nil
	case "pdf", "doc", "docx":
		return "", fmt.Errorf("%s parsing is not implemented in Go MVP", extension)
	default:
		// 保留旧 MVP 的兼容行为：未知但已由上传策略允许的文本类型按原始 UTF-8 内容处理。
		return string(content), nil
	}
}

// stripHTML 去掉简单标签并把连续空白折叠成一个空格。
func stripHTML(input string) string {
	withoutTags := htmlTagPattern.ReplaceAllString(input, " ")
	return strings.Join(strings.Fields(withoutTags), " ")
}
