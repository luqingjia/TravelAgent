<script setup lang="ts">
/** 根组件仅挂载路由；UI 库边界在子页面内隔离。 */
</script>

<template>
  <RouterView />
</template>

<style>
html,
body,
#app {
  margin: 0;
  min-height: 100%;
  font-family:
    -apple-system,
    BlinkMacSystemFont,
    'Segoe UI',
    Roboto,
    'Helvetica Neue',
    Arial,
    'Noto Sans',
    sans-serif;
  background: #f5f7fb;
}
</style>
