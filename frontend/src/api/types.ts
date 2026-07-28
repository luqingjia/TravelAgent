/** 与后端公开合同对齐的 Agent API 类型；不包含密钥字段。 */

/** 统一响应外壳。 */
export interface ApiEnvelope<T> {
  code: string
  message: string
  data: T
}

/** 模型能力字符串合同。 */
export type ModelCapability = 'chat' | 'streaming' | 'tools' | string

/** 公开模型元数据。 */
export interface PublicModel {
  id: string
  displayName: string
  provider: string
  model: string
  available: boolean
  capabilities: ModelCapability[]
}

/** GET /api/agent/models 的 data。 */
export interface ModelCatalog {
  defaultModelId: string
  models: PublicModel[]
}

/** 对话消息角色。 */
export type ChatRole = 'user' | 'assistant'

/** 请求/展示用消息。 */
export interface ChatMessage {
  role: ChatRole
  content: string
}

/** 前端消息条目，含本地 id 以便流式更新同一气泡。 */
export interface UiChatMessage extends ChatMessage {
  id: string
}

/** POST /api/agent/chat 请求体。 */
export interface ChatRequest {
  modelId?: string
  messages: ChatMessage[]
}

/** POST /api/agent/chat 成功 data。 */
export interface ChatResponseData {
  modelId: string
  message: ChatMessage
}

/** SSE message 事件 data。 */
export interface SseMessageData {
  content: string
}

/** SSE done 事件 data。 */
export interface SseDoneData {
  modelId: string
}

/** SSE error 事件 data。 */
export interface SseErrorData {
  code: string
  message: string
}
