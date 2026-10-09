<template>
  <!-- Cat 页（route.meta.layout=cat）：整条侧栏 250ms 收到 0 宽并淡出，期间不可聚焦；离开 Cat 时展开回来（原型 cat-v3） -->
  <aside class="sidebar" :class="{ collapsed, 'cat-away': catAway, 'cat-leaving': catLeaving }" :inert="catAway || undefined" :aria-hidden="catAway || undefined">
    <div v-if="macFrameless" class="traffic-space" />
    <div class="brand">
      <div class="logo"><FIcon name="play" :size="16" :stroke="2.2" /></div>
      <div v-show="!collapsed" class="brand-text">
        <b>FFmpegFree</b>
        <small>{{ versionText }}</small>
      </div>
      <button v-show="!collapsed" class="collapse-btn" title="收起导航" @click="collapsed = true">
        <el-icon :size="14"><Fold /></el-icon>
      </button>
    </div>

    <nav class="nav">
      <template v-for="item in mainNav" :key="item.key">
        <div v-if="item.separatorBefore" class="sep" />
        <SidebarItem
          :item="item"
          :collapsed="collapsed"
          :badge="item.key === 'tasks' ? tasks.runningCount : 0"
        />
      </template>
    </nav>

    <div class="grow" />

    <nav class="nav">
      <FFmpegStatusCard :collapsed="collapsed" />
      <button v-if="collapsed" class="expand-btn" title="展开导航" @click="collapsed = false">
        <el-icon :size="16"><Expand /></el-icon>
      </button>
      <SidebarItem v-for="item in bottomNav" :key="item.key" :item="item" :collapsed="collapsed" />
    </nav>
  </aside>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Expand, Fold } from '@element-plus/icons-vue'
import FIcon from '@/components/icon/FIcon.vue'
import FFmpegStatusCard from '@/components/ffmpeg/FFmpegStatusCard.vue'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { useTaskStore } from '@/stores/tasks'
import { getAppVersion, DEV_VERSION } from '@/api/about'
import SidebarItem from './SidebarItem.vue'
import { mainNav, bottomNav } from './navigation'

// 改成无边框窗口后置为 true，macOS 在顶部留出红绿灯位置
const macFrameless = false
// 版本号与关于页一致：取 GetAppVersion，失败或返回空回落“开发版”
const version = ref(DEV_VERSION)
const versionText = computed(() => (/^\d/.test(version.value) ? `v${version.value}` : version.value))
onMounted(async () => {
  try {
    version.value = (await getAppVersion()).trim() || DEV_VERSION
  } catch (e) {
    console.error('读取版本号失败', e)
    version.value = DEV_VERSION
  }
})
const route = useRoute()
const catAway = computed(() => route.meta.layout === 'cat')
// 从 Cat 返回时展开也走同样的 250ms（宽度 + 内边距 + 透明度），结束后恢复侧栏自己的过渡
const catLeaving = ref(false)
let leaveTimer: ReturnType<typeof setTimeout> | undefined
watch(catAway, (away, was) => {
  clearTimeout(leaveTimer)
  catLeaving.value = !away && !!was
  if (catLeaving.value) leaveTimer = setTimeout(() => (catLeaving.value = false), 300)
})
const ffmpeg = useFFmpegStore()
const tasks = useTaskStore()

const KEY = 'ff-sidebar-collapsed'
const collapsed = ref(localStorage.getItem(KEY) === '1')
watch(collapsed, (v) => localStorage.setItem(KEY, v ? '1' : '0'))
</script>

<style scoped>
.sidebar {
  width: var(--ff-sidebar-w);
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  padding: 0 8px 12px;
  background: var(--ff-bg-sidebar);
  border-right: 1px solid var(--ff-border);
  transition: width var(--ff-dur-base) var(--ff-ease);
  overflow: hidden;
}
.sidebar.cat-away,
.sidebar.collapsed.cat-away {
  width: 0;
  padding-left: 0;
  padding-right: 0;
  opacity: 0;
  border-right-color: transparent;
  overflow: hidden;
  white-space: nowrap;
}
.sidebar.cat-away,
.sidebar.cat-leaving {
  transition: width 250ms var(--ff-ease), padding 250ms var(--ff-ease), opacity 200ms var(--ff-ease), border-color 250ms var(--ff-ease);
}
.sidebar.collapsed {
  width: var(--ff-sidebar-w-collapsed);
  overflow: visible; /* 折叠时 ffmpeg 状态的悬停气泡在侧栏外右侧（设计说明 编码设备-v0.1），不能被裁掉；折叠后文字都已 v-show 隐藏 */
}
.traffic-space {
  height: var(--ff-titlebar-h);
  flex: none;
  --wails-draggable: drag;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 10px 16px;
  --wails-draggable: drag;
}
.collapsed .brand {
  padding: 16px 6px;
}
.logo {
  width: 28px;
  height: 28px;
  flex: none;
  border-radius: 8px;
  background: linear-gradient(135deg, #3b6ef5, #8b5cf6);
  color: #fff;
  display: grid;
  place-items: center;
}
.brand-text {
  flex: 1;
  min-width: 0;
}
.brand-text b {
  display: block;
  font-size: var(--ff-fs-md);
  font-weight: 600;
  line-height: 1.3;
}
.brand-text small {
  display: block;
  font-size: 12px;
  color: var(--ff-text-3);
  line-height: 1.2;
}
.collapse-btn,
.expand-btn {
  border: none;
  background: transparent;
  color: var(--ff-text-3);
  border-radius: var(--ff-radius-md);
  cursor: pointer;
  display: grid;
  place-items: center;
  --wails-draggable: no-drag;
}
.collapse-btn {
  width: 24px;
  height: 24px;
  opacity: 0;
  transition: opacity var(--ff-dur-fast) var(--ff-ease);
}
.sidebar:hover .collapse-btn {
  opacity: 1;
}
.expand-btn {
  height: 36px;
}
.collapse-btn:hover,
.expand-btn:hover {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
.nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.sep {
  height: 1px;
  background: var(--ff-border);
  margin: 8px 10px;
}
.grow {
  flex: 1;
}
</style>
