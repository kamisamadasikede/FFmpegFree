<template>
  <RouterLink :to="item.path" class="nav-item" :class="{ active: isActive }" :title="collapsed ? item.label : undefined">
    <FIcon :name="item.icon" />
    <span v-show="!collapsed" class="label">{{ item.label }}</span>
    <span v-if="warn" class="dot" />
    <span v-if="badge && !collapsed" class="badge">{{ badge }}</span>
  </RouterLink>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import type { NavItem } from './navigation'

const props = defineProps<{ item: NavItem; collapsed: boolean; warn?: boolean; badge?: number }>()
const route = useRoute()

const isActive = computed(() =>
  props.item.path === '/' ? route.path === '/' : route.path === props.item.path || route.path.startsWith(props.item.path + '/')
)
</script>

<style scoped>
.nav-item {
  position: relative;
  height: 36px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 10px;
  border-radius: var(--ff-radius-md);
  color: var(--ff-text-2);
  font-size: var(--ff-fs-sm);
  white-space: nowrap;
  transition: background var(--ff-dur-fast) var(--ff-ease), color var(--ff-dur-fast) var(--ff-ease);
}
.nav-item:hover {
  background: var(--ff-bg-hover);
}
.nav-item.active {
  background: var(--ff-primary-soft);
  color: var(--ff-primary);
  font-weight: 500;
}
.dot {
  position: absolute;
  left: 24px;
  top: 8px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--ff-warning);
  box-shadow: 0 0 0 2px var(--ff-bg-sidebar);
}
.badge {
  margin-left: auto;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: var(--ff-primary);
  color: var(--ff-on-primary);
  font-size: 11px;
  font-weight: 600;
  display: grid;
  place-items: center;
}
</style>
