<template>
  <div class="live">
    <nav class="seg" aria-label="直播工具">
      <RouterLink v-for="t in tabs" :key="t.to" :to="t.to" class="seg-item">
        {{ t.label }}
      </RouterLink>
    </nav>
    <MigrationNotice text="直播功能仍在开发中，当前页面为演示，尚未连接真实推流。" />
    <!-- KeepAlive：切换页签不打断进行中的推流 / 播放 -->
    <RouterView v-slot="{ Component }">
      <KeepAlive>
        <component :is="Component" />
      </KeepAlive>
    </RouterView>
  </div>
</template>

<script setup lang="ts">
import MigrationNotice from '@/components/common/MigrationNotice.vue'
// 直播页外壳：三个页签（原型 pages.html 的 .seg，宽 360）+ 页签内容。
const tabs = [
  { label: '文件推流', to: '/live/push' },
  { label: '录屏推流', to: '/live/record' },
  { label: '拉流播放', to: '/live/pull' },
]
</script>

<style scoped>
.live {
  /* 原型 .page 的内边距是 16/24/20，外壳 .app-content 是 20/24/24，这里各补 4px */
  margin: -4px 0;
  height: calc(100% + 8px);
  min-height: 600px;
  display: flex;
  flex-direction: column;
  gap: 16px;
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
</style>
