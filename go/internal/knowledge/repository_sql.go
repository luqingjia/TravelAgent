package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"travel-agent-go/internal/embedding"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

type SQLRepository struct {
	db *sqlx.DB
}

func NewSQLRepository(db *sqlx.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

func (r *SQLRepository) KnowledgeBaseExists(ctx context.Context, kbID string) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, `
SELECT EXISTS (
  SELECT 1 FROM rag.t_knowledge_base
  WHERE id = $1 AND deleted = 0 AND status = 'active'
)`, kbID)
	return exists, err
}

func (r *SQLRepository) ActiveDocumentHashExists(ctx context.Context, kbID string, contentHash string) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, `
SELECT EXISTS (
  SELECT 1 FROM rag.t_knowledge_document
  WHERE kb_id = $1 AND content_hash = $2 AND deleted = 0
)`, kbID, contentHash)
	return exists, err
}

func (r *SQLRepository) CreateDocument(ctx context.Context, doc Document) error {
	chunkConfig, err := json.Marshal(doc.ChunkConfig)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(doc.Metadata)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO rag.t_knowledge_document
  (id, kb_id, title, source_type, source_uri, file_name, file_type, file_size,
   content_hash, language, status, chunk_count, chunk_strategy, chunk_config, metadata)
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8,
   $9, $10, $11, $12, $13, $14::jsonb, $15::jsonb)`,
		doc.ID, doc.KbID, doc.Title, doc.SourceType, doc.SourceURI, doc.FileName, doc.FileType, doc.FileSize,
		doc.ContentHash, doc.Language, doc.Status, doc.ChunkCount, doc.ChunkStrategy, string(chunkConfig), string(metadata))
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}

func (r *SQLRepository) GetDocument(ctx context.Context, docID string) (Document, error) {
	var row documentRow
	err := r.db.GetContext(ctx, &row, documentSelectSQL+`
WHERE id = $1 AND deleted = 0`, docID)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, err
	}
	return row.toDocument()
}

func (r *SQLRepository) ListDocuments(ctx context.Context, kbID string, page int, size int) ([]Document, int64, error) {
	offset := (page - 1) * size
	var total int64
	if err := r.db.GetContext(ctx, &total, `
SELECT COUNT(1)
FROM rag.t_knowledge_document
WHERE kb_id = $1 AND deleted = 0`, kbID); err != nil {
		return nil, 0, err
	}
	var rows []documentRow
	if err := r.db.SelectContext(ctx, &rows, documentSelectSQL+`
WHERE kb_id = $1 AND deleted = 0
ORDER BY update_time DESC
LIMIT $2 OFFSET $3`, kbID, size, offset); err != nil {
		return nil, 0, err
	}
	documents := make([]Document, len(rows))
	for i, row := range rows {
		doc, err := row.toDocument()
		if err != nil {
			return nil, 0, err
		}
		documents[i] = doc
	}
	return documents, total, nil
}

func (r *SQLRepository) DeleteDocument(ctx context.Context, docID string) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE rag.t_knowledge_document
SET deleted = 1, update_time = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted = 0`, docID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLRepository) TryMarkProcessing(ctx context.Context, docID string) (Document, bool, error) {
	var row documentRow
	err := r.db.GetContext(ctx, &row, `
UPDATE rag.t_knowledge_document
SET status = 'processing', update_time = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted = 0 AND status <> 'processing'
RETURNING id, kb_id, title, source_type, source_uri, file_name, file_type, file_size,
  content_hash, language, status, chunk_count, chunk_strategy, chunk_config, metadata, create_time, update_time`, docID)
	if errors.Is(err, sql.ErrNoRows) {
		doc, getErr := r.GetDocument(ctx, docID)
		if getErr != nil {
			return Document{}, false, getErr
		}
		return doc, false, nil
	}
	if err != nil {
		return Document{}, false, err
	}
	doc, err := row.toDocument()
	return doc, true, err
}

