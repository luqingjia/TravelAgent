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

// 本文件集中保存知识文档的 SQL、事务和 PostgreSQL 错误映射。
// application 只看到 DocumentRepository 接口，不会依赖 sqlx、pgx 或具体表名。
// Repository 使用 sqlx 操作现有 rag schema，并实现应用层定义的文档仓储端口。
// 它只负责持久化语义；上传校验、状态转换和失败原因合并等业务规则仍由 application/domain 决定。
type Repository struct {
	// db 是由组合根创建并完成 Ping 的共享连接池，Repository 不拥有它的关闭时机。
	db *sqlx.DB
}

// 这条编译期断言不会产生运行时代码。只要 Repository 少实现或写错一个应用层接口方法，
// `go test`/`go build` 就会立即失败，而不是等组合根运行后才发现依赖不兼容。
var _ application.DocumentRepository = (*Repository)(nil)

// NewRepository 创建 PostgreSQL 文档仓储。
// 数据库连接由 platform/database 建立并由 app 组合根传入，仓储内部不读取环境变量或创建全局连接。
func NewRepository(database *sqlx.DB) (*Repository, error) {
	// nil 连接池会让第一次查询直接 panic，因此在启动构造阶段拒绝。
	if database == nil {
		return nil, fmt.Errorf("postgres database is required")
	}
	// 仓储只保存连接池引用，不在构造函数中执行迁移或建表。
	return &Repository{db: database}, nil
}

// KnowledgeBaseExists 判断知识库是否存在、未删除并且处于 active 状态。
func (r *Repository) KnowledgeBaseExists(ctx context.Context, knowledgeBaseID string) (bool, error) {
	// exists 是 sqlx 扫描 SELECT EXISTS 单行结果的目标变量。
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
	// true 表示知识库存在且当前可用，false 表示上传用例应返回 NotFound。
	return exists, nil
}

// ActiveDocumentHashExists 按“知识库 + 内容 SHA-256”检查有效文档是否重复。
// 这是一道提前失败的友好检查；并发请求最终仍由数据库部分唯一索引兜底。
func (r *Repository) ActiveDocumentHashExists(
	ctx context.Context,
	knowledgeBaseID string,
	contentHash string,
) (bool, error) {
	// 只扫描一个布尔值，不加载整份文档记录。
	var exists bool
	// 查询条件与数据库部分唯一索引保持同一业务范围：同知识库、同哈希、未删除。
	err := r.db.GetContext(ctx, &exists, `
SELECT EXISTS (
  SELECT 1 FROM rag.t_knowledge_document
  WHERE kb_id = $1 AND content_hash = $2 AND deleted = 0
)`, knowledgeBaseID, contentHash)
	if err != nil {
		return false, fmt.Errorf("check active document hash: %w", err)
	}
	// 该结果是友好预检查，并不能替代并发场景下的唯一索引。
	return exists, nil
}

// CreateDocument 写入一份由领域层创建的 pending 文档。
func (r *Repository) CreateDocument(ctx context.Context, document domain.Document) error {
	// 先做模型转换和 JSON 序列化，失败时不执行任何 SQL。
	row, err := rowFromDomain(document)
	if err != nil {
		return err
	}

	// 所有列都显式列出并使用位置参数，避免依赖数据库默认值或字符串拼接。
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
	// nil 表示 pending 文档已经成功登记，原始对象由对象存储单独保存。
	return nil
}

