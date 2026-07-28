<script setup lang="ts">
/**
 * 仅使用 Vuetify 的模型配置状态面板。
 * 文案明确：展示的是配置可用状态，不是实时网络探测。
 */
import { computed } from 'vue'

import type { PublicModel } from '@/api/types'

const props = defineProps<{
  models: PublicModel[]
  defaultModelId: string
  selectedModelId: string
  loading?: boolean
}>()

const selected = computed(() => props.models.find((m) => m.id === props.selectedModelId) ?? null)
const defaultModel = computed(() => props.models.find((m) => m.id === props.defaultModelId) ?? null)

const availableCount = computed(() => props.models.filter((m) => m.available).length)

const capabilities = computed(() => selected.value?.capabilities ?? [])
</script>

<template>
  <div class="model-status-panel" data-testid="model-status-panel">
    <VSkeletonLoader v-if="loading" type="card" />
    <VCard v-else variant="outlined">
      <VCardTitle class="text-subtitle-1">模型配置状态</VCardTitle>
      <VCardText>
        <VAlert type="info" variant="tonal" density="compact" class="mb-3" data-testid="config-status-note">
          以下为后端配置可用状态，不是实时网络探测或供应商健康检查。
        </VAlert>

        <div class="status-row">
          <span class="label">配置可用模型</span>
          <strong data-testid="available-count">{{ availableCount }} / {{ models.length }}</strong>
        </div>
        <div class="status-row">
          <span class="label">默认模型</span>
          <strong data-testid="default-model">{{ defaultModel?.displayName || defaultModelId || '—' }}</strong>
        </div>
        <div class="status-row">
          <span class="label">当前模型</span>
          <strong data-testid="current-model">{{ selected?.displayName || selectedModelId || '—' }}</strong>
        </div>
        <div class="status-row">
          <span class="label">当前可用</span>
          <strong data-testid="current-available">{{ selected ? (selected.available ? '是' : '否') : '—' }}</strong>
        </div>

        <div class="caps">
          <div class="label mb-2">能力（chat / streaming / tools）</div>
          <VChipGroup column>
            <VChip size="small" :color="capabilities.includes('chat') ? 'primary' : undefined" data-testid="cap-chat">
              chat {{ capabilities.includes('chat') ? '✓' : '—' }}
            </VChip>
            <VChip
              size="small"
              :color="capabilities.includes('streaming') ? 'primary' : undefined"
              data-testid="cap-streaming"
            >
              streaming {{ capabilities.includes('streaming') ? '✓' : '—' }}
            </VChip>
            <VChip size="small" :color="capabilities.includes('tools') ? 'primary' : undefined" data-testid="cap-tools">
              tools {{ capabilities.includes('tools') ? '✓' : '—' }}
            </VChip>
          </VChipGroup>
        </div>
      </VCardText>
    </VCard>
  </div>
</template>

<style scoped>
.model-status-panel {
  width: 100%;
}

.status-row {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}

.label {
  color: rgba(0, 0, 0, 0.6);
}

.caps {
  margin-top: 12px;
}

.mb-2 {
  margin-bottom: 8px;
}

.mb-3 {
  margin-bottom: 12px;
}
</style>
