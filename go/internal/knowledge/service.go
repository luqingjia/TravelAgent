package knowledge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/embedding"
)

type Service struct {
	repo           Repository
	storage        Storage
	embedder       embedding.Embedder
	documentConfig config.DocumentConfig
}

func NewService(repo Repository, storage Storage, embedder embedding.Embedder, documentConfig config.DocumentConfig) *Service {
	return &Service{
		repo:           repo,
		storage:        storage,
		embedder:       embedder,
		documentConfig: documentConfig,
	}
}

func (s *Service) UploadDocument(ctx context.Context, input UploadInput) (Document, error) {
	if strings.TrimSpace(input.KnowledgeBaseID) == "" {
		return Document{}, fmt.Errorf("%w: knowledge base id is empty", ErrInvalidArgument)
	}
	if input.Content == nil {
		return Document{}, fmt.Errorf("%w: file content is empty", ErrInvalidArgument)
	}
	if input.Size <= 0 {
		return Document{}, fmt.Errorf("%w: file is empty", ErrInvalidArgument)
	}
	if input.Size > s.documentConfig.MaxUploadBytes {
		return Document{}, fmt.Errorf("%w: file exceeds max upload size", ErrInvalidArgument)
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.FileName)), ".")
	if !s.documentConfig.IsExtensionAllowed(extension) {
		return Document{}, fmt.Errorf("%w: extension %q is not allowed", ErrInvalidArgument, extension)
	}
	exists, err := s.repo.KnowledgeBaseExists(ctx, input.KnowledgeBaseID)
	if err != nil {
		return Document{}, err
	}
	if !exists {
		return Document{}, ErrNotFound
	}

	content, err := io.ReadAll(io.LimitReader(input.Content, s.documentConfig.MaxUploadBytes+1))
	if err != nil {
		return Document{}, err
	}
	if int64(len(content)) != input.Size && input.Size >= 0 {
		input.Size = int64(len(content))
	}
	if int64(len(content)) > s.documentConfig.MaxUploadBytes {
		return Document{}, fmt.Errorf("%w: file exceeds max upload size", ErrInvalidArgument)
	}
	contentHash := sha256Hex(content)
	duplicate, err := s.repo.ActiveDocumentHashExists(ctx, input.KnowledgeBaseID, contentHash)
	if err != nil {
		return Document{}, err
	}
	if duplicate {
		return Document{}, ErrDuplicate
	}

	stored, err := s.storage.Put(ctx, StoredObjectInput{
		FileName:    input.FileName,
		ContentType: input.ContentType,
		Content:     content,
	})
	if err != nil {
		return Document{}, err
	}

	now := time.Now()
	metadata := cloneMap(input.Metadata)
	metadata["storedFileName"] = stored.FileName
	metadata["storedContentType"] = stored.ContentType
	document := Document{
		ID:            newID(),
		KbID:          input.KnowledgeBaseID,
		Title:         defaultString(input.Title, input.FileName),
		SourceType:    SourceTypeFile,
		SourceURI:     stored.URI,
		FileName:      input.FileName,
		FileType:      defaultString(extension, stored.ContentType),
		FileSize:      stored.Size,
		ContentHash:   contentHash,
		Language:      defaultString(input.Language, DefaultLanguage),
		Status:        DocumentStatusPending,
		ChunkCount:    0,
		ChunkStrategy: defaultString(input.ChunkStrategy, DefaultChunkStrategy),
		ChunkConfig:   cloneMap(input.ChunkConfig),
		Metadata:      metadata,
		CreateTime:    now,
		UpdateTime:    now,
	}
	if err := s.repo.CreateDocument(ctx, document); err != nil {
		_ = s.storage.Delete(ctx, stored.URI)
		return Document{}, err
	}
	return document, nil
}

func (s *Service) StartChunk(ctx context.Context, docID string, options ChunkOptions) (Document, error) {
	document, acquired, err := s.repo.TryMarkProcessing(ctx, docID)
	if err != nil {
		return Document{}, err
	}
	if !acquired {
		return Document{}, ErrAlreadyRunning
	}

	completed, err := s.startChunkAfterAcquire(ctx, document, options)
	if err != nil {
		_ = s.repo.MarkFailed(ctx, document, err.Error())
		return Document{}, err
	}
	return completed, nil
}

func (s *Service) startChunkAfterAcquire(ctx context.Context, document Document, options ChunkOptions) (Document, error) {
	content, err := s.storage.Get(ctx, document.SourceURI)
	if err != nil {
		return Document{}, err
	}
	text, err := ExtractText(content, document.FileType, document.FileName)
	if err != nil {
		return Document{}, err
	}
	chunks := ChunkText(text, options)
	if len(chunks) == 0 {
		return Document{}, fmt.Errorf("parsed chunks are empty")
	}
	for i := range chunks {
		chunks[i].ID = newID()
		chunks[i].KbID = document.KbID
		chunks[i].DocumentID = document.ID
		chunks[i].TokenCount = chunks[i].CharCount
	}
	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Content
	}
	vectors, err := s.embedder.EmbedTexts(ctx, texts)
	if err != nil {
		return Document{}, err
	}
	if len(vectors) != len(chunks) {
		return Document{}, fmt.Errorf("embedding count %d does not match chunk count %d", len(vectors), len(chunks))
	}
	for _, vector := range vectors {
		if err := embedding.ValidateDimensions(vector, 1536); err != nil {
			return Document{}, err
		}
	}
	document.Metadata = cloneMap(document.Metadata)
	delete(document.Metadata, "lastError")
	document.Status = DocumentStatusCompleted
	document.ChunkCount = len(chunks)
	document.UpdateTime = time.Now()
	if err := s.repo.ReplaceDocumentChunks(ctx, document, chunks, vectors); err != nil {
		return Document{}, err
	}
	return document, nil
}

func (s *Service) GetDocument(ctx context.Context, docID string) (Document, error) {
	return s.repo.GetDocument(ctx, docID)
}

func (s *Service) ListDocuments(ctx context.Context, kbID string, page int, size int) ([]Document, int64, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	return s.repo.ListDocuments(ctx, kbID, page, size)
}

func (s *Service) DeleteDocument(ctx context.Context, docID string) error {
	return s.repo.DeleteDocument(ctx, docID)
}

func ExtractText(content []byte, fileType string, fileName string) (string, error) {
	ext := strings.ToLower(strings.TrimPrefix(fileType, "."))
	if ext == "" {
		ext = strings.ToLower(strings.TrimPrefix(filepath.Ext(fileName), "."))
	}
	switch ext {
	case "txt", "md", "markdown":
		return string(content), nil
	case "html", "htm":
		return stripHTML(string(content)), nil
	case "pdf", "doc", "docx":
		return "", fmt.Errorf("%s parsing is not implemented in Go MVP", ext)
	default:
		return string(content), nil
	}
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func newID() string {
	var random [4]byte
	_, _ = rand.Read(random[:])
	return fmt.Sprintf("%d%s", time.Now().UnixNano(), hex.EncodeToString(random[:]))
}

func cloneMap(input map[string]any) map[string]any {
	output := map[string]any{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

func stripHTML(input string) string {
	withoutTags := htmlTagPattern.ReplaceAllString(input, " ")
	return strings.Join(strings.Fields(withoutTags), " ")
}
