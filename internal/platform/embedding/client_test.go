package embedding

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
	"github.com/luqingjia/TravelAgent/internal/platform/config"
)

// TestClientEmbedTextsSendsCompatibleRequestAndRestoresOrder 验证客户端保持现有 OpenAI 兼容协议，
// 并根据响应 index 恢复输入顺序，而不是盲目信任服务端数组顺序。
func TestClientEmbedTextsSendsCompatibleRequestAndRestoresOrder(t *testing.T) {
	// 准备：本地测试服务器检查请求路径、认证头和 JSON，并故意按 1、0 的顺序返回向量。
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/embeddings" {
			t.Errorf("request = %s %s, want POST /v1/embeddings", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", request.Header.Get("Content-Type"))
		}

		var body embeddingRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.Model != "text-embedding-v3" || body.Dimensions != application.EmbeddingDimensions || len(body.Input) != 2 {
			t.Errorf("request body = %#v", body)
		}

		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(embeddingResponse{Data: []embeddingItem{
			{Index: 1, Embedding: testVector(2)},
			{Index: 0, Embedding: testVector(1)},
		}})
	}))
	defer server.Close()

	client, err := NewClient(config.Embedding{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		Model:      "text-embedding-v3",
		Dimensions: application.EmbeddingDimensions,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// 执行：一次请求批量发送两段文本。
	vectors, err := client.EmbedTexts(t.Context(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("EmbedTexts() error = %v", err)
	}

	// 关键断言：返回结果必须与输入下标对齐，并且每个向量仍满足数据库 1536 维合同。
	if len(vectors) != 2 || len(vectors[0]) != application.EmbeddingDimensions || len(vectors[1]) != application.EmbeddingDimensions {
		t.Fatalf("vector dimensions = %d/%d", len(vectors[0]), len(vectors[1]))
	}
	if vectors[0][0] != 1 || vectors[1][0] != 2 {
		t.Fatalf("vectors were not restored by response index: %v/%v", vectors[0][0], vectors[1][0])
	}
}

// TestClientEmbedTextsRejectsWrongDimensions 验证模型即使返回 2xx，错误维度也不能进入应用处理流程。
func TestClientEmbedTextsRejectsWrongDimensions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(embeddingResponse{Data: []embeddingItem{{
			Index:     0,
			Embedding: []float32{1, 2, 3},
		}}})
	}))
	defer server.Close()

	client, err := NewClient(config.Embedding{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		Model:      "model",
		Dimensions: application.EmbeddingDimensions,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.EmbedTexts(t.Context(), []string{"text"})
	if err == nil || !strings.Contains(err.Error(), "dimensions") {
		t.Fatalf("EmbedTexts() error = %v, want dimension error", err)
	}
}

// TestClientEmbedTextsSkipsNetworkForEmptyInput 验证空批次直接返回，不制造无意义的外部请求。
func TestClientEmbedTextsSkipsNetworkForEmptyInput(t *testing.T) {
	client, err := NewClient(config.Embedding{
		APIKey:     "test-key",
		BaseURL:    "http://127.0.0.1:1",
		Model:      "model",
		Dimensions: application.EmbeddingDimensions,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	vectors, err := client.EmbedTexts(t.Context(), nil)
	if err != nil || len(vectors) != 0 {
		t.Fatalf("EmbedTexts(nil) = (%#v, %v), want empty success", vectors, err)
	}
}

// testVector 生成固定 1536 维测试向量，首元素用于识别响应顺序，其余值保持零即可。
func testVector(first float32) []float32 {
	vector := make([]float32, application.EmbeddingDimensions)
	vector[0] = first
	return vector
}
