<template>
  <div class="docs">
    <nav class="seg" aria-label="文档工具">
      <RouterLink v-for="t in tabs" :key="t.to" :to="t.to" class="seg-item">{{ t.label }}</RouterLink>
    </nav>
    <div class="row">
      <!-- KeepAlive：切换 Tab 保留各自状态（Office 列表和任务订阅、当前打开的 PDF 都不丢） -->
      <RouterView v-slot="{ Component }">
        <KeepAlive>
          <component :is="Component" />
        </KeepAlive>
      </RouterView>
      <RecentPanel />
    </div>
  </div>
</template>

<script setup lang="ts">
// 文档页外壳（设计说明 2）：分段控件 240×28（同直播页 .seg）+ 两个 Tab + 两个 Tab 共用的右侧「最近打开的 PDF」。
import { onBeforeUnmount, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import RecentPanel from '@/components/docs/RecentPanel.vue'
import { onFilesDropped } from '@/api/fileDrop'
import { dropHandlers } from '@/stores/docs'

const route = useRoute()
// OnFileDrop 只有一个入口：这里注册一次，按当前 Tab 分发（各页 KeepAlive 后自己注册 / 注销会互相清掉监听）
let offDrop: () => void = () => {}
onMounted(() => {
  offDrop = onFilesDropped((paths) => (route.path.endsWith('/pdf') ? dropHandlers.pdf : dropHandlers.office)?.(paths))
})
onBeforeUnmount(() => offDrop())

const tabs = [
  { label: 'Office 转 PDF', to: '/docs/office' },
  { label: 'PDF 预览', to: '/docs/pdf' },
]
</script>

<style scoped>
.docs {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.seg {
  display: flex;
  width: 240px;
  height: 28px;
  flex: none;
  box-sizing: border-box;
  background: var(--ff-bg-hover);
  border-radius: 6px;
  padding: 2px;
}
.seg-item {
  flex: 1;
  text-align: center;
  height: 24px;
  line-height: 24px;
  border-radius: 4px;
  color: var(--ff-text-2);
  font-size: 12px;
  transition: color var(--ff-dur-fast) var(--ff-ease), background var(--ff-dur-fast) var(--ff-ease);
}
.seg-item:hover {
  color: var(--ff-text-1);
}
.seg-item.router-link-active {
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  font-weight: 500;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.08);
}
.seg-item:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 1px;
}
.row {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: 16px;
}
</style>
