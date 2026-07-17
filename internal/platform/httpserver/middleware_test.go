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
	// Gin 使用测试模式，避免默认调试日志和路由提示污染测试输出。
	gin.SetMode(gin.TestMode)
	// 固定 ID 生成器让“新生成”和“替换超长值”的结果可以精确断言。
	middleware, err := newMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), func() string { return "generated-id" })
	if err != nil {
		t.Fatalf("newMiddleware() error = %v", err)
	}

	// Handler 把 context 中最终请求 ID 原样返回，便于同时检查上下文和响应头。
	requestIDHandler := func(context *gin.Context) {
		context.String(http.StatusOK, RequestID(context))
	}

	tests := []struct {
		// name 说明请求头所处的边界情况。
		name string
		// incoming 是调用方传入的请求 ID，空字符串表示没有请求头。
		incoming string
		// want 是中间件最终应采用并回传的 ID。
		want string
	}{
		{name: "缺失时生成", want: "generated-id"},
		{name: "合法值沿用", incoming: "caller-123", want: "caller-123"},
		{name: "超长值替换", incoming: strings.Repeat("x", maximumRequestIDLength+1), want: "generated-id"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// 每个子场景新建路由，避免中间件状态或路由注册互相影响。
			router := gin.New()
			router.Use(middleware.RequestID())
			router.GET("/", requestIDHandler)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			// 只有非空场景才设置请求头，真实覆盖“完全缺失”分支。
			if test.incoming != "" {
				request.Header.Set(RequestIDHeader, test.incoming)
			}

			// 执行请求后，正文中的 context 值应与预期一致。
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK || recorder.Body.String() != test.want {
				t.Fatalf("response = (%d, %q)", recorder.Code, recorder.Body.String())
			}
			// 响应头也必须带同一个 ID，客户端和日志才能串起完整调用链。
			if recorder.Header().Get(RequestIDHeader) != test.want {
				t.Fatalf("response request id = %q", recorder.Header().Get(RequestIDHeader))
			}
		})
	}
}

// TestAccessLogRecordsRequiredFields 验证一条请求完成后，结构化日志包含排查问题所需的固定字段。
func TestAccessLogRecordsRequiredFields(t *testing.T) {
	// JSON 日志写入内存缓冲区，测试可以逐字段检查而不依赖终端格式。
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	middleware, err := newMiddleware(logger, func() string { return "request-1" })
	if err != nil {
		t.Fatalf("newMiddleware() error = %v", err)
	}

	// RequestID 必须先执行，AccessLog 才能把同一个编号写入日志。
	router := gin.New()
	router.Use(middleware.RequestID(), middleware.AccessLog())
	router.GET("/documents", func(context *gin.Context) {
		context.Status(http.StatusAccepted)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/documents?size=20", nil))

	// 请求结束后读取完整日志文本，逐项确认排障所需字段存在。
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
	// 固定请求 ID 便于 Recovery 记录日志，但本测试只检查对客户端的响应边界。
	gin.SetMode(gin.TestMode)
	middleware, err := newMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), func() string { return "request-2" })
	if err != nil {
		t.Fatalf("newMiddleware() error = %v", err)
	}

	router := gin.New()
	router.Use(middleware.RequestID(), middleware.Recovery())
	// panic 文本故意包含敏感信息，验证响应体不会直接回显内部异常。
	router.GET("/panic", func(*gin.Context) {
		panic("database password should stay private")
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))

	// Recovery 应接住 panic 并统一返回 500，进程不能跟着崩溃。
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d, want 500", recorder.Code)
	}
	// 客户端只能得到通用错误，数据库密码字样不能出现在响应中。
	if strings.Contains(recorder.Body.String(), "database password") {
		t.Fatalf("panic response leaked internal text: %q", recorder.Body.String())
	}
}
