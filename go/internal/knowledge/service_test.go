package knowledge

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/embedding"
)

func TestUploadDocumentStoresPendingDocument(t *testing.T) {
	repo := newFakeRepository()
	storage := newFakeStorage()
	service := NewService(repo, storage, embedding.NewFakeEmbedder(1536), config.DocumentConfig{
		AllowedExtensions: map[string]struct{}{"txt": {}},
		MaxUploadBytes:    1024,
	})

	doc, err := service.UploadDocument(context.Background(), UploadInput{
		KnowledgeBaseID: "kb1",
		FileName:        "guide.txt",
		Title:           "guide",
		ContentType:     "text/plain",
		Content:         bytes.NewReader([]byte("hello travel")),
		Size:            int64(len("hello travel")),
	})
	if err != nil {
		t.Fatalf("UploadDocument returned error: %v", err)
	}
	if doc.Status != DocumentStatusPending {
		t.Fatalf("status = %q, want pending", doc.Status)
	}
	if doc.ContentHash == "" {
		t.Fatalf("content hash should be set")
	}
	if storage.putCount != 1 {
		t.Fatalf("storage put count = %d, want 1", storage.putCount)
	}
}

func TestUploadDocumentRejectsDuplicateBeforeStorage(t *testing.T) {
	repo := newFakeRepository()
	repo.duplicate = true
	storage := newFakeStorage()
	service := NewService(repo, storage, embedding.NewFakeEmbedder(1536), config.DocumentConfig{
		AllowedExtensions: map[string]struct{}{"txt": {}},
		MaxUploadBytes:    1024,
	})

	_, err := service.UploadDocument(context.Background(), UploadInput{
		KnowledgeBaseID: "kb1",
		FileName:        "guide.txt",
		Content:         bytes.NewReader([]byte("hello travel")),
		Size:            int64(len("hello travel")),
	})
	if err == nil {
		t.Fatalf("UploadDocument should reject duplicate content")
	}
	if storage.putCount != 0 {
		t.Fatalf("storage put count = %d, want 0", storage.putCount)
	}
}

func TestUploadDocumentCompensatesStorageWhenCreateFails(t *testing.T) {
	repo := newFakeRepository()
	repo.createErr = errors.New("insert failed")
	storage := newFakeStorage()
	service := NewService(repo, storage, embedding.NewFakeEmbedder(1536), config.DocumentConfig{
		AllowedExtensions: map[string]struct{}{"txt": {}},
		MaxUploadBytes:    1024,
	})

	_, err := service.UploadDocument(context.Background(), UploadInput{
		KnowledgeBaseID: "kb1",
		FileName:        "guide.txt",
		Content:         bytes.NewReader([]byte("hello travel")),
		Size:            int64(len("hello travel")),
	})
	if err == nil {
		t.Fatalf("UploadDocument should return create error")
	}
	if storage.deleteCount != 1 {
		t.Fatalf("storage delete count = %d, want 1", storage.deleteCount)
	}
}

func TestStartChunkReplacesChunksAndMarksCompleted(t *testing.T) {
	repo := newFakeRepository()
	doc := Document{
		ID:         "doc1",
		KbID:       "kb1",
		SourceURI:  "memory://doc1",
		FileName:   "guide.txt",
		FileType:   "txt",
		Status:     DocumentStatusPending,
		Metadata:   map[string]any{"lastError": "old error", "keep": "value"},
		ChunkCount: 0,
	}
	repo.documents[doc.ID] = doc
	storage := newFakeStorage()
	storage.objects[doc.SourceURI] = []byte("第一段内容。\n\n第二段内容。")

	service := NewService(repo, storage, embedding.NewFakeEmbedder(1536), config.DocumentConfig{
		AllowedExtensions: map[string]struct{}{"txt": {}},
		MaxUploadBytes:    1024,
	})

	completed, err := service.StartChunk(context.Background(), "doc1", ChunkOptions{
		TargetChars: 8,
		MaxChars:    16,
	})
	if err != nil {
		t.Fatalf("StartChunk returned error: %v", err)
	}
	if completed.Status != DocumentStatusCompleted {
		t.Fatalf("status = %q, want completed", completed.Status)
	}
	if completed.ChunkCount == 0 {
		t.Fatalf("chunk count should be greater than zero")
	}
	if _, ok := completed.Metadata["lastError"]; ok {
		t.Fatalf("lastError should be cleared on success")
	}
	if completed.Metadata["keep"] != "value" {
		t.Fatalf("existing metadata should be preserved")
	}
	if repo.replaceCount != 1 {
		t.Fatalf("replace count = %d, want 1", repo.replaceCount)
	}
}

