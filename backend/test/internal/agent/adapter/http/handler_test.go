package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	. "github.com/luqingjia/TravelAgent/internal/agent/adapter/http"
	"github.com/luqingjia/TravelAgent/internal/agent/application"
	"github.com/luqingjia/TravelAgent/internal/agent/domain"
	"github.com/luqingjia/TravelAgent/internal/platform/httpserver"
)

// TestRegisterRoutesExposesAgentEndpoints 验证 Agent 三条路由与健康检查一并注册。
func TestRegisterRoutesExposesAgentEndpoints(t *testing.T) {
	router := newTestRouter(t, &fakeAgentService{})
	actual := make([]string, 0, len(router.Routes()))
	for _, route := range router.Routes() {
		actual = append(actual, route.Method+" "+route.Path)
	}
	sort.Strings(actual)
	want := []string{
		"GET /api/agent/models",
		"GET /health",
		"POST /api/agent/chat",
		"POST /api/agent/chat/stream",
	}
	sort.Strings(want)
	if strings.Join(actual, "\n") != strings.Join(want, "\n") {
		t.Fatalf("routes =\n%s\nwant =\n%s", strings.Join(actual, "\n"), strings.Join(want, "\n"))
	}
}

// TestListModelsReturnsPublicCatalogWithoutSecrets 验证模型目录脱敏且使用统一外壳。
func TestListModelsReturnsPublicCatalogWithoutSecrets(t *testing.T) {
	service := &fakeAgentService{
		catalog: domain.CatalogView{
			DefaultModelID: "qwen-plus",
			Models: []domain.PublicModel{{
				ID: "qwen-plus", DisplayName: "Qwen Plus", Provider: "dashscope", Model: "qwen-plus",
				Enabled: true, Available: true,
				Capabilities: domain.ModelCapabilities{Chat: true, Streaming: true, Tools: true},
			}},
		},
	}
	router := newTestRouter(t, service)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/agent/models", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"code":"0"`) || !strings.Contains(body, `"defaultModelId":"qwen-plus"`) {
		t.Fatalf("envelope/body = %s", body)
	}
	if !strings.Contains(body, `"capabilities":["chat","streaming","tools"]`) {
		t.Fatalf("capabilities contract missing: %s", body)
	}
	if strings.Contains(body, "apiKey") || strings.Contains(body, "replace-me") || strings.Contains(body, "baseURL") {
		t.Fatalf("catalog leaked secret fields: %s", body)
	}
}

// TestChatReturnsAssistantMessageEnvelope 验证 JSON 对话成功响应包含 modelId 与 assistant message。
func TestChatReturnsAssistantMessageEnvelope(t *testing.T) {
	service := &fakeAgentService{
		chatResult: domain.AssistantMessage{ModelID: "qwen-plus", Content: "你好"},
	}
	router := newTestRouter(t, service)
	payload := `{"modelId":"qwen-plus","messages":[{"role":"user","content":"hi"}]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/agent/chat", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Code string `json:"code"`
		Data struct {
			ModelID string `json:"modelId"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode = %v body=%s", err, recorder.Body.String())
	}
	if envelope.Code != "0" || envelope.Data.ModelID != "qwen-plus" ||
		envelope.Data.Message.Role != "assistant" || envelope.Data.Message.Content != "你好" {
		t.Fatalf("response = %#v", envelope)
	}
	if service.lastModelID != "qwen-plus" || len(service.lastMessages) != 1 {
		t.Fatalf("service input model=%q messages=%#v", service.lastModelID, service.lastMessages)
	}
}

