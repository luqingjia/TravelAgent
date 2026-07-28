<script setup lang="ts">
/**
 * Ant Design Vue 应用壳：侧边/顶部导航与内容区。
 * 不混用 Element Plus 或 Vuetify 控件。
 * 窄屏隐藏侧栏时在顶栏提供等价导航，保证基本可用性。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Button,
  Drawer,
  Layout,
  LayoutContent,
  LayoutHeader,
  LayoutSider,
  Menu,
  MenuItem,
  TypographyText,
  Grid,
} from 'ant-design-vue'
import { ApiOutlined, MenuOutlined, MessageOutlined } from '@ant-design/icons-vue'

import { useAgentStore } from '@/stores/agent'

const route = useRoute()
const router = useRouter()
const agent = useAgentStore()
const screens = Grid.useBreakpoint()
const mobileNavOpen = ref(false)

/** md 以下视为窄屏：侧栏收起，顶栏提供菜单入口。 */
const isNarrow = computed(() => !(screens.value.md || screens.value.lg || screens.value.xl || screens.value.xxl))
const selectedKeys = computed(() => {
  if (route.path.startsWith('/models')) return ['models']
  return ['chat']
})

const selectedLabel = computed(() => {
  const model = agent.selectedModel
  if (!model) return '未选择模型'
  return model.displayName || model.id
})

function navigate(key: string) {
  if (key === 'chat') void router.push('/chat')
  if (key === 'models') void router.push('/models')
  mobileNavOpen.value = false
}

function onMenuClick(info: { key: string | number }) {
  navigate(String(info.key))
}

watch(
  () => route.fullPath,
  () => {
    mobileNavOpen.value = false
  },
)

onMounted(() => {
  void agent.loadModels()
})
</script>

<template>
  <Layout class="app-shell" has-sider>
    <LayoutSider
      class="app-shell__sider"
      theme="light"
      :width="220"
      :collapsed-width="0"
      :collapsed="isNarrow"
      :trigger="null"
      breakpoint="md"
      collapsible
    >
      <div class="app-shell__brand">TravelAgent</div>
      <Menu mode="inline" :selected-keys="selectedKeys" @click="onMenuClick">
        <MenuItem key="chat">
          <MessageOutlined />
          <span>对话</span>
        </MenuItem>
        <MenuItem key="models">
          <ApiOutlined />
          <span>模型绑定</span>
        </MenuItem>
      </Menu>
    </LayoutSider>

    <Layout>
      <LayoutHeader class="app-shell__header">
        <div class="app-shell__header-left">
          <Button
            v-if="isNarrow"
            type="text"
            data-testid="mobile-nav-button"
            aria-label="打开导航"
            @click="mobileNavOpen = true"
          >
            <MenuOutlined />
          </Button>
          <TypographyText strong>{{ route.meta.title || 'TravelAgent' }}</TypographyText>
        </div>
        <div class="app-shell__header-right">
          <TypographyText type="secondary">当前模型：</TypographyText>
          <TypographyText>{{ selectedLabel }}</TypographyText>
        </div>
      </LayoutHeader>
      <LayoutContent class="app-shell__content">
        <RouterView />
      </LayoutContent>
    </Layout>

    <Drawer
      v-model:open="mobileNavOpen"
      placement="left"
      :width="260"
      title="导航"
      :body-style="{ padding: '12px 0' }"
      data-testid="mobile-nav-drawer"
    >
      <Menu mode="inline" :selected-keys="selectedKeys" @click="onMenuClick">
        <MenuItem key="chat">
          <MessageOutlined />
          <span>对话</span>
        </MenuItem>
        <MenuItem key="models">
          <ApiOutlined />
          <span>模型绑定</span>
        </MenuItem>
      </Menu>
    </Drawer>
  </Layout>
</template>

<style scoped>
.app-shell {
  min-height: 100vh;
  background: #f5f7fb;
}

.app-shell__sider {
  border-right: 1px solid #f0f0f0;
}

.app-shell__brand {
  height: 64px;
  display: flex;
  align-items: center;
  padding: 0 20px;
  font-weight: 700;
  font-size: 16px;
  color: #1677ff;
}

.app-shell__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  background: #fff;
  border-bottom: 1px solid #f0f0f0;
  padding-inline: 20px;
  height: 64px;
  line-height: 64px;
}

.app-shell__header-left,
.app-shell__header-right {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.app-shell__content {
  padding: 16px;
  min-height: calc(100vh - 64px);
}

@media (max-width: 767px) {
  .app-shell__header {
    flex-direction: column;
    align-items: flex-start;
    height: auto;
    line-height: 1.4;
    padding: 12px 16px;
  }

  .app-shell__content {
    padding: 12px;
  }
}
</style>
