package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// UploadDocument 校验并保存一个新知识文档，但不会在上传请求中执行解析、分块或向量化。
//
// 上传和处理分成两个明确用例：上传成功只返回 pending 文档；调用方之后再显式请求 ProcessDocument。
// 这样大文件解析或模型超时不会让上传接口长时间占用连接，也便于失败后重试处理。
func (s *Service) UploadDocument(ctx context.Context, input UploadInput) (domain.Document, error) {
	// 第一步先校验完全不需要外部 I/O 的字段。越早拒绝无效请求，越不会产生数据库查询或孤儿对象。
	kbID := strings.TrimSpace(input.KnowledgeBaseID)
	if kbID == "" {
		return domain.Document{}, fmt.Errorf("%w: knowledge base id is empty", domain.ErrInvalidArgument)
	}
	if input.Content == nil {
		return domain.Document{}, fmt.Errorf("%w: file content is empty", domain.ErrInvalidArgument)
	}
	if input.Size <= 0 {
		return domain.Document{}, fmt.Errorf("%w: file is empty", domain.ErrInvalidArgument)
	}
	if input.Size > s.policy.MaxUploadBytes {
		return domain.Document{}, fmt.Errorf("%w: file exceeds max upload size", domain.ErrInvalidArgument)
	}

	// filepath.Ext 只取最后一个扩展名；统一转小写并去掉点号后再与策略集合比较。
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.FileName)), ".")
	if !s.policy.AllowsExtension(extension) {
		return domain.Document{}, fmt.Errorf("%w: extension %q is not allowed", domain.ErrInvalidArgument, extension)
	}

	// 文件只有归属于真实知识库才有意义，因此在上传对象前先确认知识库存在。
	exists, err := s.repo.KnowledgeBaseExists(ctx, kbID)
	if err != nil {
		return domain.Document{}, fmt.Errorf("check knowledge base: %w", err)
	}
	if !exists {
		return domain.Document{}, domain.ErrNotFound
	}

	// 不能只相信 multipart 头里的 Size，因为客户端可以伪造它。
	// LimitReader 最多放行“上限 + 1”字节：多出来的一个字节专门用于判断真实内容是否超限，又不会把整个大文件读进内存。
	content, err := io.ReadAll(io.LimitReader(input.Content, s.policy.MaxUploadBytes+1))
	if err != nil {
		return domain.Document{}, fmt.Errorf("read uploaded file: %w", err)
	}
	if len(content) == 0 {
		return domain.Document{}, fmt.Errorf("%w: file is empty", domain.ErrInvalidArgument)
	}
	if int64(len(content)) > s.policy.MaxUploadBytes {
		return domain.Document{}, fmt.Errorf("%w: file exceeds max upload size", domain.ErrInvalidArgument)
	}

	// 重复身份由“知识库 ID + 文件真实字节的 SHA-256”决定，文件名相同不代表内容相同。
	contentHash := sha256Hex(content)
	duplicate, err := s.repo.ActiveDocumentHashExists(ctx, kbID, contentHash)
	if err != nil {
		return domain.Document{}, fmt.Errorf("check duplicate document: %w", err)
	}
	if duplicate {
		return domain.Document{}, domain.ErrDuplicate
	}

	// 通过所有便宜校验后才写对象存储，减少无效请求制造垃圾文件的机会。
	stored, err := s.storage.Put(ctx, StoredObjectInput{
		FileName:    input.FileName,
		ContentType: input.ContentType,
		Content:     content,
	})
	if err != nil {
		return domain.Document{}, fmt.Errorf("store uploaded file: %w", err)
	}

	// metadata 需要补充存储返回的信息。先复制调用方 map，避免下面的赋值反向修改 HTTP 请求对象。
	metadata := make(map[string]any, len(input.Metadata)+2)
	for key, value := range input.Metadata {
		metadata[key] = value
	}
	metadata["storedFileName"] = stored.FileName
	metadata["storedContentType"] = stored.ContentType

	// 某些兼容存储实现可能不回填 Size；真实内容长度是可信的兜底值。
	storedSize := stored.Size
	if storedSize <= 0 {
		storedSize = int64(len(content))
	}

	// 状态、分块数量和时间不由应用层手工填写，而是交给领域构造函数统一建立合法的 pending 聚合。
	document, err := domain.NewPendingDocument(domain.NewDocument{
		ID:            s.newID(),
		KbID:          kbID,
		Title:         firstNonBlank(input.Title, input.FileName),
		SourceType:    domain.SourceTypeFile,
		SourceURI:     stored.URI,
		FileName:      input.FileName,
		FileType:      firstNonBlank(extension, stored.ContentType),
		FileSize:      storedSize,
		ContentHash:   contentHash,
		Language:      firstNonBlank(input.Language, domain.DefaultLanguage),
		ChunkStrategy: firstNonBlank(input.ChunkStrategy, domain.DefaultChunkStrategy),
		ChunkConfig:   input.ChunkConfig,
		Metadata:      metadata,
	}, s.now())
	if err != nil {
		// 对象已经写成功但领域对象不能创建时，同样要尽力补偿，避免留下无法被数据库引用的对象。
		s.compensateStoredObject(ctx, stored.URI, "build pending document", err)
		return domain.Document{}, err
	}

	// 对象存储和 PostgreSQL 无法加入同一个 ACID 事务，所以数据库失败时只能执行“尽力删除”的补偿动作。
	if err := s.repo.CreateDocument(ctx, document); err != nil {
		s.compensateStoredObject(ctx, stored.URI, "create document", err)
		return domain.Document{}, fmt.Errorf("create document: %w", err)
	}

	// 到这里数据库已经登记 pending 文档；分块和向量化会由另一个显式用例完成。
	return document, nil
}

// compensateStoredObject 尝试删除已经写入、但没有成功建立数据库记录的对象。
// 删除失败只写结构化日志；返回给调用方的必须仍是原始业务/数据库错误，否则真正根因会被补偿故障遮住。
func (s *Service) compensateStoredObject(ctx context.Context, uri string, operation string, originalErr error) {
	if cleanupErr := s.storage.Delete(ctx, uri); cleanupErr != nil {
		s.logger.ErrorContext(ctx, "compensate stored object",
			"operation", operation,
			"object_uri", uri,
			"original_error", originalErr,
			"cleanup_error", cleanupErr,
		)
	}
}

// sha256Hex 返回小写十六进制 SHA-256，用作同一知识库内的内容去重标识。
func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// firstNonBlank 返回第一个去除首尾空白后仍非空的值。
func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
