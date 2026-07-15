package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// Repository 使用 sqlx 操作现有 rag schema，并实现应用层定义的文档仓储端口。
// 它只负责持久化语义；上传校验、状态转换和失败原因合并等业务规则仍由 application/domain 决定。
type Repository struct {
	db *sqlx.DB
}

// 这条编译期断言不会产生运行时代码。只要 Repository 少实现或写错一个应用层接口方法，
// `go test`/`go build` 就会立即失败，而不是等组合根运行后才发现依赖不兼容。
var _ application.DocumentRepository = (*Repository)(nil)

// NewRepository 创建 PostgreSQL 文档仓储。
// 数据库连接由 platform/database 建立并由 app 组合根传入，仓储内部不读取环境变量或创建全局连接。
func NewRepository(database *sqlx.DB) (*Repository, error) {
	if database == nil {
		return nil, fmt.Errorf("postgres database is required")
	}
	return &Repository{db: database}, nil
}

// KnowledgeBaseExists 判断知识库是否存在、未删除并且处于 active 状态。
func (r *Repository) KnowledgeBaseExists(ctx context.Context, knowledgeBaseID string) (bool, error) {
	var exists bool
	// SELECT EXISTS 永远返回一行布尔值，比先查询整行再判断更直接，也不会把不存在当成 SQL 错误。
	err := r.db.GetContext(ctx, &exists, `
SELECT EXISTS (
  SELECT 1 FROM rag.t_knowledge_base
  WHERE id = $1 AND deleted = 0 AND status = 'active'
)`, knowledgeBaseID)
	if err != nil {
		return false, fmt.Errorf("check knowledge base %q exists: %w", knowledgeBaseID, err)
	}
	return exists, nil
}

// ActiveDocumentHashExists 按“知识库 + 内容 SHA-256”检查有效文档是否重复。
// 这是一道提前失败的友好检查；并发请求最终仍由数据库部分唯一索引兜底。
func (r *Repository) ActiveDocumentHashExists(
	ctx context.Context,
	knowledgeBaseID string,
	contentHash string,
) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, `
SELECT EXISTS (
  SELECT 1 FROM rag.t_knowledge_document
  WHERE kb_id = $1 AND content_hash = $2 AND deleted = 0
)`, knowledgeBaseID, contentHash)
	if err != nil {
		return false, fmt.Errorf("check active document hash: %w", err)
	}
	return exists, nil
}

// CreateDocument 写入一份由领域层创建的 pending 文档。
func (r *Repository) CreateDocument(ctx context.Context, document domain.Document) error {
	// 先做模型转换和 JSON 序列化，失败时不执行任何 SQL。
	row, err := rowFromDomain(document)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, `
INSERT INTO rag.t_knowledge_document
  (id, kb_id, title, source_type, source_uri, file_name, file_type, file_size,
   content_hash, language, status, chunk_count, chunk_strategy, chunk_config, metadata,
   create_time, update_time)
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8,
   $9, $10, $11, $12, $13, $14::jsonb, $15::jsonb, $16, $17)`,
		row.ID,
		row.KbID,
		row.Title,
		row.SourceType,
		row.SourceURI,
		row.FileName,
		row.FileType,
		row.FileSize,
		row.ContentHash,
		row.Language,
		row.Status,
		row.ChunkCount,
		row.ChunkStrategy,
		string(row.ChunkConfig),
		string(row.Metadata),
		row.CreateTime,
		row.UpdateTime,
	)
	if isUniqueViolation(err) {
		// PostgreSQL 23505 可能来自两个并发上传同时通过了提前重复检查。
		// 映射成稳定业务错误后，应用层会删除刚上传的对象并返回重复提示。
		return fmt.Errorf("%w: knowledge base %q already contains content hash %q", domain.ErrDuplicate, row.KbID, row.ContentHash)
	}
	if err != nil {
		return fmt.Errorf("insert document %q: %w", row.ID, err)
	}
	return nil
}

// GetDocument 查询一份未逻辑删除的文档，并把数据库行恢复为经过校验的领域对象。
func (r *Repository) GetDocument(ctx context.Context, documentID string) (domain.Document, error) {
	var row documentRow
	err := r.db.GetContext(ctx, &row, documentSelectSQL+`
WHERE id = $1 AND deleted = 0`, documentID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Document{}, fmt.Errorf("%w: document %q", domain.ErrNotFound, documentID)
	}
	if err != nil {
		return domain.Document{}, fmt.Errorf("select document %q: %w", documentID, err)
	}

	document, err := row.toDomain()
	if err != nil {
		return domain.Document{}, fmt.Errorf("convert document %q: %w", documentID, err)
	}
	return document, nil
}

