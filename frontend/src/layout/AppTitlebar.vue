<template>
  <header class="titlebar" :class="{ 'is-mac-frameless': isMac && frameless }">
    <span class="title">{{ title }}</span>
    <div class="actions"><slot /></div>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

// 目前窗口仍用系统标题栏；改成无边框窗口后把 frameless 置为 true，macOS 会留出红绿灯位置
const frameless = false
const isMac = navigator.userAgent.includes('Mac')
const route = useRoute()
const title = computed(() => {
  const titled = [...route.matched].reverse().find((r) => r.meta?.title)
  return (titled?.meta?.title as string) || 'FFmpegFree'
})
</script>

<style scoped>
.titlebar {
  height: var(--ff-titlebar-h);
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--ff-space-3);
  padding: 0 var(--ff-space-6);
  border-bottom: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  --wails-draggable: drag;
}
.titlebar.is-mac-frameless {
  padding-left: 76px;
}
.title {
  font-size: var(--ff-fs-md);
  font-weight: 600;
}
.actions {
  margin-left: auto;
  display: flex;
  gap: var(--ff-space-2);
  --wails-draggable: no-drag;
}
</style>
