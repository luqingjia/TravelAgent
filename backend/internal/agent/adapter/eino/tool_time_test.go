package einoadapter

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/luqingjia/TravelAgent/internal/agent/domain"
)

// TestGetCurrentTimeToolAcceptsValidTimezones 验证 UTC 与 Asia/Shanghai 返回可读本地时间。
func TestGetCurrentTimeToolAcceptsValidTimezones(t *testing.T) {
	timeTool, err := newGetCurrentTimeTool()
	if err != nil {
		t.Fatalf("newGetCurrentTimeTool() error = %v", err)
	}
	invokable, ok := timeTool.(tool.InvokableTool)
	if !ok {
		t.Fatal("get_current_time 必须实现 InvokableTool")
	}

	for _, timezone := range []string{"UTC", "Asia/Shanghai"} {
		output, err := invokable.InvokableRun(context.Background(), mustJSON(t, map[string]string{"timezone": timezone}))
		if err != nil {
			t.Fatalf("timezone %s error = %v", timezone, err)
		}
		var parsed getCurrentTimeOutput
		if err := json.Unmarshal([]byte(output), &parsed); err != nil {
			t.Fatalf("decode output = %v raw=%s", err, output)
		}
		if parsed.Timezone != timezone || parsed.RFC3339 == "" || parsed.Readable == "" {
			t.Fatalf("timezone %s output = %#v", timezone, parsed)
		}
	}
}

// TestGetCurrentTimeToolRejectsInvalidTimezone 验证非法 IANA 时区映射到稳定领域错误。
func TestGetCurrentTimeToolRejectsInvalidTimezone(t *testing.T) {
	timeTool, err := newGetCurrentTimeTool()
	if err != nil {
		t.Fatalf("newGetCurrentTimeTool() error = %v", err)
	}
	invokable := timeTool.(tool.InvokableTool)
	_, err = invokable.InvokableRun(context.Background(), mustJSON(t, map[string]string{"timezone": "Not/AZone"}))
	if !errors.Is(err, domain.ErrInvalidTimezone) {
		t.Fatalf("invalid timezone error = %v, want ErrInvalidTimezone", err)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(body)
}
