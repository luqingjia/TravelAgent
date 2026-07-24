package httpadapter_test

import (
	"encoding/json"
	. "github.com/luqingjia/TravelAgent/internal/knowledge/adapter/http"
	"testing"
)

// TestResultJSONKeepsCompatibleEnvelope 验证成功响应始终包含 code、message、data 三个字段。
func TestResultJSONKeepsCompatibleEnvelope(t *testing.T) {
	// 用空对象作为 data，专门观察统一响应外壳本身的字段名和顺序。
	encoded, err := json.Marshal(Success(map[string]any{}))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	// 精确比较 JSON，防止 code、message、data 被改名、遗漏或改成不兼容类型。
	if string(encoded) != `{"code":"0","message":"","data":{}}` {
		t.Fatalf("success JSON = %s", encoded)
	}
}