func TestStartChunkRecordsLatestErrorWhenEmbeddingFails(t *testing.T) {
	repo := newFakeRepository()
	doc := Document{
		ID:        "doc1",
		KbID:      "kb1",
		SourceURI: "memory://doc1",
		FileName:  "guide.txt",
		FileType:  "txt",
		Status:    DocumentStatusPending,
		Metadata:  map[string]any{"keep": "value"},
	}
	repo.documents[doc.ID] = doc
	storage := newFakeStorage()
	storage.objects[doc.SourceURI] = []byte("第一段内容。")

	service := NewService(repo, storage, failingEmbedder{}, config.DocumentConfig{
		AllowedExtensions: map[string]struct{}{"txt": {}},
		MaxUploadBytes:    1024,
	})

	_, err := service.StartChunk(context.Background(), "doc1", ChunkOptions{TargetChars: 8, MaxChars: 16})
	if err == nil {
		t.Fatalf("StartChunk should return embedding error")
	}
	failed := repo.documents["doc1"]
	if failed.Status != DocumentStatusFailed {
		t.Fatalf("status = %q, want failed", failed.Status)
	}
	if failed.Metadata["lastError"] == "" {
		t.Fatalf("lastError should be recorded")
	}
	if repo.replaceCount != 0 {
		t.Fatalf("replace count = %d, want 0", repo.replaceCount)
	}
}

type fakeRepository struct {
	duplicate    bool
	createErr    error
	documents    map[string]Document
	replaceCount int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{documents: map[string]Document{}}
}

func (r *fakeRepository) KnowledgeBaseExists(context.Context, string) (bool, error) {
	return true, nil
}

func (r *fakeRepository) ActiveDocumentHashExists(context.Context, string, string) (bool, error) {
	return r.duplicate, nil
}

func (r *fakeRepository) CreateDocument(_ context.Context, doc Document) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.documents[doc.ID] = doc
	return nil
}

func (r *fakeRepository) GetDocument(_ context.Context, docID string) (Document, error) {
	doc, ok := r.documents[docID]
	if !ok {
		return Document{}, ErrNotFound
	}
	return doc, nil
}

func (r *fakeRepository) ListDocuments(_ context.Context, kbID string, _ int, _ int) ([]Document, int64, error) {
	var docs []Document
	for _, doc := range r.documents {
		if doc.KbID == kbID {
			docs = append(docs, doc)
		}
	}
	return docs, int64(len(docs)), nil
}

func (r *fakeRepository) DeleteDocument(_ context.Context, docID string) error {
	if _, ok := r.documents[docID]; !ok {
		return ErrNotFound
	}
	delete(r.documents, docID)
	return nil
}

func (r *fakeRepository) TryMarkProcessing(_ context.Context, docID string) (Document, bool, error) {
	doc, ok := r.documents[docID]
	if !ok {
		return Document{}, false, ErrNotFound
	}
	if doc.Status == DocumentStatusProcessing {
		return doc, false, nil
	}
	doc.Status = DocumentStatusProcessing
	r.documents[docID] = doc
	return doc, true, nil
}

func (r *fakeRepository) ReplaceDocumentChunks(_ context.Context, doc Document, chunks []Chunk, vectors [][]float32) error {
	r.replaceCount++
	doc.Status = DocumentStatusCompleted
	doc.ChunkCount = len(chunks)
	delete(doc.Metadata, "lastError")
	r.documents[doc.ID] = doc
	return nil
}

func (r *fakeRepository) MarkFailed(_ context.Context, doc Document, message string) error {
	doc.Status = DocumentStatusFailed
	if doc.Metadata == nil {
		doc.Metadata = map[string]any{}
	}
	doc.Metadata["lastError"] = message
	r.documents[doc.ID] = doc
	return nil
}

type fakeStorage struct {
	objects     map[string][]byte
	putCount    int
	deleteCount int
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{objects: map[string][]byte{}}
}

func (s *fakeStorage) Put(_ context.Context, input StoredObjectInput) (StoredObject, error) {
	s.putCount++
	uri := "memory://" + input.FileName
	s.objects[uri] = input.Content
	return StoredObject{URI: uri, FileName: input.FileName, ContentType: input.ContentType, Size: int64(len(input.Content))}, nil
}

func (s *fakeStorage) Get(_ context.Context, uri string) ([]byte, error) {
	content, ok := s.objects[uri]
	if !ok {
		return nil, ErrNotFound
	}
	return content, nil
}

func (s *fakeStorage) Delete(context.Context, string) error {
	s.deleteCount++
	return nil
}

type failingEmbedder struct{}

func (failingEmbedder) EmbedTexts(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("embedding failed")
}
