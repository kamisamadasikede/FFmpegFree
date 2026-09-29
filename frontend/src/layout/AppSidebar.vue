<template>
  <aside class="sidebar" :class="{ collapsed }">
    <div class="brand">
      <div class="brand-mark">FF</div>
      <span v-show="!collapsed" class="brand-title">FFmpegFree</span>
    </div>

    <nav class="nav">
      <SidebarItem v-for="item in mainNav" :key="item.key" :item="item" :collapsed="collapsed" />
    </nav>

    <div class="nav nav-bottom">
      <SidebarItem v-for="item in bottomNav" :key="item.key" :item="item" :collapsed="collapsed" />
      <button class="collapse-btn" :title="collapsed ? '展开导航' : '收起导航'" @click="toggle">
        <el-icon :size="16"><component :is="collapsed ? Expand : Fold" /></el-icon>
      </button>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { Expand, Fold } from '@element-plus/icons-vue'
import SidebarItem from './SidebarItem.vue'
import { mainNav, bottomNav } from './navigation'

const KEY = 'ff-sidebar-collapsed'
const collapsed = ref(localStorage.getItem(KEY) === '1')
const toggle = () => (collapsed.value = !collapsed.value)
watch(collapsed, (v) => localStorage.setItem(KEY, v ? '1' : '0'))
</script>

<style scoped>
.sidebar {
  width: var(--ff-sidebar-w);
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-2);
  padding: var(--ff-space-2);
  background: var(--ff-bg-sidebar);
  border-right: 1px solid var(--ff-border);
  transition: width var(--ff-dur-base) var(--ff-ease);
  overflow: hidden;
}
.sidebar.collapsed {
  width: var(--ff-sidebar-w-collapsed);
}
.brand {
  height: calc(var(--ff-titlebar-h) - var(--ff-space-2));
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  padding: 0 var(--ff-space-1);
  --wails-draggable: drag;
}
.brand-mark {
  width: 28px;
  height: 28px;
  flex-shrink: 0;
  border-radius: var(--ff-radius-md);
  display: grid;
  place-items: center;
  background: var(--ff-primary);
  color: #fff;
  font-size: var(--ff-fs-xs);
  font-weight: 600;
}
.brand-title {
  font-size: var(--ff-fs-md);
  font-weight: 600;
  white-space: nowrap;
}
.nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.nav-bottom {
  margin-top: auto;
}
.collapse-btn {
  height: 32px;
  border: none;
  background: transparent;
  color: var(--ff-text-3);
  border-radius: var(--ff-radius-md);
  cursor: pointer;
  display: grid;
  place-items: center;
}
.collapse-btn:hover {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
</style>
