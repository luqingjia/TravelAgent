package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// TestUploadDocumentRejectsInvalidInputBeforeStorage 验证便宜的输入错误必须在调用对象存储前被拦截。
func TestUploadDocumentRejectsInvalidInputBeforeStorage(t *testing.T) {
	tests := []struct {
		name     string
		input    UploadInput
		maxBytes int64
	}{
		{
			name:     "知识库编号为空",
			input:    UploadInput{FileName: "guide.txt", Content: bytes.NewReader([]byte("hello")), Size: 5},
			maxBytes: 16,
		},
		{
			name:     "文件读取器为空",
			input:    UploadInput{KnowledgeBaseID: "kb-1", FileName: "guide.txt", Size: 5},
			maxBytes: 16,
		},
		{
			name:     "声明为空文件",
			input:    UploadInput{KnowledgeBaseID: "kb-1", FileName: "guide.txt", Content: bytes.NewReader(nil), Size: 0},
			maxBytes: 16,
		},
		{
			name:     "声明大小超限",
			input:    UploadInput{KnowledgeBaseID: "kb-1", FileName: "guide.txt", Content: bytes.NewReader([]byte("hello")), Size: 17},
			maxBytes: 16,
		},
		{
			name:     "扩展名不允许",
			input:    UploadInput{KnowledgeBaseID: "kb-1", FileName: "guide.exe", Content: bytes.NewReader([]byte("hello")), Size: 5},
			maxBytes: 16,
		},
		{
			name:     "真实内容超过声明大小和上限",
			input:    UploadInput{KnowledgeBaseID: "kb-1", FileName: "guide.txt", Content: bytes.NewReader([]byte("123456789")), Size: 1},
			maxBytes: 8,
		},
		{
			name:     "读取结果实际为空",
			input:    UploadInput{KnowledgeBaseID: "kb-1", FileName: "guide.txt", Content: bytes.NewReader(nil), Size: 1},
			maxBytes: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 准备：知识库存在，确保失败只由当前输入触发；存储 fake 用调用记录证明有没有被越过。
			service, _, storage := newUploadService(t, tt.maxBytes)

			// 执行：调用上传用例。
			_, err := service.UploadDocument(context.Background(), tt.input)

			// 断言：错误属于参数错误，并且对象存储完全没有收到 Put。
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("UploadDocument() error = %v, want ErrInvalidArgument", err)
			}
			if len(storage.putInputs) != 0 {
				t.Fatalf("storage Put calls = %d, want 0", len(storage.putInputs))
			}
		})
	}
}

// TestUploadDocumentChecksKnowledgeBaseAndDuplicateBeforeStorage 验证不存在和重复内容都不会产生孤儿对象。
func TestUploadDocumentChecksKnowledgeBaseAndDuplicateBeforeStorage(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*fakeRepository)
		want      error
	}{
		{
			name: "知识库不存在",
			configure: func(repo *fakeRepository) {
				repo.knowledgeBaseExists = false
			},
			want: domain.ErrNotFound,
		},
		{
			name: "内容哈希重复",
			configure: func(repo *fakeRepository) {
				repo.knowledgeBaseExists = true
				repo.duplicate = true
			},
			want: domain.ErrDuplicate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, repo, storage := newUploadService(t, 1024)
			tt.configure(repo)

			_, err := service.UploadDocument(context.Background(), validUploadInput())

			if !errors.Is(err, tt.want) {
				t.Fatalf("UploadDocument() error = %v, want %v", err, tt.want)
			}
			if len(storage.putInputs) != 0 {
				t.Fatalf("storage Put calls = %d, want 0", len(storage.putInputs))
			}
		})
	}
}

