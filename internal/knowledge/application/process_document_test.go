package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// TestProcessDocumentRejectsWhenProcessingOwnershipIsNotAcquired 验证并发请求没有抢到处理权时立即退出。
func TestProcessDocumentRejectsWhenProcessingOwnershipIsNotAcquired(t *testing.T) {
	service, repo, storage, _ := newProcessingService(t)
	repo.processingAcquired = false

	_, err := service.ProcessDocument(context.Background(), "doc-1", domain.ChunkOptions{})

	if !errors.Is(err, domain.ErrAlreadyRunning) {
		t.Fatalf("ProcessDocument() error = %v, want ErrAlreadyRunning", err)
	}
	if len(storage.getURIs) != 0 {
		t.Fatalf("storage Get calls = %d, want 0", len(storage.getURIs))
	}
}

// TestProcessDocumentMarksFailedWhenStorageReadFails 验证取得处理权后的任何失败都会保存 failed 状态。
func TestProcessDocumentMarksFailedWhenStorageReadFails(t *testing.T) {
	service, repo, storage, _ := newProcessingService(t)
	readErr := errors.New("object unavailable")
	storage.getErr = readErr

	_, err := service.ProcessDocument(context.Background(), "doc-1", domain.ChunkOptions{})

	if !errors.Is(err, readErr) {
		t.Fatalf("ProcessDocument() error = %v, want storage error", err)
	}
	assertFailedDocument(t, repo, "object unavailable")
	if len(repo.replacedDocuments) != 0 {
		t.Fatalf("Replace calls = %d, want 0", len(repo.replacedDocuments))
	}
}

// TestProcessDocumentRejectsIncompletePreparedResults 验证空文本、向量数量和维度错误都不能进入替换事务。
func TestProcessDocumentRejectsIncompletePreparedResults(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*fakeStorage, *fakeEmbedder)
	}{
		{
			name: "解析文本为空",
			configure: func(storage *fakeStorage, _ *fakeEmbedder) {
				storage.getResult = []byte(" \n\t ")
			},
		},
		{
			name: "向量数量不匹配",
			configure: func(storage *fakeStorage, embedder *fakeEmbedder) {
				storage.getResult = []byte("hello travel")
				embedder.vectors = [][][]float32{{}}
			},
		},
		{
			name: "向量维度不是1536",
			configure: func(storage *fakeStorage, embedder *fakeEmbedder) {
				storage.getResult = []byte("hello travel")
				embedder.vectors = [][][]float32{{{1, 2}}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, repo, storage, embedder := newProcessingService(t)
			tt.configure(storage, embedder)

			_, err := service.ProcessDocument(context.Background(), "doc-1", domain.ChunkOptions{})

			if err == nil {
				t.Fatal("ProcessDocument() error = nil, want preparation error")
			}
			assertFailedDocument(t, repo, "")
			if len(repo.replacedDocuments) != 0 {
				t.Fatalf("Replace calls = %d, want 0", len(repo.replacedDocuments))
			}
		})
	}
}

// TestProcessDocumentReplacesChunksAndCompletesDocument 验证完整结果只通过一次原子替换提交。
func TestProcessDocumentReplacesChunksAndCompletesDocument(t *testing.T) {
	service, repo, storage, embedder := newProcessingService(t)
	storage.getResult = []byte("hello travel")
	embedder.vectors = [][][]float32{{vectorWithDimensions(EmbeddingDimensions)}}

	completed, err := service.ProcessDocument(context.Background(), "doc-1", domain.ChunkOptions{})
	if err != nil {
		t.Fatalf("ProcessDocument() error = %v", err)
	}

	// 关键断言：领域状态完成、分块数量正确，只清除 lastError 并保留 source。
	if completed.Status != domain.StatusCompleted || completed.ChunkCount != 1 {
		t.Fatalf("completed document = %#v", completed)
	}
	if _, exists := completed.Metadata["lastError"]; exists || completed.Metadata["source"] != "manual" {
		t.Fatalf("completed metadata = %#v", completed.Metadata)
	}
	if len(repo.replacedDocuments) != 1 || len(repo.replacedChunks) != 1 || len(repo.replacedVectors) != 1 {
		t.Fatalf("replace records = documents %d, chunks %d, vectors %d", len(repo.replacedDocuments), len(repo.replacedChunks), len(repo.replacedVectors))
	}

	// 持久化前每个 Chunk 已补齐归属、ID、token 数量和原文位置，能够通过领域校验。
	chunks := repo.replacedChunks[0]
	if len(chunks) != 1 {
		t.Fatalf("chunk count = %d, want 1", len(chunks))
	}
	if err := chunks[0].Validate(); err != nil {
		t.Fatalf("persisted chunk Validate() error = %v", err)
	}
	if chunks[0].TokenCount != chunks[0].CharCount {
		t.Fatalf("token count = %d, char count = %d", chunks[0].TokenCount, chunks[0].CharCount)
	}
	if len(repo.failedDocuments) != 0 {
		t.Fatalf("MarkFailed calls = %d, want 0", len(repo.failedDocuments))
	}
}

