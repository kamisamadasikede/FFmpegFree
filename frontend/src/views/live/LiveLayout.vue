<template>
  <div class="live">
    <div v-if="!liveIsReal()" class="lv-demo" role="status">
      <FIcon name="warn" :size="16" />
      <span>直播功能仍在开发中，当前页面为演示，尚未连接真实推流。</span>
    </div>
    <div class="body">
    <nav class="seg" aria-label="直播工具">
      <RouterLink v-for="t in tabs" :key="t.to" :to="t.to" class="seg-item">
        {{ t.label }}
      </RouterLink>
    </nav>
    <!-- KeepAlive：切换页签不打断进行中的推流 / 播放 -->
    <RouterView v-slot="{ Component }">
      <KeepAlive>
        <component :is="Component" />
      </KeepAlive>
    </RouterView>
    </div>
  </div>
</template>

<script setup lang="ts">
import FIcon from '@/components/icon/FIcon.vue'
import { liveIsReal } from '@/api/live'
// 直播页外壳：三个页签（原型 pages.html 的 .seg，宽 360）+ 页签内容。
const tabs = [
  { label: '文件推流', to: '/live/push' },
  { label: '录屏推流', to: '/live/record' },
  { label: '拉流播放', to: '/live/pull' },
]
</script>

<style scoped>
.live {
  /* 原型 .page 内边距 16/24/20（提示条通栏 32px），外壳 .app-content 是 20/24/24：抵消外壳内边距，由 .body 按设计稿自己留白 */
  margin: -20px -24px -24px;
  height: calc(100% + 44px);
  min-height: 600px;
  display: flex;
  flex-direction: column;
}
.lv-demo {
  height: 32px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 24px;
  background: color-mix(in srgb, var(--ff-warning) 10%, var(--ff-bg-app));
  border-bottom: 1px solid color-mix(in srgb, var(--ff-warning) 28%, transparent);
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-1);
  white-space: nowrap;
}
.lv-demo svg {
  color: var(--ff-warning-text);
}
.body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px 24px 20px;
}
.seg {
  display: flex;
  width: 360px;
  flex: none;
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
  font-size: var(--ff-fs-xs);
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
</style>
