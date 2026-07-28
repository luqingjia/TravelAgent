/**
 * Agent 对话与模型选择 Pinia store。
 * 仅将 selectedModelId 持久化到 localStorage；消息刷新清空。
 */

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { AgentApiError, chatJson, chatStream, fetchModelCatalog, isAbortError } from '@/api/agent'
import type { ChatMessage, PublicModel, UiChatMessage } from '@/api/types'

export type AgentStatus = 'idle' | 'loading' | 'streaming'

export const SELECTED_MODEL_STORAGE_KEY = 'travelagent.agent.selectedModelId'

/** 前端发送历史裁剪上限；后端限制仍是最终权威。 */
export const MAX_CLIENT_HISTORY = 30

function createMessageId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `msg_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`
}

function readStoredModelId(): string | null {
  try {
    return localStorage.getItem(SELECTED_MODEL_STORAGE_KEY)
  } catch {
    return null
  }
}

function writeStoredModelId(id: string | null): void {
  try {
    if (!id) {
      localStorage.removeItem(SELECTED_MODEL_STORAGE_KEY)
      return
    }
    localStorage.setItem(SELECTED_MODEL_STORAGE_KEY, id)
  } catch {
    // 存储不可用时静默忽略，不阻塞对话
  }
}

export const useAgentStore = defineStore('agent', () => {
  const models = ref<PublicModel[]>([])
  const defaultModelId = ref<string>('')
  const selectedModelId = ref<string>('')
  const messages = ref<UiChatMessage[]>([])
  const status = ref<AgentStatus>('idle')
  const error = ref<string | null>(null)
  const fallbackNotice = ref<string | null>(null)
  const receivedStreamContent = ref(false)
  const abortController = ref<AbortController | null>(null)

  const selectedModel = computed(() => models.value.find((m) => m.id === selectedModelId.value) ?? null)
  const availableModels = computed(() => models.value.filter((m) => m.available))
  const isGenerating = computed(() => status.value === 'loading' || status.value === 'streaming')
  const canSend = computed(() => {
    if (isGenerating.value) return false
    if (!selectedModelId.value) return false
    const model = selectedModel.value
    return Boolean(model?.available)
  })

  function clearError(): void {
    error.value = null
  }

  function clearFallbackNotice(): void {
    fallbackNotice.value = null
  }

  function setSelectedModelId(id: string): void {
    const target = models.value.find((m) => m.id === id)
    if (!target || !target.available) {
      error.value = '只能选择后端已配置且可用的模型'
      return
    }
    selectedModelId.value = id
    writeStoredModelId(id)
    clearError()
  }

  function applyCatalog(catalog: { defaultModelId: string; models: PublicModel[] }, options?: { showInvalidNotice?: boolean }): void {
    models.value = catalog.models ?? []
    defaultModelId.value = catalog.defaultModelId ?? ''

    const stored = readStoredModelId()
    const storedModel = stored ? models.value.find((m) => m.id === stored && m.available) : undefined
    if (storedModel) {
      selectedModelId.value = storedModel.id
      if (options?.showInvalidNotice) {
        // 有效恢复时不提示
      }
      return
    }

    // 存储失效或无存储：回退默认可用模型
    const defaultAvailable = models.value.find((m) => m.id === defaultModelId.value && m.available)
    const firstAvailable = models.value.find((m) => m.available)
    const next = defaultAvailable ?? firstAvailable
    selectedModelId.value = next?.id ?? ''
    writeStoredModelId(selectedModelId.value || null)

    if (stored && !storedModel && options?.showInvalidNotice !== false) {
      fallbackNotice.value = '已保存的模型不可用，已切换到默认模型'
    }
  }

  async function loadModels(): Promise<void> {
    error.value = null
    try {
      const catalog = await fetchModelCatalog()
      applyCatalog(catalog)
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载模型目录失败'
      models.value = []
      selectedModelId.value = ''
    }
  }

  function trimHistory(history: ChatMessage[]): ChatMessage[] {
    if (history.length <= MAX_CLIENT_HISTORY) {
      return history
    }
    return history.slice(history.length - MAX_CLIENT_HISTORY)
  }

  function buildRequestMessages(includingUser: ChatMessage): ChatMessage[] {
    const prior = messages.value.map((m) => ({ role: m.role, content: m.content }))
    // messages 此时已包含刚追加的 user 与空 assistant；请求只发完整消息，去掉空 assistant
    const withoutEmptyAssistant = prior.filter((m, index) => {
      if (index === prior.length - 1 && m.role === 'assistant' && m.content === '') {
        return false
      }
      return true
    })
    // 若 user 已在 prior 中则直接用；否则附加
    const last = withoutEmptyAssistant[withoutEmptyAssistant.length - 1]
    const hasUser =
      withoutEmptyAssistant.length > 0 &&
      last != null &&
      last.role === 'user' &&
      last.content === includingUser.content
    const full = hasUser ? withoutEmptyAssistant : [...withoutEmptyAssistant, includingUser]
    return trimHistory(full)
  }

  async function sendMessage(rawInput: string): Promise<void> {
    const content = rawInput.trim()
    if (!content) {
      return
    }
    if (isGenerating.value) {
      return
    }
    if (!canSend.value) {
      error.value = '没有可用模型，请先在模型页选择'
      return
    }

    error.value = null
    receivedStreamContent.value = false

    const userMessage: UiChatMessage = {
      id: createMessageId(),
      role: 'user',
      content,
    }
    const assistantMessage: UiChatMessage = {
      id: createMessageId(),
      role: 'assistant',
      content: '',
    }
    messages.value = [...messages.value, userMessage, assistantMessage]
    const assistantId = assistantMessage.id

    const controller = new AbortController()
    abortController.value = controller
    status.value = 'streaming'

    const requestBody = {
      modelId: selectedModelId.value,
      messages: buildRequestMessages({ role: 'user', content }),
    }

    try {
      await chatStream(requestBody, {
        signal: controller.signal,
        onMessage: (chunk) => {
          if (chunk) {
            receivedStreamContent.value = true
          }
          const list = messages.value
          const index = list.findIndex((m) => m.id === assistantId)
          if (index >= 0) {
            const current = list[index]
            if (!current) {
              return
            }
            const next: UiChatMessage = { ...current, content: current.content + chunk }
            messages.value = [...list.slice(0, index), next, ...list.slice(index + 1)]
          }
        },
        onDone: () => {
          status.value = 'idle'
          abortController.value = null
        },
        onErrorEvent: (code, message) => {
          // 后端 error 事件：保留已显示文本，不 JSON 重发
          error.value = message || code
          status.value = 'idle'
          abortController.value = null
        },
      })
    } catch (err) {
      if (isAbortError(err) || controller.signal.aborted) {
        // 用户停止：不显示系统故障
        status.value = 'idle'
        abortController.value = null
        // 若助手仍为空，去掉占位避免残留空气泡
        if (!receivedStreamContent.value) {
          messages.value = messages.value.filter((m) => m.id !== assistantId)
        }
        return
      }

      // 仅网络/协议失败可 JSON 降级；HTTP 4xx/业务错误与 SSE error 事件都不重发。
      const canFallback =
        !receivedStreamContent.value &&
        (err instanceof AgentApiError
          ? err.kind === 'protocol' || (err.kind === 'http' && err.status === 0)
          : true)

      if (canFallback) {
        // 删除空占位并 JSON 降级；后端 error 事件路径不会进入此处
        messages.value = messages.value.filter((m) => m.id !== assistantId)
        status.value = 'loading'
        try {
          const result = await chatJson(requestBody, controller.signal)
          messages.value = [
            ...messages.value,
            {
              id: createMessageId(),
              role: 'assistant',
              content: result.message?.content ?? '',
            },
          ]
          error.value = null
        } catch (jsonErr) {
          if (isAbortError(jsonErr) || controller.signal.aborted) {
            status.value = 'idle'
            abortController.value = null
            return
          }
          error.value = jsonErr instanceof Error ? jsonErr.message : '对话失败'
        } finally {
          status.value = 'idle'
          abortController.value = null
        }
        return
      }

      error.value = err instanceof Error ? err.message : '对话失败'
      status.value = 'idle'
      abortController.value = null
    }
  }

  function stopGeneration(): void {
    const controller = abortController.value
    if (controller) {
      controller.abort()
    }
    // 状态在 sendMessage catch/abort 分支收敛；此处立即恢复可发送
    status.value = 'idle'
    abortController.value = null
  }

  function resetMessages(): void {
    messages.value = []
    error.value = null
    receivedStreamContent.value = false
  }

  return {
    models,
    defaultModelId,
    selectedModelId,
    messages,
    status,
    error,
    fallbackNotice,
    receivedStreamContent,
    abortController,
    selectedModel,
    availableModels,
    isGenerating,
    canSend,
    clearError,
    clearFallbackNotice,
    setSelectedModelId,
    applyCatalog,
    loadModels,
    sendMessage,
    stopGeneration,
    resetMessages,
    trimHistory,
    buildRequestMessages,
  }
})
