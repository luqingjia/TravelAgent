// Package domain 保存 Agent 限界上下文的稳定业务规则：模型公开元数据、消息历史不变量与错误分类。
//
// 本包只依赖 Go 标准库，不导入 Gin、Eino、配置或 HTTP 类型。
// API Key、带凭据 URL 与完整 prompt 不得进入领域公开结构；HTTP/Eino 边界再显式转换。
package domain

// 本文件定义模型公开元数据和目录视图。
// API Key、带凭据 URL 等敏感字段不得进入这些公开结构。
// 领域对象不携带 json tag；HTTP 边界再显式转换成 DTO。

// ModelCapabilities 描述当前模型对客户端可见的能力开关。
type ModelCapabilities struct {
	// Chat 表示支持普通 JSON 完整对话。
	Chat bool
	// Streaming 表示支持 SSE 流式对话。
	Streaming bool
	// Tools 表示已注册至少一种工具。
	Tools bool
}

// PublicModel 是模型目录中可安全返回给前端的元数据。
// 只包含 ID、展示名、供应商、模型名、配置可用状态和能力，不包含密钥。
type PublicModel struct {
	// ID 是配置中的稳定模型标识。
	ID string
	// DisplayName 是前端展示名称。
	DisplayName string
	// Provider 是供应商标识，例如 dashscope。
	Provider string
	// Model 是供应商侧模型名称。
	Model string
	// Enabled 表示配置上是否启用；禁用模型不可选择。
	Enabled bool
	// Available 表示配置可用且运行时已准备好。
	Available bool
	// Capabilities 描述 chat/streaming/tools 能力。
	Capabilities ModelCapabilities
}

// CatalogView 是模型目录查询结果。
type CatalogView struct {
	// DefaultModelID 是未指定模型时使用的默认 ID。
	DefaultModelID string
	// Models 是全部已配置模型的公开元数据，包含禁用项以便前端展示。
	Models []PublicModel
}
