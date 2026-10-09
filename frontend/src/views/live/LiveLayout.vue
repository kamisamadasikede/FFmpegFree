<template>
  <div class="lv-root">
    <div v-if="!liveIsReal() && !shot" class="lv-demo" role="status">
      <FIcon name="warn" :size="16" />
      <span>直播功能仍在开发中，当前页面为演示，尚未连接真实推流。</span>
    </div>
    <div ref="bodyEl" class="body">
    <div class="top">
    <nav class="seg" aria-label="直播工具">
      <i v-if="segIdx >= 0" class="seg-ink" aria-hidden="true" :style="{ '--i': segIdx, '--n': tabs.length }" />
      <RouterLink v-for="t in tabs" :key="t.to" :to="t.to" class="seg-item">
        {{ t.label }}
      </RouterLink>
    </nav>
    <span class="sp" />
    <LiveSessionEntry />
    </div>
    <!-- 直播转码回退提示条：Tab 条下方的通栏条（设计稿 182/190）；只在推流页签（文件 / 录屏）显示，拉流播放不转码 -->
    <LiveFallbackNotice v-if="isPushTab" />
    <!-- KeepAlive：切换页签不打断进行中的推流 / 拉流。预览在离开时拆掉，回来时重建 -->
    <RouterView v-slot="{ Component }">
      <KeepAlive>
        <component :is="Component" />
      </KeepAlive>
    </RouterView>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useFadeOnChange } from '@/composables/useTabMotion'
import { useRoute } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import LiveFallbackNotice from '@/components/live/LiveFallbackNotice.vue'
import { liveIsReal } from '@/api/live'
import LiveSessionEntry from '@/components/live/LiveSessionEntry.vue'
import { lpVisual } from './lpVisual'
defineOptions({ name: 'LiveLayout' })
// 直播页外壳：三个页签（原型 pages.html 的 .seg，宽 360）+ 页签内容。
const route = useRoute()
// 切页签：页签内容（.body 最后一个子元素）淡入；页签条、会话入口、回退提示条不动
const bodyEl = ref<HTMLElement | null>(null)
useFadeOnChange(() => route.path, () => bodyEl.value?.lastElementChild)
const isPushTab = computed(() => route.path.startsWith('/live/push') || route.path.startsWith('/live/record'))
const shot = !!lpVisual
const tabs = [
  { label: '文件推流', to: '/live/push' },
  { label: '录屏推流', to: '/live/record' },
  { label: '拉流播放', to: '/live/pull' },
]
const segIdx = computed(() => tabs.findIndex((t) => route.path.startsWith(t.to)))
</script>

<style scoped>
.lv-root {
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
  padding: 16px 20px 20px;
}
.top { display: flex; align-items: center; gap: 12px; flex: none; min-width: 0; }
.top .sp { flex: 1; min-width: 0; }
.seg {
  display: flex;
  width: 288px;
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
/* 动画 P1：选中底块是一个滑块，切页签时 translate 滑过去（各项等宽，只动 transform）；减少动效时瞬间 */
.seg {
  position: relative;
}
.seg-ink {
  position: absolute;
  top: 2px;
  left: 2px;
  height: 24px;
  width: calc((100% - 4px) / var(--n));
  translate: calc(var(--i) * 100%) 0;
  border-radius: 4px;
  background: var(--ff-bg-surface);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.08);
  transition: translate var(--ff-dur-base) var(--ff-ease);
  pointer-events: none;
}
.seg-item {
  position: relative;
}
.seg:has(.seg-ink) .seg-item.router-link-active {
  background: transparent;
  box-shadow: none;
}
</style>
