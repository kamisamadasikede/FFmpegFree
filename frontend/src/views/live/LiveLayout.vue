<template>
  <div class="lv-root">
    <div v-if="!liveIsReal() && !shot" class="lv-demo" role="status">
      <FIcon name="warn" :size="16" />
      <span>直播功能仍在开发中，当前页面为演示，尚未连接真实推流。</span>
    </div>
    <div class="body">
    <div class="top">
    <nav class="seg" aria-label="直播工具">
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
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import LiveFallbackNotice from '@/components/live/LiveFallbackNotice.vue'
import { liveIsReal } from '@/api/live'
import LiveSessionEntry from '@/components/live/LiveSessionEntry.vue'
import { lpVisual } from './lpVisual'
defineOptions({ name: 'LiveLayout' })
// 直播页外壳：三个页签（原型 pages.html 的 .seg，宽 360）+ 页签内容。
const route = useRoute()
const isPushTab = computed(() => route.path.startsWith('/live/push') || route.path.startsWith('/live/record'))
const shot = !!lpVisual
const tabs = [
  { label: '文件推流', to: '/live/push' },
  { label: '录屏推流', to: '/live/record' },
  { label: '拉流播放', to: '/live/pull' },
]
</script>

<style scoped>
.lv-root {
  /* 原型 .page 内边距 16/24/20（提示条通栏 32px）。外壳 .app-content 用的是
     --ff-page-pad-x/y（1240/1024/720 三档会变小），这里按同一组变量抵消，
     否则窄窗口下会多留一圈空白、内容被挤窄。由 .body 按设计稿自己留白。 */
  margin: calc(-1 * var(--ff-page-pad-y)) calc(-1 * var(--ff-page-pad-x));
  height: calc(100% + var(--ff-page-pad-y) * 2);
  min-height: 560px;
  display: flex;
  flex-direction: column;
}
.lv-demo {
  height: 32px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 var(--ff-page-pad-x);
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
  padding: 16px var(--ff-page-pad-x) var(--ff-page-pad-y);
}
.top { display: flex; align-items: center; gap: 12px; flex: none; min-width: 0; }
.top .sp { flex: 1; min-width: 0; }
/* v0.58 与文档页同一套页签样式 */
.seg {
  display: inline-flex;
  gap: 4px;
  flex: none;
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

/* v0.58 自适应：窄窗口 */
@media (max-width: 1240px) {
  .body {
    gap: 12px;
  }
}

@media (max-width: 1024px) {
  /* 提示条文案较长，窄窗口允许换行（原来 nowrap 会被侧栏裁掉） */
  .lv-demo {
    height: auto;
    min-height: 32px;
    padding: 6px var(--ff-page-pad-x);
    white-space: normal;
    line-height: 1.45;
    align-items: flex-start;
  }
  .lv-demo svg {
    flex: none;
    margin-top: 1px;
    width: 14px;
    height: 14px;
  }
}

@media (max-width: 720px) {
  .body {
    gap: 10px;
    padding-top: 12px;
  }
  .top {
    gap: 8px;
  }
  .seg-item {
    height: 30px;
    line-height: 30px;
    padding: 0 10px;
    font-size: 12.5px;
  }
  .seg-item.router-link-active {
    line-height: 28px;
  }
}
</style>
