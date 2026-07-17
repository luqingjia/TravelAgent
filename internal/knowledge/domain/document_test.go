package domain_test

import (
	"testing"
	"time"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// TestNewPendingDocumentBuildsValidAggregate 验证“刚上传的文档”必须从 pending 状态开始。
// 这个场景同时检查 map 防御性复制，避免调用方后续修改请求数据时，偷偷改坏领域对象内部状态。
func TestNewPendingDocumentBuildsValidAggregate(t *testing.T) {
	// 准备：固定时间和完整的新文档输入，让时间、默认状态以及元数据都能被精确断言。
	now := time.Date(2026, time.July, 14, 10, 0, 0, 0, time.UTC)
	input := domain.NewDocument{
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
		ChunkStrategy: "structure_aware",
		ChunkConfig:   map[string]any{"targetChars": 800},
		Metadata:      map[string]any{"source": "manual"},
	}

	// 执行：通过领域构造函数创建文档，而不是让应用层随手拼一个可能不合法的结构体。
	document, err := domain.NewPendingDocument(input, now)
	if err != nil {
		t.Fatalf("NewPendingDocument() error = %v", err)
	}

	// 断言：新文档必须处于 pending，尚未产生任何分块，创建和更新时间都来自同一个时钟。
	if document.Status != domain.StatusPending {
		t.Fatalf("status = %q, want %q", document.Status, domain.StatusPending)
	}
	if document.ChunkCount != 0 {
		t.Fatalf("chunk count = %d, want 0", document.ChunkCount)
	}
	if !document.CreateTime.Equal(now) || !document.UpdateTime.Equal(now) {
		t.Fatalf("times = (%v, %v), want both %v", document.CreateTime, document.UpdateTime, now)
	}

	// 关键断言：修改输入 map 后，已经创建的聚合不能跟着变化，否则层与层之间会共享可变状态。
	input.Metadata["source"] = "changed"
	input.ChunkConfig["targetChars"] = 1
	if document.Metadata["source"] != "manual" {
		t.Fatalf("metadata source = %#v, want manual", document.Metadata["source"])
	}
	if document.ChunkConfig["targetChars"] != 800 {
		t.Fatalf("chunk config = %#v, want targetChars=800", document.ChunkConfig)
	}
}

// TestDocumentMarkFailedPreservesMetadata 验证处理失败时只覆盖 lastError，不能丢掉原有业务元数据。
func TestDocumentMarkFailedPreservesMetadata(t *testing.T) {
	// 准备：模拟一个已被抢占、正在处理的文档，并保留上传来源信息。
	document := validProcessingDocument()
	failedAt := time.Date(2026, time.July, 14, 10, 1, 0, 0, time.UTC)

	// 执行：让领域对象自己完成 processing -> failed 的合法状态转换。
	failed, err := document.MarkFailed("embedding timeout", failedAt)
	if err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}

	// 断言：失败状态和最近错误被记录，原有 source 元数据仍在，更新时间也随转换前进。
	if failed.Status != domain.StatusFailed {
		t.Fatalf("status = %q, want %q", failed.Status, domain.StatusFailed)
	}
	if failed.Metadata["source"] != "manual" || failed.Metadata["lastError"] != "embedding timeout" {
		t.Fatalf("metadata = %#v", failed.Metadata)
	}
	if !failed.UpdateTime.Equal(failedAt) {
		t.Fatalf("update time = %v, want %v", failed.UpdateTime, failedAt)
	}

	// 关键断言：方法返回新值，原对象不能被原地修改，便于失败重试和测试比较前后状态。
	if document.Status != domain.StatusProcessing || document.Metadata["lastError"] != "old error" {
		t.Fatalf("original document was mutated: %#v", document)
	}
}

// TestDocumentMarkCompletedClearsOnlyLastError 验证成功完成后只清理失败痕迹，其他元数据必须保留。
func TestDocumentMarkCompletedClearsOnlyLastError(t *testing.T) {
	// 准备：使用带有历史失败信息的 processing 文档，模拟一次成功重试。
	document := validProcessingDocument()
	completedAt := time.Date(2026, time.July, 14, 10, 2, 0, 0, time.UTC)
	chunkConfig := map[string]any{"minChars": 200, "targetChars": 800, "maxChars": 1200}

	// 执行：完成状态由领域方法统一设置，应用层只提供已经验证过的分块数量和配置。
	completed, err := document.MarkCompleted(3, chunkConfig, completedAt)
	if err != nil {
		t.Fatalf("MarkCompleted() error = %v", err)
	}

	// 断言：状态、数量和更新时间正确；lastError 被删除，但 source 仍然存在。
	if completed.Status != domain.StatusCompleted || completed.ChunkCount != 3 {
		t.Fatalf("completed = %#v", completed)
	}
	if _, exists := completed.Metadata["lastError"]; exists {
		t.Fatalf("lastError should be removed: %#v", completed.Metadata)
	}
	if completed.Metadata["source"] != "manual" {
		t.Fatalf("metadata source = %#v, want manual", completed.Metadata["source"])
	}
	if !completed.UpdateTime.Equal(completedAt) {
		t.Fatalf("update time = %v, want %v", completed.UpdateTime, completedAt)
	}

	// 修改调用方传入的配置，确认完成后的领域对象持有自己的副本。
	chunkConfig["targetChars"] = 1
	if completed.ChunkConfig["targetChars"] != 800 {
		t.Fatalf("chunk config = %#v, want targetChars=800", completed.ChunkConfig)
	}
}

// TestRestoreDocumentRejectsBrokenSnapshot 验证从数据库恢复聚合时不会把非法历史数据带进业务层。
func TestRestoreDocumentRejectsBrokenSnapshot(t *testing.T) {
	tests := []struct {
		// name 描述数据库快照违反的领域规则。
		name string
		// mutate 从合法 processing 文档出发只破坏一个字段。
		mutate func(domain.Document) domain.Document
	}{
		{
			name: "非法状态",
			mutate: func(document domain.Document) domain.Document {
				document.Status = domain.DocumentStatus("unknown")
				return document
			},
		},
		{
			name: "负数分块数量",
			mutate: func(document domain.Document) domain.Document {
				document.ChunkCount = -1
				return document
			},
		},
		{
			name: "空文档编号",
			mutate: func(document domain.Document) domain.Document {
				document.ID = ""
				return document
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 准备：从一个合法快照出发，只破坏当前子场景关心的一个不变量。
			broken := tt.mutate(validProcessingDocument())

			// 执行：恢复函数必须统一验证数据库快照，而不是相信外部数据永远正确。
			_, err := domain.RestoreDocument(broken)

			// 断言：每一种非法快照都必须被拒绝，防止错误继续扩散到应用流程。
			if err == nil {
				t.Fatal("RestoreDocument() error = nil, want validation error")
			}
		})
	}
}

// validProcessingDocument 提供所有测试共享的合法 processing 快照。
// 把公共准备逻辑集中在这里，可以让每个测试只突出自己要验证的业务规则。
func validProcessingDocument() domain.Document {
	// 固定创建时间让状态转换后的更新时间可以做精确比较。
	createdAt := time.Date(2026, time.July, 14, 9, 0, 0, 0, time.UTC)
	// 返回完整合法快照，并保留旧错误与业务来源用于检查 map 复制和清理范围。
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
		ChunkStrategy: "structure_aware",
		ChunkConfig:   map[string]any{"targetChars": 800},
		Metadata:      map[string]any{"source": "manual", "lastError": "old error"},
		CreateTime:    createdAt,
		UpdateTime:    createdAt,
	}
}
