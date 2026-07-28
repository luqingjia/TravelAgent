// Package einoadapter 使用 CloudWeGo Eino 创建 ChatModel、ChatModelAgent、Runner 和工具集合。
//
// 本包可以依赖 Eino 与 openai 扩展，但只能实现 application 定义的小接口。
// 不得把 API Key 写入日志，也不得在错误消息中回显完整 prompt 或供应商响应。
package einoadapter

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/luqingjia/TravelAgent/internal/agent/application"
	"github.com/luqingjia/TravelAgent/internal/agent/domain"
	"github.com/luqingjia/TravelAgent/internal/platform/config"
)

// Runtime 按模型 ID 持有不可变 Runner 目录，并实现 application.AgentRuntime。
type Runtime struct {
	// catalog 提供脱敏模型目录。
	catalog application.ModelCatalog
	// runners 以稳定 model ID 索引已启用模型的 Runner。
	runners map[string]*adk.Runner
	// maxIterations 仅用于构造期记录；真正限制已写入各 Agent。
	maxIterations int
}

// staticCatalog 是基于配置快照的只读模型目录。
type staticCatalog struct {
	view domain.CatalogView
}

// List 返回构造时冻结的目录视图。
func (catalog *staticCatalog) List() domain.CatalogView {
	return catalog.view
}

// modelRuntime 保存单个模型构造结果。
type modelRuntime struct {
	public domain.PublicModel
	runner *adk.Runner
}

// NewRuntime 根据配置为每个启用模型创建 OpenAI ChatModel、工具、ChatModelAgent 和 Runner。
// 禁用模型仍会出现在公开目录中，但 Available=false 且没有 Runner。
func NewRuntime(ctx context.Context, agentConfig config.Agent) (*Runtime, application.ModelCatalog, error) {
	if len(agentConfig.Models) == 0 {
		return nil, nil, fmt.Errorf("agent models are required")
	}
	if agentConfig.MaxIterations <= 0 {
		return nil, nil, fmt.Errorf("agent max iterations must be positive")
	}

	// 工具对所有模型共享同一实现：只做本地时区换算。
	timeTool, err := newGetCurrentTimeTool()
	if err != nil {
		return nil, nil, fmt.Errorf("create get_current_time tool: %w", err)
	}

	publicModels := make([]domain.PublicModel, 0, len(agentConfig.Models))
	runners := make(map[string]*adk.Runner, len(agentConfig.Models))

	for _, modelConfig := range agentConfig.Models {
		public := domain.PublicModel{
			ID:          modelConfig.ID,
			DisplayName: modelConfig.DisplayName,
			Provider:    modelConfig.Provider,
			Model:       modelConfig.Model,
			Enabled:     modelConfig.Enabled,
			// 默认不可用；只有成功创建 Runner 后才标记可用。
			Available: false,
			Capabilities: domain.ModelCapabilities{
				Chat:      true,
				Streaming: true,
				Tools:     true,
			},
		}
		// 禁用模型只进入目录，不创建外部客户端。
		if !modelConfig.Enabled {
			publicModels = append(publicModels, public)
			continue
		}

		entry, err := buildModelRuntime(ctx, modelConfig, agentConfig.MaxIterations, timeTool)
		if err != nil {
			return nil, nil, fmt.Errorf("create agent runtime for model %s: %w", modelConfig.ID, err)
		}
		public = entry.public
		runners[modelConfig.ID] = entry.runner
		publicModels = append(publicModels, public)
	}

	catalog := &staticCatalog{
		view: domain.CatalogView{
			DefaultModelID: agentConfig.DefaultModelID,
			Models:         publicModels,
		},
	}
	return &Runtime{
		catalog:       catalog,
		runners:       runners,
		maxIterations: agentConfig.MaxIterations,
	}, catalog, nil
}

