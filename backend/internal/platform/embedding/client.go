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
	// configuration 保存已经校验过的模型、维度、地址和密钥配置。
	configuration config.Embedding
	// httpClient 统一控制请求超时，并允许未来在测试中替换传输层。
	httpClient *http.Client
}

// 编译期确认 Client 完整实现应用层端口，避免以后改方法签名时直到运行才发现组装失败。
var _ application.Embedder = (*Client)(nil)

// NewClient 校验稳定配置并创建带请求超时的 Embedding 客户端。
func NewClient(configuration config.Embedding) (*Client, error) {
	// APIKey 为空时任何真实请求都会失败，因此启动阶段立即拒绝。
	if strings.TrimSpace(configuration.APIKey) == "" {
		return nil, fmt.Errorf("EMBEDDING_API_KEY is required")
	}
	// 模型专有名称必须明确提供给兼容协议。
	if strings.TrimSpace(configuration.Model) == "" {
		return nil, fmt.Errorf("EMBEDDING_MODEL is required")
	}
	// 客户端维度必须与应用合同和数据库 vector(1536) 一致。
	if configuration.Dimensions != application.EmbeddingDimensions {
		return nil, fmt.Errorf(
			"EMBEDDING_DIMENSIONS must be %d",
			application.EmbeddingDimensions,
		)
	}
	// 非正超时无法提供可靠请求生命周期。
	if configuration.Timeout <= 0 {
		return nil, fmt.Errorf("EMBEDDING_TIMEOUT must be positive")
	}
	// BaseURL 必须是包含协议和主机的绝对 HTTP 地址。
	parsedURL, err := url.Parse(strings.TrimSpace(configuration.BaseURL))
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("EMBEDDING_BASE_URL must be an absolute http or https URL")
	}

	// 配置全部合法后构造专用 HTTP 客户端。
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
	// 空输入直接返回同类型空切片，调用方无需处理 nil 特例。
	if len(texts) == 0 {
		// 空批次没有任何业务价值，直接返回空结果，也避免模型平台产生一次计费请求。
		return [][]float32{}, nil
	}

	// 只序列化外部协议真正需要的模型、输入和维度字段。
	requestBody, err := json.Marshal(embeddingRequest{
		Model:      client.configuration.Model,
		Input:      texts,
		Dimensions: client.configuration.Dimensions,
	})
	// 请求 JSON 编码失败时尚未发起网络调用。
	if err != nil {
		return nil, fmt.Errorf("encode embedding request: %w", err)
	}

	// TrimRight 防止配置末尾斜杠与固定路径拼出双斜杠。
	endpoint := strings.TrimRight(client.configuration.BaseURL, "/") + "/v1/embeddings"
	// 请求绑定调用方 Context，客户端取消会继续传递到网络层。
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("build embedding request: %w", err)
	}
	// Bearer token 只写入请求头，任何错误和日志都不能输出 API key。
	request.Header.Set("Authorization", "Bearer "+client.configuration.APIKey)
	// 明确声明 JSON，确保兼容服务按正确协议解析正文。
	request.Header.Set("Content-Type", "application/json")

	// Do 会执行连接、发送、等待和读取响应头，并同时受 Context 与 Client Timeout 控制。
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call embedding API: %w", err)
	}
	// 无论后续状态或 JSON 解码是否失败，都必须关闭响应体以复用连接。
	defer response.Body.Close()

	// 只有 2xx 状态被视为协议成功。
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		// 不把响应正文原样返回，第三方错误页可能包含请求细节；状态码足够定位协议级故障。
		return nil, fmt.Errorf("embedding API returned HTTP status %d", response.StatusCode)
	}

	// decoded 只接收 data 字段，其他第三方扩展字段由 JSON 解码器忽略。
	var decoded embeddingResponse
	// 响应必须是符合协议的完整 JSON。
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	// 返回项数量必须与输入文本数量一致，不能接受缺项或额外项。
	if len(decoded.Data) != len(texts) {
		return nil, fmt.Errorf(
			"embedding response count %d does not match input count %d",
			len(decoded.Data),
			len(texts),
		)
	}

	// 服务端允许按自己的顺序返回 data，因此按每项 index 放回固定下标，保持与 texts 一一对应。
	vectors := make([][]float32, len(texts))
	// seen 用来检测服务端重复返回同一下标。
	seen := make([]bool, len(texts))
	// 按响应中的 Index 恢复原输入顺序，而不是相信 data 数组顺序。
	for _, item := range decoded.Data {
		// 越界下标无法映射回任何输入文本。
		if item.Index < 0 || item.Index >= len(vectors) {
			return nil, fmt.Errorf("embedding response index %d is out of range", item.Index)
		}
		// 同一下标出现两次意味着一定有其他输入缺失。
		if seen[item.Index] {
			return nil, fmt.Errorf("embedding response index %d is duplicated", item.Index)
		}
		// 每一项都要独立检查维度，不能只检查批次总数。
		if len(item.Embedding) != client.configuration.Dimensions {
			return nil, fmt.Errorf(
				"embedding response index %d dimensions = %d, want %d",
				item.Index,
				len(item.Embedding),
				client.configuration.Dimensions,
			)
		}

		// 把向量写回它对应的原始输入位置。
		vectors[item.Index] = item.Embedding
		// 标记该位置已经出现，供后续重复检测。
		seen[item.Index] = true
	}

	// 返回顺序与 texts 完全一致的二维向量切片。
	return vectors, nil
}

// embeddingRequest 是外部模型 API 的 JSON 请求模型，不允许被 HTTP Handler 或领域层直接复用。
type embeddingRequest struct {
	// Model 是外部服务识别的模型专有名称。
	Model string `json:"model"`
	// Input 保持应用层传入文本顺序。
	Input []string `json:"input"`
	// Dimensions 请求固定 1536 维输出。
	Dimensions int `json:"dimensions,omitempty"`
}

// embeddingResponse 只声明当前用例真正读取的 data 字段，第三方返回的其他字段会被 JSON 解码器忽略。
type embeddingResponse struct {
	// Data 是每个输入对应的下标和向量结果。
	Data []embeddingItem `json:"data"`
}

// embeddingItem 保存单个响应下标与向量；Index 用于恢复输入顺序。
type embeddingItem struct {
	// Index 指向原始 Input 中的位置。
	Index int `json:"index"`
	// Embedding 是该输入对应的浮点向量。
	Embedding []float32 `json:"embedding"`
}
