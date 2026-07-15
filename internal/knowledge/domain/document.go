package domain

import (
	"fmt"
	"strings"
	"time"
)

// NewDocument 是创建新文档时允许应用层提供的数据。
//
// 它刻意不包含 Status、ChunkCount 和时间字段：这些值属于领域规则，必须由
// NewPendingDocument 统一生成，不能让 HTTP 请求或应用服务随意指定。
type NewDocument struct {
	ID            string
	KbID          string
	Title         string
	SourceType    SourceType
	SourceURI     string
	FileName      string
	FileType      string
	FileSize      int64
	ContentHash   string
	Language      string
	ChunkStrategy string
	ChunkConfig   map[string]any
	Metadata      map[string]any
}

// Document 是知识文档聚合根。
//
// 聚合根的意思可以简单理解为：凡是会影响文档状态、分块数量和失败信息的修改，
// 都必须通过它的方法完成。这里没有 json/db tag，因为 HTTP 和数据库各自有自己的
// 外层模型；领域对象不应该因为接口字段名或数据库列名变化而改变。
type Document struct {
	ID            string
	KbID          string
	Title         string
	SourceType    SourceType
	SourceURI     string
	FileName      string
	FileType      string
	FileSize      int64
	ContentHash   string
	Language      string
	Status        DocumentStatus
	ChunkCount    int
	ChunkStrategy string
	ChunkConfig   map[string]any
	Metadata      map[string]any
	CreateTime    time.Time
	UpdateTime    time.Time
}

// NewPendingDocument 根据上传结果创建一个待处理文档。
// 它把新文档强制初始化为 pending、零分块，并复制 map，避免外层继续修改输入时污染聚合。
func NewPendingDocument(input NewDocument, now time.Time) (Document, error) {
	// 状态和时间由领域层统一决定，调用方没有机会伪造“已完成”的新文档。
	document := Document{
		ID:            strings.TrimSpace(input.ID),
		KbID:          strings.TrimSpace(input.KbID),
		Title:         input.Title,
		SourceType:    input.SourceType,
		SourceURI:     strings.TrimSpace(input.SourceURI),
		FileName:      input.FileName,
		FileType:      input.FileType,
		FileSize:      input.FileSize,
		ContentHash:   strings.TrimSpace(input.ContentHash),
		Language:      input.Language,
		Status:        StatusPending,
		ChunkCount:    0,
		ChunkStrategy: input.ChunkStrategy,
		ChunkConfig:   cloneMap(input.ChunkConfig),
		Metadata:      cloneMap(input.Metadata),
		CreateTime:    now,
		UpdateTime:    now,
	}

	// 创建完成后仍走同一套不变量校验，保证“新建”和“数据库恢复”不会形成两套标准。
	if err := validateDocument(document); err != nil {
		return Document{}, err
	}
	return document, nil
}

// RestoreDocument 把数据库适配器扫描出的快照恢复成可信的领域对象。
// 数据库数据也可能来自旧版本或人工修改，因此进入业务层前必须再次校验，不能盲目信任。
func RestoreDocument(snapshot Document) (Document, error) {
	// 先复制 map，确保数据库行模型释放或复用自己的缓冲区后，不会影响已经恢复的聚合。
	restored := snapshot
	restored.ChunkConfig = cloneMap(snapshot.ChunkConfig)
	restored.Metadata = cloneMap(snapshot.Metadata)

	if err := validateDocument(restored); err != nil {
		return Document{}, err
	}
	return restored, nil
}

// MarkProcessing 返回进入 processing 状态的新文档值。
// 真正的并发互斥仍由 PostgreSQL 条件更新保证；这个方法只表达取得处理权之后的领域状态。
func (d Document) MarkProcessing(now time.Time) (Document, error) {
	if d.Status == StatusProcessing {
		return Document{}, fmt.Errorf("%w: document %q is already processing", ErrInvalidTransition, d.ID)
	}
	if !d.Status.Valid() {
		return Document{}, fmt.Errorf("%w: unsupported current status %q", ErrInvalidTransition, d.Status)
	}

	next := d.clone()
	next.Status = StatusProcessing
	next.UpdateTime = now
	if err := validateDocument(next); err != nil {
		return Document{}, err
	}
	return next, nil
}

