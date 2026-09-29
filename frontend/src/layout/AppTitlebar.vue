<template>
  <header class="titlebar">
    <h1>{{ meta.title }}</h1>
    <span v-if="meta.subtitle" class="crumb">{{ meta.subtitle }}</span>
    <span class="sp" />
    <button class="iconbtn" title="搜索" @click="onSearch"><FIcon name="search" /></button>
    <button class="iconbtn" :title="isDark ? '切换到浅色' : '切换到暗色'" @click="toggleTheme">
      <FIcon :name="isDark ? 'sun' : 'moon'" />
    </button>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { useTheme } from '@/composables/useTheme'

const route = useRoute()
const { mode, isDark } = useTheme()

const meta = computed(() => {
  const top = route.matched[0]?.meta ?? {}
  return { title: (top.title as string) || 'FFmpegFree', subtitle: top.subtitle as string | undefined }
})

function toggleTheme() {
  mode.value = isDark.value ? 'light' : 'dark'
}

function onSearch() {
  // TODO: 全局搜索（文件、任务、预设）待产品定义
  ElMessage.info('搜索功能开发中')
}
</script>

<style scoped>
.titlebar {
  height: var(--ff-titlebar-h);
  flex: none;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 16px 0 24px;
  border-bottom: 1px solid var(--ff-border);
  --wails-draggable: drag;
}
h1 {
  margin: 0;
  font-size: var(--ff-fs-md);
  font-weight: 600;
}
.crumb {
  color: var(--ff-text-3);
  font-size: var(--ff-fs-xs);
}
.sp {
  flex: 1;
}
.iconbtn {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  border-radius: var(--ff-radius-md);
  display: grid;
  place-items: center;
  color: var(--ff-text-2);
  cursor: pointer;
  --wails-draggable: no-drag;
}
.iconbtn:hover {
  background: var(--ff-bg-hover);
}
</style>
