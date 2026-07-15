// Package postgres 实现知识文档应用层所需的 PostgreSQL 持久化端口。
//
// 这个包可以认识 sqlx、PostgreSQL 列名和 JSONB，但不能把这些细节塞回 domain。
// 数据库行与领域对象之间全部显式转换，数据库中的旧数据也必须经过领域不变量校验。
package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// documentRow 是 rag.t_knowledge_document 查询结果在 Go 中的数据库行模型。
//
// db tag 只服务于 sqlx 的列映射，因此它留在最外层适配器。领域对象不携带这些 tag，
// 这样数据库改列名时不会迫使核心业务类型跟着改变。
type documentRow struct {
	ID            string    `db:"id"`
	KbID          string    `db:"kb_id"`
	Title         string    `db:"title"`
	SourceType    string    `db:"source_type"`
	SourceURI     string    `db:"source_uri"`
	FileName      string    `db:"file_name"`
	FileType      string    `db:"file_type"`
	FileSize      int64     `db:"file_size"`
	ContentHash   string    `db:"content_hash"`
	Language      string    `db:"language"`
	Status        string    `db:"status"`
	ChunkCount    int       `db:"chunk_count"`
	ChunkStrategy string    `db:"chunk_strategy"`
	ChunkConfig   []byte    `db:"chunk_config"`
	Metadata      []byte    `db:"metadata"`
	CreateTime    time.Time `db:"create_time"`
	UpdateTime    time.Time `db:"update_time"`
}

// rowFromDomain 把经过领域规则约束的文档转换成可写入 PostgreSQL 的行模型。
// JSON map 在这里序列化，因为“怎样保存 JSONB”属于数据库适配器职责，而不是领域规则。
func rowFromDomain(document domain.Document) (documentRow, error) {
	// Document 的字段是导出的，外层理论上仍能手工拼出非法值。写库前再恢复一次，
	// 相当于在持久化边界做最后一道不变量检查，避免把坏状态永久写进数据库。
	validated, err := domain.RestoreDocument(document)
	if err != nil {
		return documentRow{}, fmt.Errorf("validate document before persistence: %w", err)
	}

	chunkConfig, err := json.Marshal(validated.ChunkConfig)
	if err != nil {
		return documentRow{}, fmt.Errorf("marshal document chunk config: %w", err)
	}
	metadata, err := json.Marshal(validated.Metadata)
	if err != nil {
		return documentRow{}, fmt.Errorf("marshal document metadata: %w", err)
	}

	// 这里故意逐字段赋值，不使用反射或通用映射器。虽然代码稍长，但审查数据库兼容性时
	// 可以一眼看到每个领域字段最终落到哪一列，也不会悄悄漏掉新字段。
	return documentRow{
		ID:            validated.ID,
		KbID:          validated.KbID,
		Title:         validated.Title,
		SourceType:    string(validated.SourceType),
		SourceURI:     validated.SourceURI,
		FileName:      validated.FileName,
		FileType:      validated.FileType,
		FileSize:      validated.FileSize,
		ContentHash:   validated.ContentHash,
		Language:      validated.Language,
		Status:        string(validated.Status),
		ChunkCount:    validated.ChunkCount,
		ChunkStrategy: validated.ChunkStrategy,
		ChunkConfig:   chunkConfig,
		Metadata:      metadata,
		CreateTime:    validated.CreateTime,
		UpdateTime:    validated.UpdateTime,
	}, nil
}

// toDomain 把 sqlx 扫描得到的行恢复成领域对象。
// 任何 JSON 损坏或非法状态都会返回错误，不能让“字段看起来差不多”的半成品进入应用层。
func (row documentRow) toDomain() (domain.Document, error) {
	chunkConfig, err := decodeJSONMap("chunk_config", row.ChunkConfig)
	if err != nil {
		return domain.Document{}, err
	}
	metadata, err := decodeJSONMap("metadata", row.Metadata)
	if err != nil {
		return domain.Document{}, err
	}

	// 先显式构造快照，再交给 RestoreDocument 统一校验 ID、状态、时间和数量等不变量。
	document, err := domain.RestoreDocument(domain.Document{
		ID:            row.ID,
		KbID:          row.KbID,
		Title:         row.Title,
		SourceType:    domain.SourceType(row.SourceType),
		SourceURI:     row.SourceURI,
		FileName:      row.FileName,
		FileType:      row.FileType,
		FileSize:      row.FileSize,
		ContentHash:   row.ContentHash,
		Language:      row.Language,
		Status:        domain.DocumentStatus(row.Status),
		ChunkCount:    row.ChunkCount,
		ChunkStrategy: row.ChunkStrategy,
		ChunkConfig:   chunkConfig,
		Metadata:      metadata,
		CreateTime:    row.CreateTime,
		UpdateTime:    row.UpdateTime,
	})
	if err != nil {
		return domain.Document{}, fmt.Errorf("restore document %q from database: %w", row.ID, err)
	}

	return document, nil
}

// decodeJSONMap 统一处理 PostgreSQL JSONB 列扫描后的字节。
// 空列按空对象处理，避免应用层到处判断 nil；非空但损坏的 JSON 则明确报出列名。
func decodeJSONMap(column string, value []byte) (map[string]any, error) {
	decoded := map[string]any{}
	if len(value) == 0 {
		return decoded, nil
	}
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil, fmt.Errorf("decode document %s: %w", column, err)
	}
	if decoded == nil {
		// JSON 的 null 会解码成 nil map。领域层希望拿到可安全写入的 map，因此转成空对象。
		decoded = map[string]any{}
	}
	return decoded, nil
}