// MarkCompleted 返回成功完成处理后的新文档值。
// 它记录最新分块数量和配置，并且只删除 lastError，不能误删文件来源等其他元数据。
func (d Document) MarkCompleted(chunkCount int, chunkConfig map[string]any, now time.Time) (Document, error) {
	if d.Status != StatusProcessing {
		return Document{}, fmt.Errorf("%w: cannot complete document %q from %q", ErrInvalidTransition, d.ID, d.Status)
	}
	if chunkCount < 0 {
		return Document{}, fmt.Errorf("%w: chunk count must not be negative", ErrInvalidArgument)
	}

	next := d.clone()
	next.Status = StatusCompleted
	next.ChunkCount = chunkCount
	next.ChunkConfig = cloneMap(chunkConfig)
	delete(next.Metadata, "lastError")
	next.UpdateTime = now

	if err := validateDocument(next); err != nil {
		return Document{}, err
	}
	return next, nil
}

// MarkFailed 返回记录了最近失败原因的新文档值。
// 原有 metadata 会被完整保留，只覆盖 lastError，便于排查问题时仍能看到文档来源等上下文。
func (d Document) MarkFailed(message string, now time.Time) (Document, error) {
	if d.Status != StatusProcessing {
		return Document{}, fmt.Errorf("%w: cannot fail document %q from %q", ErrInvalidTransition, d.ID, d.Status)
	}
	if strings.TrimSpace(message) == "" {
		return Document{}, fmt.Errorf("%w: failure message is empty", ErrInvalidArgument)
	}

	next := d.clone()
	next.Status = StatusFailed
	next.Metadata["lastError"] = message
	next.UpdateTime = now

	if err := validateDocument(next); err != nil {
		return Document{}, err
	}
	return next, nil
}

// clone 返回一个不与原对象共享可变 map 的文档副本。
// Go 的结构体赋值只会复制 map 的引用，所以必须显式复制，才能真正做到“返回新值、不改原值”。
func (d Document) clone() Document {
	cloned := d
	cloned.ChunkConfig = cloneMap(d.ChunkConfig)
	cloned.Metadata = cloneMap(d.Metadata)
	return cloned
}

// validateDocument 集中维护 Document 的基础不变量。
// 这里只校验所有入口都必须满足的规则；“是否允许某次状态转换”由具体领域方法单独判断。
func validateDocument(document Document) error {
	if strings.TrimSpace(document.ID) == "" {
		return fmt.Errorf("%w: document id is empty", ErrInvalidArgument)
	}
	if strings.TrimSpace(document.KbID) == "" {
		return fmt.Errorf("%w: knowledge base id is empty", ErrInvalidArgument)
	}
	if !document.SourceType.Valid() {
		return fmt.Errorf("%w: unsupported source type %q", ErrInvalidArgument, document.SourceType)
	}
	if !document.Status.Valid() {
		return fmt.Errorf("%w: unsupported document status %q", ErrInvalidArgument, document.Status)
	}
	if document.FileSize < 0 {
		return fmt.Errorf("%w: file size must not be negative", ErrInvalidArgument)
	}
	if document.ChunkCount < 0 {
		return fmt.Errorf("%w: chunk count must not be negative", ErrInvalidArgument)
	}
	if document.CreateTime.IsZero() || document.UpdateTime.IsZero() {
		return fmt.Errorf("%w: document timestamps must not be zero", ErrInvalidArgument)
	}
	if document.UpdateTime.Before(document.CreateTime) {
		return fmt.Errorf("%w: update time is before create time", ErrInvalidArgument)
	}
	return nil
}

// cloneMap 递归复制常见 JSON map/slice 容器。
// metadata 和 chunkConfig 最终来自 JSON；只做第一层复制时，嵌套对象仍会共享引用，因此这里把嵌套容器也复制掉。
func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = cloneJSONValue(value)
	}
	return output
}

// cloneJSONValue 复制 JSON 能表达的可变容器；字符串、数字、布尔值等不可变值可以直接复用。
func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneJSONValue(item)
		}
		return cloned
	default:
		return typed
	}
}
