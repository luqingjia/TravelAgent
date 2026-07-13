package knowledge

import (
	"context"
	"errors"
	"time"
)

const (
	DocumentStatusPending    = "pending"
	DocumentStatusProcessing = "processing"
	DocumentStatusCompleted  = "completed"
	DocumentStatusFailed     = "failed"

	SourceTypeFile       = "file"
	DefaultLanguage      = "zh"
	DefaultChunkStrategy = "structure_aware"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrDuplicate       = errors.New("同一知识库已存在相同内容的文档")
	ErrAlreadyRunning  = errors.New("文档正在处理中")
	ErrInvalidArgument = errors.New("invalid argument")
)

type Document struct {
	ID            string         `db:"id" json:"id"`
	KbID          string         `db:"kb_id" json:"kbId"`
	Title         string         `db:"title" json:"title"`
	SourceType    string         `db:"source_type" json:"sourceType"`
	SourceURI     string         `db:"source_uri" json:"sourceUri"`
	FileName      string         `db:"file_name" json:"fileName"`
	FileType      string         `db:"file_type" json:"fileType"`
	FileSize      int64          `db:"file_size" json:"fileSize"`
	ContentHash   string         `db:"content_hash" json:"contentHash"`
	Language      string         `db:"language" json:"language"`
	Status        string         `db:"status" json:"status"`
	ChunkCount    int            `db:"chunk_count" json:"chunkCount"`
	ChunkStrategy string         `db:"chunk_strategy" json:"chunkStrategy"`
	ChunkConfig   map[string]any `db:"-" json:"chunkConfig"`
	Metadata      map[string]any `db:"-" json:"metadata"`
	CreateTime    time.Time      `db:"create_time" json:"createTime"`
	UpdateTime    time.Time      `db:"update_time" json:"updateTime"`
}

type Chunk struct {
	ID            string         `json:"id"`
	KbID          string         `json:"kbId"`
	DocumentID    string         `json:"documentId"`
	Index         int            `json:"chunkIndex"`
	Content       string         `json:"content"`
	TokenCount    int            `json:"tokenCount"`
	CharCount     int            `json:"charCount"`
	StartPosition int            `json:"startPosition"`
	EndPosition   int            `json:"endPosition"`
	Metadata      map[string]any `json:"metadata"`
}

type ChunkOptions struct {
	MinChars    int `json:"minChars"`
	TargetChars int `json:"targetChars"`
	MaxChars    int `json:"maxChars"`
}

type UploadInput struct {
	KnowledgeBaseID string
	FileName        string
	Title           string
	ContentType     string
	Language        string
	ChunkStrategy   string
	ChunkConfig     map[string]any
	Metadata        map[string]any
	Content         interface {
		Read([]byte) (int, error)
	}
	Size int64
}

type StoredObjectInput struct {
	FileName    string
	ContentType string
	Content     []byte
}

type StoredObject struct {
	URI         string
	FileName    string
	ContentType string
	Size        int64
}

type Repository interface {
	KnowledgeBaseExists(ctx context.Context, kbID string) (bool, error)
	ActiveDocumentHashExists(ctx context.Context, kbID string, contentHash string) (bool, error)
	CreateDocument(ctx context.Context, doc Document) error
	GetDocument(ctx context.Context, docID string) (Document, error)
	ListDocuments(ctx context.Context, kbID string, page int, size int) ([]Document, int64, error)
	DeleteDocument(ctx context.Context, docID string) error
	TryMarkProcessing(ctx context.Context, docID string) (Document, bool, error)
	ReplaceDocumentChunks(ctx context.Context, doc Document, chunks []Chunk, vectors [][]float32) error
	MarkFailed(ctx context.Context, doc Document, message string) error
}

type Storage interface {
	Put(ctx context.Context, input StoredObjectInput) (StoredObject, error)
	Get(ctx context.Context, uri string) ([]byte, error)
	Delete(ctx context.Context, uri string) error
}