// ListDocuments 分页查询一个知识库中未逻辑删除的文档，并返回同一过滤条件下的总数。
func (r *Repository) ListDocuments(
	ctx context.Context,
	knowledgeBaseID string,
	page int,
	size int,
) ([]domain.Document, int64, error) {
	// application 已保证 page/size 为正数；这里按从 0 开始的 SQL OFFSET 换算。
	offset := (page - 1) * size

	var total int64
	if err := r.db.GetContext(ctx, &total, `
SELECT COUNT(1)
FROM rag.t_knowledge_document
WHERE kb_id = $1 AND deleted = 0`, knowledgeBaseID); err != nil {
		return nil, 0, fmt.Errorf("count documents for knowledge base %q: %w", knowledgeBaseID, err)
	}

	var rows []documentRow
	if err := r.db.SelectContext(ctx, &rows, documentSelectSQL+`
WHERE kb_id = $1 AND deleted = 0
ORDER BY update_time DESC
LIMIT $2 OFFSET $3`, knowledgeBaseID, size, offset); err != nil {
		return nil, 0, fmt.Errorf("list documents for knowledge base %q: %w", knowledgeBaseID, err)
	}

	// 预先按结果行数分配切片，再逐行做显式模型转换；任一脏数据都会终止本次响应。
	documents := make([]domain.Document, len(rows))
	for index, row := range rows {
		document, err := row.toDomain()
		if err != nil {
			return nil, 0, fmt.Errorf("convert listed document at index %d: %w", index, err)
		}
		documents[index] = document
	}
	return documents, total, nil
}

// DeleteDocument 对文档执行逻辑删除，保持现有 schema 和历史查询语义兼容。
func (r *Repository) DeleteDocument(ctx context.Context, documentID string) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE rag.t_knowledge_document
SET deleted = 1, update_time = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted = 0`, documentID)
	if err != nil {
		return fmt.Errorf("logically delete document %q: %w", documentID, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted document row count: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: document %q", domain.ErrNotFound, documentID)
	}
	return nil
}

// TryMarkProcessing 使用一条带条件的 UPDATE 原子取得文档处理权。
// acquired=false 表示文档存在但已经是 processing；不存在则返回 ErrNotFound。
func (r *Repository) TryMarkProcessing(
	ctx context.Context,
	documentID string,
) (domain.Document, bool, error) {
	var row documentRow
	// 条件更新和 RETURNING 在数据库内作为一个原子操作执行，两个并发请求不可能同时成功取得处理权。
	err := r.db.GetContext(ctx, &row, `
