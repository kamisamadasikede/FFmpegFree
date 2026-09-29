<template>
  <el-tooltip v-if="gated" content="需要先安装 ffmpeg" placement="right" :show-after="300" :hide-after="0">
    <a
      class="nav-item gated"
      :class="{ active: isActive }"
      role="link"
      tabindex="0"
      aria-disabled="true"
      :aria-label="item.label"
      @click.prevent="openInstall"
      @keydown.enter.prevent="openInstall"
      @keydown.space.prevent="openInstall"
    >
      <FIcon :name="item.icon" />
      <span v-show="!collapsed" class="label">{{ item.label }}</span>
      <span v-if="warn" class="dot" />
    </a>
  </el-tooltip>
  <RouterLink v-else :to="item.path" class="nav-item" :class="{ active: isActive }" :title="collapsed ? item.label : undefined">
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
import { useFFmpegStore } from '@/stores/ffmpeg'
import type { NavItem } from './navigation'

const props = defineProps<{ item: NavItem; collapsed: boolean; warn?: boolean; badge?: number }>()
const route = useRoute()
const ffmpeg = useFFmpegStore()

// ffmpeg 不可用时，依赖它的入口保持可见但置灰：不导航，点击改为重新打开安装对话框（「稍后」不会让它恢复）
const gated = computed(() => !!props.item.needsFFmpeg && ffmpeg.featuresBlocked)
function openInstall() {
  ffmpeg.dialogOpen = true
}

// route.matched 为空说明首次导航还没完成（此时 route.path 是初始的 '/'，会让「转换」闪一下高亮），先不高亮任何项
const isActive = computed(
  () =>
    route.matched.length > 0 &&
    (props.item.path === '/' ? route.path === '/' : route.path === props.item.path || route.path.startsWith(props.item.path + '/')),
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
.nav-item.gated {
  color: var(--ff-text-3);
  cursor: pointer;
}
.nav-item.gated:hover {
  background: transparent;
}
.nav-item.gated.active {
  background: var(--ff-primary-soft);
  font-weight: 400;
}
.nav-item.gated:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: -2px;
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
  background: var(--ff-badge-bg);
  color: var(--ff-on-primary);
  font-size: 11px;
  font-weight: 600;
  display: grid;
  place-items: center;
}
</style>
