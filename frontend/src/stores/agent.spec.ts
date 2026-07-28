import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { AgentApiError, type StreamChatHandlers } from '@/api/agent'
import { SELECTED_MODEL_STORAGE_KEY, useAgentStore } from './agent'
import type { ChatRequest, ChatResponseData, ModelCatalog, PublicModel } from '@/api/types'

const fetchModelCatalog = vi.fn<(...args: unknown[]) => Promise<ModelCatalog>>()
const chatStream = vi.fn<(request: ChatRequest, handlers: StreamChatHandlers) => Promise<void>>()
const chatJson = vi.fn<(...args: unknown[]) => Promise<ChatResponseData>>()

vi.mock('@/api/agent', async () => {
  const actual = await vi.importActual<typeof import('@/api/agent')>('@/api/agent')
  return {
    ...actual,
    fetchModelCatalog: () => fetchModelCatalog(),
    chatStream: (request: ChatRequest, handlers: StreamChatHandlers) => chatStream(request, handlers),
    chatJson: (request: ChatRequest, signal?: AbortSignal) => chatJson(request, signal),
  }
})

const sampleModels: PublicModel[] = [
  {
    id: 'default',
    displayName: 'Default',
    provider: 'mock',
    model: 'm1',
    available: true,
    capabilities: ['chat', 'streaming', 'tools'],
  },
  {
    id: 'alt',
    displayName: 'Alt',
    provider: 'mock',
    model: 'm2',
    available: true,
    capabilities: ['chat', 'streaming'],
  },
  {
    id: 'down',
    displayName: 'Down',
    provider: 'mock',
    model: 'm3',
    available: false,
    capabilities: ['chat'],
  },
]

