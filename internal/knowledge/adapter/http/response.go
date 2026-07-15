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
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// Success 构造成功响应，业务码固定为 0。
func Success(data any) Result {
	return Result{Code: SuccessCode, Message: "", Data: data}
}

// Failure 构造失败响应；失败没有有效业务数据，因此 data 明确输出 null。
func Failure(code string, message string) Result {
	return Result{Code: code, Message: message, Data: nil}
}

// DocumentResponse 是领域文档在 HTTP 边界的 JSON 模型。
// 显式 DTO 保留旧 camelCase 字段名，同时让 domain 不再携带 json tag。
type DocumentResponse struct {
	ID            string                `json:"id"`
	KbID          string                `json:"kbId"`
	Title         string                `json:"title"`
	SourceType    domain.SourceType     `json:"sourceType"`
	SourceURI     string                `json:"sourceUri"`
	FileName      string                `json:"fileName"`
	FileType      string                `json:"fileType"`
	FileSize      int64                 `json:"fileSize"`
	ContentHash   string                `json:"contentHash"`
	Language      string                `json:"language"`
	Status        domain.DocumentStatus `json:"status"`
	ChunkCount    int                   `json:"chunkCount"`
	ChunkStrategy string                `json:"chunkStrategy"`
	ChunkConfig   map[string]any        `json:"chunkConfig"`
	Metadata      map[string]any        `json:"metadata"`
	CreateTime    time.Time             `json:"createTime"`
	UpdateTime    time.Time             `json:"updateTime"`
}

// PageResult 保持旧列表接口的 records/total/current/size 分页结构。
type PageResult struct {
	Records []DocumentResponse `json:"records"`
	Total   int64              `json:"total"`
	Current int                `json:"current"`
	Size    int                `json:"size"`
}

// documentResponseFromDomain 在 HTTP 边界逐字段转换领域对象，避免直接序列化 domain。
func documentResponseFromDomain(document domain.Document) DocumentResponse {
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
	responses := make([]DocumentResponse, len(documents))
	for index, document := range documents {
		responses[index] = documentResponseFromDomain(document)
	}
	return responses
}

// errorResponse 集中维护领域错误到 HTTP 状态和业务码的映射。
// errors.Is 能穿透应用层的 %w 包装，因此底层可以补充操作上下文而不破坏对外分类。
func errorResponse(err error) (int, Result) {
	status := http.StatusInternalServerError
	code := ServiceErrorCode
	// 未分类错误按服务端故障处理，客户端只拿到通用提示。
	// 真实错误链仍然由 Handler 记录到服务端日志，避免数据库、存储或第三方模型错误细节泄漏到响应体。
	message := "internal server error"

	switch {
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
		code = ClientErrorCode
		message = err.Error()
	case errors.Is(err, domain.ErrInvalidArgument),
		errors.Is(err, domain.ErrDuplicate),
		errors.Is(err, domain.ErrAlreadyRunning),
		errors.Is(err, domain.ErrInvalidTransition):
		status = http.StatusBadRequest
		code = ClientErrorCode
		message = err.Error()
	}

	return status, Failure(code, message)
}

// cloneJSONMap 复制 HTTP DTO 中的 JSON map，避免响应序列化期间与领域对象共享可变引用。
func cloneJSONMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = cloneJSONValue(value)
	}
	return cloned
}

// cloneJSONValue 递归复制 JSON 可以表达的 map 和 slice 容器。
func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneJSONMap(typed)
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
