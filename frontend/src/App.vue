<template>
  <div class="app-shell">
    <AppSidebar />
    <div class="app-main">
      <AppTitlebar />
      <main class="app-content">
        <RouterView />
      </main>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import AppSidebar from './layout/AppSidebar.vue'
import AppTitlebar from './layout/AppTitlebar.vue'
import { useTheme } from './composables/useTheme'

useTheme()

// 过渡期：推流结果仍走 v1 的 SSE，任务 store 接上 task:status 事件后删除
let eventSource: EventSource | null = null

onMounted(() => {
  eventSource = new EventSource('http://localhost:19200/api/sse')
  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      if (data.status === 'failed') {
        ElMessage.error(`推流失败: ${data.error}`)
      } else {
        ElMessage.success(`推流已完成: ${data.error}`)
      }
    } catch (error) {
      console.error('SSE 数据解析失败:', error)
    }
  }
  eventSource.onerror = (error) => console.warn('SSE 连接异常:', error)
})

onUnmounted(() => {
  eventSource?.close()
  eventSource = null
})
</script>

<style scoped>
.app-shell {
  display: flex;
  height: 100vh;
  min-width: 1024px;
  min-height: 680px;
  overflow: hidden;
}
.app-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.app-content {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: var(--ff-space-6);
}
</style>