// GetDocument 查询一份未逻辑删除的文档，并把数据库行恢复为经过校验的领域对象。
func (r *Repository) GetDocument(ctx context.Context, documentID string) (domain.Document, error) {
	// documentRow 接收完整列清单，随后再显式恢复领域对象。
	var row documentRow
	// 查询只允许读取未逻辑删除文档。
	err := r.db.GetContext(ctx, &row, documentSelectSQL+`
WHERE id = $1 AND deleted = 0`, documentID)
	// sql.ErrNoRows 映射成稳定领域错误，应用层和 HTTP 层无需认识数据库错误。
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Document{}, fmt.Errorf("%w: document %q", domain.ErrNotFound, documentID)
	}
	// 其他数据库故障保留原始错误链和文档 ID 上下文。
	if err != nil {
		return domain.Document{}, fmt.Errorf("select document %q: %w", documentID, err)
	}

	// 数据库查询成功后还要解码 JSON 并校验领域不变量。
	document, err := row.toDomain()
	if err != nil {
		return domain.Document{}, fmt.Errorf("convert document %q: %w", documentID, err)
	}
	// 返回的领域对象不携带 db tag 或 JSONB 字节。
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

	// total 使用 int64 对应 PostgreSQL COUNT 返回范围。
	var total int64
	// 总数和列表使用完全相同的知识库与 deleted 条件。
	if err := r.db.GetContext(ctx, &total, `
SELECT COUNT(1)
FROM rag.t_knowledge_document
WHERE kb_id = $1 AND deleted = 0`, knowledgeBaseID); err != nil {
		return nil, 0, fmt.Errorf("count documents for knowledge base %q: %w", knowledgeBaseID, err)
	}

	// rows 保存当前页数据库行，空页会得到空切片而不是 NotFound。
	var rows []documentRow
	// 更新时间倒序保证最近处理或更新的文档排在前面。
	if err := r.db.SelectContext(ctx, &rows, documentSelectSQL+`
WHERE kb_id = $1 AND deleted = 0
ORDER BY update_time DESC
LIMIT $2 OFFSET $3`, knowledgeBaseID, size, offset); err != nil {
		return nil, 0, fmt.Errorf("list documents for knowledge base %q: %w", knowledgeBaseID, err)
	}

	// 预先按结果行数分配切片，再逐行做显式模型转换；任一脏数据都会终止本次响应。
	documents := make([]domain.Document, len(rows))
	// 每一行都独立解码和校验，不能让单条脏数据悄悄混入响应。
	for index, row := range rows {
		document, err := row.toDomain()
		if err != nil {
			return nil, 0, fmt.Errorf("convert listed document at index %d: %w", index, err)
		}
		// 转换成功后按原查询顺序写入目标切片。
		documents[index] = document
	}
	// 同时返回当前页文档和完整总数。
	return documents, total, nil
}

// DeleteDocument 对文档执行逻辑删除，保持现有 schema 和历史查询语义兼容。
func (r *Repository) DeleteDocument(ctx context.Context, documentID string) error {
	// 当前兼容语义只更新 deleted 和 update_time，不物理删除文档行。
	result, err := r.db.ExecContext(ctx, `
UPDATE rag.t_knowledge_document
SET deleted = 1, update_time = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted = 0`, documentID)
	// SQL 执行失败时保留文档 ID，方便定位具体删除操作。
	if err != nil {
		return fmt.Errorf("logically delete document %q: %w", documentID, err)
	}

	// RowsAffected 用来区分真正删除成功和目标不存在/已删除。
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted document row count: %w", err)
	}
	// 零行表示没有可从未删除状态切换的目标文档。
	if affected == 0 {
		return fmt.Errorf("%w: document %q", domain.ErrNotFound, documentID)
	}
	// 至少一行被更新后，当前删除用例完成。
	return nil
}

// TryMarkProcessing 使用一条带条件的 UPDATE 原子取得文档处理权。
// acquired=false 表示文档存在但已经是 processing；不存在则返回 ErrNotFound。
func (r *Repository) TryMarkProcessing(
	ctx context.Context,
	documentID string,
) (domain.Document, bool, error) {
	// RETURNING 会把抢占成功后的 processing 行直接扫描回来。
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
		// 再查询也找不到时，直接返回稳定 NotFound 或真实数据库错误。
		if getErr != nil {
			return domain.Document{}, false, getErr
		}
		// 文档存在说明它已是 processing，返回当前聚合和 acquired=false。
		return document, false, nil
	}
	// 条件更新本身的其他错误不能被解释为“未抢占”。
	if err != nil {
		return domain.Document{}, false, fmt.Errorf("mark document %q processing: %w", documentID, err)
	}

	// 抢占成功后的 RETURNING 行仍需经过 JSON 解码和领域不变量校验。
	document, err := row.toDomain()
	if err != nil {
		return domain.Document{}, false, fmt.Errorf("convert processing document %q: %w", documentID, err)
	}
	// acquired=true 明确告诉应用层可以开始对象存储和模型等慢操作。
	return document, true, nil
}

