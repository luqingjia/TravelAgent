package application

import (
	"context"
	"io"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// DocumentRepository 是知识文档用例真正需要的持久化能力集合。
//
// 接口定义在使用方 application，而不是 PostgreSQL 包中。这样应用层只依赖自己需要的
// 小合同，数据库适配器负责实现它，测试也可以直接传入内存 fake。
type DocumentRepository interface {
	KnowledgeBaseExists(ctx context.Context, kbID string) (bool, error)
	ActiveDocumentHashExists(ctx context.Context, kbID string, contentHash string) (bool, error)
	CreateDocument(ctx context.Context, document domain.Document) error
	GetDocument(ctx context.Context, docID string) (domain.Document, error)
	ListDocuments(ctx context.Context, kbID string, page int, size int) ([]domain.Document, int64, error)
	DeleteDocument(ctx context.Context, docID string) error
	TryMarkProcessing(ctx context.Context, docID string) (domain.Document, bool, error)
	ReplaceDocumentChunks(ctx context.Context, document domain.Document, chunks []domain.Chunk, vectors [][]float32) error
	MarkFailed(ctx context.Context, document domain.Document) error
}

// StoredObjectInput 是应用层交给对象存储的完整文件内容。
// FileName 和 ContentType 用于保留上传语义，Content 是经过大小限制后读出的安全字节切片。
type StoredObjectInput struct {
	FileName    string
	ContentType string
	Content     []byte
}

// StoredObject 是对象存储成功后返回给应用层的稳定结果。
// URI 是后续读取和删除的唯一定位信息，其余字段用于形成文档记录和元数据。
type StoredObject struct {
	URI         string
	FileName    string
	ContentType string
	Size        int64
}

// ObjectStorage 隔离本地文件系统与 S3/RustFS 的实现差异。
type ObjectStorage interface {
	Put(ctx context.Context, input StoredObjectInput) (StoredObject, error)
	Get(ctx context.Context, uri string) ([]byte, error)
	Delete(ctx context.Context, uri string) error
}

// Embedder 表示把一批文本转换成一批向量的最小能力。
// 应用层会额外验证返回数量和维度，不能假设外部模型永远遵守合同。
type Embedder interface {
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
}

// UploadInput 是上传用例接收的框架无关输入。
// HTTP 适配器负责从 multipart 请求构造它，应用层不直接依赖 gin.Context。
type UploadInput struct {
	KnowledgeBaseID string
	FileName        string
	Title           string
	ContentType     string
	Language        string
	ChunkStrategy   string
	ChunkConfig     map[string]any
	Metadata        map[string]any
	Content         io.Reader
	Size            int64
}
