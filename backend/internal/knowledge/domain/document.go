package domain

import (
	"fmt"
	"strings"
	"time"
)

// 本文件维护 Document 聚合的创建、恢复、状态转换和深复制规则。
// 外层只能通过这些入口得到可信文档，不能绕过领域方法直接拼出任意状态。
// NewDocument 是创建新文档时允许应用层提供的数据。
//
// 它刻意不包含 Status、ChunkCount 和时间字段：这些值属于领域规则，必须由
// NewPendingDocument 统一生成，不能让 HTTP 请求或应用服务随意指定。
type NewDocument struct {
	// ID 是应用层生成的文档唯一标识。
	ID string
	// KbID 指定文档归属的知识库。
	KbID string
	// Title 是用户看到的文档标题。
	Title string
	// SourceType 表示文档来源，目前只允许文件上传。
	SourceType SourceType
	// SourceURI 是对象存储返回的稳定读取地址。
	SourceURI string
	// FileName 保留上传时的原始文件名。
	FileName string
	// FileType 是去掉点号并归一化后的文件扩展名。
	FileType string
	// FileSize 是实际读取到的文件字节数。
	FileSize int64
	// ContentHash 是完整内容的 SHA-256，用于同知识库去重。
	ContentHash string
	// Language 表示文档主要语言，未指定时使用默认中文。
	Language string
	// ChunkStrategy 记录当前使用的分块策略名称。
	ChunkStrategy string
	// ChunkConfig 保存客户端或默认分块配置的独立副本。
	ChunkConfig map[string]any
	// Metadata 保存文件来源之外的扩展业务信息。
	Metadata map[string]any
}

// Document 是知识文档聚合根。
//
// 聚合根的意思可以简单理解为：凡是会影响文档状态、分块数量和失败信息的修改，
// 都必须通过它的方法完成。这里没有 json/db tag，因为 HTTP 和数据库各自有自己的
// 外层模型；领域对象不应该因为接口字段名或数据库列名变化而改变。
type Document struct {
	// ID 是文档聚合的唯一标识。
	ID string
	// KbID 是文档所属知识库的标识。
	KbID string
	// Title 是文档展示标题。
	Title string
	// SourceType 说明文档怎样进入系统。
	SourceType SourceType
	// SourceURI 指向对象存储中的原始文件。
	SourceURI string
	// FileName 保留原始文件名，供展示和文本类型判断。
	FileName string
	// FileType 保存归一化扩展名。
	FileType string
	// FileSize 保存真实内容字节数。
	FileSize int64
	// ContentHash 用于识别相同内容。
	ContentHash string
	// Language 表示内容语言。
	Language string
	// Status 是 pending、processing、completed、failed 之一。
	Status DocumentStatus
	// ChunkCount 是最近一次成功处理后持久化的分块数量。
	ChunkCount int
	// ChunkStrategy 记录处理该文档所采用的策略。
	ChunkStrategy string
	// ChunkConfig 保存最近一次完成处理时使用的配置。
	ChunkConfig map[string]any
	// Metadata 保存扩展信息和最近一次失败原因 lastError。
	Metadata map[string]any
	// CreateTime 是文档首次登记时间。
	CreateTime time.Time
	// UpdateTime 是最近一次状态或内容变化时间。
	UpdateTime time.Time
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
		// 构造结果不满足不变量时返回零值，防止半合法文档逃出领域层。
		return Document{}, err
	}
	// 返回的 map 已完成深复制，调用方后续修改输入不会影响该聚合。
	return document, nil
}

// RestoreDocument 把数据库适配器扫描出的快照恢复成可信的领域对象。
// 数据库数据也可能来自旧版本或人工修改，因此进入业务层前必须再次校验，不能盲目信任。
func RestoreDocument(snapshot Document) (Document, error) {
	// 先复制 map，确保数据库行模型释放或复用自己的缓冲区后，不会影响已经恢复的聚合。
	restored := snapshot
	// ChunkConfig 可能包含嵌套对象，必须深复制而不是只复制最外层 map 指针。
	restored.ChunkConfig = cloneMap(snapshot.ChunkConfig)
	// Metadata 同样可能包含嵌套数组或对象，恢复后不能继续共享数据库行模型的引用。
	restored.Metadata = cloneMap(snapshot.Metadata)

	// 即使数据已经来自数据库，也要检查旧版本数据或人工修改是否破坏领域不变量。
	if err := validateDocument(restored); err != nil {
		return Document{}, err
	}
	// 校验通过后，应用层拿到的是可安全继续状态转换的独立聚合。
	return restored, nil
}

