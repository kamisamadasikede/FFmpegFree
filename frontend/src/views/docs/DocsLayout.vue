<template>
  <div class="docs">
    <nav class="seg" aria-label="文档工具">
      <RouterLink v-for="t in tabs" :key="t.to" :to="t.to" class="seg-item">{{ t.label }}</RouterLink>
    </nav>
    <div class="row" :class="{ 'with-recent': !isConvert }">
      <!-- KeepAlive：切换 Tab 保留各自状态（Office 列表和任务订阅、当前打开的 PDF 都不丢） -->
      <RouterView v-slot="{ Component }">
        <KeepAlive>
          <component :is="Component" />
        </KeepAlive>
      </RouterView>
      <RecentPanel v-if="!isConvert" />
    </div>
  </div>
</template>

<script setup lang="ts">
// 文档页外壳（设计说明 2）：分段控件 240×28（同直播页 .seg）+ 两个 Tab + 两个 Tab 共用的右侧「最近打开的 PDF」。
import { computed, onBeforeUnmount, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import RecentPanel from '@/components/docs/RecentPanel.vue'
import { onFilesDropped } from '@/api/fileDrop'
import { dropHandlers } from '@/stores/docs'

const route = useRoute()
// v0.26 文档转换页自带左右两栏，不显示「最近打开的 PDF」
const isConvert = computed(() => route.path.endsWith('/office'))
// OnFileDrop 只有一个入口：这里注册一次，按当前 Tab 分发（各页 KeepAlive 后自己注册 / 注销会互相清掉监听）
let offDrop: () => void = () => {}
onMounted(() => {
  offDrop = onFilesDropped((paths) => (route.path.endsWith('/pdf') ? dropHandlers.pdf : dropHandlers.office)?.(paths))
})
onBeforeUnmount(() => offDrop())

const tabs = [
  { label: '文档转换', to: '/docs/office' },
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
/* v0.58 对齐原型：页签改卡片式（选中卡片面 + 1px 描边），不再用深底段控 */
.seg {
  display: inline-flex;
  gap: 4px;
  flex: none;
  align-self: flex-start;
  padding: 0;
  background: transparent;
  border-radius: 0;
}
.seg-item {
  height: 34px;
  line-height: 34px;
  padding: 0 14px;
  border-radius: var(--ff-radius-md);
  color: var(--ff-text-2);
  font-size: var(--ff-fs-sm);
  transition:
    color var(--ff-dur-fast) var(--ff-ease),
    background var(--ff-dur-fast) var(--ff-ease),
    box-shadow var(--ff-dur-fast) var(--ff-ease);
}
.seg-item:hover {
  color: var(--ff-text-1);
  background: var(--ff-bg-hover);
}
.seg-item.router-link-active {
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  font-weight: 600;
  box-shadow: var(--ff-shadow-1);
  border: 1px solid var(--ff-border);
  line-height: 32px;
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

/* v0.58 自适应：PDF 预览页的「最近打开」右栏在窄窗口下压掉阅读区（≤1024 只剩 300 宽）。
   ≤900 时右栏改到内容下方、横向铺满；文档转换页本来就不显示右栏，不受影响。 */
@media (max-width: 1024px) {
  .docs {
    gap: 12px;
  }
  .row {
    flex-direction: column;
    min-height: 0;
    overflow-y: auto;
  }
  .row.with-recent > :first-child {
    flex: 1 0 auto;
    min-height: 320px;
  }
  .row :deep(.recent) {
    width: auto;
    flex: none;
    max-height: 280px;
    min-height: 160px;
  }
}

@media (max-width: 720px) {
  .seg-item {
    height: 30px;
    line-height: 30px;
    padding: 0 10px;
    font-size: 12.5px;
  }
  .seg-item.router-link-active {
    line-height: 28px;
  }
  .row.with-recent > :first-child {
    min-height: 260px;
  }
  .row :deep(.recent) {
    max-height: 220px;
  }
}
</style>
