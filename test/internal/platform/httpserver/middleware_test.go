package httpserver_test

import (
	"bytes"
	. "github.com/luqingjia/TravelAgent/internal/platform/httpserver"
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
	middleware, err := NewMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err != nil {
		t.Fatalf("NewMiddleware() error = %v", err)
	}

	requestIDHandler := func(context *gin.Context) {
		context.String(http.StatusOK, RequestID(context))
	}

	tests := []struct {
		name      string
		incoming  string
		want      string
		generated bool
	}{
		{name: "缺失时生成", generated: true},
		{name: "合法值沿用", incoming: "caller-123", want: "caller-123"},
		{name: "超长值替换", incoming: strings.Repeat("x", 129), generated: true},
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
			if recorder.Code != http.StatusOK {
				t.Fatalf("response status = %d", recorder.Code)
			}
			got := recorder.Body.String()
			if test.generated {
				if got == "" || got == test.incoming {
					t.Fatalf("generated request id = %q, incoming = %q", got, test.incoming)
				}
			} else if got != test.want {
				t.Fatalf("request id = %q, want %q", got, test.want)
			}
			if recorder.Header().Get(RequestIDHeader) != got {
				t.Fatalf("response request id = %q, want %q", recorder.Header().Get(RequestIDHeader), got)
			}
		})
	}
}

// TestAccessLogRecordsRequiredFields 验证一条请求完成后，结构化日志包含排查问题所需的固定字段。
func TestAccessLogRecordsRequiredFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	middleware, err := NewMiddleware(logger)
	if err != nil {
		t.Fatalf("NewMiddleware() error = %v", err)
	}

	router := gin.New()
	router.Use(middleware.RequestID(), middleware.AccessLog())
	router.GET("/documents", func(context *gin.Context) {
		context.Status(http.StatusAccepted)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/documents?size=20", nil)
	request.Header.Set(RequestIDHeader, "request-1")
	router.ServeHTTP(recorder, request)

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
	middleware, err := NewMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err != nil {
		t.Fatalf("NewMiddleware() error = %v", err)
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