// preparedChunkWrite 保存进入替换事务前已经完成的序列化结果。
// 事务内只做数据库语句，不再执行可能失败的 JSON/向量格式转换，从而尽量缩短持锁时间。
type preparedChunkWrite struct {
	// chunk 是已经补齐 ID、归属和位置并通过领域校验的分块。
	chunk domain.Chunk
	// chunkMetadataJSON 是写 t_knowledge_chunk.metadata 的 JSON 文本。
	chunkMetadataJSON string
	// vectorMetadataJSON 是写 t_knowledge_vector.metadata 的关联信息。
	vectorMetadataJSON string
	// vectorLiteral 是经过维度校验的 pgvector 文本字面量。
	vectorLiteral string
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
	// 仓储只接受领域层已经生成的 completed 聚合，不代替领域层做状态转换。
	if document.Status != domain.StatusCompleted {
		return fmt.Errorf("%w: replacement document status is %q", domain.ErrInvalidArgument, document.Status)
	}
	// 聚合记录的分块数必须与即将写入的切片长度一致。
	if document.ChunkCount != len(chunks) {
		return fmt.Errorf("%w: document chunk count %d does not match prepared chunks %d", domain.ErrInvalidArgument, document.ChunkCount, len(chunks))
	}

	// 文档 JSON 也先在事务外完成；row 里的 Metadata/ChunkConfig 会用于最后一次 UPDATE。
	row, err := rowFromDomain(document)
	// JSON 序列化或领域校验失败时，数据库事务尚未开始。
	if err != nil {
		return err
	}
	// 分块 metadata、向量 metadata 和 vector 文本全部在事务外准备。
	prepared, err := prepareChunkWrites(document, chunks, vectors)
	if err != nil {
		return err
	}

	// 只有全部输入已经验证完成才开启短事务。
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin document replacement transaction: %w", err)
	}
	// committed 由 defer 判断是否需要回滚，初始值必须是 false。
	committed := false
	// 无论下面从哪一个 return 提前退出，只要没有成功提交就执行回滚。
	// 这比依赖某个可能被 := 遮蔽的 err 变量可靠，也正是事务代码最容易踩坑的地方。
	defer func() {
		// Commit 已明确成功时，事务已经结束，不再调用 Rollback。
		if committed {
			return
		}
		// 所有提前返回都会走到这里尝试回滚。
		rollbackErr := tx.Rollback()
		// 回滚成功或事务已经结束都不需要追加错误。
		if rollbackErr == nil || errors.Is(rollbackErr, sql.ErrTxDone) {
			return
		}
		// 极端情况下主流程没有错误但回滚失败，就把回滚错误作为返回错误。
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
		// item 与 chunks、vectors 使用同一下标，保存已经准备好的全部参数。
		chunk := item.chunk
		// 每个分块单独执行 INSERT，任意失败都会触发整个事务回滚。
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
		// 向量 ID 直接复用 chunk ID，保证一份分块只对应一条向量记录。
		chunk := item.chunk
		// ON CONFLICT 是安全兜底；正常流程已先删除旧向量。
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
	// UPDATE 必须真正命中文档，否则新分块不能与消失的聚合一起提交。
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read completed document row count: %w", err)
	}
	// 零行可能表示并发删除，按 NotFound 失败并回滚前面所有写入。
	if affected == 0 {
		return fmt.Errorf("%w: document %q disappeared during replacement", domain.ErrNotFound, document.ID)
	}

	// 最后提交。只有 Commit 明确成功后才把 committed 置为 true，defer 才会跳过回滚。
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit document %q replacement: %w", document.ID, err)
	}
	// 只有 Commit 返回 nil 才关闭 defer 回滚开关。
	committed = true
	// nil 表示旧数据删除、新数据写入和文档状态更新已经一起提交。
	return nil
}