func (r *SQLRepository) ReplaceDocumentChunks(ctx context.Context, doc Document, chunks []Chunk, vectors [][]float32) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("chunk count %d does not match vector count %d", len(chunks), len(vectors))
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `
DELETE FROM rag.t_knowledge_vector
WHERE id IN (
  SELECT id FROM rag.t_knowledge_chunk WHERE document_id = $1
)`, doc.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
DELETE FROM rag.t_knowledge_chunk WHERE document_id = $1`, doc.ID); err != nil {
		return err
	}
	for i, chunk := range chunks {
		metadata, marshalErr := json.Marshal(chunk.Metadata)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO rag.t_knowledge_chunk
  (id, kb_id, document_id, chunk_index, content, token_count, char_count,
   start_position, end_position, metadata, enabled, deleted)
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, 1, 0)`,
			chunk.ID, doc.KbID, doc.ID, chunk.Index, chunk.Content, chunk.TokenCount, chunk.CharCount,
			chunk.StartPosition, chunk.EndPosition, string(metadata)); err != nil {
			return err
		}
		vectorText, vectorErr := embedding.VectorText(vectors[i])
		if vectorErr != nil {
			return vectorErr
		}
		vectorMetadata, marshalErr := json.Marshal(map[string]any{
			"kbId":       doc.KbID,
			"documentId": doc.ID,
			"chunkId":    chunk.ID,
			"chunkIndex": chunk.Index,
		})
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO rag.t_knowledge_vector (id, content, metadata, embedding)
VALUES ($1, $2, $3::jsonb, $4::vector)
ON CONFLICT (id) DO UPDATE SET
  content = EXCLUDED.content,
  metadata = EXCLUDED.metadata,
  embedding = EXCLUDED.embedding`,
			chunk.ID, chunk.Content, string(vectorMetadata), vectorText); err != nil {
			return err
		}
	}
	metadata, err := json.Marshal(doc.Metadata)
	if err != nil {
		return err
	}
	chunkConfig, err := json.Marshal(doc.ChunkConfig)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
UPDATE rag.t_knowledge_document
SET status = 'completed',
    chunk_count = $2,
    chunk_config = $3::jsonb,
    metadata = $4::jsonb,
    update_time = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted = 0`, doc.ID, len(chunks), string(chunkConfig), string(metadata)); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (r *SQLRepository) MarkFailed(ctx context.Context, doc Document, message string) error {
	metadata := cloneMap(doc.Metadata)
	metadata["lastError"] = message
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE rag.t_knowledge_document
SET status = 'failed', metadata = $2::jsonb, update_time = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted = 0`, doc.ID, string(metadataJSON))
	return err
}

const documentSelectSQL = `
SELECT id, kb_id, title, source_type, source_uri, file_name, file_type, file_size,
  content_hash, language, status, chunk_count, chunk_strategy, chunk_config, metadata, create_time, update_time
FROM rag.t_knowledge_document
`

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

func (r documentRow) toDocument() (Document, error) {
	chunkConfig := map[string]any{}
	if len(r.ChunkConfig) > 0 {
		if err := json.Unmarshal(r.ChunkConfig, &chunkConfig); err != nil {
			return Document{}, err
		}
	}
	metadata := map[string]any{}
	if len(r.Metadata) > 0 {
		if err := json.Unmarshal(r.Metadata, &metadata); err != nil {
			return Document{}, err
		}
	}
	return Document{
		ID:            r.ID,
		KbID:          r.KbID,
		Title:         r.Title,
		SourceType:    r.SourceType,
		SourceURI:     r.SourceURI,
		FileName:      r.FileName,
		FileType:      r.FileType,
		FileSize:      r.FileSize,
		ContentHash:   r.ContentHash,
		Language:      r.Language,
		Status:        r.Status,
		ChunkCount:    r.ChunkCount,
		ChunkStrategy: r.ChunkStrategy,
		ChunkConfig:   chunkConfig,
		Metadata:      metadata,
		CreateTime:    r.CreateTime,
		UpdateTime:    r.UpdateTime,
	}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
