package postgres_test

import (
	. "github.com/luqingjia/TravelAgent/internal/knowledge/adapter/postgres"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// TestReplaceDocumentChunksRejectsMismatchedCountsBeforeTransaction 验证分块和向量数量不一致时不会开启数据库事务。
func TestReplaceDocumentChunksRejectsMismatchedCountsBeforeTransaction(t *testing.T) {
	repository, err := NewRepository(&sqlx.DB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	document := completedDocumentFixture(t, 1)
	chunk := validChunkFixture()
	err = repository.ReplaceDocumentChunks(t.Context(), document, []domain.Chunk{chunk}, nil)
	if err == nil || !strings.Contains(err.Error(), "vector count") {
		t.Fatalf("ReplaceDocumentChunks() error = %v, want vector count mismatch", err)
	}
}

// TestReplaceDocumentChunksValidatesEmbeddingDimensionsBeforeTransaction 验证 1536 维向量合同仍在持久化边界执行。
func TestReplaceDocumentChunksValidatesEmbeddingDimensionsBeforeTransaction(t *testing.T) {
	repository, err := NewRepository(&sqlx.DB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	document := completedDocumentFixture(t, 1)
	chunk := validChunkFixture()
	wrongVector := make([]float32, application.EmbeddingDimensions-1)
	err = repository.ReplaceDocumentChunks(t.Context(), document, []domain.Chunk{chunk}, [][]float32{wrongVector})
	if err == nil || !strings.Contains(err.Error(), "dimensions") {
		t.Fatalf("ReplaceDocumentChunks() error = %v, want dimension error", err)
	}
}

func completedDocumentFixture(t *testing.T, chunkCount int) domain.Document {
	t.Helper()
	now := time.Date(2026, time.July, 14, 9, 30, 0, 0, time.UTC)
	document, err := domain.RestoreDocument(domain.Document{
		ID:            "doc-1",
		KbID:          "kb-1",
		Title:         "企业级 Go 项目说明",
		SourceType:    domain.SourceTypeFile,
		SourceURI:     "s3://knowledge/doc-1.md",
		FileName:      "travel-agent.md",
		FileType:      "md",
		FileSize:      2048,
		ContentHash:   "sha256-value",
		Language:      "zh",
		Status:        domain.StatusCompleted,
		ChunkCount:    chunkCount,
		ChunkStrategy: domain.DefaultChunkStrategy,
		ChunkConfig:   map[string]any{"targetChars": 800},
		Metadata:      map[string]any{"contentType": "text/markdown"},
		CreateTime:    now,
		UpdateTime:    now,
	})
	if err != nil {
		t.Fatalf("RestoreDocument() error = %v", err)
	}
	return document
}

func validChunkFixture() domain.Chunk {
	return domain.Chunk{
		ID:            "chunk-1",
		KbID:          "kb-1",
		DocumentID:    "doc-1",
		Index:         0,
		Content:       "第一段",
		TokenCount:    3,
		CharCount:     3,
		StartPosition: 0,
		EndPosition:   len("第一段"),
		Metadata:      map[string]any{},
	}
}
