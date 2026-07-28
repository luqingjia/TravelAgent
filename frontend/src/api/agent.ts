/**
 * 集中 Agent API 客户端：模型目录、JSON 对话、POST SSE 流式对话。
 * Base URL 读取 VITE_API_BASE_URL；为空时使用同源路径。
 */

import { parseSseStream } from './sse'
import type {
  ApiEnvelope,
  ChatRequest,
  ChatResponseData,
  ModelCatalog,
  SseDoneData,
  SseErrorData,
  SseMessageData,
} from './types'

/** 流式对话回调。 */
export interface StreamChatHandlers {
  onMessage: (chunk: string) => void
  onDone: (modelId: string) => void
  onErrorEvent: (code: string, message: string) => void
  signal?: AbortSignal
}

/** 业务 API 错误（HTTP 非 2xx 或 envelope code 非 0）。 */
export class AgentApiError extends Error {
  readonly code: string
  readonly status: number
  readonly kind: 'http' | 'business' | 'protocol'

  constructor(
    message: string,
    options: { code?: string; status?: number; kind?: 'http' | 'business' | 'protocol' } = {},
  ) {
    super(message)
    this.name = 'AgentApiError'
    this.code = options.code ?? 'CLIENT'
    this.status = options.status ?? 0
    this.kind = options.kind ?? 'http'
  }
}

/** 解析 API 根路径：空环境变量表示同源。 */
export function resolveApiBaseUrl(envValue: string | undefined = import.meta.env.VITE_API_BASE_URL): string {
  if (envValue == null || envValue === '') {
    return ''
  }
  return envValue.replace(/\/+$/, '')
}

function buildUrl(path: string): string {
  const base = resolveApiBaseUrl()
  if (!path.startsWith('/')) {
    return `${base}/${path}`
  }
  return `${base}${path}`
}

async function readEnvelope<T>(response: Response): Promise<T> {
  let body: ApiEnvelope<T>
  try {
    body = (await response.json()) as ApiEnvelope<T>
  } catch {
    throw new AgentApiError('invalid JSON response', {
      status: response.status,
      kind: 'protocol',
    })
  }

  if (!response.ok) {
    throw new AgentApiError(body.message || `request failed with status ${response.status}`, {
      code: body.code || String(response.status),
      status: response.status,
      kind: 'http',
    })
  }

  if (body.code !== '0') {
    throw new AgentApiError(body.message || 'business error', {
      code: body.code,
      status: response.status,
      kind: 'business',
    })
  }

  return body.data
}

/** GET /api/agent/models */
export async function fetchModelCatalog(signal?: AbortSignal): Promise<ModelCatalog> {
  const response = await fetch(buildUrl('/api/agent/models'), {
    method: 'GET',
    headers: { Accept: 'application/json' },
    signal,
  })
  return readEnvelope<ModelCatalog>(response)
}

/** POST /api/agent/chat（完整 JSON 回复） */
export async function chatJson(request: ChatRequest, signal?: AbortSignal): Promise<ChatResponseData> {
  const response = await fetch(buildUrl('/api/agent/chat'), {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(request),
    signal,
  })
  return readEnvelope<ChatResponseData>(response)
}

/**
 * POST /api/agent/chat/stream
 * 使用 fetch + ReadableStream，禁止 EventSource。
 */
export async function chatStream(request: ChatRequest, handlers: StreamChatHandlers): Promise<void> {
  let response: Response
  try {
    response = await fetch(buildUrl('/api/agent/chat/stream'), {
      method: 'POST',
      headers: {
        Accept: 'text/event-stream',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(request),
      signal: handlers.signal,
    })
  } catch (error) {
    if (isAbortError(error)) {
      throw error
    }
    throw new AgentApiError(error instanceof Error ? error.message : 'network error', {
      kind: 'http',
      status: 0,
    })
  }

  const contentType = response.headers.get('content-type') || ''

  // 预检失败时后端返回 JSON 4xx，尚未进入 SSE
  if (!response.ok) {
    if (contentType.includes('application/json')) {
      await readEnvelope<unknown>(response)
    }
    throw new AgentApiError(`stream request failed with status ${response.status}`, {
      status: response.status,
      kind: 'http',
    })
  }

  if (!response.body) {
    throw new AgentApiError('stream response body is empty', { kind: 'protocol', status: response.status })
  }

  // 非 event-stream 视作协议失败（可触发降级条件判断）
  if (contentType && !contentType.includes('text/event-stream') && !contentType.includes('text/plain')) {
    throw new AgentApiError(`unexpected stream content-type: ${contentType}`, {
      kind: 'protocol',
      status: response.status,
    })
  }

  let sawTerminal = false

  for await (const event of parseSseStream(response.body, { signal: handlers.signal })) {
    if (event.event === 'message') {
      let payload: SseMessageData
      try {
        payload = JSON.parse(event.data) as SseMessageData
      } catch {
        throw new AgentApiError('invalid message event payload', { kind: 'protocol' })
      }
      handlers.onMessage(payload.content ?? '')
    } else if (event.event === 'done') {
      let payload: SseDoneData
      try {
        payload = JSON.parse(event.data) as SseDoneData
      } catch {
        throw new AgentApiError('invalid done event payload', { kind: 'protocol' })
      }
      sawTerminal = true
      handlers.onDone(payload.modelId ?? '')
      return
    } else if (event.event === 'error') {
      let payload: SseErrorData
      try {
        payload = JSON.parse(event.data) as SseErrorData
      } catch {
        throw new AgentApiError('invalid error event payload', { kind: 'protocol' })
      }
      sawTerminal = true
      handlers.onErrorEvent(payload.code ?? 'B000001', payload.message || 'stream error')
      return
    }
  }

  if (!sawTerminal) {
    throw new AgentApiError('stream ended without done/error event', { kind: 'protocol' })
  }
}

export function isAbortError(error: unknown): boolean {
  if (!error || typeof error !== 'object') {
    return false
  }
  const name = (error as { name?: string }).name
  return name === 'AbortError'
}
