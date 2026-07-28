package application

import (
	"context"

	"github.com/luqingjia/TravelAgent/internal/agent/domain"
)

// 本文件定义 Agent 应用层对外端口和框架无关的流事件。
// application 只依赖 domain 与自己的小接口，不导入 Gin 或 Eino 具体类型。

// EventKind 描述流式事件类型。
type EventKind string

const (
	// EventKindMessage 表示助手文本增量。
	EventKindMessage EventKind = "message"
	// EventKindDone 表示本轮对话成功结束。
	EventKindDone EventKind = "done"
	// EventKindError 表示运行过程中的失败。
	EventKindError EventKind = "error"
)

// StreamEvent 是 Runtime 向应用层/HTTP 层传递的框架无关事件。
type StreamEvent struct {
	// Kind 是 message/done/error 之一。
	Kind EventKind
	// Content 在 message 事件中携带文本增量。
	Content string
	// ModelID 在 done 事件中标识实际使用的模型。
	ModelID string
	// Code 在 error 事件中携带稳定业务码。
	Code string
	// Message 在 error 事件中携带可公开消息。
	Message string
}

// ModelCatalog 提供只读模型目录。
type ModelCatalog interface {
	// List 返回脱敏后的模型目录视图。
	List() domain.CatalogView
}

// AgentRuntime 封装 Eino 等具体运行时，提供同步与流式对话能力。
type AgentRuntime interface {
	// Chat 执行完整对话并返回拼接后的助手消息。
	Chat(ctx context.Context, modelID string, messages []domain.Message) (domain.AssistantMessage, error)
	// Stream 启动流式对话，返回只读事件通道；通道在结束或失败后关闭。
	Stream(ctx context.Context, modelID string, messages []domain.Message) (<-chan StreamEvent, error)
}
