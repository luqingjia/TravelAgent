package httpadapter

// chatRequest 是 JSON 对话和 SSE 流式对话共用的请求体。
type chatRequest struct {
	// ModelID 可选；空值时使用配置的默认模型。
	ModelID string `json:"modelId"`
	// Messages 是调用方携带的受限历史，最后一条必须是 user。
	Messages []chatMessageRequest `json:"messages"`
}

// chatMessageRequest 是单条对话消息 DTO。
type chatMessageRequest struct {
	// Role 只能是 user 或 assistant。
	Role string `json:"role"`
	// Content 是消息正文。
	Content string `json:"content"`
}

// chatResponse 是普通 JSON 对话成功后的 data 负载。
type chatResponse struct {
	// ModelID 是实际使用的模型。
	ModelID string `json:"modelId"`
	// Message 是完整助手消息。
	Message chatMessageResponse `json:"message"`
}

// chatMessageResponse 是助手消息 DTO。
type chatMessageResponse struct {
	// Role 固定为 assistant。
	Role string `json:"role"`
	// Content 是完整助手文本。
	Content string `json:"content"`
}

// sseMessageData 是 SSE message 事件 data。
type sseMessageData struct {
	Content string `json:"content"`
}

// sseDoneData 是 SSE done 事件 data。
type sseDoneData struct {
	ModelID string `json:"modelId"`
}

// sseErrorData 是 SSE error 事件 data。
type sseErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// modelCatalogResponse 是模型目录 HTTP DTO。
type modelCatalogResponse struct {
	DefaultModelID string                `json:"defaultModelId"`
	Models         []publicModelResponse `json:"models"`
}

// publicModelResponse 是脱敏模型元数据 DTO。
// 不包含 enabled 原始配置字段的额外语义：只用 available 表示配置可用。
type publicModelResponse struct {
	ID           string   `json:"id"`
	DisplayName  string   `json:"displayName"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	Available    bool     `json:"available"`
	Capabilities []string `json:"capabilities"`
}
