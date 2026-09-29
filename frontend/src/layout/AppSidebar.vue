<template>
  <aside class="sidebar" :class="{ collapsed }">
    <div v-if="macFrameless" class="traffic-space" />
    <div class="brand">
      <div class="logo"><FIcon name="play" :size="16" :stroke="2.2" /></div>
      <div v-show="!collapsed" class="brand-text">
        <b>FFmpegFree</b>
        <small>v{{ version }}</small>
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

    <FFmpegStatusCard v-if="!collapsed" />
    <nav class="nav">
      <button v-if="collapsed" class="expand-btn" title="展开导航" @click="collapsed = false">
        <el-icon :size="16"><Expand /></el-icon>
      </button>
      <SidebarItem v-for="item in bottomNav" :key="item.key" :item="item" :collapsed="collapsed" />
    </nav>
  </aside>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { Expand, Fold } from '@element-plus/icons-vue'
import FIcon from '@/components/icon/FIcon.vue'
import FFmpegStatusCard from '@/components/ffmpeg/FFmpegStatusCard.vue'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { useTaskStore } from '@/stores/tasks'
import SidebarItem from './SidebarItem.vue'
import { mainNav, bottomNav } from './navigation'

// 改成无边框窗口后置为 true，macOS 在顶部留出红绿灯位置
const macFrameless = false
const version = '2.0'
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
.sidebar.collapsed {
  width: var(--ff-sidebar-w-collapsed);
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