// TestUploadDocumentCreatesPendingDocument 验证合法上传只创建 pending 文档，不在上传请求中偷偷开始分块。
func TestUploadDocumentCreatesPendingDocument(t *testing.T) {
	service, repo, storage := newUploadService(t, 1024)
	storage.putResult = StoredObject{
		URI:         "s3://travel-agent/2026/guide.txt",
		FileName:    "guide.txt",
		ContentType: "text/plain",
		Size:        int64(len("hello travel")),
	}

	document, err := service.UploadDocument(context.Background(), validUploadInput())
	if err != nil {
		t.Fatalf("UploadDocument() error = %v", err)
	}

	// 关键断言：领域构造器设置 pending/0，固定 ID 和时钟证明 Service 使用的是注入依赖。
	if document.ID != "fixed-id" || document.Status != domain.StatusPending || document.ChunkCount != 0 {
		t.Fatalf("document = %#v", document)
	}
	if !document.CreateTime.Equal(fixedNow) || !document.UpdateTime.Equal(fixedNow) {
		t.Fatalf("document times = (%v, %v)", document.CreateTime, document.UpdateTime)
	}

	// 内容哈希必须按真实字节计算；文件名相同但内容不同的文档不能被误判为重复。
	hash := sha256.Sum256([]byte("hello travel"))
	wantHash := hex.EncodeToString(hash[:])
	if document.ContentHash != wantHash {
		t.Fatalf("content hash = %q, want %q", document.ContentHash, wantHash)
	}
	if document.Language != domain.DefaultLanguage || document.ChunkStrategy != domain.DefaultChunkStrategy {
		t.Fatalf("defaults = language %q, strategy %q", document.Language, document.ChunkStrategy)
	}
	if document.Metadata["storedFileName"] != "guide.txt" || document.Metadata["storedContentType"] != "text/plain" {
		t.Fatalf("metadata = %#v", document.Metadata)
	}
	if len(storage.putInputs) != 1 || len(repo.createdDocuments) != 1 {
		t.Fatalf("Put calls = %d, Create calls = %d", len(storage.putInputs), len(repo.createdDocuments))
	}
}

// TestUploadDocumentCompensatesStorageWhenCreateFails 验证数据库写入失败后会尽力删除已经上传的对象。
func TestUploadDocumentCompensatesStorageWhenCreateFails(t *testing.T) {
	service, repo, storage := newUploadService(t, 1024)
	insertErr := errors.New("insert failed")
	repo.createErr = insertErr
	storage.putResult = StoredObject{
		URI:         "s3://travel-agent/orphan.txt",
		FileName:    "guide.txt",
		ContentType: "text/plain",
		Size:        int64(len("hello travel")),
	}
	// 同时让补偿失败，证明补偿错误只记录日志，不会盖住真正需要返回给调用方的数据库错误。
	storage.deleteErr = errors.New("cleanup failed")

	_, err := service.UploadDocument(context.Background(), validUploadInput())

	if !errors.Is(err, insertErr) {
		t.Fatalf("UploadDocument() error = %v, want original insert error", err)
	}
	if len(storage.deleteURIs) != 1 || storage.deleteURIs[0] != "s3://travel-agent/orphan.txt" {
		t.Fatalf("storage Delete URIs = %#v", storage.deleteURIs)
	}
}

// newUploadService 创建上传测试专用的 Service，并返回可检查调用记录的 fake。
func newUploadService(t *testing.T, maxBytes int64) (*Service, *fakeRepository, *fakeStorage) {
	t.Helper()
	deps := validDependencies()
	repo := deps.Repository.(*fakeRepository)
	repo.knowledgeBaseExists = true
	storage := deps.Storage.(*fakeStorage)
	deps.Policy.MaxUploadBytes = maxBytes

	service, err := NewService(deps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service, repo, storage
}

// validUploadInput 返回一个每次都使用全新 Reader 的合法上传请求。
func validUploadInput() UploadInput {
	content := []byte("hello travel")
	return UploadInput{
		KnowledgeBaseID: "kb-1",
		FileName:        "guide.txt",
		Title:           "旅行指南",
		ContentType:     "text/plain",
		Metadata:        map[string]any{"source": "manual"},
		Content:         bytes.NewReader(content),
		Size:            int64(len(content)),
	}
}
