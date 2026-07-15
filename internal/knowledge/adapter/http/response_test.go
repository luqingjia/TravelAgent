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
	encoded, err := json.Marshal(Success(map[string]any{}))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if string(encoded) != `{"code":"0","message":"","data":{}}` {
		t.Fatalf("success JSON = %s", encoded)
	}
}

// TestErrorResponseMapsStableBusinessCategories 验证领域错误到 HTTP 状态和项目业务码的映射不会在拆层后漂移。
func TestErrorResponseMapsStableBusinessCategories(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantStatus    int
		wantCode      string
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
			// Act：直接调用映射函数，避免 Handler、Gin 或日志干扰这个纯错误契约测试。
			status, result := errorResponse(test.err)
			if status != test.wantStatus || result.Code != test.wantCode || result.Message == "" {
				t.Fatalf("errorResponse() = (%d, %#v)", status, result)
			}
			if test.wantMessage != "" && result.Message != test.wantMessage {
				t.Fatalf("message = %q, want %q", result.Message, test.wantMessage)
			}
			if test.forbidMessage != "" && strings.Contains(result.Message, test.forbidMessage) {
				t.Fatalf("message %q leaks forbidden detail %q", result.Message, test.forbidMessage)
			}
		})
	}
}
