package httpserver

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRequestIDGeneratesReusesAndReplaces 验证请求 ID 的三条边界：缺失时生成、合法时沿用、超长时替换。
func TestRequestIDGeneratesReusesAndReplaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	middleware, err := newMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), func() string { return "generated-id" })
	if err != nil {
		t.Fatalf("newMiddleware() error = %v", err)
	}

	requestIDHandler := func(context *gin.Context) {
		context.String(http.StatusOK, RequestID(context))
	}

	tests := []struct {
		name     string
		incoming string
		want     string
	}{
		{name: "缺失时生成", want: "generated-id"},
		{name: "合法值沿用", incoming: "caller-123", want: "caller-123"},
		{name: "超长值替换", incoming: strings.Repeat("x", maximumRequestIDLength+1), want: "generated-id"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(middleware.RequestID())
			router.GET("/", requestIDHandler)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.incoming != "" {
				request.Header.Set(RequestIDHeader, test.incoming)
			}

			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK || recorder.Body.String() != test.want {
				t.Fatalf("response = (%d, %q)", recorder.Code, recorder.Body.String())
			}
			if recorder.Header().Get(RequestIDHeader) != test.want {
				t.Fatalf("response request id = %q", recorder.Header().Get(RequestIDHeader))
			}
		})
	}
}

// TestAccessLogRecordsRequiredFields 验证一条请求完成后，结构化日志包含排查问题所需的固定字段。
func TestAccessLogRecordsRequiredFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	middleware, err := newMiddleware(logger, func() string { return "request-1" })
	if err != nil {
		t.Fatalf("newMiddleware() error = %v", err)
	}

	router := gin.New()
	router.Use(middleware.RequestID(), middleware.AccessLog())
	router.GET("/documents", func(context *gin.Context) {
		context.Status(http.StatusAccepted)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/documents?size=20", nil))

	logText := output.String()
	for _, expected := range []string{
		`"request_id":"request-1"`,
		`"method":"GET"`,
		`"path":"/documents"`,
		`"status":202`,
		`"latency_ms":`,
	} {
		if !strings.Contains(logText, expected) {
			t.Fatalf("access log %q missing %q", logText, expected)
		}
	}
}

// TestRecoveryReturns500WithoutLeakingPanic 验证 panic 被转换成 500，响应体不会暴露内部 panic 文本。
func TestRecoveryReturns500WithoutLeakingPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	middleware, err := newMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), func() string { return "request-2" })
	if err != nil {
		t.Fatalf("newMiddleware() error = %v", err)
	}

	router := gin.New()
	router.Use(middleware.RequestID(), middleware.Recovery())
	router.GET("/panic", func(*gin.Context) {
		panic("database password should stay private")
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d, want 500", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "database password") {
		t.Fatalf("panic response leaked internal text: %q", recorder.Body.String())
	}
}
