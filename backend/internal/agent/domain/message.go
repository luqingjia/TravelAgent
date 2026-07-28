package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// 本文件定义对话消息值对象和历史校验规则。
// 领域层只依赖标准库，不感知 Gin、Eino 或配置来源。

// Role 是允许进入 Agent 的消息角色。
type Role string

const (
	// RoleUser 表示调用方输入。
	RoleUser Role = "user"
	// RoleAssistant 表示历史中的助手回复。
	RoleAssistant Role = "assistant"
)

// Message 是一轮对话中的单条消息。
// 领域对象不携带 json tag，HTTP/Eino 边界再显式转换。
type Message struct {
	// Role 只能是 user 或 assistant。
	Role Role
	// Content 是消息正文，不能为空。
	Content string
}

// AssistantMessage 是一次完整对话后返回的助手结果。
type AssistantMessage struct {
	// ModelID 是实际执行对话的模型 ID。
	ModelID string
	// Content 是拼接后的完整助手文本。
	Content string
}

// HistoryLimits 保存消息历史的安全上限。
// 数值来自配置，但校验规则属于领域不变量。
type HistoryLimits struct {
	// MaxMessages 是允许的最大消息条数。
	MaxMessages int
	// MaxMessageChars 是单条消息最大字符数（按 Unicode 码点计数）。
	MaxMessageChars int
	// MaxTotalChars 是整段历史最大字符数。
	MaxTotalChars int
}

// ValidateHistory 校验消息角色、最后一条 user 约束以及数量/字符限制。
// 失败时包装 ErrInvalidArgument，供 HTTP 映射为稳定 4xx。
func ValidateHistory(messages []Message, limits HistoryLimits) error {
	// 空历史无法形成有效对话。
	if len(messages) == 0 {
		return fmt.Errorf("%w: messages cannot be empty", ErrInvalidArgument)
	}
	// 配置层应保证限制为正数；这里仍做防御，避免零值绕过安全边界。
	if limits.MaxMessages <= 0 || limits.MaxMessageChars <= 0 || limits.MaxTotalChars <= 0 {
		return fmt.Errorf("%w: history limits must be positive", ErrInvalidArgument)
	}
	// 条数限制优先检查，避免后续无意义扫描。
	if len(messages) > limits.MaxMessages {
		return fmt.Errorf("%w: message count exceeds limit %d", ErrInvalidArgument, limits.MaxMessages)
	}

	totalChars := 0
	for index, message := range messages {
		// 角色白名单只允许 user/assistant，系统提示由服务端注入。
		if message.Role != RoleUser && message.Role != RoleAssistant {
			return fmt.Errorf("%w: message[%d] role must be user or assistant", ErrInvalidArgument, index)
		}
		// 去掉首尾空白后的正文不能为空。
		content := strings.TrimSpace(message.Content)
		if content == "" {
			return fmt.Errorf("%w: message[%d] content cannot be empty", ErrInvalidArgument, index)
		}
		// 使用 Unicode 码点计数，避免中文被按字节误判。
		charCount := utf8.RuneCountInString(content)
		if charCount > limits.MaxMessageChars {
			return fmt.Errorf("%w: message[%d] exceeds %d characters", ErrInvalidArgument, index, limits.MaxMessageChars)
		}
		totalChars += charCount
	}
	// 总字符数限制防止超大上下文拖垮模型调用。
	if totalChars > limits.MaxTotalChars {
		return fmt.Errorf("%w: total message characters exceed limit %d", ErrInvalidArgument, limits.MaxTotalChars)
	}
	// 最后一条必须是 user，保证本轮有新的用户输入。
	last := messages[len(messages)-1]
	if last.Role != RoleUser {
		return fmt.Errorf("%w: last message must be user", ErrInvalidArgument)
	}
	return nil
}