UPDATE rag.t_knowledge_document
SET status = 'processing', update_time = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted = 0 AND status <> 'processing'
RETURNING id, kb_id, title, source_type, source_uri, file_name, file_type, file_size,
  content_hash, language, status, chunk_count, chunk_strategy, chunk_config, metadata, create_time, update_time`, documentID)
	if errors.Is(err, sql.ErrNoRows) {
		// 零行可能有两种含义：文档不存在，或它已被另一个请求置为 processing。
		// 再读一次当前文档，既能区分两种情况，也能把现有聚合返回给应用层。
		document, getErr := r.GetDocument(ctx, documentID)
		if getErr != nil {
			return domain.Document{}, false, getErr
		}
		return document, false, nil
	}
	if err != nil {
		return domain.Document{}, false, fmt.Errorf("mark document %q processing: %w", documentID, err)
	}

	document, err := row.toDomain()
	if err != nil {
		return domain.Document{}, false, fmt.Errorf("convert processing document %q: %w", documentID, err)
	}
	return document, true, nil
}

// preparedChunkWrite 保存进入替换事务前已经完成的序列化结果。
// 事务内只做数据库语句，不再执行可能失败的 JSON/向量格式转换，从而尽量缩短持锁时间。
type preparedChunkWrite struct {
	chunk              domain.Chunk
	chunkMetadataJSON  string
	vectorMetadataJSON string
	vectorLiteral      string
}

// ReplaceDocumentChunks 在一个短事务内原子替换文档的所有分块、向量和完成状态。
// 任一步失败都会回滚，所以调用方只会看到“全部换新成功”或“旧数据完整保留”两种结果。
func (r *Repository) ReplaceDocumentChunks(
	ctx context.Context,
	document domain.Document,
	chunks []domain.Chunk,
	vectors [][]float32,
) (returnErr error) {
	// 数量不一致时下标配对不再可信，必须在开启事务前拒绝。
	if len(chunks) != len(vectors) {
		return fmt.Errorf("chunk count %d does not match vector count %d", len(chunks), len(vectors))
	}
	if document.Status != domain.StatusCompleted {
		return fmt.Errorf("%w: replacement document status is %q", domain.ErrInvalidArgument, document.Status)
	}
	if document.ChunkCount != len(chunks) {
		return fmt.Errorf("%w: document chunk count %d does not match prepared chunks %d", domain.ErrInvalidArgument, document.ChunkCount, len(chunks))
	}

	// 文档 JSON 也先在事务外完成；row 里的 Metadata/ChunkConfig 会用于最后一次 UPDATE。
	row, err := rowFromDomain(document)
	if err != nil {
		return err
	}
	prepared, err := prepareChunkWrites(document, chunks, vectors)
	if err != nil {
		return err
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin document replacement transaction: %w", err)
	}
	committed := false
	// 无论下面从哪一个 return 提前退出，只要没有成功提交就执行回滚。
	// 这比依赖某个可能被 := 遮蔽的 err 变量可靠，也正是事务代码最容易踩坑的地方。
	defer func() {
		if committed {
			return
		}
		rollbackErr := tx.Rollback()
		if rollbackErr == nil || errors.Is(rollbackErr, sql.ErrTxDone) {
			return
		}
		if returnErr == nil {
			returnErr = fmt.Errorf("rollback document replacement: %w", rollbackErr)
			return
		}
		// 保留原始失败为 %w，回滚错误作为额外上下文，不能把真正失败步骤覆盖掉。
		returnErr = fmt.Errorf("%w; rollback document replacement also failed: %v", returnErr, rollbackErr)
	}()

	// 第一步先删除旧向量。向量 ID 来自旧分块子查询，因此必须在删旧分块之前执行。
	if _, err := tx.ExecContext(ctx, `
DELETE FROM rag.t_knowledge_vector
WHERE id IN (
  SELECT id FROM rag.t_knowledge_chunk WHERE document_id = $1
)`, document.ID); err != nil {
		return fmt.Errorf("delete old vectors for document %q: %w", document.ID, err)
	}

	// 第二步物理删除旧分块。替换场景不能使用逻辑删除，否则旧索引和唯一约束可能继续干扰新数据。
	if _, err := tx.ExecContext(ctx, `
DELETE FROM rag.t_knowledge_chunk WHERE document_id = $1`, document.ID); err != nil {
		return fmt.Errorf("delete old chunks for document %q: %w", document.ID, err)
	}

	// 第三步先写完全部新分块，再进入向量写入阶段，使事务顺序清楚、便于审计。
	for index, item := range prepared {
		chunk := item.chunk
		if _, err := tx.ExecContext(ctx, `
INSERT INTO rag.t_knowledge_chunk
  (id, kb_id, document_id, chunk_index, content, token_count, char_count,
   start_position, end_position, metadata, enabled, deleted)
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, 1, 0)`,
			chunk.ID,
			chunk.KbID,
			chunk.DocumentID,
			chunk.Index,
			chunk.Content,
			chunk.TokenCount,
			chunk.CharCount,
			chunk.StartPosition,
			chunk.EndPosition,
			item.chunkMetadataJSON,
		); err != nil {
			return fmt.Errorf("insert chunk %d for document %q: %w", index, document.ID, err)
		}
	}

	// 第四步按相同顺序写向量。显式 ::jsonb 和 ::vector 保证 pgx 不需要猜测 PostgreSQL 专属类型。
	for index, item := range prepared {
		chunk := item.chunk
		if _, err := tx.ExecContext(ctx, `
INSERT INTO rag.t_knowledge_vector (id, content, metadata, embedding)
VALUES ($1, $2, $3::jsonb, $4::vector)
ON CONFLICT (id) DO UPDATE SET
  content = EXCLUDED.content,
  metadata = EXCLUDED.metadata,
  embedding = EXCLUDED.embedding`,
			chunk.ID,
			chunk.Content,
			item.vectorMetadataJSON,
			item.vectorLiteral,
		); err != nil {
			return fmt.Errorf("upsert vector %d for document %q: %w", index, document.ID, err)
		}
	}

	// 第五步只有新分块和向量都成功后才持久化 completed 聚合，包含最新配置、元数据和更新时间。
	result, err := tx.ExecContext(ctx, `
