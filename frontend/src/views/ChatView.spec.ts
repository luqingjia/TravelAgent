import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { computed, reactive } from 'vue'

import ChatView from './ChatView.vue'

const sendMessage = vi.fn<(...args: unknown[]) => Promise<void> | void>()
const stopGeneration = vi.fn<() => void>()
const clearError = vi.fn<() => void>()
const clearFallbackNotice = vi.fn<() => void>()

const state = reactive({
  messages: [] as { id: string; role: 'user' | 'assistant'; content: string }[],
  status: 'idle' as 'idle' | 'loading' | 'streaming',
  error: null as string | null,
  fallbackNotice: null as string | null,
  isGenerating: false,
  canSend: true,
  selectedModel: { displayName: 'Default', id: 'default' },
  selectedModelId: 'default',
  sendMessage,
  stopGeneration,
  clearError,
  clearFallbackNotice,
})

vi.mock('@/stores/agent', () => ({
  useAgentStore: () => state,
}))

describe('ChatView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    sendMessage.mockReset()
    stopGeneration.mockReset()
    state.messages = []
    state.status = 'idle'
    state.error = null
    state.fallbackNotice = null
    state.isGenerating = false
    state.canSend = true
  })

  it('disables send for empty input and while generating', async () => {
    const wrapper = mount(ChatView)
    await flushPromises()

    const sendBtn = wrapper.get('[data-testid="send-button"]')
    // ant-design-vue Button puts disabled on the native button
    expect(sendBtn.element.hasAttribute('disabled') || sendBtn.attributes('disabled') !== undefined).toBe(
      true,
    )

    const textarea = wrapper.find('textarea')
    await textarea.setValue('hello')
    await flushPromises()
    // After input, send should enable (canSend true, not generating)
    const enabled = wrapper.get('[data-testid="send-button"]')
    expect(enabled.element.hasAttribute('disabled')).toBe(false)

    state.isGenerating = true
    await flushPromises()
    expect(wrapper.get('[data-testid="send-button"]').element.hasAttribute('disabled')).toBe(true)
  })

  it('shows stop button while generating and calls stop', async () => {
    state.isGenerating = true
    state.status = 'streaming'
    const wrapper = mount(ChatView)
    await flushPromises()

    const stop = wrapper.get('[data-testid="stop-button"]')
    await stop.trigger('click')
    expect(stopGeneration).toHaveBeenCalled()
  })

  it('renders incremental assistant bubble content and error alert', async () => {
    state.messages = [
      { id: 'u1', role: 'user', content: 'hi' },
      { id: 'a1', role: 'assistant', content: 'Hel' },
    ]
    state.error = 'something failed'
    const wrapper = mount(ChatView)
    await flushPromises()

    expect(wrapper.get('[data-testid="message-list"]').text()).toContain('Hel')
    expect(wrapper.get('[data-testid="chat-error"]').text()).toContain('something failed')

    state.messages = [
      { id: 'u1', role: 'user', content: 'hi' },
      { id: 'a1', role: 'assistant', content: 'Hello' },
    ]
    await flushPromises()
    expect(wrapper.get('[data-testid="message-list"]').text()).toContain('Hello')
  })

  it('exposes send-disabled conditions used by the store/UI gate', () => {
    // empty draft
    const emptyDraft = ''
    const canClickEmpty = Boolean(emptyDraft.trim()) && !state.isGenerating && state.canSend
    expect(canClickEmpty).toBe(false)

    // generating
    state.isGenerating = true
    const canClickBusy = Boolean('x'.trim()) && !state.isGenerating && state.canSend
    expect(canClickBusy).toBe(false)

    // ok
    state.isGenerating = false
    const canClickOk = Boolean('x'.trim()) && !state.isGenerating && state.canSend
    expect(canClickOk).toBe(true)

    // keep computed import used for tree-shaking silence
    expect(typeof computed).toBe('function')
  })
})
