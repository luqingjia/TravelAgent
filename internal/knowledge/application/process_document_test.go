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
	// fake 仓储返回 acquired=false，模拟另一请求已经抢到同一文档的处理权。
	service, repo, storage, _ := newProcessingService(t)
	repo.processingAcquired = false

	// 当前请求必须立即停止，不能继续读取对象或调用模型。
	_, err := service.ProcessDocument(context.Background(), "doc-1", domain.ChunkOptions{})

	// 业务层对外返回稳定的“正在处理”错误，而不是数据库更新行数等技术细节。
	if !errors.Is(err, domain.ErrAlreadyRunning) {
		t.Fatalf("ProcessDocument() error = %v, want ErrAlreadyRunning", err)
	}
	// 没有处理权时读取源文件会造成重复计算，因此 Get 调用次数必须为零。
	if len(storage.getURIs) != 0 {
		t.Fatalf("storage Get calls = %d, want 0", len(storage.getURIs))
	}
}

// TestProcessDocumentMarksFailedWhenStorageReadFails 验证取得处理权后的任何失败都会保存 failed 状态。
func TestProcessDocumentMarksFailedWhenStorageReadFails(t *testing.T) {
	// 已成功抢占 processing 状态，但对象存储在读取原文件时返回故障。
	service, repo, storage, _ := newProcessingService(t)
	readErr := errors.New("object unavailable")
	storage.getErr = readErr

	// 执行后应用服务既要返回原始读取错误，也要尽力把聚合改成 failed。
	_, err := service.ProcessDocument(context.Background(), "doc-1", domain.ChunkOptions{})

	if !errors.Is(err, readErr) {
		t.Fatalf("ProcessDocument() error = %v, want storage error", err)
	}
	// lastError 应保留可定位的错误内容，且不能进入最终替换事务。
	assertFailedDocument(t, repo, "object unavailable")
	if len(repo.replacedDocuments) != 0 {
		t.Fatalf("Replace calls = %d, want 0", len(repo.replacedDocuments))
	}
}

// TestProcessDocumentRejectsIncompletePreparedResults 验证空文本、向量数量和维度错误都不能进入替换事务。
func TestProcessDocumentRejectsIncompletePreparedResults(t *testing.T) {
	tests := []struct {
		// name 说明准备结果在哪一步不完整。
		name string
		// configure 只改变存储正文或模型返回，其他处理依赖保持合法。
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
			// 每个子场景新建 fake，避免向量队列和调用记录相互污染。
			service, repo, storage, embedder := newProcessingService(t)
			tt.configure(storage, embedder)

			// 执行完整处理流程，错误应发生在事务提交之前。
			_, err := service.ProcessDocument(context.Background(), "doc-1", domain.ChunkOptions{})

			// 任一准备结果不完整都必须标记失败，不能把半成品写进数据库。
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
	// 存储返回一段有效文本，模型返回与单个分块一一对应的 1536 维向量。
	service, repo, storage, embedder := newProcessingService(t)
	storage.getResult = []byte("hello travel")
	embedder.vectors = [][][]float32{{vectorWithDimensions(EmbeddingDimensions)}}

	// 使用空选项触发默认分块配置，执行从读取到原子替换的完整成功路径。
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
	// 前面的读取、分块和 Embedding 全部成功，只在最终数据库替换事务注入故障。
	service, repo, storage, embedder := newProcessingService(t)
	storage.getResult = []byte("hello travel")
	embedder.vectors = [][][]float32{{vectorWithDimensions(EmbeddingDimensions)}}
	replaceErr := errors.New("replace transaction failed")
	repo.replaceErr = replaceErr
	// 再让状态回写失败，确认它只被记录，不会把 replaceErr 换成第二个错误。
	repo.markFailedErr = errors.New("mark failed unavailable")

	// 执行后即使 failed 回写也失败，调用方仍应收到最初的事务错误。
	_, err := service.ProcessDocument(context.Background(), "doc-1", domain.ChunkOptions{})

	if !errors.Is(err, replaceErr) {
		t.Fatalf("ProcessDocument() error = %v, want replace error", err)
	}
	assertFailedDocument(t, repo, "replace transaction failed")
}

// newProcessingService 创建一组已成功取得 processing 状态的测试依赖。
func newProcessingService(t *testing.T) (*Service, *fakeRepository, *fakeStorage, *fakeEmbedder) {
	// 标记帮助函数，使构造失败行号回到具体业务场景。
	t.Helper()
	// 从通用合法依赖中取出三个 fake，配置成“已经抢到处理权”的默认成功起点。
	deps := validDependencies()
	repo := deps.Repository.(*fakeRepository)
	repo.processingAcquired = true
	repo.processingDocument = processingDocumentFixture()
	storage := deps.Storage.(*fakeStorage)
	embedder := deps.Embedder.(*fakeEmbedder)

	// 仍然使用生产构造器创建 Service，确保依赖校验没有被测试绕过。
	service, err := NewService(deps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service, repo, storage, embedder
}

// processingDocumentFixture 模拟 PostgreSQL 原子抢占后返回的合法 processing 聚合。
func processingDocumentFixture() domain.Document {
	// 创建时间早于固定当前时间一小时，处理完成后可观察 UpdateTime 正常前进。
	createdAt := fixedNow.Add(-time.Hour)
	// 字段保持完整合法，metadata 同时带业务来源和旧错误，便于验证成功与失败转换。
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
	// 先分配准确长度，再写入可识别的递增小数，内容本身不影响维度合同测试。
	vector := make([]float32, dimensions)
	for index := range vector {
		vector[index] = float32(index) / 1000
	}
	return vector
}

// assertFailedDocument 检查应用层确实把领域转换后的 failed 聚合交给了仓储。
func assertFailedDocument(t *testing.T, repo *fakeRepository, messagePart string) {
	// 标记帮助函数，断言失败时显示调用它的测试行号。
	t.Helper()
	// 一次处理失败只应回写一次，重复调用可能覆盖更早的错误信息。
	if len(repo.failedDocuments) != 1 {
		t.Fatalf("MarkFailed calls = %d, want 1", len(repo.failedDocuments))
	}
	// failed 聚合必须保留 source，并完成 processing 到 failed 的状态转换。
	failed := repo.failedDocuments[0]
	if failed.Status != domain.StatusFailed || failed.Metadata["source"] != "manual" {
		t.Fatalf("failed document = %#v", failed)
	}
	// 某些场景还要求 lastError 包含原始故障；空字符串表示只检查状态，不限定文案。
	if messagePart != "" && !strings.Contains(failed.Metadata["lastError"].(string), messagePart) {
		t.Fatalf("lastError = %#v, want to contain %q", failed.Metadata["lastError"], messagePart)
	}
}
