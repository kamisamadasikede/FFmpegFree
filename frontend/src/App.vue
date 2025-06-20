
<script lang="ts" setup>
import {onMounted, onUnmounted, ref} from 'vue';
import {ElMenu, ElMenuItem, ElMessage} from 'element-plus';
import MenuComponent from './components/MenuComponent.vue';
const handleResize = () => {
  // 触发某些布局更新逻辑（例如通知图表重绘、刷新容器宽高）
  console.log('窗口大小改变:', window.innerWidth, window.innerHeight)
  // 如果你用了 ECharts、地图等库，可以在这里调用 resize 方法
}
onMounted(() => {
  window.addEventListener('resize', handleResize)
  const eventSource = new EventSource('http://localhost:8000/api/sse')

  eventSource.onmessage = (event) => {
    const data = JSON.parse(event.data)
    console.log('收到 SSE 推流状态更新:', data)
    if (data.status === 'failed') {
      ElMessage.error(`推流失败: ${data.error}`)
    } else {
      ElMessage.success(`推流已完成: ${data.error}`)
    }
  }

})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
})
const activeMenu = ref('1');
</script>
<template>
  <div style="display: flex;">
    <MenuComponent  style="width: 20%;min-width: 200px;max-width: 200px"/>
    <div style="flex: 1; width: 80%;min-width: 1200px">
      <router-view></router-view>
    </div>
  </div>
</template>

<style>

</style>