// prepareChunkWrites 校验分块归属，并预先生成事务所需的 JSON 和 pgvector 字符串。
func prepareChunkWrites(
	document domain.Document,
	chunks []domain.Chunk,
	vectors [][]float32,
) ([]preparedChunkWrite, error) {
	// 结果长度与 chunks 保持一致，后续事务按同一下标读取。
	prepared := make([]preparedChunkWrite, len(chunks))
	// 逐项完成所有可能失败的 CPU 格式转换，避免把这些工作带入事务。
	for index, chunk := range chunks {
		// 每个分块必须满足 ID、内容、位置和计数不变量。
		if err := chunk.Validate(); err != nil {
			return nil, fmt.Errorf("validate chunk %d before persistence: %w", index, err)
		}
		// 分块归属必须与当前 completed 文档完全一致，防止跨文档误写。
		if chunk.KbID != document.KbID || chunk.DocumentID != document.ID {
			return nil, fmt.Errorf(
				"%w: chunk %q does not belong to document %q",
				domain.ErrInvalidArgument,
				chunk.ID,
				document.ID,
			)
		}

		// 分块自身 metadata 序列化后用于 chunk 表 JSONB 列。
		chunkMetadata, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata for chunk %q: %w", chunk.ID, err)
		}
		// 向量 metadata 保存检索结果回溯文档所需的最小关联字段。
		vectorMetadata, err := json.Marshal(map[string]any{
			"kbId":       document.KbID,
			"documentId": document.ID,
			"chunkId":    chunk.ID,
			"chunkIndex": chunk.Index,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal vector metadata for chunk %q: %w", chunk.ID, err)
		}
		// 维度校验和 pgvector 文本格式化在开启事务前完成。
		literal, err := vectorText(vectors[index], application.EmbeddingDimensions)
		if err != nil {
			return nil, fmt.Errorf("format vector for chunk %q: %w", chunk.ID, err)
		}

		// 把同一下标的领域分块和所有序列化结果打包保存。
		prepared[index] = preparedChunkWrite{
			chunk:              chunk,
			chunkMetadataJSON:  string(chunkMetadata),
			vectorMetadataJSON: string(vectorMetadata),
			vectorLiteral:      literal,
		}
	}
	// 全部准备成功后，调用方才允许开启数据库替换事务。
	return prepared, nil
}

// MarkFailed 持久化领域聚合已经生成的 failed 状态和 lastError 元数据。
// 仓储不重新拼失败消息，避免数据库层和领域层出现两套状态转换规则。
func (r *Repository) MarkFailed(ctx context.Context, document domain.Document) error {
	// 仓储只接受领域层已经完成 failed 转换的聚合。
	if document.Status != domain.StatusFailed {
		return fmt.Errorf("%w: failure document status is %q", domain.ErrInvalidArgument, document.Status)
	}
	// 写库前统一验证聚合并序列化 metadata。
	row, err := rowFromDomain(document)
	if err != nil {
		return err
	}

	// 只更新状态、metadata 和更新时间，不改动旧分块和向量。
	result, err := r.db.ExecContext(ctx, `
UPDATE rag.t_knowledge_document
SET status = $2, metadata = $3::jsonb, update_time = $4
WHERE id = $1 AND deleted = 0`, document.ID, row.Status, string(row.Metadata), row.UpdateTime)
	if err != nil {
		return fmt.Errorf("mark document %q failed: %w", document.ID, err)
	}
	// RowsAffected 用于确认文档在失败回写期间没有被删除。
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read failed document row count: %w", err)
	}
	// 文档消失时返回 NotFound，但应用层仍会优先保留最初的处理错误。
	if affected == 0 {
		return fmt.Errorf("%w: document %q", domain.ErrNotFound, document.ID)
	}
	// nil 表示最近失败原因已经成功保存。
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
	// errors.As 会沿 %w 错误链寻找 pgx 暴露的 PostgreSQL 错误对象。
	var postgresError *pgconn.PgError
	// 只有 SQLSTATE 23505 代表唯一约束冲突，其他数据库错误不能误报为重复文档。
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