// TestProcessDocumentReturnsReplaceErrorAndAttemptsFailureWriteback 验证事务失败时保留原错误并尽力回写 failed。
func TestProcessDocumentReturnsReplaceErrorAndAttemptsFailureWriteback(t *testing.T) {
	service, repo, storage, embedder := newProcessingService(t)
	storage.getResult = []byte("hello travel")
	embedder.vectors = [][][]float32{{vectorWithDimensions(EmbeddingDimensions)}}
	replaceErr := errors.New("replace transaction failed")
	repo.replaceErr = replaceErr
	// 再让状态回写失败，确认它只被记录，不会把 replaceErr 换成第二个错误。
	repo.markFailedErr = errors.New("mark failed unavailable")

	_, err := service.ProcessDocument(context.Background(), "doc-1", domain.ChunkOptions{})

	if !errors.Is(err, replaceErr) {
		t.Fatalf("ProcessDocument() error = %v, want replace error", err)
	}
	assertFailedDocument(t, repo, "replace transaction failed")
}

// newProcessingService 创建一组已成功取得 processing 状态的测试依赖。
func newProcessingService(t *testing.T) (*Service, *fakeRepository, *fakeStorage, *fakeEmbedder) {
	t.Helper()
	deps := validDependencies()
	repo := deps.Repository.(*fakeRepository)
	repo.processingAcquired = true
	repo.processingDocument = processingDocumentFixture()
	storage := deps.Storage.(*fakeStorage)
	embedder := deps.Embedder.(*fakeEmbedder)

	service, err := NewService(deps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service, repo, storage, embedder
}

// processingDocumentFixture 模拟 PostgreSQL 原子抢占后返回的合法 processing 聚合。
func processingDocumentFixture() domain.Document {
	createdAt := fixedNow.Add(-time.Hour)
	return domain.Document{
		ID:            "doc-1",
		KbID:          "kb-1",
		Title:         "旅行指南",
		SourceType:    domain.SourceTypeFile,
		SourceURI:     "s3://travel-agent/guide.txt",
		FileName:      "guide.txt",
		FileType:      "txt",
		FileSize:      12,
		ContentHash:   "sha256-value",
		Language:      "zh",
		Status:        domain.StatusProcessing,
		ChunkCount:    0,
		ChunkStrategy: domain.DefaultChunkStrategy,
		ChunkConfig:   map[string]any{},
		Metadata:      map[string]any{"source": "manual", "lastError": "old error"},
		CreateTime:    createdAt,
		UpdateTime:    createdAt,
	}
}

// vectorWithDimensions 构造指定维度的向量，避免在测试里手写 1536 个数字。
func vectorWithDimensions(dimensions int) []float32 {
	vector := make([]float32, dimensions)
	for index := range vector {
		vector[index] = float32(index) / 1000
	}
	return vector
}

// assertFailedDocument 检查应用层确实把领域转换后的 failed 聚合交给了仓储。
func assertFailedDocument(t *testing.T, repo *fakeRepository, messagePart string) {
	t.Helper()
	if len(repo.failedDocuments) != 1 {
		t.Fatalf("MarkFailed calls = %d, want 1", len(repo.failedDocuments))
	}
	failed := repo.failedDocuments[0]
	if failed.Status != domain.StatusFailed || failed.Metadata["source"] != "manual" {
		t.Fatalf("failed document = %#v", failed)
	}
	if messagePart != "" && !strings.Contains(failed.Metadata["lastError"].(string), messagePart) {
		t.Fatalf("lastError = %#v, want to contain %q", failed.Metadata["lastError"], messagePart)
	}
}
