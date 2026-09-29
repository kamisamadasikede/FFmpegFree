<template>
  <RouterLink :to="item.path" class="nav-item" :class="{ active: isActive }" :title="collapsed ? item.label : undefined">
    <span class="icon-wrap">
      <el-icon :size="20"><component :is="item.icon" /></el-icon>
      <!-- ffmpeg 缺失警告点：下一步接上 ffmpeg 状态 store 后启用 -->
      <span v-if="warn" class="warn-dot" />
    </span>
    <span v-show="!collapsed" class="label">{{ item.label }}</span>
    <span v-if="badge && !collapsed" class="badge">{{ badge }}</span>
  </RouterLink>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import type { NavItem } from './navigation'

const props = defineProps<{ item: NavItem; collapsed: boolean; warn?: boolean; badge?: number }>()
const route = useRoute()

const isActive = computed(() =>
  props.item.path === '/' ? route.path === '/' : route.path === props.item.path || route.path.startsWith(props.item.path + '/')
)
</script>

<style scoped>
.nav-item {
  height: 36px;
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  padding: 0 var(--ff-space-2);
  border-radius: var(--ff-radius-md);
  color: var(--ff-text-2);
  font-size: var(--ff-fs-sm);
  white-space: nowrap;
  transition: background var(--ff-dur-fast) var(--ff-ease), color var(--ff-dur-fast) var(--ff-ease);
}
.nav-item:hover {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
.nav-item.active {
  background: var(--ff-primary-soft);
  color: var(--ff-primary);
  font-weight: 500;
}
.icon-wrap {
  position: relative;
  display: grid;
  place-items: center;
  width: 20px;
  flex-shrink: 0;
}
.warn-dot {
  position: absolute;
  top: -1px;
  right: -2px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--ff-warning);
}
.badge {
  margin-left: auto;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: var(--ff-primary);
  color: #fff;
  font-size: 11px;
  line-height: 18px;
  text-align: center;
}
</style>
