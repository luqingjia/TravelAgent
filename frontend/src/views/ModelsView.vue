<script setup lang="ts">
/**
 * Element Plus 模型绑定页：列表、筛选、单选绑定。
 * 只能选择后端已有模型；无 API Key / 新增 / 编辑供应商表单。
 * 自定义 namespace tael 需与 Sass 编译 CSS 一致。
 */
import { computed, ref } from 'vue'
import {
  ElAlert,
  ElConfigProvider,
  ElEmpty,
  ElInput,
  ElRadio,
  ElRadioGroup,
  ElTable,
  ElTableColumn,
  ElTag,
} from 'element-plus'

import ModelStatusPanel from '@/components/ModelStatusPanel.vue'
import { useAgentStore } from '@/stores/agent'

const agent = useAgentStore()
const filterText = ref('')

const filteredModels = computed(() => {
  const q = filterText.value.trim().toLowerCase()
  if (!q) return agent.models
  return agent.models.filter((m) => {
    const hay = `${m.id} ${m.displayName} ${m.provider} ${m.model}`.toLowerCase()
    return hay.includes(q)
  })
})

function onSelect(id: string | number | boolean | undefined) {
  if (typeof id !== 'string' || !id) return
  agent.setSelectedModelId(id)
}
</script>

<template>
  <div class="models-view" data-testid="models-view">
    <!-- Element Plus 区域：namespace 必须与 styles/element/index.scss 一致 -->
    <ElConfigProvider namespace="tael">
      <div class="models-view__panel">
        <h2 class="models-view__title">模型绑定</h2>
        <p class="models-view__hint">
          仅可选择后端已配置的模型。不支持录入 API Key、新增或编辑供应商。
        </p>

        <ElAlert
          v-if="agent.fallbackNotice"
          type="info"
          show-icon
          :closable="true"
          class="models-view__alert"
          data-testid="models-fallback"
          :title="agent.fallbackNotice"
          @close="agent.clearFallbackNotice()"
        />
        <ElAlert
          v-if="agent.error"
          type="error"
          show-icon
          :closable="true"
          class="models-view__alert"
          data-testid="models-error"
          :title="agent.error"
          @close="agent.clearError()"
        />

        <ElInput
          v-model="filterText"
          clearable
          placeholder="筛选显示名 / 提供商 / 模型"
          class="models-view__filter"
          data-testid="model-filter"
        />

        <ElEmpty v-if="filteredModels.length === 0" description="没有可展示的模型" />

        <ElRadioGroup
          v-else
          :model-value="agent.selectedModelId"
          class="models-view__radio-group"
          data-testid="model-radio-group"
          @update:model-value="onSelect"
        >
          <ElTable :data="filteredModels" stripe style="width: 100%" data-testid="model-table">
            <ElTableColumn label="选择" width="72">
              <template #default="{ row }">
                <ElRadio :value="row.id" :disabled="!row.available" :data-testid="`select-${row.id}`" />
              </template>
            </ElTableColumn>
            <ElTableColumn prop="displayName" label="显示名" min-width="140" />
            <ElTableColumn prop="id" label="ID" min-width="120" />
            <ElTableColumn prop="provider" label="提供商" min-width="100" />
            <ElTableColumn prop="model" label="模型" min-width="140" />
            <ElTableColumn label="可用" width="90">
              <template #default="{ row }">
                <ElTag :type="row.available ? 'success' : 'info'" size="small">
                  {{ row.available ? '可用' : '不可用' }}
                </ElTag>
              </template>
            </ElTableColumn>
            <ElTableColumn label="能力" min-width="160">
              <template #default="{ row }">
                <span>{{ (row.capabilities || []).join(', ') }}</span>
              </template>
            </ElTableColumn>
          </ElTable>
        </ElRadioGroup>
      </div>
    </ElConfigProvider>

    <!-- Vuetify 仅出现在独立状态面板 -->
    <div class="models-view__status">
      <ModelStatusPanel
        :models="agent.models"
        :default-model-id="agent.defaultModelId"
        :selected-model-id="agent.selectedModelId"
      />
    </div>
  </div>
</template>

<style scoped>
.models-view {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(260px, 320px);
  gap: 16px;
  align-items: start;
}

.models-view__panel {
  background: #fff;
  border-radius: 12px;
  padding: 16px;
  border: 1px solid #f0f0f0;
}

.models-view__title {
  margin: 0 0 8px;
  font-size: 18px;
}

.models-view__hint {
  margin: 0 0 12px;
  color: rgba(0, 0, 0, 0.55);
}

.models-view__alert {
  margin-bottom: 12px;
}

.models-view__filter {
  margin-bottom: 12px;
  max-width: 420px;
}

.models-view__radio-group {
  width: 100%;
  display: block;
}

.models-view__status {
  position: sticky;
  top: 16px;
}

@media (max-width: 960px) {
  .models-view {
    grid-template-columns: 1fr;
  }

  .models-view__status {
    position: static;
  }
}
</style>
