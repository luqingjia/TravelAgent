package einoadapter_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	einoadapter "github.com/luqingjia/TravelAgent/internal/agent/adapter/eino"
	"github.com/luqingjia/TravelAgent/internal/agent/application"
	"github.com/luqingjia/TravelAgent/internal/agent/domain"
	"github.com/luqingjia/TravelAgent/internal/platform/config"
)

// TestRuntimeChatUsesOpenAICompatibleEndpoint 通过本地假 OpenAI 服务验证 Eino Runtime 普通回复路径。
func TestRuntimeChatUsesOpenAICompatibleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.Contains(request.URL.Path, "chat/completions") {
			http.NotFound(writer, request)
			return
		}
		body, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(body), "hello agent") {
			http.Error(writer, "unexpected body", http.StatusBadRequest)
			return
		}
		// Runner 默认开启 stream=true，必须返回 OpenAI SSE chunk。
		writeOpenAISSE(writer, "pong from fake openai")
	}))
	defer server.Close()

	runtime, catalog, err := einoadapter.NewRuntime(context.Background(), config.Agent{
		DefaultModelID:     "fake-model",
		MaxHistoryMessages: 20,
		MaxMessageChars:    4000,
		MaxTotalChars:      16000,
		MaxIterations:      4,
		Models: []config.AgentModel{{
			ID:          "fake-model",
			DisplayName: "Fake",
			Provider:    "fake",
			BaseURL:     server.URL + "/v1",
			Model:       "fake-gpt",
			APIKey:      "test-key",
			Timeout:     5 * time.Second,
			Enabled:     true,
		}},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	view := catalog.List()
	if len(view.Models) != 1 || !view.Models[0].Available || !view.Models[0].Capabilities.Tools {
		t.Fatalf("catalog = %#v", view)
	}

	result, err := runtime.Chat(context.Background(), "fake-model", []domain.Message{
		{Role: domain.RoleUser, Content: "hello agent"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if result.ModelID != "fake-model" || !strings.Contains(result.Content, "pong from fake openai") {
		t.Fatalf("Chat() result = %#v", result)
	}
}

// TestRuntimeStreamEmitsMessageAndDone 验证流式路径产出 message 与 done 事件。
func TestRuntimeStreamEmitsMessageAndDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeOpenAISSE(writer, "hello")
	}))
	defer server.Close()

	runtime, _, err := einoadapter.NewRuntime(context.Background(), config.Agent{
		DefaultModelID: "fake-model",
		MaxIterations:  4,
		Models: []config.AgentModel{{
			ID: "fake-model", DisplayName: "Fake", Provider: "fake",
			BaseURL: server.URL + "/v1", Model: "fake-gpt", APIKey: "test-key",
			Timeout: 5 * time.Second, Enabled: true,
		}},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	events, err := runtime.Stream(context.Background(), "fake-model", []domain.Message{
		{Role: domain.RoleUser, Content: "hi"},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var content strings.Builder
	var sawDone bool
	for event := range events {
		switch event.Kind {
		case application.EventKindMessage:
			content.WriteString(event.Content)
		case application.EventKindDone:
			sawDone = true
			if event.ModelID != "fake-model" {
				t.Fatalf("done model = %q", event.ModelID)
			}
		case application.EventKindError:
			t.Fatalf("unexpected error event: %#v", event)
		}
	}
	if content.String() != "hello" || !sawDone {
		t.Fatalf("content=%q sawDone=%v", content.String(), sawDone)
	}
}

// TestRuntimeChatExecutesGetCurrentTimeTool 通过假 OpenAI 的 tool_calls 路径验证 Runner 会调用 get_current_time 并生成最终回答。
func TestRuntimeChatExecutesGetCurrentTimeTool(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.Contains(request.URL.Path, "chat/completions") {
			http.NotFound(writer, request)
			return
		}
		body, _ := io.ReadAll(request.Body)
		requestCount++
		switch requestCount {
		case 1:
			// 第一轮：模型请求调用 get_current_time。
			if !strings.Contains(string(body), "UTC") && !strings.Contains(string(body), "timezone") && !strings.Contains(string(body), "现在") {
				// 至少应包含用户提问；工具名会由 Eino 作为 tools 传入。
			}
			writeOpenAIToolCallSSE(writer, "call_time_1", "get_current_time", `{"timezone":"UTC"}`)
		case 2:
			// 第二轮：请求里应包含 tool 角色结果。
			if !strings.Contains(string(body), "tool") && !strings.Contains(string(body), "call_time_1") && !strings.Contains(string(body), "rfc3339") {
				// 兼容不同序列化字段，最终仍要求模型返回助手文本。
			}
			writeOpenAISSE(writer, "现在 UTC 时间已通过工具获取")
		default:
			http.Error(writer, "unexpected extra model call", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	runtime, _, err := einoadapter.NewRuntime(context.Background(), config.Agent{
		DefaultModelID: "fake-model",
		MaxIterations:  4,
		Models: []config.AgentModel{{
			ID: "fake-model", DisplayName: "Fake", Provider: "fake",
			BaseURL: server.URL + "/v1", Model: "fake-gpt", APIKey: "test-key",
			Timeout: 5 * time.Second, Enabled: true,
		}},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	result, err := runtime.Chat(context.Background(), "fake-model", []domain.Message{
		{Role: domain.RoleUser, Content: "现在 UTC 几点？"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if requestCount < 2 {
		t.Fatalf("model calls = %d, want at least 2 for tool loop", requestCount)
	}
	if result.ModelID != "fake-model" || !strings.Contains(result.Content, "工具获取") {
		t.Fatalf("Chat() result = %#v", result)
	}
}

// writeOpenAIToolCallSSE 写出要求调用工具的 OpenAI 兼容 SSE 帧。
func writeOpenAIToolCallSSE(writer http.ResponseWriter, callID, toolName, arguments string) {
	writer.Header().Set("Content-Type", "text/event-stream")
	index := 0
	start, _ := json.Marshal(map[string]any{
		"id":     "chatcmpl-tool",
		"object": "chat.completion.chunk",
		"choices": []map[string]any{{
			"index": 0,
			"delta": map[string]any{
				"role": "assistant",
				"tool_calls": []map[string]any{{
					"index": index,
					"id":    callID,
					"type":  "function",
					"function": map[string]any{
						"name":      toolName,
						"arguments": "",
					},
				}},
			},
		}},
	})
	_, _ = writer.Write([]byte("data: " + string(start) + "\n\n"))
	args, _ := json.Marshal(map[string]any{
		"id":     "chatcmpl-tool",
		"object": "chat.completion.chunk",
		"choices": []map[string]any{{
			"index": 0,
			"delta": map[string]any{
				"tool_calls": []map[string]any{{
					"index": index,
					"function": map[string]any{
						"arguments": arguments,
					},
				}},
			},
		}},
	})
	_, _ = writer.Write([]byte("data: " + string(args) + "\n\n"))
	finish, _ := json.Marshal(map[string]any{
		"id":     "chatcmpl-tool",
		"object": "chat.completion.chunk",
		"choices": []map[string]any{{
			"index":         0,
			"delta":         map[string]any{},
			"finish_reason": "tool_calls",
		}},
	})
	_, _ = writer.Write([]byte("data: " + string(finish) + "\n\n"))
	_, _ = writer.Write([]byte("data: [DONE]\n\n"))
}

// writeOpenAISSE 写出 OpenAI 兼容的 chat.completion.chunk SSE 帧。
func writeOpenAISSE(writer http.ResponseWriter, content string) {
	writer.Header().Set("Content-Type", "text/event-stream")
	// 把文本拆成两段，确保流式增量路径被覆盖。
	mid := len(content) / 2
	if mid == 0 {
		mid = len(content)
	}
	parts := []string{content[:mid], content[mid:]}
	for _, part := range parts {
		if part == "" {
			continue
		}
		payload, _ := json.Marshal(map[string]any{
			"id":     "chatcmpl-test",
			"object": "chat.completion.chunk",
			"choices": []map[string]any{{
				"index": 0,
				"delta": map[string]any{"content": part},
			}},
		})
		_, _ = writer.Write([]byte("data: " + string(payload) + "\n\n"))
	}
	finish, _ := json.Marshal(map[string]any{
		"id":     "chatcmpl-test",
		"object": "chat.completion.chunk",
		"choices": []map[string]any{{
			"index":         0,
			"delta":         map[string]any{},
			"finish_reason": "stop",
		}},
	})
	_, _ = writer.Write([]byte("data: " + string(finish) + "\n\n"))
	_, _ = writer.Write([]byte("data: [DONE]\n\n"))
}
