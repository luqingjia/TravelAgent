package httpadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/luqingjia/TravelAgent/internal/agent/application"
	"github.com/luqingjia/TravelAgent/internal/agent/domain"
	"github.com/luqingjia/TravelAgent/internal/platform/httpserver"
)

// AgentService 是 HTTP Handler 真正调用的应用用例集合。
// 接口定义在使用方 HTTP 适配器，测试可传 fake，生产注入 *application.Service。
type AgentService interface {
	// ListModels 返回脱敏模型目录。
	ListModels() domain.CatalogView
	// Chat 执行普通 JSON 对话。
	Chat(ctx context.Context, modelID string, messages []domain.Message) (domain.AssistantMessage, error)
	// Stream 启动流式对话，预检失败直接返回 error。
	Stream(ctx context.Context, modelID string, messages []domain.Message) (<-chan application.StreamEvent, error)
}

// Handler 保存应用服务和日志器，不从 gin.Context 做服务定位。
type Handler struct {
	service AgentService
	logger  *slog.Logger
}

// 编译期确认应用服务满足 Handler 接口。
var _ AgentService = (*application.Service)(nil)

// NewHandler 校验依赖并创建 Agent HTTP 处理器。
func NewHandler(service AgentService, logger *slog.Logger) (*Handler, error) {
	if service == nil {
		return nil, fmt.Errorf("agent application service is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("agent HTTP logger is required")
	}
	return &Handler{service: service, logger: logger}, nil
}

// listModels 返回公开模型目录，不包含 API Key。
func (handler *Handler) listModels(context *gin.Context) {
	context.JSON(http.StatusOK, Success(catalogResponseFromDomain(handler.service.ListModels())))
}

// chat 处理普通 JSON 对话。
func (handler *Handler) chat(context *gin.Context) {
	request, messages, err := bindChatRequest(context)
	if err != nil {
		handler.writeError(context, err)
		return
	}
	result, err := handler.service.Chat(context.Request.Context(), request.ModelID, messages)
	if err != nil {
		handler.writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, Success(chatResponse{
		ModelID: result.ModelID,
		Message: chatMessageResponse{
			Role:    string(domain.RoleAssistant),
			Content: result.Content,
		},
	}))
}

// chatStream 处理 SSE 流式对话。
// 预检错误返回 JSON 4xx；开始流后只发送 event，不再改 HTTP 状态码。
func (handler *Handler) chatStream(context *gin.Context) {
	request, messages, err := bindChatRequest(context)
	if err != nil {
		handler.writeError(context, err)
		return
	}

	// 先完成应用层预检；失败时仍可写 JSON。
	events, err := handler.service.Stream(context.Request.Context(), request.ModelID, messages)
	if err != nil {
		handler.writeError(context, err)
		return
	}

	// 设置 SSE 响应头；此后只能写 event framing。
	writer := context.Writer
	header := writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)
	flusher, canFlush := writer.(http.Flusher)

	// 浏览器断开时 Gin 会取消 Request.Context，循环中检测后停止消费 Runtime。
	requestCtx := context.Request.Context()
	for {
		select {
		case <-requestCtx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := writeSSEEvent(writer, event); err != nil {
				handler.logger.ErrorContext(requestCtx, "write SSE event failed",
					"request_id", httpserver.RequestID(context),
					"error", err,
				)
				return
			}
			if canFlush {
				flusher.Flush()
			}
			// error 事件后结束流，状态码保持 200。
			if event.Kind == application.EventKindError {
				return
			}
		}
	}
}

// bindChatRequest 解析 JSON 请求体并转换为领域消息。
func bindChatRequest(context *gin.Context) (chatRequest, []domain.Message, error) {
	var request chatRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		return chatRequest{}, nil, fmt.Errorf("%w: invalid chat request body", domain.ErrInvalidArgument)
	}
	messages := make([]domain.Message, 0, len(request.Messages))
	for _, item := range request.Messages {
		messages = append(messages, domain.Message{
			Role:    domain.Role(item.Role),
			Content: item.Content,
		})
	}
	return request, messages, nil
}

// writeSSEEvent 按 SSE 规范写出单个 event。
func writeSSEEvent(writer http.ResponseWriter, event application.StreamEvent) error {
	var (
		eventName string
		payload   any
	)
	switch event.Kind {
	case application.EventKindMessage:
		eventName = "message"
		payload = sseMessageData{Content: event.Content}
	case application.EventKindDone:
		eventName = "done"
		payload = sseDoneData{ModelID: event.ModelID}
	case application.EventKindError:
		eventName = "error"
		payload = sseErrorData{Code: event.Code, Message: event.Message}
	default:
		return fmt.Errorf("unknown stream event kind %q", event.Kind)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// event 行 + data 行 + 空行结束一个 SSE 帧。
	if _, err := fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", eventName, body); err != nil {
		return err
	}
	return nil
}

// writeError 记录服务端错误并写出统一 JSON 外壳。
func (handler *Handler) writeError(context *gin.Context, err error) {
	status, result := errorResponse(err)
	// 5xx 记录完整错误链，但不记录 prompt 或密钥。
	if status >= http.StatusInternalServerError {
		handler.logger.ErrorContext(context.Request.Context(), "agent request failed",
			"request_id", httpserver.RequestID(context),
			"error", err,
		)
	}
	context.JSON(status, result)
}
