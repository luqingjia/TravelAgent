package httpadapter

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// TestResultJSONKeepsCompatibleEnvelope 验证成功响应始终包含 code、message、data 三个字段。
func TestResultJSONKeepsCompatibleEnvelope(t *testing.T) {
	// 用空对象作为 data，专门观察统一响应外壳本身的字段名和顺序。
	encoded, err := json.Marshal(Success(map[string]any{}))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	// 精确比较 JSON，防止 code、message、data 被改名、遗漏或改成不兼容类型。
	if string(encoded) != `{"code":"0","message":"","data":{}}` {
		t.Fatalf("success JSON = %s", encoded)
	}
}

// TestErrorResponseMapsStableBusinessCategories 验证领域错误到 HTTP 状态和项目业务码的映射不会在拆层后漂移。
func TestErrorResponseMapsStableBusinessCategories(t *testing.T) {
	tests := []struct {
		// name 是子场景名称，失败时直接说明是哪类业务错误映射漂移。
		name string
		// err 是领域层或基础设施层交给 HTTP 边界的原始错误。
		err error
		// wantStatus、wantCode 是客户端依赖的稳定 HTTP 状态和业务码。
		wantStatus int
		wantCode   string
		// wantMessage 要求精确提示；forbidMessage 确保内部错误细节不会泄漏。
		wantMessage   string
		forbidMessage string
	}{
		{name: "参数错误", err: domain.ErrInvalidArgument, wantStatus: http.StatusBadRequest, wantCode: ClientErrorCode},
		{name: "内容重复", err: domain.ErrDuplicate, wantStatus: http.StatusBadRequest, wantCode: ClientErrorCode},
		{name: "正在处理", err: domain.ErrAlreadyRunning, wantStatus: http.StatusBadRequest, wantCode: ClientErrorCode},
		{name: "非法状态转换", err: domain.ErrInvalidTransition, wantStatus: http.StatusBadRequest, wantCode: ClientErrorCode},
		{name: "文档不存在", err: domain.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: ClientErrorCode},
		// 未知错误通常来自数据库、对象存储或第三方模型等基础设施。
		// 这类错误可能包含内部拓扑、表名、主机名甚至凭据片段，所以响应给客户端时只能给通用提示。
		{name: "未知服务故障", err: errors.New("database offline"), wantStatus: http.StatusInternalServerError, wantCode: ServiceErrorCode, wantMessage: "internal server error", forbidMessage: "database offline"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// 执行：直接调用映射函数，避免 Handler、Gin 或日志干扰这个纯错误契约测试。
			status, result := errorResponse(test.err)
			// 第一层断言检查状态、业务码和非空提示，保证每个类别都有完整客户端响应。
			if status != test.wantStatus || result.Code != test.wantCode || result.Message == "" {
				t.Fatalf("errorResponse() = (%d, %#v)", status, result)
			}
			// 需要固定提示的场景必须逐字匹配，避免无意中把内部错误返回给客户端。
			if test.wantMessage != "" && result.Message != test.wantMessage {
				t.Fatalf("message = %q, want %q", result.Message, test.wantMessage)
			}
			// 禁止内容断言是最后一道防泄漏检查，未知错误只能返回通用文案。
			if test.forbidMessage != "" && strings.Contains(result.Message, test.forbidMessage) {
				t.Fatalf("message %q leaks forbidden detail %q", result.Message, test.forbidMessage)
			}
		})
	}
}
