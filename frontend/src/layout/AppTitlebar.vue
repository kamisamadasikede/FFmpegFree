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
import { APP_NAME } from '@/config/brand'
import { useTheme } from '@/composables/useTheme'

const route = useRoute()
const { mode, isDark } = useTheme()

const meta = computed(() => {
  const root = route.matched[0]?.meta ?? {}
  // 子路由自己带 title 时用子路由的（v0.26 文档转换页）
  const leaf = route.matched[route.matched.length - 1]?.meta ?? {}
  const top = leaf.title ? leaf : root
  return { title: (top.title as string) || APP_NAME, subtitle: top.subtitle as string | undefined }
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
  gap: var(--ff-space-3);
  padding: 0 var(--ff-space-6);
  border-bottom: 1px solid var(--ff-border);
  background: var(--ff-bg-app);
  --wails-draggable: drag;
}
h1 {
  margin: 0;
  font-size: 17px;
  font-weight: 650;
  letter-spacing: -0.01em;
  white-space: nowrap;
}
.crumb {
  color: var(--ff-text-3);
  font-size: 12.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.sp {
  flex: 1;
}
.iconbtn {
  width: 32px;
  height: 32px;
  border: none;
  background: transparent;
  border-radius: var(--ff-radius-md);
  display: grid;
  place-items: center;
  color: var(--ff-text-2);
  cursor: pointer;
  --wails-draggable: no-drag;
  transition:
    background var(--ff-dur-fast) var(--ff-ease),
    color var(--ff-dur-fast) var(--ff-ease);
}
.iconbtn:hover {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}

/* ---------- 窄窗口：标题收小、副标题让位 ---------- */
@media (max-width: 1024px) {
  .titlebar {
    padding: 0 var(--ff-space-4);
    gap: 10px;
  }
  h1 {
    font-size: 15px;
  }
  .crumb {
    display: none;
  }
}

@media (max-width: 720px) {
  .titlebar {
    padding: 0 var(--ff-space-3);
    gap: 8px;
  }
  .iconbtn {
    width: 28px;
    height: 28px;
  }
}
</style>