// buildModelRuntime 为单个启用模型装配 ChatModel + Agent + Runner。
func buildModelRuntime(
	ctx context.Context,
	modelConfig config.AgentModel,
	maxIterations int,
	timeTool tool.BaseTool,
) (modelRuntime, error) {
	// openai.NewChatModel 使用 OpenAI 兼容协议；BaseURL/Model/APIKey/Timeout 来自配置。
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  modelConfig.APIKey,
		BaseURL: modelConfig.BaseURL,
		Model:   modelConfig.Model,
		Timeout: modelConfig.Timeout,
	})
	if err != nil {
		return modelRuntime{}, fmt.Errorf("create chat model: %w", err)
	}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        sanitizeAgentName(modelConfig.ID),
		Description: "TravelAgent chat assistant for model " + modelConfig.ID,
		Instruction: "You are TravelAgent assistant. Use tools when needed. Prefer concise, accurate answers.",
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{timeTool},
			},
		},
		// MaxIterations 必须有限，防止模型与工具无限循环。
		MaxIterations: maxIterations,
	})
	if err != nil {
		return modelRuntime{}, fmt.Errorf("create chat model agent: %w", err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
	})
	return modelRuntime{
		public: domain.PublicModel{
			ID:          modelConfig.ID,
			DisplayName: modelConfig.DisplayName,
			Provider:    modelConfig.Provider,
			Model:       modelConfig.Model,
			Enabled:     true,
			Available:   true,
			Capabilities: domain.ModelCapabilities{
				Chat:      true,
				Streaming: true,
				Tools:     true,
			},
		},
		runner: runner,
	}, nil
}

// Catalog 返回构造期冻结的模型目录。
func (runtime *Runtime) Catalog() application.ModelCatalog {
	return runtime.catalog
}

// Chat 收集同一运行路径的流式增量，返回完整助手消息。
func (runtime *Runtime) Chat(ctx context.Context, modelID string, messages []domain.Message) (domain.AssistantMessage, error) {
	events, err := runtime.Stream(ctx, modelID, messages)
	if err != nil {
		return domain.AssistantMessage{}, err
	}
	var builder strings.Builder
	for event := range events {
		switch event.Kind {
		case application.EventKindMessage:
			builder.WriteString(event.Content)
		case application.EventKindDone:
			return domain.AssistantMessage{ModelID: event.ModelID, Content: builder.String()}, nil
		case application.EventKindError:
			// 流内错误转换为返回值，供 JSON 接口映射。
			return domain.AssistantMessage{}, fmt.Errorf("%w: %s", domain.ErrAgentFailed, event.Message)
		}
	}
	// 通道关闭但没有 done/error 时，若已有内容仍视为成功，否则判失败。
	content := builder.String()
	if content == "" {
		return domain.AssistantMessage{}, fmt.Errorf("%w: empty agent response", domain.ErrAgentFailed)
	}
	return domain.AssistantMessage{ModelID: modelID, Content: content}, nil
}

// Stream 启动指定模型的 Runner，并把 Eino 事件映射为框架无关 StreamEvent。
func (runtime *Runtime) Stream(ctx context.Context, modelID string, messages []domain.Message) (<-chan application.StreamEvent, error) {
	runner, ok := runtime.runners[modelID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", domain.ErrModelUnavailable, modelID)
	}
	schemaMessages, err := toSchemaMessages(messages)
	if err != nil {
		return nil, err
	}

	out := make(chan application.StreamEvent, 16)
	go func() {
		defer close(out)
		// Runner.Run 返回异步事件迭代器；客户端取消时停止遍历。
		iterator := runner.Run(ctx, schemaMessages)
		for {
			// Context 取消优先检查，避免在已断开连接后继续消费模型输出。
			if ctx.Err() != nil {
				sendEvent(out, application.StreamEvent{
					Kind:    application.EventKindError,
					Code:    "B000001",
					Message: "request canceled",
				})
				return
			}
			event, ok := iterator.Next()
			if !ok {
				// 正常结束：发送 done。
				sendEvent(out, application.StreamEvent{
					Kind:    application.EventKindDone,
					ModelID: modelID,
				})
				return
			}
			if event == nil {
				continue
			}
			if event.Err != nil {
				sendEvent(out, application.StreamEvent{
					Kind:    application.EventKindError,
					Code:    "B000001",
					Message: "agent run failed",
				})
				return
			}
			if event.Output == nil || event.Output.MessageOutput == nil {
				continue
			}
			// 只向客户端转发助手文本；工具消息保留在 Eino 内部循环。
			if event.Output.MessageOutput.Role != schema.Assistant {
				// 流式工具消息仍需关闭底层 stream，防止泄漏。
				closeMessageVariant(event.Output.MessageOutput)
				continue
			}
			if err := emitAssistantContent(ctx, out, event.Output.MessageOutput); err != nil {
				sendEvent(out, application.StreamEvent{
					Kind:    application.EventKindError,
					Code:    "B000001",
					Message: "agent run failed",
				})
				return
			}
		}
	}()
	return out, nil
}