// MarkProcessing 返回进入 processing 状态的新文档值。
// 真正的并发互斥仍由 PostgreSQL 条件更新保证；这个方法只表达取得处理权之后的领域状态。
func (d Document) MarkProcessing(now time.Time) (Document, error) {
	// 已经 processing 说明其他流程已取得处理权，不能重复进入同一状态。
	if d.Status == StatusProcessing {
		return Document{}, fmt.Errorf("%w: document %q is already processing", ErrInvalidTransition, d.ID)
	}
	// 非法状态通常意味着恢复数据损坏，不能继续尝试状态转换。
	if !d.Status.Valid() {
		return Document{}, fmt.Errorf("%w: unsupported current status %q", ErrInvalidTransition, d.Status)
	}

	// clone 保证状态变化不会原地污染调用方仍持有的旧文档值。
	next := d.clone()
	// 取得处理权后只修改状态和更新时间，其他业务字段保持原样。
	next.Status = StatusProcessing
	next.UpdateTime = now
	// 转换后的新值再次走完整不变量检查。
	if err := validateDocument(next); err != nil {
		return Document{}, err
	}
	// 调用方决定何时把这个 processing 新值持久化。
	return next, nil
}

// MarkCompleted 返回成功完成处理后的新文档值。
// 它记录最新分块数量和配置，并且只删除 lastError，不能误删文件来源等其他元数据。
func (d Document) MarkCompleted(chunkCount int, chunkConfig map[string]any, now time.Time) (Document, error) {
	// 只有已经成功抢占到 processing 的文档才允许标记完成。
	if d.Status != StatusProcessing {
		return Document{}, fmt.Errorf("%w: cannot complete document %q from %q", ErrInvalidTransition, d.ID, d.Status)
	}
	// 负分块数无法表示真实处理结果，必须在更新聚合前拒绝。
	if chunkCount < 0 {
		return Document{}, fmt.Errorf("%w: chunk count must not be negative", ErrInvalidArgument)
	}

	// 复制旧值后再写入成功结果，事务失败时旧对象仍保持 processing。
	next := d.clone()
	// completed 表示分块和向量已经全部准备完成，接下来只差仓储事务提交。
	next.Status = StatusCompleted
	// 分块数必须与准备写入仓储的切片数量一致。
	next.ChunkCount = chunkCount
	// 使用独立配置副本，防止调用方在事务执行期间修改 map。
	next.ChunkConfig = cloneMap(chunkConfig)
	// 新一次成功会清除旧失败原因，但保留其他 metadata。
	delete(next.Metadata, "lastError")
	// 更新时间记录本次成功处理发生的业务时间。
	next.UpdateTime = now

	// 完成态仍必须满足文档 ID、状态、时间等基础约束。
	if err := validateDocument(next); err != nil {
		return Document{}, err
	}
	// 返回合法 completed 新值，仓储会把它与分块、向量一起提交。
	return next, nil
}

// MarkFailed 返回记录了最近失败原因的新文档值。
// 原有 metadata 会被完整保留，只覆盖 lastError，便于排查问题时仍能看到文档来源等上下文。
func (d Document) MarkFailed(message string, now time.Time) (Document, error) {
	// 只有正在处理的文档才可能记录一次处理失败。
	if d.Status != StatusProcessing {
		return Document{}, fmt.Errorf("%w: cannot fail document %q from %q", ErrInvalidTransition, d.ID, d.Status)
	}
	// 空失败原因无法帮助排查问题，也不允许写入 metadata.lastError。
	if strings.TrimSpace(message) == "" {
		return Document{}, fmt.Errorf("%w: failure message is empty", ErrInvalidArgument)
	}

	// 复制旧聚合，确保失败回写不会直接修改调用方的 processing 值。
	next := d.clone()
	// 状态切换为 failed，表示本轮处理已结束但可以由后续请求重试。
	next.Status = StatusFailed
	// 只覆盖最近错误字段，原有来源和用户 metadata 继续保留。
	next.Metadata["lastError"] = message
	// 更新时间记录失败发生的业务时间。
	next.UpdateTime = now

	// 写库前再次确认失败态聚合仍然合法。
	if err := validateDocument(next); err != nil {
		return Document{}, err
	}
	// 返回新失败值，应用层再尽力交给仓储持久化。
	return next, nil
}

