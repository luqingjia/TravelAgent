// Package embedding 实现 application.Embedder 所需的外部向量模型客户端。
//
// 当前协议兼容 OpenAI embeddings API，但业务层只看见“文本批次进、向量批次出”的小接口；
// API key、HTTP、JSON 和模型地址都被限制在这个最外层包中。
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
	"github.com/luqingjia/TravelAgent/internal/platform/config"
)

// Client 调用兼容 OpenAI 的 /v1/embeddings HTTP 接口。
type Client struct {
	configuration config.Embedding
	httpClient    *http.Client
}

// 编译期确认 Client 完整实现应用层端口，避免以后改方法签名时直到运行才发现组装失败。
var _ application.Embedder = (*Client)(nil)

// NewClient 校验稳定配置并创建带请求超时的 Embedding 客户端。
func NewClient(configuration config.Embedding) (*Client, error) {
	if strings.TrimSpace(configuration.APIKey) == "" {
		return nil, fmt.Errorf("EMBEDDING_API_KEY is required")
	}
	if strings.TrimSpace(configuration.Model) == "" {
		return nil, fmt.Errorf("EMBEDDING_MODEL is required")
	}
	if configuration.Dimensions != application.EmbeddingDimensions {
		return nil, fmt.Errorf(
			"EMBEDDING_DIMENSIONS must be %d",
			application.EmbeddingDimensions,
		)
	}
	if configuration.Timeout <= 0 {
		return nil, fmt.Errorf("EMBEDDING_TIMEOUT must be positive")
	}
	parsedURL, err := url.Parse(strings.TrimSpace(configuration.BaseURL))
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("EMBEDDING_BASE_URL must be an absolute http or https URL")
	}

	return &Client{
		configuration: configuration,
		httpClient: &http.Client{
			// 这是整个请求（连接、发送、等待、读取）的兜底时间；调用方 context 仍可更早取消。
			Timeout: configuration.Timeout,
		},
	}, nil
}

// EmbedTexts 把一批文本按原顺序转换成一批固定 1536 维向量。
func (client *Client) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		// 空批次没有任何业务价值，直接返回空结果，也避免模型平台产生一次计费请求。
		return [][]float32{}, nil
	}

	requestBody, err := json.Marshal(embeddingRequest{
		Model:      client.configuration.Model,
		Input:      texts,
		Dimensions: client.configuration.Dimensions,
	})
	if err != nil {
		return nil, fmt.Errorf("encode embedding request: %w", err)
	}

	endpoint := strings.TrimRight(client.configuration.BaseURL, "/") + "/v1/embeddings"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("build embedding request: %w", err)
	}
	// Bearer token 只写入请求头，任何错误和日志都不能输出 API key。
	request.Header.Set("Authorization", "Bearer "+client.configuration.APIKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call embedding API: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		// 不把响应正文原样返回，第三方错误页可能包含请求细节；状态码足够定位协议级故障。
		return nil, fmt.Errorf("embedding API returned HTTP status %d", response.StatusCode)
	}

	var decoded embeddingResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(decoded.Data) != len(texts) {
		return nil, fmt.Errorf(
			"embedding response count %d does not match input count %d",
			len(decoded.Data),
			len(texts),
		)
	}

	// 服务端允许按自己的顺序返回 data，因此按每项 index 放回固定下标，保持与 texts 一一对应。
	vectors := make([][]float32, len(texts))
	seen := make([]bool, len(texts))
	for _, item := range decoded.Data {
		if item.Index < 0 || item.Index >= len(vectors) {
			return nil, fmt.Errorf("embedding response index %d is out of range", item.Index)
		}
		if seen[item.Index] {
			return nil, fmt.Errorf("embedding response index %d is duplicated", item.Index)
		}
		if len(item.Embedding) != client.configuration.Dimensions {
			return nil, fmt.Errorf(
				"embedding response index %d dimensions = %d, want %d",
				item.Index,
				len(item.Embedding),
				client.configuration.Dimensions,
			)
		}

		vectors[item.Index] = item.Embedding
		seen[item.Index] = true
	}

	return vectors, nil
}

// embeddingRequest 是外部模型 API 的 JSON 请求模型，不允许被 HTTP Handler 或领域层直接复用。
type embeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

// embeddingResponse 只声明当前用例真正读取的 data 字段，第三方返回的其他字段会被 JSON 解码器忽略。
type embeddingResponse struct {
	Data []embeddingItem `json:"data"`
}

// embeddingItem 保存单个响应下标与向量；Index 用于恢复输入顺序。
type embeddingItem struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}