// emitAssistantContent 把非流式或流式助手消息转换成 message 事件。
func emitAssistantContent(ctx context.Context, out chan<- application.StreamEvent, variant *adk.MessageVariant) error {
	if variant == nil {
		return nil
	}
	if variant.IsStreaming && variant.MessageStream != nil {
		// 建议开启自动关闭，防止未读完的 stream 泄漏。
		variant.MessageStream.SetAutomaticClose()
		for {
			if ctx.Err() != nil {
				variant.MessageStream.Close()
				return ctx.Err()
			}
			chunk, err := variant.MessageStream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if chunk == nil || chunk.Content == "" {
				continue
			}
			if !sendEvent(out, application.StreamEvent{
				Kind:    application.EventKindMessage,
				Content: chunk.Content,
			}) {
				return context.Canceled
			}
		}
	}
	if variant.Message != nil && variant.Message.Content != "" {
		if !sendEvent(out, application.StreamEvent{
			Kind:    application.EventKindMessage,
			Content: variant.Message.Content,
		}) {
			return context.Canceled
		}
	}
	return nil
}

// closeMessageVariant 关闭可能存在的消息流。
func closeMessageVariant(variant *adk.MessageVariant) {
	if variant == nil || variant.MessageStream == nil {
		return
	}
	variant.MessageStream.SetAutomaticClose()
	variant.MessageStream.Close()
}

// sendEvent 尝试发送事件；通道已关闭或阻塞时可扩展，这里使用带缓冲通道。
func sendEvent(out chan<- application.StreamEvent, event application.StreamEvent) bool {
	// 使用 defer/recover 防御向已关闭通道发送；正常路径不会关闭。
	defer func() { _ = recover() }()
	out <- event
	return true
}

// toSchemaMessages 把领域消息转换成 Eino schema 消息。
func toSchemaMessages(messages []domain.Message) ([]*schema.Message, error) {
	result := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case domain.RoleUser:
			result = append(result, schema.UserMessage(message.Content))
		case domain.RoleAssistant:
			result = append(result, schema.AssistantMessage(message.Content, nil))
		default:
			return nil, fmt.Errorf("%w: unsupported role %q", domain.ErrInvalidArgument, message.Role)
		}
	}
	return result, nil
}

// sanitizeAgentName 把模型 ID 转成 Eino Agent 可接受的名称。
func sanitizeAgentName(modelID string) string {
	// 空格和路径分隔符替换成下划线，保证 Name 稳定可读。
	name := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '/', '\\', ':':
			return '_'
		default:
			return r
		}
	}, strings.TrimSpace(modelID))
	if name == "" {
		return "travel_agent"
	}
	return name
}

// 编译期确认 Runtime 实现 AgentRuntime。
var (
	_ application.AgentRuntime = (*Runtime)(nil)
	_ application.ModelCatalog = (*staticCatalog)(nil)
)
