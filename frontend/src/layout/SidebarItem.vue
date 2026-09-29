<template>
  <el-tooltip v-if="gated" content="需要先安装 ffmpeg" effect="light" placement="right" :show-after="300" :hide-after="0">
    <a
      class="nav-item gated"
      :class="{ active: isActive }"
      role="button"
      tabindex="0"
      :aria-label="item.label"
      @click.prevent="openInstall"
      @keydown.enter.prevent="openInstall"
      @keydown.space.prevent="openInstall"
    >
      <FIcon :name="item.icon" />
      <span v-show="!collapsed" class="label">{{ item.label }}</span>
    </a>
  </el-tooltip>
  <RouterLink v-else :to="item.path" class="nav-item" :class="{ active: isActive }" :title="collapsed ? item.label : undefined">
    <FIcon :name="item.icon" />
    <span v-show="!collapsed" class="label">{{ item.label }}</span>
    <span v-if="badge && !collapsed" class="badge">{{ badge }}</span>
  </RouterLink>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import { useFFmpegStore } from '@/stores/ffmpeg'
import type { NavItem } from './navigation'

const props = defineProps<{ item: NavItem; collapsed: boolean; badge?: number }>()
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
  gap: 10px; /* 与原型 .nav a 一致（10px），不改成 4 的倍数以免侧栏与设计稿错位 */
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
  background: transparent; /* 置灰且是当前页：不高亮（原型 31/32） */
  font-weight: 400;
}
.nav-item.gated:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: -2px;
}
.badge {
  margin-left: auto;
  min-width: 18px;
  height: 18px;
  padding: 0 4px;
  border-radius: 9px;
  background: var(--ff-badge-bg);
  color: var(--ff-on-primary);
  font-size: 12px;
  font-weight: 600;
  display: grid;
  place-items: center;
}
</style>