// clone 返回一个不与原对象共享可变 map 的文档副本。
// Go 的结构体赋值只会复制 map 的引用，所以必须显式复制，才能真正做到“返回新值、不改原值”。
func (d Document) clone() Document {
	// 普通字段通过结构体赋值按值复制。
	cloned := d
	// map 是引用类型，所以分块配置必须单独深复制。
	cloned.ChunkConfig = cloneMap(d.ChunkConfig)
	// metadata 也要深复制，尤其要隔离 lastError 的新增和删除。
	cloned.Metadata = cloneMap(d.Metadata)
	// 返回的副本可以独立修改，不会反向影响原聚合。
	return cloned
}

// validateDocument 集中维护 Document 的基础不变量。
// 这里只校验所有入口都必须满足的规则；“是否允许某次状态转换”由具体领域方法单独判断。
func validateDocument(document Document) error {
	// 文档 ID 是所有查询和关联表写入的基础，空值不能进入系统。
	if strings.TrimSpace(document.ID) == "" {
		return fmt.Errorf("%w: document id is empty", ErrInvalidArgument)
	}
	// 知识库 ID 缺失会让文档脱离业务范围。
	if strings.TrimSpace(document.KbID) == "" {
		return fmt.Errorf("%w: knowledge base id is empty", ErrInvalidArgument)
	}
	// 来源类型必须是领域层明确支持的值。
	if !document.SourceType.Valid() {
		return fmt.Errorf("%w: unsupported source type %q", ErrInvalidArgument, document.SourceType)
	}
	// 状态必须属于完整生命周期枚举，不能接受数据库中的任意字符串。
	if !document.Status.Valid() {
		return fmt.Errorf("%w: unsupported document status %q", ErrInvalidArgument, document.Status)
	}
	// 文件大小可以为零，但绝不能是负数。
	if document.FileSize < 0 {
		return fmt.Errorf("%w: file size must not be negative", ErrInvalidArgument)
	}
	// 分块数量在 pending 和 failed 时可以为零，但不能为负数。
	if document.ChunkCount < 0 {
		return fmt.Errorf("%w: chunk count must not be negative", ErrInvalidArgument)
	}
	// 创建和更新时间必须由构造或恢复入口明确提供。
	if document.CreateTime.IsZero() || document.UpdateTime.IsZero() {
		return fmt.Errorf("%w: document timestamps must not be zero", ErrInvalidArgument)
	}
	// 更新时间早于创建时间说明快照时间线已经损坏。
	if document.UpdateTime.Before(document.CreateTime) {
		return fmt.Errorf("%w: update time is before create time", ErrInvalidArgument)
	}
	// 所有通用不变量都满足后，文档才被视为可信领域对象。
	return nil
}

// cloneMap 递归复制常见 JSON map/slice 容器。
// metadata 和 chunkConfig 最终来自 JSON；只做第一层复制时，嵌套对象仍会共享引用，因此这里把嵌套容器也复制掉。
func cloneMap(input map[string]any) map[string]any {
	// nil map 的 len 为零，make 会返回一个可安全写入的空 map，简化上层逻辑。
	output := make(map[string]any, len(input))
	// 每个值都继续交给 cloneJSONValue，确保嵌套容器也被复制。
	for key, value := range input {
		output[key] = cloneJSONValue(value)
	}
	// 返回的新 map 与输入没有可变容器引用共享。
	return output
}

// cloneJSONValue 复制 JSON 能表达的可变容器；字符串、数字、布尔值等不可变值可以直接复用。
func cloneJSONValue(value any) any {
	// 根据 JSON 解码后常见的动态类型选择复制方式。
	switch typed := value.(type) {
	case map[string]any:
		// 嵌套对象递归走 cloneMap。
		return cloneMap(typed)
	case []any:
		// 数组先创建同长度切片，再逐项递归复制。
		cloned := make([]any, len(typed))
		for index, item := range typed {
			// 数组元素仍可能是对象或数组，不能只复制切片头。
			cloned[index] = cloneJSONValue(item)
		}
		// 返回与原数组完全隔离的新切片。
		return cloned
	default:
		// 字符串、数字、布尔值和 nil 没有内部可变引用，可以直接返回。
		return typed
	}
}
