<template>
  <!-- Cat 页（route.meta.layout=cat）：整条侧栏 250ms 收到 0 宽并淡出，期间不可聚焦；离开 Cat 时展开回来（原型 cat-v3） -->
  <aside
    class="sidebar"
    :class="{ collapsed, 'cat-away': catAway, 'cat-leaving': catLeaving }"
    :inert="catAway || undefined"
    :aria-hidden="catAway || undefined"
  >
    <div v-if="macFrameless" class="traffic-space" />
    <div class="brand">
      <AppLogo :size="30" :decorative="!collapsed" />
      <div v-show="!collapsed" class="brand-text">
        <b>{{ APP_NAME }}</b>
      </div>
      <span v-show="!collapsed" class="ver">{{ versionText }}</span>
      <button v-show="!collapsed" class="collapse-btn" title="收起导航" @click="collapsed = true">
        <el-icon :size="14"><Fold /></el-icon>
      </button>
    </div>

    <div class="side-scroll">
      <nav v-for="group in navGroups" :key="group.label" class="nav-group">
        <div v-show="!collapsed" class="nav-label">{{ group.label }}</div>
        <SidebarItem
          v-for="item in group.items"
          :key="item.key"
          :item="item"
          :collapsed="collapsed"
          :badge="item.key === 'tasks' ? tasks.runningCount : 0"
        />
      </nav>
    </div>

    <div class="side-foot">
      <FFmpegStatusCard :collapsed="collapsed" />
      <button v-if="collapsed" class="expand-btn" title="展开导航" @click="collapsed = false">
        <el-icon :size="16"><Expand /></el-icon>
      </button>
      <SidebarItem v-for="item in bottomNav" :key="item.key" :item="item" :collapsed="collapsed" />
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Expand, Fold } from '@element-plus/icons-vue'
import AppLogo from '@/components/brand/AppLogo.vue'
import { APP_NAME } from '@/config/brand'
import FFmpegStatusCard from '@/components/ffmpeg/FFmpegStatusCard.vue'
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
// 从 Cat 返回时展开也走同样的 250ms（宽度 + 内边距 + 透明度）
const catLeaving = ref(false)
let leaveTimer: ReturnType<typeof setTimeout> | undefined
watch(catAway, (away, was) => {
  clearTimeout(leaveTimer)
  catLeaving.value = !away && !!was
  if (catLeaving.value) leaveTimer = setTimeout(() => (catLeaving.value = false), 300)
})
const tasks = useTaskStore()

/**
 * 导航分组（原型：主功能 / 工具与系统）。
 * navigation.ts 仍是唯一数据源，这里只做分组切分，不新增第二份清单。
 */
const navGroups = computed(() => [
  { label: '主功能', items: mainNav.filter((i) => i.key !== 'tasks') },
  { label: '工具与系统', items: mainNav.filter((i) => i.key === 'tasks') },
])

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
  transition:
    width 250ms var(--ff-ease),
    padding 250ms var(--ff-ease),
    opacity 200ms var(--ff-ease),
    border-color 250ms var(--ff-ease);
}
.sidebar.collapsed {
  width: var(--ff-sidebar-w-collapsed);
  overflow: visible; /* 折叠时 ffmpeg 状态的悬停气泡在侧栏外右侧，不能被裁掉 */
}
.traffic-space {
  height: var(--ff-titlebar-h);
  flex: none;
  --wails-draggable: drag;
}

/* 品牌区：与顶栏同高（原型 .side-brand） */
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  height: var(--ff-titlebar-h);
  padding: 0 18px;
  flex: none;
  --wails-draggable: drag;
}
.brand-text {
  flex: 1;
  min-width: 0;
}
.brand-text b {
  display: block;
  font-size: var(--ff-fs-md);
  font-weight: 650;
  letter-spacing: 0.2px;
  line-height: 1.3;
}
.ver {
  font-size: 11px;
  color: var(--ff-text-3);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
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
  height: 34px;
  margin: 4px 0;
}
.collapse-btn:hover,
.expand-btn:hover {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}

/* 导航区：可滚动，分组标题 */
.side-scroll {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 6px 10px 12px;
}
.nav-group + .nav-group {
  margin-top: 18px;
}
.nav-label {
  padding: 6px 10px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ff-text-3);
}
.nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.side-foot {
  flex: none;
  padding: 10px;
  border-top: 1px solid var(--ff-border);
}

/* ---------- 窄窗口：侧栏收成图标轨 ---------- */
@media (max-width: 1024px) {
  .brand {
    padding: 0;
    justify-content: center;
    gap: 0;
  }
  /* v-show 是按「用户手动折叠」控制的，窄窗口靠 CSS 强制隐藏文字，避免竖排断行 */
  .brand-text,
  .ver,
  .nav-label,
  .collapse-btn {
    display: none !important;
  }
  .side-scroll {
    padding: 8px;
  }
  .nav-group + .nav-group {
    margin-top: 10px;
  }
  .side-foot {
    padding: 8px;
  }
}
</style>
