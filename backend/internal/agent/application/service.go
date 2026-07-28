// Package application 编排 Agent 模型选择、消息历史校验以及 Chat/Stream 共享路径。
//
// 本包定义自己需要的 ModelCatalog/AgentRuntime 小接口，不导入 Gin、Eino、sqlx 或 AWS SDK。
// 具体模型目录和 Eino Runner 由 adapter 实现，并由 internal/app 组合根注入。
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/luqingjia/TravelAgent/internal/agent/domain"
)

// Service 是 Agent 业务用例入口。
// Chat 与 Stream 共用模型解析和历史校验，避免两套业务逻辑漂移。
type Service struct {
	// catalog 提供公开模型目录与默认模型 ID。
	catalog ModelCatalog
	// runtime 执行具体模型对话。
	runtime AgentRuntime
	// limits 是消息历史安全上限。
	limits domain.HistoryLimits
}

// Dependencies 是构造 Service 需要的长期依赖。
type Dependencies struct {
	// Catalog 是模型目录端口。
	Catalog ModelCatalog
	// Runtime 是 Agent 运行时端口。
	Runtime AgentRuntime
	// Limits 是历史限制；必须为正数。
	Limits domain.HistoryLimits
}

// NewService 校验依赖并创建 Agent 应用服务。
func NewService(deps Dependencies) (*Service, error) {
	// 目录缺失会导致 ListModels 和默认模型解析无法工作。
	if deps.Catalog == nil {
		return nil, fmt.Errorf("agent model catalog is required")
	}
	// 运行时缺失时无法进入对话路径。
	if deps.Runtime == nil {
		return nil, fmt.Errorf("agent runtime is required")
	}
	// 限制必须为正，防止配置错误被静默成零值。
	if deps.Limits.MaxMessages <= 0 || deps.Limits.MaxMessageChars <= 0 || deps.Limits.MaxTotalChars <= 0 {
		return nil, fmt.Errorf("agent history limits must be positive")
	}
	return &Service{
		catalog: deps.Catalog,
		runtime: deps.Runtime,
		limits:  deps.Limits,
	}, nil
}

// ListModels 返回脱敏模型目录。
func (service *Service) ListModels() domain.CatalogView {
	return service.catalog.List()
}

// Chat 解析模型、校验历史后调用 Runtime.Chat。
func (service *Service) Chat(ctx context.Context, modelID string, messages []domain.Message) (domain.AssistantMessage, error) {
	// 共享准备路径：解析模型 + 校验消息。
	selectedModelID, prepared, err := service.prepare(modelID, messages)
	if err != nil {
		return domain.AssistantMessage{}, err
	}
	// Context 必须向下传递，以便客户端取消时停止模型调用。
	result, err := service.runtime.Chat(ctx, selectedModelID, prepared)
	if err != nil {
		return domain.AssistantMessage{}, classifyRuntimeError(err)
	}
	return result, nil
}

// Stream 解析模型、校验历史后调用 Runtime.Stream。
// 预检失败直接返回 error，由 HTTP 层映射为 JSON 4xx；流开始后的错误走事件通道。
func (service *Service) Stream(ctx context.Context, modelID string, messages []domain.Message) (<-chan StreamEvent, error) {
	selectedModelID, prepared, err := service.prepare(modelID, messages)
	if err != nil {
		return nil, err
	}
	events, err := service.runtime.Stream(ctx, selectedModelID, prepared)
	if err != nil {
		return nil, classifyRuntimeError(err)
	}
	return events, nil
}

// prepare 是 Chat/Stream 的共享编排：默认模型解析、可用性检查、历史校验。
func (service *Service) prepare(modelID string, messages []domain.Message) (string, []domain.Message, error) {
	// 先校验历史，避免无谓目录查找。
	if err := domain.ValidateHistory(messages, service.limits); err != nil {
		return "", nil, err
	}
	// 归一化调用方输入的模型 ID；空值使用目录默认模型。
	selected := strings.TrimSpace(modelID)
	catalog := service.catalog.List()
	if selected == "" {
		selected = strings.TrimSpace(catalog.DefaultModelID)
	}
	if selected == "" {
		return "", nil, fmt.Errorf("%w: default model is not configured", domain.ErrInvalidArgument)
	}

	// 在目录中查找目标模型，并检查启用/可用状态。
	var found *domain.PublicModel
	for index := range catalog.Models {
		if catalog.Models[index].ID == selected {
			found = &catalog.Models[index]
			break
		}
	}
	if found == nil {
		return "", nil, fmt.Errorf("%w: %s", domain.ErrModelNotFound, selected)
	}
	if !found.Enabled {
		return "", nil, fmt.Errorf("%w: %s", domain.ErrModelDisabled, selected)
	}
	if !found.Available {
		return "", nil, fmt.Errorf("%w: %s", domain.ErrModelUnavailable, selected)
	}

	// 返回拷贝后的消息切片，防止调用方在异步流过程中修改底层数组。
	prepared := append([]domain.Message(nil), messages...)
	return selected, prepared, nil
}

// classifyRuntimeError 把运行时未知错误收敛为 ErrAgentFailed，同时保留可识别的领域错误。
func classifyRuntimeError(err error) error {
	if err == nil {
		return nil
	}
	// 已是领域分类错误时直接透传，HTTP 可稳定映射。
	if isDomainError(err) {
		return err
	}
	// 模型/Agent 运行失败只公开通用类别，技术细节保留在错误链中供服务端日志使用。
	return fmt.Errorf("%w: %v", domain.ErrAgentFailed, err)
}

// isDomainError 判断错误是否已经属于 Agent 领域分类。
func isDomainError(err error) bool {
	return errors.Is(err, domain.ErrInvalidArgument) ||
		errors.Is(err, domain.ErrModelNotFound) ||
		errors.Is(err, domain.ErrModelDisabled) ||
		errors.Is(err, domain.ErrModelUnavailable) ||
		errors.Is(err, domain.ErrInvalidTimezone) ||
		errors.Is(err, domain.ErrAgentFailed)
}
