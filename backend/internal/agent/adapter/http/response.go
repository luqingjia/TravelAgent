// Package httpadapter 把 Gin 请求转换成 Agent 应用用例输入，并把结果转换成稳定 HTTP/SSE 输出。
//
// 本包可以依赖 Gin 与 application/domain，但不能直接访问 Eino、数据库或对象存储实现。
// 统一响应外壳与 knowledge 保持 code/message/data 兼容，但类型定义在本包内，避免跨限界上下文导入。
package httpadapter

import (
	"errors"
	"net/http"

	"github.com/luqingjia/TravelAgent/internal/agent/domain"
)

const (
	// SuccessCode 保持与现有知识接口一致的成功业务码。
	SuccessCode = "0"
	// ClientErrorCode 表示参数、模型不可用等调用方可处理问题。
	ClientErrorCode = "A000001"
	// ServiceErrorCode 表示未分类的服务端故障。
	ServiceErrorCode = "B000001"
)

// Result 是 Agent 接口使用的统一响应外壳。
// 三个字段不使用 omitempty，保证空 message 或 nil data 仍有稳定 JSON 形状。
type Result struct {
	// Code 是前端稳定识别的业务码。
	Code string `json:"code"`
	// Message 成功时为空字符串，失败时为可理解短原因。
	Message string `json:"message"`
	// Data 保存具体响应；失败时明确为 null。
	Data any `json:"data"`
}

// Success 构造成功响应。
func Success(data any) Result {
	return Result{Code: SuccessCode, Message: "", Data: data}
}

// Failure 构造失败响应。
func Failure(code string, message string) Result {
	return Result{Code: code, Message: message, Data: nil}
}

// catalogResponseFromDomain 在 HTTP 边界显式转换模型目录，避免直接序列化 domain。
func catalogResponseFromDomain(view domain.CatalogView) modelCatalogResponse {
	models := make([]publicModelResponse, 0, len(view.Models))
	for _, model := range view.Models {
		models = append(models, publicModelResponse{
			ID:           model.ID,
			DisplayName:  model.DisplayName,
			Provider:     model.Provider,
			Model:        model.Model,
			Available:    model.Available,
			Capabilities: capabilitiesToStrings(model.Capabilities),
		})
	}
	return modelCatalogResponse{
		DefaultModelID: view.DefaultModelID,
		Models:         models,
	}
}

// capabilitiesToStrings 把能力开关转成稳定字符串数组合同。
func capabilitiesToStrings(capabilities domain.ModelCapabilities) []string {
	result := make([]string, 0, 3)
	if capabilities.Chat {
		result = append(result, "chat")
	}
	if capabilities.Streaming {
		result = append(result, "streaming")
	}
	if capabilities.Tools {
		result = append(result, "tools")
	}
	return result
}

// errorResponse 把领域错误映射为 HTTP 状态和统一外壳。
// 未知错误只返回通用消息，真实错误链由 Handler 记录到服务端日志。
func errorResponse(err error) (int, Result) {
	status := http.StatusInternalServerError
	code := ServiceErrorCode
	message := "internal server error"

	switch {
	case errors.Is(err, domain.ErrModelNotFound):
		status = http.StatusNotFound
		code = ClientErrorCode
		message = err.Error()
	case errors.Is(err, domain.ErrInvalidArgument),
		errors.Is(err, domain.ErrModelDisabled),
		errors.Is(err, domain.ErrModelUnavailable),
		errors.Is(err, domain.ErrInvalidTimezone):
		status = http.StatusBadRequest
		code = ClientErrorCode
		message = err.Error()
	case errors.Is(err, domain.ErrAgentFailed):
		// 模型运行失败不向客户端暴露供应商细节。
		status = http.StatusInternalServerError
		code = ServiceErrorCode
		message = "agent run failed"
	}
	return status, Failure(code, message)
}
