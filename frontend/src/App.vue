
<script lang="ts" setup>
import { onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import MenuComponent from './components/MenuComponent.vue'

let eventSource: EventSource | null = null

const handleResize = () => {
  // 触发布局更新逻辑（例如通知图表重绘、刷新容器宽高）
  console.log('窗口大小改变:', window.innerWidth, window.innerHeight)
}

onMounted(() => {
  window.addEventListener('resize', handleResize)
  eventSource = new EventSource('http://localhost:19200/api/sse')

  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      console.log('收到 SSE 推流状态更新:', data)
      if (data.status === 'failed') {
        ElMessage.error(`推流失败: ${data.error}`)
      } else {
        ElMessage.success(`推流已完成: ${data.error}`)
      }
    } catch (error) {
      console.error('SSE 数据解析失败:', error)
    }
  }

  eventSource.onerror = (error) => {
    console.warn('SSE 连接异常:', error)
  }
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
})
</script>
<template>
  <div class="app-shell">
    <aside class="app-sidebar">
      <MenuComponent />
    </aside>
    <main class="app-main">
      <header class="app-header">
        <div>
          <div class="app-title">FFmpegFree</div>
          <div class="app-subtitle">音视频 / 文档 / 流媒体工具箱</div>
        </div>
      </header>
      <div class="page-shell">
        <router-view></router-view>
      </div>
    </main>
  </div>
</template>
