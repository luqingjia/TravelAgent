package application

import (
	"context"
	"io"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// 本文件是应用层与外部世界之间的合同清单。接口只描述业务用例真正会调用的方法，
// PostgreSQL、S3、本地文件系统和 Embedding 客户端都在外层实现这些合同。
// DocumentRepository 是知识文档用例真正需要的持久化能力集合。
//
// 接口定义在使用方 application，而不是 PostgreSQL 包中。这样应用层只依赖自己需要的
// 小合同，数据库适配器负责实现它，测试也可以直接传入内存 fake。
type DocumentRepository interface {
	// KnowledgeBaseExists 检查目标知识库是否存在，上传前用它阻止孤儿文档。
	KnowledgeBaseExists(ctx context.Context, kbID string) (bool, error)
	// ActiveDocumentHashExists 检查同一知识库是否已有相同内容的未删除文档。
	ActiveDocumentHashExists(ctx context.Context, kbID string, contentHash string) (bool, error)
	// CreateDocument 持久化一个已经通过领域校验的 pending 文档。
	CreateDocument(ctx context.Context, document domain.Document) error
	// GetDocument 按 ID 读取一个未删除文档并恢复成领域对象。
	GetDocument(ctx context.Context, docID string) (domain.Document, error)
	// ListDocuments 返回当前页文档和总数，分页 SQL 由具体仓储实现。
	ListDocuments(ctx context.Context, kbID string, page int, size int) ([]domain.Document, int64, error)
	// DeleteDocument 执行当前约定的逻辑删除。
	DeleteDocument(ctx context.Context, docID string) error
	// TryMarkProcessing 使用原子条件更新抢占处理权，并告诉应用层是否抢占成功。
	TryMarkProcessing(ctx context.Context, docID string) (domain.Document, bool, error)
	// ReplaceDocumentChunks 在一个短事务中替换旧分块、旧向量和文档完成状态。
	ReplaceDocumentChunks(ctx context.Context, document domain.Document, chunks []domain.Chunk, vectors [][]float32) error
	// MarkFailed 保存失败后的完整领域对象，同时保留原有 metadata。
	MarkFailed(ctx context.Context, document domain.Document) error
}

// StoredObjectInput 是应用层交给对象存储的完整文件内容。
// FileName 和 ContentType 用于保留上传语义，Content 是经过大小限制后读出的安全字节切片。
type StoredObjectInput struct {
	// FileName 是用户上传时的原始文件名，用于生成安全对象名和保留展示信息。
	FileName string
	// ContentType 是 HTTP 上传携带的媒体类型，存储实现可以把它写入对象元数据。
	ContentType string
	// Content 是应用层已经完成大小限制后读取出的完整内容。
	Content []byte
}

// StoredObject 是对象存储成功后返回给应用层的稳定结果。
// URI 是后续读取和删除的唯一定位信息，其余字段用于形成文档记录和元数据。
type StoredObject struct {
	// URI 是应用层后续读取和删除对象时使用的稳定定位符。
	URI string
	// FileName 是存储实现最终确认的文件名。
	FileName string
	// ContentType 是存储对象对应的媒体类型。
	ContentType string
	// Size 是实际写入的字节数，不能盲目信任客户端声明大小。
	Size int64
}

// ObjectStorage 隔离本地文件系统与 S3/RustFS 的实现差异。
type ObjectStorage interface {
	// Put 写入一个完整对象，失败时不能返回可继续使用的 URI。
	Put(ctx context.Context, input StoredObjectInput) (StoredObject, error)
	// Get 根据稳定 URI 读取原始字节，供显式分块流程使用。
	Get(ctx context.Context, uri string) ([]byte, error)
	// Delete 删除指定对象，主要用于数据库创建失败后的补偿。
	Delete(ctx context.Context, uri string) error
}

// Embedder 表示把一批文本转换成一批向量的最小能力。
// 应用层会额外验证返回数量和维度，不能假设外部模型永远遵守合同。
type Embedder interface {
	// EmbedTexts 按输入顺序返回向量；应用层还会检查数量和 1536 维约束。
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
}

// UploadInput 是上传用例接收的框架无关输入。
// HTTP 适配器负责从 multipart 请求构造它，应用层不直接依赖 gin.Context。
type UploadInput struct {
	// KnowledgeBaseID 指定文档要归属的知识库。
	KnowledgeBaseID string
	// FileName 是 multipart 上传中的原始文件名。
	FileName string
	// Title 是对外展示标题；为空时应用层会回退到文件名。
	Title string
	// ContentType 是上传请求声明的媒体类型。
	ContentType string
	// Language 是文档语言；为空时使用领域默认值。
	Language string
	// ChunkStrategy 保存当前兼容的分块策略名称。
	ChunkStrategy string
	// ChunkConfig 保存客户端附带的分块扩展配置。
	ChunkConfig map[string]any
	// Metadata 保存客户端附带的业务扩展信息。
	Metadata map[string]any
	// Content 提供文件字节流，应用层通过限长读取防止超大上传。
	Content io.Reader
	// Size 是 multipart 头声明的大小，只用于提前拒绝，真实大小仍以读取结果为准。
	Size int64
}
