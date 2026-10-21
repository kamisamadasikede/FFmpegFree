<template>
  <div class="settings-layout">
    <nav class="panel snav" aria-label="设置分类">
      <a
        v-for="item in items"
        :key="item.key"
        :href="item.to"
        class="snav-item"
        :class="{ on: active === item.key }"
        :aria-current="active === item.key ? 'page' : undefined"
        @click.prevent="go(item)"
      >
        {{ item.label }}
      </a>
    </nav>
    <div class="sbody">
      <RouterView />
    </div>
  </div>
</template>

<script setup lang="ts">
// 设置页外壳（设计稿 proto/pages.html ?page=settings）：左侧分类导航 + 右侧内容。
// 「外观 / ffmpeg / 转换」是通用页里的三个分组，点击滚动到对应分组；「关于」是独立子路由。
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { encoderPanelVisible } from '@/api/encoder'
import { ENCODER_PANEL_TITLE } from '@/errors/encoderMessages'
import { ENCODER_SECTION_ID, ENCODER_SECTION_QUERY } from '@/api/encoderTask'
import { scrollBehavior } from '@/utils/motion'
import { convertV24On } from '@/api/convertRecords'

interface Item {
  key: string
  label: string
  to: string
  /** 通用页里的分组 id；没有 = 独立子路由 */
  section?: string
}

// “编码设备”一项只在这一块会显示时才出现（ENCODER_BACKEND_READY 且在 Wails 里；纯浏览器只有 ?enc=），与 Settings.vue 里面板的显示条件同一个 encoderPanelVisible()
const items: Item[] = [
  { key: 'appearance', label: '外观', to: '/settings/general', section: 'sec-appearance' },
  { key: 'ffmpeg', label: '转换组件', to: '/settings/general', section: 'sec-ffmpeg' },
  { key: 'asrcomp', label: '语音识别组件', to: '/settings/general', section: 'sec-asrcomp' },
  { key: 'doccomp', label: '文档组件', to: '/settings/general', section: 'sec-doccomp' },
  ...(encoderPanelVisible() ? [{ key: 'encoder', label: ENCODER_PANEL_TITLE, to: '/settings/general', section: ENCODER_SECTION_ID }] : []),
  { key: 'convert', label: '转换', to: '/settings/general', section: 'sec-convert' },
  ...(convertV24On() ? [{ key: 'storage', label: '存储', to: '/settings/general', section: 'sec-storage' }] : []),
  { key: 'about', label: '关于', to: '/settings/about' },
]

const route = useRoute()
const router = useRouter()
const section = ref(route.query.section === ENCODER_SECTION_QUERY && encoderPanelVisible() ? 'encoder' : 'appearance')
// 从提示条的“编码设置”跳来（?section=encoder）时，子导航高亮“编码设备”；定位由 Settings.vue 做
watch(() => route.query.section, (q) => { if (q === ENCODER_SECTION_QUERY && encoderPanelVisible()) section.value = 'encoder' })
if (route.query.section === 'storage' && convertV24On()) section.value = 'storage'
watch(() => route.query.section, (q) => { if (q === 'storage' && convertV24On()) section.value = 'storage' })
const active = computed(() => (route.path.endsWith('/about') ? 'about' : section.value))

async function go(item: Item) {
  if (!item.section) {
    await router.push(item.to)
    return
  }
  section.value = item.key
  if (route.path !== item.to) {
    await router.push(item.to)
    await nextTick()
  }
  document.getElementById(item.section)?.scrollIntoView({ behavior: scrollBehavior(), block: 'start' })
}
</script>

<style scoped>
.settings-layout {
  display: flex;
  align-items: flex-start;
  gap: var(--ff-space-4);
}
.snav {
  width: 176px;
  flex: none;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  position: sticky;
  top: 0;
  background: transparent;
  border: 0;
}
.snav-item {
  height: 34px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  border-radius: var(--ff-radius-md);
  color: var(--ff-text-2);
  font-size: var(--ff-fs-sm);
  transition:
    background var(--ff-dur-fast) var(--ff-ease),
    color var(--ff-dur-fast) var(--ff-ease);
}
.snav-item:hover {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
/* 选中态：软底 + 主色文字（与原型 .subnav a.is-on 一致） */
.snav-item.on {
  background: var(--ff-bg-active);
  color: var(--ff-text-1);
  font-weight: 600;
}
.snav-item:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: -2px;
}
.sbody {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-4);
}

/* ============================================================================
   v0.58 自适应：设置子导航（176px 竖排）+ 内容
   窄窗口下 176 固定宽 + 内容区被挤，表单行（说明 + 控件）叠在一起。
   ≤900 子导航改成上方一行横向可滚动的胶囊，内容整宽落到下面。
   ============================================================================ */
@media (max-width: 1024px) {
  .settings-layout {
    flex-direction: column;
    align-items: stretch;
    gap: 12px;
  }
  .snav {
    width: auto;
    position: static;
    flex-direction: row;
    flex-wrap: nowrap;
    gap: 4px;
    overflow-x: auto;
    overscroll-behavior-x: contain;
    padding-bottom: 2px;
    scrollbar-width: none;
  }
  .snav::-webkit-scrollbar {
    display: none;
  }
  .snav-item {
    flex: none;
    height: 30px;
    padding: 0 12px;
    border: 1px solid transparent;
    white-space: nowrap;
  }
  .snav-item.on {
    border-color: var(--ff-border);
    background: var(--ff-bg-surface);
    box-shadow: var(--ff-shadow-1);
  }
}

@media (max-width: 720px) {
  .snav-item {
    height: 28px;
    padding: 0 10px;
    font-size: 12.5px;
  }
}
</style>