UPDATE rag.t_knowledge_document
SET status = $2,
    chunk_count = $3,
    chunk_config = $4::jsonb,
    metadata = $5::jsonb,
    update_time = $6
WHERE id = $1 AND deleted = 0`,
		document.ID,
		row.Status,
		row.ChunkCount,
		string(row.ChunkConfig),
		string(row.Metadata),
		row.UpdateTime,
	)
	if err != nil {
		return fmt.Errorf("mark document %q completed: %w", document.ID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read completed document row count: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: document %q disappeared during replacement", domain.ErrNotFound, document.ID)
	}

	// 最后提交。只有 Commit 明确成功后才把 committed 置为 true，defer 才会跳过回滚。
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit document %q replacement: %w", document.ID, err)
	}
	committed = true
	return nil
}

// prepareChunkWrites 校验分块归属，并预先生成事务所需的 JSON 和 pgvector 字符串。
func prepareChunkWrites(
	document domain.Document,
	chunks []domain.Chunk,
	vectors [][]float32,
) ([]preparedChunkWrite, error) {
	prepared := make([]preparedChunkWrite, len(chunks))
	for index, chunk := range chunks {
		if err := chunk.Validate(); err != nil {
			return nil, fmt.Errorf("validate chunk %d before persistence: %w", index, err)
		}
		if chunk.KbID != document.KbID || chunk.DocumentID != document.ID {
			return nil, fmt.Errorf(
				"%w: chunk %q does not belong to document %q",
				domain.ErrInvalidArgument,
				chunk.ID,
				document.ID,
			)
		}

		chunkMetadata, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata for chunk %q: %w", chunk.ID, err)
		}
		vectorMetadata, err := json.Marshal(map[string]any{
			"kbId":       document.KbID,
			"documentId": document.ID,
			"chunkId":    chunk.ID,
			"chunkIndex": chunk.Index,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal vector metadata for chunk %q: %w", chunk.ID, err)
		}
		literal, err := vectorText(vectors[index], application.EmbeddingDimensions)
		if err != nil {
			return nil, fmt.Errorf("format vector for chunk %q: %w", chunk.ID, err)
		}

		prepared[index] = preparedChunkWrite{
			chunk:              chunk,
			chunkMetadataJSON:  string(chunkMetadata),
			vectorMetadataJSON: string(vectorMetadata),
			vectorLiteral:      literal,
		}
	}
	return prepared, nil
}

// MarkFailed 持久化领域聚合已经生成的 failed 状态和 lastError 元数据。
// 仓储不重新拼失败消息，避免数据库层和领域层出现两套状态转换规则。
func (r *Repository) MarkFailed(ctx context.Context, document domain.Document) error {
	if document.Status != domain.StatusFailed {
		return fmt.Errorf("%w: failure document status is %q", domain.ErrInvalidArgument, document.Status)
	}
	row, err := rowFromDomain(document)
	if err != nil {
		return err
	}

	result, err := r.db.ExecContext(ctx, `
UPDATE rag.t_knowledge_document
SET status = $2, metadata = $3::jsonb, update_time = $4
WHERE id = $1 AND deleted = 0`, document.ID, row.Status, string(row.Metadata), row.UpdateTime)
	if err != nil {
		return fmt.Errorf("mark document %q failed: %w", document.ID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read failed document row count: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: document %q", domain.ErrNotFound, document.ID)
	}
	return nil
}

// documentSelectSQL 是所有文档查询共用的完整列清单。
// 集中维护可以避免 Get/List 漏扫新字段，同时保留 WHERE/ORDER/LIMIT 由具体用例追加的灵活性。
const documentSelectSQL = `
SELECT id, kb_id, title, source_type, source_uri, file_name, file_type, file_size,
  content_hash, language, status, chunk_count, chunk_strategy, chunk_config, metadata, create_time, update_time
FROM rag.t_knowledge_document
`

// isUniqueViolation 沿错误链查找 pgx 的 PostgreSQL 错误，并只识别唯一约束代码 23505。
func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
