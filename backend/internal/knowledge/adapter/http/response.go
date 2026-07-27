// Package httpadapter 把 Gin 请求转换成知识文档应用用例输入，并把领域结果转换成稳定 HTTP DTO。
//
// 这个包可以依赖 Gin 和 application/domain，但不能直接访问 PostgreSQL、S3 或 Embedding 具体实现。
package httpadapter

import (
	"errors"
	"net/http"
	"time"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

const (
	// SuccessCode 保持原项目 Result<T> 的成功业务码。
	SuccessCode = "0"
	// ClientErrorCode 表示参数、状态冲突、重复或资源不存在等调用方可处理问题。
	ClientErrorCode = "A000001"
	// ServiceErrorCode 表示未分类的服务端故障。
	ServiceErrorCode = "B000001"
)

// Result 是所有知识接口共同使用的兼容响应外壳。
// 三个字段不使用 omitempty，保证空 message 或 nil data 仍有稳定 JSON 形状。
type Result struct {
	// Code 是前端稳定识别的业务码，不等同于 HTTP 状态码。
	Code string `json:"code"`
	// Message 是成功时空字符串、失败时客户端可理解的简短原因。
	Message string `json:"message"`
	// Data 保存具体响应 DTO；失败或删除成功时可以明确为 null。
	Data any `json:"data"`
}

// Success 构造成功响应，业务码固定为 0。
func Success(data any) Result {
	// 所有成功响应都固定使用业务码 0，避免各 Handler 自行拼装。
	return Result{Code: SuccessCode, Message: "", Data: data}
}

// Failure 构造失败响应；失败没有有效业务数据，因此 data 明确输出 null。
func Failure(code string, message string) Result {
	// 失败数据固定为 nil，防止调用方误用不完整业务结果。
	return Result{Code: code, Message: message, Data: nil}
}

// DocumentResponse 是领域文档在 HTTP 边界的 JSON 模型。
// 显式 DTO 保留旧 camelCase 字段名，同时让 domain 不再携带 json tag。
type DocumentResponse struct {
	// ID 是文档唯一标识。
	ID string `json:"id"`
	// KbID 使用旧接口 camelCase 字段 kbId 表示知识库归属。
	KbID string `json:"kbId"`
	// Title 是文档展示标题。
	Title string `json:"title"`
	// SourceType 表示内容来源类型。
	SourceType domain.SourceType `json:"sourceType"`
	// SourceURI 是原始文件在对象存储中的定位地址。
	SourceURI string `json:"sourceUri"`
	// FileName 是用户上传的文件名。
	FileName string `json:"fileName"`
	// FileType 是归一化文件类型。
	FileType string `json:"fileType"`
	// FileSize 是真实文件字节数。
	FileSize int64 `json:"fileSize"`
	// ContentHash 是用于去重的 SHA-256。
	ContentHash string `json:"contentHash"`
	// Language 是文档主要语言。
	Language string `json:"language"`
	// Status 是当前处理生命周期状态。
	Status domain.DocumentStatus `json:"status"`
	// ChunkCount 是最近一次成功处理后的分块数量。
	ChunkCount int `json:"chunkCount"`
	// ChunkStrategy 是文档采用的分块策略名称。
	ChunkStrategy string `json:"chunkStrategy"`
	// ChunkConfig 是返回给客户端的独立配置副本。
	ChunkConfig map[string]any `json:"chunkConfig"`
	// Metadata 是返回给客户端的独立扩展信息副本。
	Metadata map[string]any `json:"metadata"`
	// CreateTime 是文档创建时间。
	CreateTime time.Time `json:"createTime"`
	// UpdateTime 是文档最后更新时间。
	UpdateTime time.Time `json:"updateTime"`
}

// PageResult 保持旧列表接口的 records/total/current/size 分页结构。
type PageResult struct {
	// Records 是当前页的文档 DTO，顺序与仓储返回保持一致。
	Records []DocumentResponse `json:"records"`
	// Total 是符合条件的文档总数，不是当前页长度。
	Total int64 `json:"total"`
	// Current 是本次实际采用的页码。
	Current int `json:"current"`
	// Size 是本次实际采用的每页数量。
	Size int `json:"size"`
}

// documentResponseFromDomain 在 HTTP 边界逐字段转换领域对象，避免直接序列化 domain。
func documentResponseFromDomain(document domain.Document) DocumentResponse {
	// 每个字段显式映射可以让领域模型和外部 JSON 合同独立演进并接受审查。
	return DocumentResponse{
		ID:            document.ID,
		KbID:          document.KbID,
		Title:         document.Title,
		SourceType:    document.SourceType,
		SourceURI:     document.SourceURI,
		FileName:      document.FileName,
		FileType:      document.FileType,
		FileSize:      document.FileSize,
		ContentHash:   document.ContentHash,
		Language:      document.Language,
		Status:        document.Status,
		ChunkCount:    document.ChunkCount,
		ChunkStrategy: document.ChunkStrategy,
		ChunkConfig:   cloneJSONMap(document.ChunkConfig),
		Metadata:      cloneJSONMap(document.Metadata),
		CreateTime:    document.CreateTime,
		UpdateTime:    document.UpdateTime,
	}
}

// documentResponsesFromDomain 批量转换列表，并保留原顺序。
func documentResponsesFromDomain(documents []domain.Document) []DocumentResponse {
	// 预先创建最终长度切片，循环中按同一下标写入，不发生顺序变化。
	responses := make([]DocumentResponse, len(documents))
	// 逐个复用单文档转换函数，避免列表和详情字段映射不一致。
	for index, document := range documents {
		responses[index] = documentResponseFromDomain(document)
	}
	// 返回的 DTO 切片不再持有领域 map 的可变引用。
	return responses
}

// errorResponse 集中维护领域错误到 HTTP 状态和业务码的映射。
// errors.Is 能穿透应用层的 %w 包装，因此底层可以补充操作上下文而不破坏对外分类。
func errorResponse(err error) (int, Result) {
	// 默认先按未知服务端故障处理，只有明确业务错误才降级为 4xx。
	status := http.StatusInternalServerError
	// 未分类错误使用服务端业务码。
	code := ServiceErrorCode
	// 未分类错误按服务端故障处理，客户端只拿到通用提示。
	// 真实错误链仍然由 Handler 记录到服务端日志，避免数据库、存储或第三方模型错误细节泄漏到响应体。
	message := "internal server error"

	// errors.Is 可以穿透多层 %w，按最内层稳定领域错误决定公开分类。
	switch {
	case errors.Is(err, domain.ErrNotFound):
		// 资源不存在使用 HTTP 404，同时保留客户端错误业务码。
		status = http.StatusNotFound
		code = ClientErrorCode
		message = err.Error()
	case errors.Is(err, domain.ErrInvalidArgument),
		errors.Is(err, domain.ErrDuplicate),
		errors.Is(err, domain.ErrAlreadyRunning),
		errors.Is(err, domain.ErrInvalidTransition):
		// 参数、重复、处理冲突和非法状态转换都属于当前客户端可处理问题。
		status = http.StatusBadRequest
		code = ClientErrorCode
		message = err.Error()
	}

	// 统一通过 Failure 构造稳定 code/message/data 结构。
	return status, Failure(code, message)
}

// cloneJSONMap 复制 HTTP DTO 中的 JSON map，避免响应序列化期间与领域对象共享可变引用。
func cloneJSONMap(source map[string]any) map[string]any {
	// nil 输入会得到可安全序列化为空对象的非 nil map。
	cloned := make(map[string]any, len(source))
	// 每个值继续递归复制，不能只复制最外层 map。
	for key, value := range source {
		cloned[key] = cloneJSONValue(value)
	}
	// 返回值可以由 JSON 编码器独立读取，不会与领域对象并发共享容器。
	return cloned
}

// cloneJSONValue 递归复制 JSON 可以表达的 map 和 slice 容器。
func cloneJSONValue(value any) any {
	// JSON 动态值只有对象和数组需要深复制，标量可以按值复用。
	switch typed := value.(type) {
	case map[string]any:
		// 嵌套对象复用 map 深复制函数。
		return cloneJSONMap(typed)
	case []any:
		// 数组先分配同长度切片，再递归复制每个元素。
		cloned := make([]any, len(typed))
		for index, item := range typed {
			// 元素可能仍然是嵌套对象或数组。
			cloned[index] = cloneJSONValue(item)
		}
		// 返回与输入数组没有共享底层存储的新切片。
		return cloned
	default:
		// 字符串、数字、布尔值和 nil 直接返回即可。
		return typed
	}
}