// TestChatPrecheckReturnsJSONClientError 验证流开始前的参数错误返回 JSON 4xx。
func TestChatPrecheckReturnsJSONClientError(t *testing.T) {
	service := &fakeAgentService{chatErr: domain.ErrInvalidArgument}
	router := newTestRouter(t, service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/agent/chat", strings.NewReader(`{"messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"A000001"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

// TestStreamWritesSSEHeadersAndEvents 验证 SSE headers、message/done framing 与 flush 语义。
func TestStreamWritesSSEHeadersAndEvents(t *testing.T) {
	service := &fakeAgentService{
		streamEvents: []application.StreamEvent{
			{Kind: application.EventKindMessage, Content: "你"},
			{Kind: application.EventKindMessage, Content: "好"},
			{Kind: application.EventKindDone, ModelID: "qwen-plus"},
		},
	}
	router := newTestRouter(t, service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/agent/chat/stream",
		strings.NewReader(`{"modelId":"qwen-plus","messages":[{"role":"user","content":"hi"}]}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if ct := recorder.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if recorder.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("Cache-Control = %q", recorder.Header().Get("Cache-Control"))
	}
	if recorder.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatalf("X-Accel-Buffering = %q", recorder.Header().Get("X-Accel-Buffering"))
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "event: message\ndata: {\"content\":\"你\"}") {
		t.Fatalf("missing message event: %s", body)
	}
	if !strings.Contains(body, "event: done\ndata: {\"modelId\":\"qwen-plus\"}") {
		t.Fatalf("missing done event: %s", body)
	}
}

// TestStreamPrecheckKeepsJSONErrorBeforeSSE 验证流开始前错误仍是 JSON，而不是 SSE。
func TestStreamPrecheckKeepsJSONErrorBeforeSSE(t *testing.T) {
	service := &fakeAgentService{streamErr: domain.ErrModelNotFound}
	router := newTestRouter(t, service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/agent/chat/stream",
		strings.NewReader(`{"modelId":"missing","messages":[{"role":"user","content":"hi"}]}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("precheck should not start SSE: %v", recorder.Header())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"A000001"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

// TestStreamRuntimeErrorUsesErrorEvent 验证流开始后的运行错误通过 error 事件发送且保持 200。
func TestStreamRuntimeErrorUsesErrorEvent(t *testing.T) {
	service := &fakeAgentService{
		streamEvents: []application.StreamEvent{
			{Kind: application.EventKindMessage, Content: "partial"},
			{Kind: application.EventKindError, Code: "B000001", Message: "agent run failed"},
		},
	}
	router := newTestRouter(t, service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/agent/chat/stream",
		strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "event: error") || !strings.Contains(body, `"code":"B000001"`) {
		t.Fatalf("missing error event: %s", body)
	}
}

// TestStreamCancelsWhenClientDisconnects 验证客户端断开时请求 Context 被取消并停止消费。
func TestStreamCancelsWhenClientDisconnects(t *testing.T) {
	started := make(chan struct{})
	service := &fakeAgentService{
		blockStream:   true,
		streamStarted: started,
	}
	router := newTestRouter(t, service)

	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/api/agent/chat/stream",
		strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		router.ServeHTTP(recorder, request)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not start")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not stop after cancel")
	}
}

func newTestRouter(t *testing.T, service AgentService) *gin.Engine {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, err := NewHandler(service, logger)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	middleware, err := httpserver.NewMiddleware(logger)
	if err != nil {
		t.Fatalf("NewMiddleware() error = %v", err)
	}
	router := httpserver.NewRouter(middleware)
	RegisterRoutes(router, handler)
	return router
}

type fakeAgentService struct {
	catalog       domain.CatalogView
	chatResult    domain.AssistantMessage
	chatErr       error
	streamEvents  []application.StreamEvent
	streamErr     error
	blockStream   bool
	streamStarted chan struct{}
	lastModelID   string
	lastMessages  []domain.Message
}

func (service *fakeAgentService) ListModels() domain.CatalogView {
	return service.catalog
}

func (service *fakeAgentService) Chat(ctx context.Context, modelID string, messages []domain.Message) (domain.AssistantMessage, error) {
	_ = ctx
	service.lastModelID = modelID
	service.lastMessages = append([]domain.Message(nil), messages...)
	if service.chatErr != nil {
		return domain.AssistantMessage{}, service.chatErr
	}
	return service.chatResult, nil
}

func (service *fakeAgentService) Stream(ctx context.Context, modelID string, messages []domain.Message) (<-chan application.StreamEvent, error) {
	service.lastModelID = modelID
	service.lastMessages = append([]domain.Message(nil), messages...)
	if service.streamErr != nil {
		return nil, service.streamErr
	}
	out := make(chan application.StreamEvent, len(service.streamEvents)+1)
	go func() {
		defer close(out)
		if service.streamStarted != nil {
			close(service.streamStarted)
		}
		if service.blockStream {
			<-ctx.Done()
			return
		}
		for _, event := range service.streamEvents {
			select {
			case <-ctx.Done():
				return
			case out <- event:
			}
		}
	}()
	return out, nil
}

// 确保 fake 编译期满足接口；errors 导入用于可能扩展断言。
var _ = errors.New
var _ = bytes.NewReader
