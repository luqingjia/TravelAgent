package domain

import "errors"

// 本文件声明 Agent 限界上下文的稳定业务错误类别。
// 外层使用 fmt.Errorf 与 %w 包装，既保留业务分类，也保留服务端诊断链。
// HTTP 适配器只通过 errors.Is 映射状态码，禁止根据错误字符串分类。

var (
	// ErrInvalidArgument 表示请求参数、消息历史或时区不满足业务约束。
	ErrInvalidArgument = errors.New("invalid argument")
	// ErrModelNotFound 表示请求的模型 ID 不存在于当前配置目录。
	ErrModelNotFound = errors.New("model not found")
	// ErrModelDisabled 表示模型存在但当前未启用，不可选择。
	ErrModelDisabled = errors.New("model is disabled")
	// ErrModelUnavailable 表示模型配置不完整或运行时尚未准备好。
	ErrModelUnavailable = errors.New("model is unavailable")
	// ErrInvalidTimezone 表示 get_current_time 收到了无法加载的 IANA 时区。
	ErrInvalidTimezone = errors.New("invalid timezone")
	// ErrAgentFailed 表示模型或 Agent 运行失败，客户端只能看到通用消息。
	ErrAgentFailed = errors.New("agent run failed")
)
