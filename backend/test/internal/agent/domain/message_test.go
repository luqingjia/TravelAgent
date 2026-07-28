package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/luqingjia/TravelAgent/internal/agent/domain"
)

// TestValidateHistoryAcceptsUserAndAssistant 验证合法角色、最后一条 user 以及默认限制下可通过。
func TestValidateHistoryAcceptsUserAndAssistant(t *testing.T) {
	// 准备：一条助手历史 + 最新用户输入。
	messages := []domain.Message{
		{Role: domain.RoleAssistant, Content: "你好"},
		{Role: domain.RoleUser, Content: "现在几点"},
	}
	limits := domain.HistoryLimits{MaxMessages: 20, MaxMessageChars: 4000, MaxTotalChars: 16000}

	// 动作：执行领域历史校验。
	if err := domain.ValidateHistory(messages, limits); err != nil {
		t.Fatalf("ValidateHistory() error = %v", err)
	}
}

// TestValidateHistoryRejectsInvalidRoleAndLastMessage 验证角色白名单和最后一条必须是 user。
func TestValidateHistoryRejectsInvalidRoleAndLastMessage(t *testing.T) {
	limits := domain.HistoryLimits{MaxMessages: 20, MaxMessageChars: 4000, MaxTotalChars: 16000}

	// 场景一：系统角色不允许进入调用方历史。
	err := domain.ValidateHistory([]domain.Message{
		{Role: domain.Role("system"), Content: "x"},
		{Role: domain.RoleUser, Content: "y"},
	}, limits)
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("system role error = %v, want ErrInvalidArgument", err)
	}

	// 场景二：最后一条是 assistant，无法形成新的用户轮次。
	err = domain.ValidateHistory([]domain.Message{
		{Role: domain.RoleUser, Content: "hi"},
		{Role: domain.RoleAssistant, Content: "hello"},
	}, limits)
	if !errors.Is(err, domain.ErrInvalidArgument) || !strings.Contains(err.Error(), "last message") {
		t.Fatalf("last message error = %v", err)
	}
}

// TestValidateHistoryEnforcesCountAndCharacterLimits 验证条数、单条字符和总字符限制。
func TestValidateHistoryEnforcesCountAndCharacterLimits(t *testing.T) {
	// 场景一：条数超限。
	messages := []domain.Message{
		{Role: domain.RoleUser, Content: "a"},
		{Role: domain.RoleAssistant, Content: "b"},
		{Role: domain.RoleUser, Content: "c"},
	}
	err := domain.ValidateHistory(messages, domain.HistoryLimits{MaxMessages: 2, MaxMessageChars: 10, MaxTotalChars: 100})
	if !errors.Is(err, domain.ErrInvalidArgument) || !strings.Contains(err.Error(), "message count") {
		t.Fatalf("count limit error = %v", err)
	}

	// 场景二：单条中文字符按码点计数后超限。
	err = domain.ValidateHistory([]domain.Message{
		{Role: domain.RoleUser, Content: "一二三"},
	}, domain.HistoryLimits{MaxMessages: 5, MaxMessageChars: 2, MaxTotalChars: 100})
	if !errors.Is(err, domain.ErrInvalidArgument) || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("message char limit error = %v", err)
	}

	// 场景三：总字符超限。
	err = domain.ValidateHistory([]domain.Message{
		{Role: domain.RoleUser, Content: "ab"},
		{Role: domain.RoleAssistant, Content: "cd"},
		{Role: domain.RoleUser, Content: "ef"},
	}, domain.HistoryLimits{MaxMessages: 10, MaxMessageChars: 10, MaxTotalChars: 5})
	if !errors.Is(err, domain.ErrInvalidArgument) || !strings.Contains(err.Error(), "total message") {
		t.Fatalf("total char limit error = %v", err)
	}
}
