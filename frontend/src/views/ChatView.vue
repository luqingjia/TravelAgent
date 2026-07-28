<script setup lang="ts">
/**
 * Ant Design Vue 对话页：消息区、输入、停止、错误与模型标识。
 * 不在此区域混用 Element Plus / Vuetify。
 */
import { computed, nextTick, ref, watch } from 'vue'
import { Alert, Button, Card, Empty, Input, Space, Spin, Tag, TypographyText } from 'ant-design-vue'

import { useAgentStore } from '@/stores/agent'

const agent = useAgentStore()
const draft = ref('')
const listRef = ref<HTMLElement | null>(null)

const sendDisabled = computed(() => {
  if (!draft.value.trim()) return true
  if (agent.isGenerating) return true
  if (!agent.canSend) return true
  return false
})

const modelLabel = computed(() => agent.selectedModel?.displayName || agent.selectedModelId || '未选择')

async function scrollToBottom() {
  await nextTick()
  const el = listRef.value
  if (el) {
    el.scrollTop = el.scrollHeight
  }
}

watch(
  () => agent.messages.map((m) => m.content).join('\0'),
  () => {
    void scrollToBottom()
  },
)

async function onSend() {
  const text = draft.value
  if (sendDisabled.value) return
  draft.value = ''
  await agent.sendMessage(text)
}

function onStop() {
  agent.stopGeneration()
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    void onSend()
  }
}
</script>

<template>
  <div class="chat-view" data-testid="chat-view">
    <Card class="chat-view__card" :bordered="false">
      <template #title>
        <Space>
          <span>Agent 对话</span>
          <Tag color="blue" data-testid="model-tag">{{ modelLabel }}</Tag>
          <Tag v-if="agent.status === 'streaming'" color="processing">生成中</Tag>
          <Tag v-else-if="agent.status === 'loading'" color="processing">请求中</Tag>
        </Space>
      </template>

      <Alert
        v-if="agent.fallbackNotice"
        type="info"
        show-icon
        closable
        class="chat-view__alert"
        data-testid="fallback-notice"
        :message="agent.fallbackNotice"
        @close="agent.clearFallbackNotice()"
      />
      <Alert
        v-if="agent.error"
        type="error"
        show-icon
        closable
        class="chat-view__alert"
        data-testid="chat-error"
        :message="agent.error"
        @close="agent.clearError()"
      />

      <div ref="listRef" class="chat-view__messages" data-testid="message-list">
        <Empty v-if="agent.messages.length === 0" description="发送第一条消息开始对话（刷新后消息会清空）" />
        <div
          v-for="msg in agent.messages"
          :key="msg.id"
          class="chat-bubble"
          :class="`chat-bubble--${msg.role}`"
          :data-testid="`message-${msg.role}`"
          :data-role="msg.role"
        >
          <div class="chat-bubble__role">{{ msg.role === 'user' ? '你' : '助手' }}</div>
          <div class="chat-bubble__content" data-testid="message-content">
            <template v-if="msg.role === 'assistant' && !msg.content && agent.isGenerating">
              <Spin size="small" />
              <TypographyText type="secondary"> 正在生成…</TypographyText>
            </template>
            <template v-else>
              {{ msg.content }}
            </template>
          </div>
        </div>
      </div>

      <div class="chat-view__composer">
        <Input.TextArea
          v-model:value="draft"
          :rows="3"
          :disabled="agent.isGenerating"
          placeholder="输入消息，Enter 发送，Shift+Enter 换行"
          data-testid="chat-input"
          @keydown="onKeydown"
        />
        <div class="chat-view__actions">
          <Button
            v-if="agent.isGenerating"
            danger
            data-testid="stop-button"
            @click="onStop"
          >
            停止
          </Button>
          <Button
            type="primary"
            :disabled="sendDisabled"
            data-testid="send-button"
            @click="onSend"
          >
            发送
          </Button>
        </div>
      </div>
    </Card>
  </div>
</template>

<style scoped>
.chat-view {
  max-width: 960px;
  margin: 0 auto;
}

.chat-view__card {
  min-height: calc(100vh - 120px);
  display: flex;
  flex-direction: column;
  background: #fff;
}

.chat-view__alert {
  margin-bottom: 12px;
}

.chat-view__messages {
  flex: 1;
  min-height: 320px;
  max-height: calc(100vh - 320px);
  overflow: auto;
  padding: 8px 4px 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.chat-bubble {
  max-width: min(720px, 92%);
  padding: 10px 12px;
  border-radius: 12px;
  background: #f5f5f5;
}

.chat-bubble--user {
  align-self: flex-end;
  background: #e6f4ff;
}

.chat-bubble--assistant {
  align-self: flex-start;
  background: #fafafa;
  border: 1px solid #f0f0f0;
}

.chat-bubble__role {
  font-size: 12px;
  color: rgba(0, 0, 0, 0.45);
  margin-bottom: 4px;
}

.chat-bubble__content {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.6;
}

.chat-view__composer {
  border-top: 1px solid #f0f0f0;
  padding-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.chat-view__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

@media (max-width: 767px) {
  .chat-view__messages {
    max-height: calc(100vh - 360px);
    min-height: 240px;
  }
}
</style>