describe('useAgentStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    fetchModelCatalog.mockReset()
    chatStream.mockReset()
    chatJson.mockReset()
  })

  it('loads catalog and restores valid selected model from localStorage', async () => {
    localStorage.setItem(SELECTED_MODEL_STORAGE_KEY, 'alt')
    fetchModelCatalog.mockResolvedValue({ defaultModelId: 'default', models: sampleModels })

    const store = useAgentStore()
    await store.loadModels()

    expect(store.selectedModelId).toBe('alt')
    expect(store.fallbackNotice).toBeNull()
    expect(store.models).toHaveLength(3)
  })

  it('falls back to default when stored model is invalid and shows notice once', async () => {
    localStorage.setItem(SELECTED_MODEL_STORAGE_KEY, 'missing')
    fetchModelCatalog.mockResolvedValue({ defaultModelId: 'default', models: sampleModels })

    const store = useAgentStore()
    await store.loadModels()

    expect(store.selectedModelId).toBe('default')
    expect(store.fallbackNotice).toContain('不可用')
    expect(localStorage.getItem(SELECTED_MODEL_STORAGE_KEY)).toBe('default')
  })

  it('only persists selectedModelId and never messages', async () => {
    fetchModelCatalog.mockResolvedValue({ defaultModelId: 'default', models: sampleModels })
    const store = useAgentStore()
    await store.loadModels()
    store.messages = [
      { id: '1', role: 'user', content: 'hi' },
      { id: '2', role: 'assistant', content: 'yo' },
    ]
    store.setSelectedModelId('alt')

    expect(localStorage.getItem(SELECTED_MODEL_STORAGE_KEY)).toBe('alt')
    expect(localStorage.getItem('messages')).toBeNull()
    expect(JSON.stringify(localStorage)).not.toContain('hi')
  })

  it('appends stream chunks into a single assistant bubble', async () => {
    fetchModelCatalog.mockResolvedValue({ defaultModelId: 'default', models: sampleModels })
    chatStream.mockImplementation(async (_req, handlers) => {
      handlers.onMessage('Hel')
      handlers.onMessage('lo')
      handlers.onDone('default')
    })

    const store = useAgentStore()
    await store.loadModels()
    await store.sendMessage('ping')

    expect(store.messages).toHaveLength(2)
    expect(store.messages[0]?.content).toBe('ping')
    expect(store.messages[1]?.role).toBe('assistant')
    expect(store.messages[1]?.content).toBe('Hello')
    expect(store.receivedStreamContent).toBe(true)
    expect(store.status).toBe('idle')
  })

  it('falls back to JSON only when no stream content yet and network/protocol fails', async () => {
    fetchModelCatalog.mockResolvedValue({ defaultModelId: 'default', models: sampleModels })
    chatStream.mockRejectedValue(new AgentApiError('network down', { kind: 'http', status: 0 }))
    chatJson.mockResolvedValue({
      modelId: 'default',
      message: { role: 'assistant', content: 'json-reply' },
    })

    const store = useAgentStore()
    await store.loadModels()
    await store.sendMessage('hello')

    expect(chatJson).toHaveBeenCalledTimes(1)
    expect(store.messages.map((m) => m.content)).toEqual(['hello', 'json-reply'])
  })

  it('does not JSON-retry on HTTP business/preflight failures even without stream content', async () => {
    fetchModelCatalog.mockResolvedValue({ defaultModelId: 'default', models: sampleModels })
    chatStream.mockRejectedValue(
      new AgentApiError('model unavailable', { kind: 'http', status: 400, code: 'A000001' }),
    )

    const store = useAgentStore()
    await store.loadModels()
    await store.sendMessage('hello')

    expect(chatJson).not.toHaveBeenCalled()
    expect(store.error).toContain('model unavailable')
  })

  it('does not JSON-retry after SSE error event even without content', async () => {
    fetchModelCatalog.mockResolvedValue({ defaultModelId: 'default', models: sampleModels })
    chatStream.mockImplementation(async (_req, handlers) => {
      handlers.onErrorEvent('B000001', 'agent run failed')
    })

    const store = useAgentStore()
    await store.loadModels()
    await store.sendMessage('hello')

    expect(chatJson).not.toHaveBeenCalled()
    expect(store.error).toContain('agent run failed')
  })

  it('does not JSON-retry after partial stream content', async () => {
    fetchModelCatalog.mockResolvedValue({ defaultModelId: 'default', models: sampleModels })
    chatStream.mockImplementation(async (_req, handlers) => {
      handlers.onMessage('partial')
      throw new AgentApiError('broken pipe', { kind: 'protocol' })
    })

    const store = useAgentStore()
    await store.loadModels()
    await store.sendMessage('hello')

    expect(chatJson).not.toHaveBeenCalled()
    expect(store.messages[1]?.content).toBe('partial')
    expect(store.error).toBeTruthy()
  })

  it('stop aborts generation without treating cancel as system fault', async () => {
    fetchModelCatalog.mockResolvedValue({ defaultModelId: 'default', models: sampleModels })
    chatStream.mockImplementation(async (_req, handlers) => {
      return new Promise((_resolve, reject) => {
        handlers.signal?.addEventListener('abort', () => {
          const err = new DOMException('The operation was aborted.', 'AbortError')
          reject(err)
        })
      })
    })

    const store = useAgentStore()
    await store.loadModels()
    const pending = store.sendMessage('hello')
    // allow send to start
    await Promise.resolve()
    store.stopGeneration()
    await pending

    expect(store.status).toBe('idle')
    expect(store.error).toBeNull()
  })

  it('disables send when empty input or while generating or without available model', async () => {
    fetchModelCatalog.mockResolvedValue({ defaultModelId: 'default', models: sampleModels })
    const store = useAgentStore()
    await store.loadModels()

    await store.sendMessage('   ')
    expect(store.messages).toHaveLength(0)

    store.status = 'streaming'
    await store.sendMessage('x')
    expect(store.messages).toHaveLength(0)

    store.status = 'idle'
    store.selectedModelId = 'down'
    await store.sendMessage('x')
    expect(store.messages).toHaveLength(0)
  })
})
